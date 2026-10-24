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
	// Append path and query
	resolved := baseURL.ResolveReference(&url.URL{
		Path:     path,
		RawQuery: query,
	})
	return resolved.String(), nil
}

// ReplayOptions configures a webhook replay operation.
type ReplayOptions struct {
	EventID     string
	TargetURL   string
	Provider    string
	Secret      string
	Mutations   []string
	Concurrency int
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

// Execute performs the replay according to ReplayOptions.
func (re *ReplayEngine) Execute(ctx context.Context, opts ReplayOptions) ([]ReplayResult, error) {
	event, err := re.store.GetEvent(opts.EventID)
	if err != nil {
		return nil, fmt.Errorf("retrieving event %s: %w", opts.EventID, err)
	}

	targetURL := opts.TargetURL
	if targetURL == "" {
		targetURL = event.TargetURL
	}
	if targetURL == "" {
		return nil, fmt.Errorf("target URL is required (event has no recorded target URL)")
	}

	// 1. Mutate payload if requested
	body := event.Body
	if len(opts.Mutations) > 0 {
		mutated, err := mutator.MutateJSON(body, opts.Mutations)
		if err != nil {
			return nil, fmt.Errorf("mutating payload: %w", err)
		}
		body = mutated
	}

	// 2. Determine provider and re-sign
	provider := strings.ToLower(opts.Provider)
	if provider == "" || provider == signer.ProviderAuto {
		provider = event.Provider
		if provider == "" || provider == signer.ProviderNone {
			provider = signer.DetectProvider(event.Headers)
		}
	}

	// Build headers
	headers := make(http.Header)
	for k, vv := range event.Headers {
		for _, v := range vv {
			headers.Add(k, v)
		}
	}

	// Re-sign if secret is provided or provider is active
	now := time.Now().UTC()
	if opts.Secret != "" {
		sig := signer.New(provider, opts.Secret)
		sigHeaders, err := sig.Sign(body, now)
		if err != nil {
			return nil, fmt.Errorf("generating signatures: %w", err)
		}
		for k, vv := range sigHeaders {
			headers.Del(k)
			for _, v := range vv {
				headers.Set(k, v)
			}
		}
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	results := make([]ReplayResult, concurrency)

	// Concurrency synchronization barrier: all goroutines wait on startGate
	var wg sync.WaitGroup
	startGate := make(chan struct{})
	var counter int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req, err := http.NewRequestWithContext(ctx, event.Method, targetURL, bytes.NewReader(body))
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

	// Record replay in store
	for _, res := range results {
		errStr := ""
		if res.Error != nil {
			errStr = res.Error.Error()
		}
		_ = re.store.RecordReplay(&store.ReplayRecord{
			ID:        fmt.Sprintf("rep_%d_%d", time.Now().UnixNano(), res.Index),
			EventID:   opts.EventID,
			Timestamp: time.Now().UTC(),
			TargetURL: targetURL,
			Status:    res.StatusCode,
			LatencyMs: res.LatencyMs,
			Success:   res.StatusCode >= 200 && res.StatusCode < 300,
			Error:     errStr,
		})
	}

	return results, nil
}
