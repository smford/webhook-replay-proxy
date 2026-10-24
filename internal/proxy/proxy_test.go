package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/signer"
	"github.com/smford/webhook-replay-proxy/internal/store"
)

func TestProxyCaptureAndReplay(t *testing.T) {
	// 1. Target server that verifies Stripe HMAC signatures
	var targetCalls int64
	secret := "whsec_super_secret_test_key"
	stripeVerifier := signer.New(signer.ProviderStripe, secret)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&targetCalls, 1)
		body, _ := io.ReadAll(r.Body)

		if err := stripeVerifier.Verify(r.Header, body, 5*time.Minute); err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer targetServer.Close()

	// 2. Open SQLite store
	dbPath := filepath.Join(t.TempDir(), "proxy_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// 3. Start proxy server
	proxySrv := NewServer(ServerConfig{
		Addr:       "127.0.0.1:0",
		ForwardURL: targetServer.URL,
		Store:      st,
		Version:    "0.1.0",
	})

	proxyHTTP := httptest.NewServer(proxySrv.httpServer.Handler)
	defer proxyHTTP.Close()

	// 4. Send initial Stripe webhook to proxy
	initialBody := []byte(`{"type":"payment_intent.succeeded","data":{"object":{"amount":2000}}}`)
	sig := signer.New(signer.ProviderStripe, secret)
	headers, err := sig.Sign(initialBody, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to sign initial request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, proxyHTTP.URL+"/webhooks", bytes.NewReader(initialBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("initial webhook request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected proxy status 200, got %d", resp.StatusCode)
	}

	// Verify target was called
	if atomic.LoadInt64(&targetCalls) != 1 {
		t.Fatalf("expected target calls 1, got %d", targetCalls)
	}

	// 5. Verify event was stored in SQLite
	events, err := st.ListEvents(10)
	if err != nil || len(events) == 0 {
		t.Fatalf("failed to retrieve stored events: %v", err)
	}
	eventID := events[0].ID

	// 6. Replay with mutation and concurrency = 5
	replayEngine := NewReplayEngine(st)
	results, err := replayEngine.Execute(context.Background(), ReplayOptions{
		EventID:     eventID,
		TargetURL:   targetServer.URL,
		Provider:    "stripe",
		Secret:      secret,
		Mutations:   []string{"data.object.amount=9999", "type=payment_intent.payment_failed"},
		Concurrency: 5,
	})
	if err != nil {
		t.Fatalf("ReplayEngine.Execute failed: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	for i, res := range results {
		if res.Error != nil {
			t.Errorf("result %d returned error: %v", i, res.Error)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("result %d expected status 200, got %d, body: %s", i, res.StatusCode, res.ResponseBody)
		}
	}

	// 7. Test GenerateCurl
	curlCmd, err := replayEngine.GenerateCurl(ReplayOptions{
		EventID:   eventID,
		TargetURL: targetServer.URL,
		Provider:  "stripe",
		Secret:    secret,
		Mutations: []string{"data.object.amount=9999"},
	})
	if err != nil {
		t.Fatalf("GenerateCurl failed: %v", err)
	}
	if !bytes.Contains([]byte(curlCmd), []byte("curl -X POST")) {
		t.Errorf("curlCmd should contain curl -X POST: %s", curlCmd)
	}
	if !bytes.Contains([]byte(curlCmd), []byte("Stripe-Signature")) {
		t.Errorf("curlCmd should contain Stripe-Signature: %s", curlCmd)
	}
	if !bytes.Contains([]byte(curlCmd), []byte("9999")) {
		t.Errorf("curlCmd should contain mutated amount 9999: %s", curlCmd)
	}
}
