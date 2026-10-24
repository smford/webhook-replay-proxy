package signer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Supported webhook signing providers.
const (
	ProviderStripe   = "stripe"
	ProviderGitHub   = "github"
	ProviderSlack    = "slack"
	ProviderShopify  = "shopify"
	ProviderStandard = "standard"
	ProviderAuto     = "auto"
	ProviderNone     = "none"
)

// ErrInvalidSignature is returned when signature verification fails.
var ErrInvalidSignature = errors.New("signature verification failed")

// ErrUnknownProvider is returned when an unsupported provider is requested.
var ErrUnknownProvider = errors.New("unknown signing provider")

// ErrMissingSecret is returned when a secret is required but not provided.
var ErrMissingSecret = errors.New("signing secret is required")

// DetectProvider inspects HTTP headers to determine the webhook signing provider.
func DetectProvider(headers http.Header) string {
	for k := range headers {
		lower := strings.ToLower(k)
		switch lower {
		case "stripe-signature":
			return ProviderStripe
		case "x-hub-signature-256", "x-hub-signature":
			return ProviderGitHub
		case "x-slack-signature":
			return ProviderSlack
		case "x-shopify-hmac-sha256", "x-shopify-topic":
			return ProviderShopify
		case "webhook-signature", "webhook-id", "svix-id", "svix-signature":
			return ProviderStandard
		}
	}
	return ProviderNone
}

// Signer generates and verifies cryptographic signatures for webhook requests.
type Signer struct {
	Provider string
	Secret   string
}

// New creates a new Signer for the specified provider and secret.
func New(provider, secret string) *Signer {
	return &Signer{
		Provider: strings.ToLower(provider),
		Secret:   secret,
	}
}

// Sign generates the necessary cryptographic headers for the given body and timestamp.
// If timestamp is zero, time.Now() is used.
func (s *Signer) Sign(body []byte, ts time.Time) (http.Header, error) {
	if s.Secret == "" && s.Provider != ProviderNone {
		return nil, ErrMissingSecret
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	headers := make(http.Header)
	unixTs := ts.Unix()

	switch s.Provider {
	case ProviderStripe:
		// Signed payload: <unix_ts>.<body bytes>
		payload := fmt.Sprintf("%d.%s", unixTs, string(body))
		mac := computeHMACSHA256([]byte(s.Secret), []byte(payload))
		sigHex := hex.EncodeToString(mac)
		headers.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", unixTs, sigHex))
		return headers, nil

	case ProviderGitHub:
		// Signed payload: <body bytes>
		mac := computeHMACSHA256([]byte(s.Secret), body)
		sigHex := hex.EncodeToString(mac)
		headers.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%s", sigHex))
		return headers, nil

	case ProviderSlack:
		// Signed payload: v0:<unix_ts>:<body bytes>
		payload := fmt.Sprintf("v0:%d:%s", unixTs, string(body))
		mac := computeHMACSHA256([]byte(s.Secret), []byte(payload))
		sigHex := hex.EncodeToString(mac)
		headers.Set("X-Slack-Request-Timestamp", strconv.FormatInt(unixTs, 10))
		headers.Set("X-Slack-Signature", fmt.Sprintf("v0=%s", sigHex))
		return headers, nil

	case ProviderShopify:
		// Signed payload: <body bytes>, base64 output
		mac := computeHMACSHA256([]byte(s.Secret), body)
		sigB64 := base64.StdEncoding.EncodeToString(mac)
		headers.Set("X-Shopify-Hmac-Sha256", sigB64)
		return headers, nil

	case ProviderStandard:
		// Svix / Standard Webhooks
		// Signed payload: <id>.<timestamp>.<body bytes>
		secretBytes, err := parseStandardSecret(s.Secret)
		if err != nil {
			return nil, err
		}
		msgID := fmt.Sprintf("msg_%d", ts.UnixNano())
		payload := fmt.Sprintf("%s.%d.%s", msgID, unixTs, string(body))
		mac := computeHMACSHA256(secretBytes, []byte(payload))
		sigB64 := base64.StdEncoding.EncodeToString(mac)
		headers.Set("webhook-id", msgID)
		headers.Set("webhook-timestamp", strconv.FormatInt(unixTs, 10))
		headers.Set("webhook-signature", fmt.Sprintf("v1,%s", sigB64))
		return headers, nil

	case ProviderNone, "":
		return headers, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, s.Provider)
	}
}

// Verify validates that the signature header in the request matches the body and secret.
func (s *Signer) Verify(headers http.Header, body []byte, tolerance time.Duration) error {
	if s.Secret == "" {
		return ErrMissingSecret
	}

	switch s.Provider {
	case ProviderStripe:
		header := headers.Get("Stripe-Signature")
		if header == "" {
			return fmt.Errorf("%w: missing Stripe-Signature header", ErrInvalidSignature)
		}
		var timestampStr, signatureHex string
		parts := strings.Split(header, ",")
		for _, part := range parts {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 {
				if kv[0] == "t" {
					timestampStr = kv[1]
				} else if kv[0] == "v1" {
					signatureHex = kv[1]
				}
			}
		}
		if timestampStr == "" || signatureHex == "" {
			return fmt.Errorf("%w: invalid Stripe-Signature format", ErrInvalidSignature)
		}
		tsUnix, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			return fmt.Errorf("%w: invalid timestamp in Stripe-Signature", ErrInvalidSignature)
		}
		if tolerance > 0 {
			eventTime := time.Unix(tsUnix, 0)
			if time.Since(eventTime).Abs() > tolerance {
				return fmt.Errorf("%w: timestamp outside tolerance", ErrInvalidSignature)
			}
		}
		payload := fmt.Sprintf("%d.%s", tsUnix, string(body))
		expectedMac := computeHMACSHA256([]byte(s.Secret), []byte(payload))
		actualMac, err := hex.DecodeString(signatureHex)
		if err != nil || !hmac.Equal(expectedMac, actualMac) {
			return ErrInvalidSignature
		}
		return nil

	case ProviderGitHub:
		header := headers.Get("X-Hub-Signature-256")
		if header == "" {
			return fmt.Errorf("%w: missing X-Hub-Signature-256 header", ErrInvalidSignature)
		}
		if !strings.HasPrefix(header, "sha256=") {
			return fmt.Errorf("%w: invalid signature prefix", ErrInvalidSignature)
		}
		actualMac, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
		if err != nil {
			return ErrInvalidSignature
		}
		expectedMac := computeHMACSHA256([]byte(s.Secret), body)
		if !hmac.Equal(expectedMac, actualMac) {
			return ErrInvalidSignature
		}
		return nil

	case ProviderSlack:
		sigHeader := headers.Get("X-Slack-Signature")
		tsHeader := headers.Get("X-Slack-Request-Timestamp")
		if sigHeader == "" || tsHeader == "" {
			return fmt.Errorf("%w: missing Slack signature or timestamp headers", ErrInvalidSignature)
		}
		tsUnix, err := strconv.ParseInt(tsHeader, 10, 64)
		if err != nil {
			return fmt.Errorf("%w: invalid Slack timestamp", ErrInvalidSignature)
		}
		if tolerance > 0 {
			eventTime := time.Unix(tsUnix, 0)
			if time.Since(eventTime).Abs() > tolerance {
				return fmt.Errorf("%w: timestamp outside tolerance", ErrInvalidSignature)
			}
		}
		if !strings.HasPrefix(sigHeader, "v0=") {
			return fmt.Errorf("%w: invalid Slack signature prefix", ErrInvalidSignature)
		}
		actualMac, err := hex.DecodeString(strings.TrimPrefix(sigHeader, "v0="))
		if err != nil {
			return ErrInvalidSignature
		}
		payload := fmt.Sprintf("v0:%d:%s", tsUnix, string(body))
		expectedMac := computeHMACSHA256([]byte(s.Secret), []byte(payload))
		if !hmac.Equal(expectedMac, actualMac) {
			return ErrInvalidSignature
		}
		return nil

	case ProviderShopify:
		sigHeader := headers.Get("X-Shopify-Hmac-Sha256")
		if sigHeader == "" {
			return fmt.Errorf("%w: missing X-Shopify-Hmac-Sha256 header", ErrInvalidSignature)
		}
		actualMac, err := base64.StdEncoding.DecodeString(sigHeader)
		if err != nil {
			return ErrInvalidSignature
		}
		expectedMac := computeHMACSHA256([]byte(s.Secret), body)
		if !hmac.Equal(expectedMac, actualMac) {
			return ErrInvalidSignature
		}
		return nil

	case ProviderStandard:
		msgID := headers.Get("webhook-id")
		tsHeader := headers.Get("webhook-timestamp")
		sigHeader := headers.Get("webhook-signature")
		if msgID == "" || tsHeader == "" || sigHeader == "" {
			return fmt.Errorf("%w: missing Standard Webhook headers", ErrInvalidSignature)
		}
		tsUnix, err := strconv.ParseInt(tsHeader, 10, 64)
		if err != nil {
			return fmt.Errorf("%w: invalid webhook timestamp", ErrInvalidSignature)
		}
		if tolerance > 0 {
			eventTime := time.Unix(tsUnix, 0)
			if time.Since(eventTime).Abs() > tolerance {
				return fmt.Errorf("%w: timestamp outside tolerance", ErrInvalidSignature)
			}
		}
		secretBytes, err := parseStandardSecret(s.Secret)
		if err != nil {
			return err
		}
		payload := fmt.Sprintf("%s.%d.%s", msgID, tsUnix, string(body))
		expectedMac := computeHMACSHA256(secretBytes, []byte(payload))

		// Signature format: v1,<base64>
		signatures := strings.Split(sigHeader, " ")
		for _, sig := range signatures {
			parts := strings.SplitN(sig, ",", 2)
			if len(parts) == 2 && parts[0] == "v1" {
				actualMac, err := base64.StdEncoding.DecodeString(parts[1])
				if err == nil && hmac.Equal(expectedMac, actualMac) {
					return nil
				}
			}
		}
		return ErrInvalidSignature

	default:
		return fmt.Errorf("%w: %s", ErrUnknownProvider, s.Provider)
	}
}

func computeHMACSHA256(key, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

func parseStandardSecret(secret string) ([]byte, error) {
	if strings.HasPrefix(secret, "whsec_") {
		trimmed := strings.TrimPrefix(secret, "whsec_")
		b, err := base64.StdEncoding.DecodeString(trimmed)
		if err == nil && len(b) > 0 {
			return b, nil
		}
	}
	return []byte(secret), nil
}
