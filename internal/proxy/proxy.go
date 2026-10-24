package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/mutator"
	"github.com/smford/webhook-replay-proxy/internal/signer"
	"github.com/smford/webhook-replay-proxy/internal/store"
)

// ServerConfig holds capture listener configuration.
type ServerConfig struct {
	Addr       string
	ForwardURL string
	Store      *store.Store
	Version    string
	EnableUI   bool
}

// Server handles listening and forwarding incoming webhooks.
type Server struct {
	cfg        ServerConfig
	httpServer *http.Server
	client     *http.Client
}

// NewServer initializes a new proxy capture server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/version", s.handleVersion)
	mux.HandleFunc("/", s.handleWebhook)

	// Register Web UI and API routes
	s.registerUIRoutes(mux)

	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// ErrServerClosed returns http.ErrServerClosed.
func ErrServerClosed() error {
	return http.ErrServerClosed
}

// Start begins listening for incoming requests.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully terminates the capture server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"version":%q}`, s.cfg.Version)
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"service":"webhook-replay-proxy","version":%q,"status":"running","dashboard":"/_dashboard"}`+"\n", s.cfg.Version)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	provider := signer.DetectProvider(r.Header)
	eventID := fmt.Sprintf("evt_%d", time.Now().UnixNano())
	now := time.Now().UTC()

	var (
		forwardStatus int
		respBodyStr   string
		latencyMs     int64
	)

	// If forwarding is configured, forward request
	if s.cfg.ForwardURL != "" {
		targetURL, err := resolveForwardURL(s.cfg.ForwardURL, r.URL.Path, r.URL.RawQuery)
		if err == nil {
			start := time.Now()
			req, reqErr := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(bodyBytes))
			if reqErr == nil {
				// Copy original headers
				for k, vv := range r.Header {
					for _, v := range vv {
						req.Header.Add(k, v)
					}
				}

				resp, err := s.client.Do(req)
				latencyMs = time.Since(start).Milliseconds()
				if err == nil {
					forwardStatus = resp.StatusCode
					respBytes, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					respBodyStr = string(respBytes)
				} else {
					forwardStatus = http.StatusBadGateway
					respBodyStr = err.Error()
				}
			}
		}
	}

	event := &store.Event{
		ID:            eventID,
		Timestamp:     now,
		Provider:      provider,
		Method:        r.Method,
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Headers:       r.Header,
		Body:          bodyBytes,
		TargetURL:     s.cfg.ForwardURL,
		ForwardStatus: forwardStatus,
		ResponseBody:  respBodyStr,
		LatencyMs:     latencyMs,
	}

	if s.cfg.Store != nil {
		_ = s.cfg.Store.SaveEvent(event)
	}

	fmt.Printf("[CAPTURED] %s | Provider: %-8s | %s %s | Forwarded: %d (%dms)\n",
		eventID, provider, r.Method, r.URL.Path, forwardStatus, latencyMs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"recorded","id":%q,"forward_status":%d}`+"\n", eventID, forwardStatus)
}

func resolveForwardURL(baseStr, path, query string) (string, error) {
	baseURL, err := url.Parse(baseStr)
	if err != nil {
		return "", err
	}
	resolved := baseURL.ResolveReference(&url.URL{
		Path:     path,
		RawQuery: query,
	})
	return resolved.String(), nil
}

// ReplayOptions configures a webhook replay operation.
type ReplayOptions struct {
	EventID        string
	FixturePath    string
	TargetURL      string
	Provider       string
	Secret         string
	Mutations      []string
	Concurrency    int
	Delay          time.Duration
	ShowDiff       bool
	CorruptSig     bool
	CorruptJSON    bool
	DropHeaders    []string
	TimeOffset     time.Duration
	FixedTimestamp int64
	AssertStatus   string
	MaxLatency     time.Duration
}

// ReplayResult captures the result of a single replayed request.
type ReplayResult struct {
	Index        int
	StatusCode   int
	ResponseBody string
	LatencyMs    int64
	Error        error
}

// ReplayEngine handles re-signing, mutating, and firing webhook requests.
type ReplayEngine struct {
	store  *store.Store
	client *http.Client
}

// NewReplayEngine initializes a ReplayEngine.
func NewReplayEngine(s *store.Store) *ReplayEngine {
	return &ReplayEngine{
		store: s,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PrepareReplay builds the mutated body, signature headers, and resolved target URL without firing.
func (re *ReplayEngine) PrepareReplay(opts ReplayOptions) ([]byte, http.Header, string, error) {
	var event *store.Event
	var err error

	if opts.FixturePath != "" {
		event, err = LoadFixture(opts.FixturePath)
		if err != nil {
			return nil, nil, "", fmt.Errorf("loading fixture %s: %w", opts.FixturePath, err)
		}
	} else {
		if re.store == nil {
			return nil, nil, "", fmt.Errorf("store is uninitialized (or provide --file)")
		}
		event, err = re.store.GetEvent(opts.EventID)
		if err != nil {
			return nil, nil, "", fmt.Errorf("retrieving event %s: %w", opts.EventID, err)
		}
	}

	return re.PrepareReplayEvent(event, opts)
}

// PrepareReplayEvent prepares body and headers directly from a store.Event.
func (re *ReplayEngine) PrepareReplayEvent(event *store.Event, opts ReplayOptions) ([]byte, http.Header, string, error) {
	targetURL := opts.TargetURL
	if targetURL == "" {
		targetURL = event.TargetURL
	}
	if targetURL == "" {
		return nil, nil, "", fmt.Errorf("target URL is required (event has no recorded target URL)")
	}

	body := event.Body
	if len(opts.Mutations) > 0 {
		mutated, err := mutator.MutateJSON(body, opts.Mutations)
		if err != nil {
			return nil, nil, "", fmt.Errorf("mutating payload: %w", err)
		}
		body = mutated
	}

	// Negative testing: Corrupt JSON syntax if requested
	if opts.CorruptJSON {
		if len(body) > 0 && body[len(body)-1] == '}' {
			body = body[:len(body)-1]
		} else {
			body = append(body, []byte(`,"corrupted_syntax":[`)...)
		}
	}

	provider := strings.ToLower(opts.Provider)
	if provider == "" || provider == signer.ProviderAuto {
		provider = event.Provider
		if provider == "" || provider == signer.ProviderNone {
			provider = signer.DetectProvider(event.Headers)
		}
	}

	headers := make(http.Header)
	for k, vv := range event.Headers {
		for _, v := range vv {
			headers.Add(k, v)
		}
	}

	// Compute signing timestamp with offset or fixed timestamp
	signingTime := time.Now().UTC()
	if opts.FixedTimestamp > 0 {
		signingTime = time.Unix(opts.FixedTimestamp, 0).UTC()
	} else if opts.TimeOffset != 0 {
		signingTime = signingTime.Add(opts.TimeOffset)
	}

	if opts.Secret != "" {
		sig := signer.New(provider, opts.Secret)
		sigHeaders, err := sig.Sign(body, signingTime)
		if err != nil {
			return nil, nil, "", fmt.Errorf("generating signatures: %w", err)
		}
		for k, vv := range sigHeaders {
			headers.Del(k)
			for _, v := range vv {
				headers.Set(k, v)
			}
		}
	}

	// Negative testing: Corrupt signature if requested
	if opts.CorruptSig {
		corruptSignature(headers)
	}

	// Negative testing: Drop specified headers if requested
	for _, drop := range opts.DropHeaders {
		headers.Del(drop)
	}

	return body, headers, targetURL, nil
}

// GenerateCurl formats an executable curl command string for the replay.
func (re *ReplayEngine) GenerateCurl(opts ReplayOptions) (string, error) {
	body, headers, targetURL, err := re.PrepareReplay(opts)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("curl -X POST %q \\\n", targetURL))
	for k, vv := range headers {
		for _, v := range vv {
			sb.WriteString(fmt.Sprintf("  -H %q \\\n", fmt.Sprintf("%s: %s", k, v)))
		}
	}
	sb.WriteString(fmt.Sprintf("  --data %q", string(body)))
	return sb.String(), nil
}

// Execute performs the replay according to ReplayOptions.
func (re *ReplayEngine) Execute(ctx context.Context, opts ReplayOptions) ([]ReplayResult, error) {
	var event *store.Event
	var err error

	if opts.FixturePath != "" {
		event, err = LoadFixture(opts.FixturePath)
		if err != nil {
			return nil, fmt.Errorf("loading fixture %s: %w", opts.FixturePath, err)
		}
	} else {
		if re.store == nil {
			return nil, fmt.Errorf("store is uninitialized (or provide --file)")
		}
		event, err = re.store.GetEvent(opts.EventID)
		if err != nil {
			return nil, fmt.Errorf("retrieving event %s: %w", opts.EventID, err)
		}
	}

	if opts.ShowDiff && len(opts.Mutations) > 0 {
		mutated, err := mutator.MutateJSON(event.Body, opts.Mutations)
		if err == nil {
			fmt.Println("--- Payload Diff ---")
			fmt.Print(mutator.DiffJSON(event.Body, mutated))
			fmt.Println("--------------------")
		}
	}

	results, err := re.ExecuteEvent(ctx, event, opts)
	if err != nil {
		return results, err
	}

	// Evaluate assertions if requested
	if opts.AssertStatus != "" || opts.MaxLatency > 0 {
		if assertErr := AssertReplay(results, opts.AssertStatus, opts.MaxLatency); assertErr != nil {
			return results, assertErr
		}
	}

	return results, nil
}

// ExecuteEvent fires requests for the given event and records results.
func (re *ReplayEngine) ExecuteEvent(ctx context.Context, event *store.Event, opts ReplayOptions) ([]ReplayResult, error) {
	body, headers, targetURL, err := re.PrepareReplayEvent(event, opts)
	if err != nil {
		return nil, err
	}

	method := event.Method
	if method == "" {
		method = http.MethodPost
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	results := make([]ReplayResult, concurrency)

	// If Delay > 0, run sequentially with pacing
	if opts.Delay > 0 && concurrency > 1 {
		for i := 0; i < concurrency; i++ {
			if i > 0 {
				select {
				case <-ctx.Done():
					return results, ctx.Err()
				case <-time.After(opts.Delay):
				}
			}
			req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
			if err != nil {
				results[i] = ReplayResult{Index: i, Error: err}
				continue
			}
			for k, vv := range headers {
				for _, v := range vv {
					req.Header.Add(k, v)
				}
			}
			start := time.Now()
			resp, err := re.client.Do(req)
			latency := time.Since(start).Milliseconds()
			if err != nil {
				results[i] = ReplayResult{Index: i, LatencyMs: latency, Error: err}
				continue
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			results[i] = ReplayResult{
				Index:        i,
				StatusCode:   resp.StatusCode,
				ResponseBody: string(respBody),
				LatencyMs:    latency,
			}
		}
	} else {
		// Concurrency synchronization barrier: all goroutines wait on startGate
		var wg sync.WaitGroup
		startGate := make(chan struct{})
		var counter int64

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
				if err != nil {
					results[idx] = ReplayResult{Index: idx, Error: err}
					return
				}

				for k, vv := range headers {
					for _, v := range vv {
						req.Header.Add(k, v)
					}
				}

				// Wait at barrier until all goroutines are initialized
				<-startGate

				start := time.Now()
				resp, err := re.client.Do(req)
				latency := time.Since(start).Milliseconds()

				if err != nil {
					results[idx] = ReplayResult{
						Index:     idx,
						LatencyMs: latency,
						Error:     err,
					}
					atomic.AddInt64(&counter, 1)
					return
				}
				defer resp.Body.Close()

				respBody, _ := io.ReadAll(resp.Body)
				results[idx] = ReplayResult{
					Index:        idx,
					StatusCode:   resp.StatusCode,
					ResponseBody: string(respBody),
					LatencyMs:    latency,
				}
				atomic.AddInt64(&counter, 1)
			}(i)
		}

		// Release all goroutines simultaneously
		close(startGate)
		wg.Wait()
	}

	// Record replay in store if store is present
	if re.store != nil && event.ID != "" {
		for _, res := range results {
			errStr := ""
			if res.Error != nil {
				errStr = res.Error.Error()
			}
			_ = re.store.RecordReplay(&store.ReplayRecord{
				ID:        fmt.Sprintf("rep_%d_%d", time.Now().UnixNano(), res.Index),
				EventID:   event.ID,
				Timestamp: time.Now().UTC(),
				TargetURL: targetURL,
				Status:    res.StatusCode,
				LatencyMs: res.LatencyMs,
				Success:   res.StatusCode >= 200 && res.StatusCode < 300,
				Error:     errStr,
			})
		}
	}

	return results, nil
}

func corruptSignature(headers http.Header) {
	mutated := false
	for k := range headers {
		lower := strings.ToLower(k)
		switch lower {
		case "stripe-signature":
			val := headers.Get(k)
			if idx := strings.Index(val, "v1="); idx != -1 {
				headers.Set(k, val[:idx+3]+"deadbeef00000000000000000000000000000000000000000000000000000000")
			} else {
				headers.Set(k, "t=0,v1=deadbeef00000000000000000000000000000000000000000000000000000000")
			}
			mutated = true
		case "x-hub-signature-256":
			headers.Set(k, "sha256=deadbeef00000000000000000000000000000000000000000000000000000000")
			mutated = true
		case "x-hub-signature":
			headers.Set(k, "sha1=deadbeef00000000000000000000000000000000")
			mutated = true
		case "x-slack-signature":
			headers.Set(k, "v0=deadbeef00000000000000000000000000000000000000000000000000000000")
			mutated = true
		case "x-shopify-hmac-sha256":
			headers.Set(k, "deadbeef00000000000000000000000000000000000=")
			mutated = true
		case "svix-signature", "webhook-signature":
			headers.Set(k, "v1,deadbeef00000000000000000000000000000000000000000000000000000000")
			mutated = true
		}
	}
	if !mutated {
		headers.Set("X-Webhook-Signature", "corrupted_signature_deadbeef")
	}
}
