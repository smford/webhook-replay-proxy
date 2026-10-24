package signer

import (
	"net/http"
	"testing"
	"time"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected string
	}{
		{
			name:     "stripe",
			headers:  http.Header{"Stripe-Signature": []string{"t=123,v1=abc"}},
			expected: ProviderStripe,
		},
		{
			name:     "github",
			headers:  http.Header{"X-Hub-Signature-256": []string{"sha256=abc"}},
			expected: ProviderGitHub,
		},
		{
			name:     "slack",
			headers:  http.Header{"X-Slack-Signature": []string{"v0=abc"}},
			expected: ProviderSlack,
		},
		{
			name:     "shopify",
			headers:  http.Header{"X-Shopify-Hmac-Sha256": []string{"abc"}},
			expected: ProviderShopify,
		},
		{
			name:     "standard",
			headers:  http.Header{"Webhook-Signature": []string{"v1,abc"}},
			expected: ProviderStandard,
		},
		{
			name:     "none",
			headers:  http.Header{"Content-Type": []string{"application/json"}},
			expected: ProviderNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectProvider(tt.headers)
			if got != tt.expected {
				t.Errorf("DetectProvider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	providers := []struct {
		provider string
		secret   string
	}{
		{ProviderStripe, "whsec_test_stripe_secret_12345"},
		{ProviderGitHub, "gh_webhook_secret_99999"},
		{ProviderSlack, "slack_signing_secret_xyz"},
		{ProviderShopify, "shpss_secret_key_abcdef"},
		{ProviderStandard, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"},
	}

	body := []byte(`{"event":"charge.succeeded","amount":4200,"currency":"usd"}`)
	now := time.Now().UTC()

	for _, tc := range providers {
		t.Run(tc.provider, func(t *testing.T) {
			s := New(tc.provider, tc.secret)
			headers, err := s.Sign(body, now)
			if err != nil {
				t.Fatalf("Sign() failed: %v", err)
			}

			// Verify with valid signature
			if err := s.Verify(headers, body, 5*time.Minute); err != nil {
				t.Fatalf("Verify() failed with valid signature: %v", err)
			}

			// Verify with tampered body fails
			tamperedBody := []byte(`{"event":"charge.succeeded","amount":999999,"currency":"usd"}`)
			if err := s.Verify(headers, tamperedBody, 5*time.Minute); err == nil {
				t.Fatalf("Verify() should fail with tampered body")
			}

			// Verify with wrong secret fails
			wrongSigner := New(tc.provider, "wrong_secret_12345")
			if err := wrongSigner.Verify(headers, body, 5*time.Minute); err == nil {
				t.Fatalf("Verify() should fail with wrong secret")
			}
		})
	}
}

func TestExpiredSignature(t *testing.T) {
	s := New(ProviderStripe, "whsec_test_secret")
	body := []byte(`{"event":"test"}`)
	past := time.Now().Add(-10 * time.Minute)

	headers, err := s.Sign(body, past)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}

	err = s.Verify(headers, body, 5*time.Minute)
	if err == nil {
		t.Fatalf("expected verification failure due to expired timestamp")
	}
}
