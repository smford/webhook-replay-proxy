package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/signer"
	"github.com/smford/webhook-replay-proxy/internal/store"
)

func TestProxyCaptureAndReplay(t *testing.T) {
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

	dbPath := filepath.Join(t.TempDir(), "proxy_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	proxySrv := NewServer(ServerConfig{
		Addr:       "127.0.0.1:0",
		ForwardURL: targetServer.URL,
		Store:      st,
		Version:    "0.2.0",
	})

	proxyHTTP := httptest.NewServer(proxySrv.httpServer.Handler)
	defer proxyHTTP.Close()

	initialBody := []byte(`{"type":"payment_intent.succeeded","data":{"object":{"amount":2000}}}`)
	sig := signer.New(signer.ProviderStripe, secret)
	headers, err := sig.Sign(initialBody, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to sign initial payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, proxyHTTP.URL+"/webhooks", bytes.NewReader(initialBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send request to proxy: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected proxy status 200, got %d", resp.StatusCode)
	}

	var recResp struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		ForwardStatus int    `json:"forward_status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&recResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	_ = resp.Body.Close()

	if recResp.ForwardStatus != http.StatusOK {
		t.Fatalf("expected forward status 200, got %d", recResp.ForwardStatus)
	}

	engine := NewReplayEngine(st)
	results, err := engine.Execute(context.Background(), ReplayOptions{
		EventID:     recResp.ID,
		TargetURL:   targetServer.URL,
		Provider:    "stripe",
		Secret:      secret,
		Mutations:   []string{"data.object.amount=5000"},
		Concurrency: 2,
		ShowDiff:    true,
	})
	if err != nil {
		t.Fatalf("failed to replay event: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 replay results, got %d", len(results))
	}
	for _, res := range results {
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 from target, got %d (err: %v)", res.StatusCode, res.Error)
		}
	}
}

func TestFixtureExportAndLoad(t *testing.T) {
	event := &store.Event{
		ID:        "evt_fixture_123",
		Timestamp: time.Now().UTC(),
		Provider:  "github",
		Method:    "POST",
		Path:      "/gh-webhook",
		Headers: map[string][]string{
			"X-Hub-Signature-256": {"sha256=12345"},
			"Content-Type":        {"application/json"},
		},
		Body:          []byte(`{"action":"opened","issue":{"number":42}}`),
		TargetURL:     "http://localhost:8080/receiver",
		ForwardStatus: 200,
	}

	data, err := ExportFixture(event)
	if err != nil {
		t.Fatalf("ExportFixture failed: %v", err)
	}

	tmpFile := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	loaded, err := LoadFixture(tmpFile)
	if err != nil {
		t.Fatalf("LoadFixture failed: %v", err)
	}

	if loaded.ID != event.ID {
		t.Errorf("expected ID %s, got %s", event.ID, loaded.ID)
	}
	if loaded.Provider != "github" {
		t.Errorf("expected provider github, got %s", loaded.Provider)
	}
	var expectedObj, loadedObj map[string]interface{}
	_ = json.Unmarshal(event.Body, &expectedObj)
	_ = json.Unmarshal(loaded.Body, &loadedObj)
	if expectedObj["action"] != loadedObj["action"] {
		t.Errorf("expected action %v, got %v", expectedObj["action"], loadedObj["action"])
	}

	engine := NewReplayEngine(nil)
	curlCmd, err := engine.GenerateCurl(ReplayOptions{
		FixturePath: tmpFile,
		TargetURL:   "http://localhost:9000/webhook",
	})
	if err != nil {
		t.Fatalf("GenerateCurl from fixture failed: %v", err)
	}
	if !strings.Contains(curlCmd, "http://localhost:9000/webhook") {
		t.Errorf("expected target URL in curl, got: %s", curlCmd)
	}
}

func TestAssertReplay(t *testing.T) {
	results := []ReplayResult{
		{Index: 0, StatusCode: 200, LatencyMs: 25},
		{Index: 1, StatusCode: 409, LatencyMs: 30},
		{Index: 2, StatusCode: 409, LatencyMs: 15},
	}

	// 1. Successful count assertion
	if err := AssertReplay(results, "200:1,409:2", 100*time.Millisecond); err != nil {
		t.Errorf("expected assertion to succeed, got: %v", err)
	}

	// 2. Failing count assertion
	if err := AssertReplay(results, "200:2,409:1", 0); err == nil {
		t.Errorf("expected assertion failure for incorrect count, but got nil")
	}

	// 3. Failing latency assertion
	if err := AssertReplay(results, "", 20*time.Millisecond); err == nil {
		t.Errorf("expected latency assertion failure, but got nil")
	}

	// 4. Single code format match
	successResults := []ReplayResult{
		{Index: 0, StatusCode: 200, LatencyMs: 10},
		{Index: 1, StatusCode: 200, LatencyMs: 12},
	}
	if err := AssertReplay(successResults, "200", 50*time.Millisecond); err != nil {
		t.Errorf("expected all-200 assertion to pass, got: %v", err)
	}
	if err := AssertReplay(results, "200", 0); err == nil {
		t.Errorf("expected all-200 assertion to fail on mixed results, got nil")
	}
}

func TestChaosNegativeTesting(t *testing.T) {
	secret := "whsec_chaos_test"
	stripeVerifier := signer.New(signer.ProviderStripe, secret)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		// Test JSON validity
		var js map[string]interface{}
		if err := json.Unmarshal(body, &js); err != nil {
			http.Error(w, "malformed json: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Test signature validity
		if err := stripeVerifier.Verify(r.Header, body, 1*time.Minute); err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer targetServer.Close()

	event := &store.Event{
		ID:        "evt_chaos",
		Timestamp: time.Now().UTC(),
		Provider:  "stripe",
		Method:    "POST",
		Path:      "/webhook",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body:      []byte(`{"type":"customer.created","data":{"id":"cus_123"}}`),
		TargetURL: targetServer.URL,
	}

	engine := NewReplayEngine(nil)

	// 1. Test --corrupt-sig: Should result in HTTP 401 Unauthorized
	corruptSigRes, err := engine.ExecuteEvent(context.Background(), event, ReplayOptions{
		TargetURL:  targetServer.URL,
		Provider:   "stripe",
		Secret:     secret,
		CorruptSig: true,
	})
	if err != nil {
		t.Fatalf("corrupt-sig execute error: %v", err)
	}
	if len(corruptSigRes) != 1 || corruptSigRes[0].StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for corrupted signature, got %d", corruptSigRes[0].StatusCode)
	}

	// 2. Test --corrupt-json: Should result in HTTP 400 Bad Request
	corruptJSONRes, err := engine.ExecuteEvent(context.Background(), event, ReplayOptions{
		TargetURL:   targetServer.URL,
		Provider:    "stripe",
		Secret:      secret,
		CorruptJSON: true,
	})
	if err != nil {
		t.Fatalf("corrupt-json execute error: %v", err)
	}
	if len(corruptJSONRes) != 1 || corruptJSONRes[0].StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for corrupted JSON, got %d", corruptJSONRes[0].StatusCode)
	}

	// 3. Test --time-offset: Expired timestamp (> 1m) should fail verification
	expiredRes, err := engine.ExecuteEvent(context.Background(), event, ReplayOptions{
		TargetURL:  targetServer.URL,
		Provider:   "stripe",
		Secret:     secret,
		TimeOffset: -10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("time-offset execute error: %v", err)
	}
	if len(expiredRes) != 1 || expiredRes[0].StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired timestamp, got %d", expiredRes[0].StatusCode)
	}

	// 4. Test --drop-header: Dropping Stripe-Signature should fail verification
	dropHeaderRes, err := engine.ExecuteEvent(context.Background(), event, ReplayOptions{
		TargetURL:   targetServer.URL,
		Provider:    "stripe",
		Secret:      secret,
		DropHeaders: []string{"Stripe-Signature"},
	})
	if err != nil {
		t.Fatalf("drop-header execute error: %v", err)
	}
	if len(dropHeaderRes) != 1 || dropHeaderRes[0].StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for dropped signature header, got %d", dropHeaderRes[0].StatusCode)
	}
}

func TestUIDashboardAndAPI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ui_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	_ = st.SaveEvent(&store.Event{
		ID:            "evt_test_ui",
		Timestamp:     time.Now().UTC(),
		Provider:      "stripe",
		Method:        "POST",
		Path:          "/hook",
		Body:          []byte(`{"test":"ok"}`),
		ForwardStatus: 200,
	})

	srv := NewServer(ServerConfig{
		Addr:    "127.0.0.1:0",
		Store:   st,
		Version: "0.2.0",
	})

	testHTTP := httptest.NewServer(srv.httpServer.Handler)
	defer testHTTP.Close()

	// 1. GET /_dashboard
	res, err := http.Get(testHTTP.URL + "/_dashboard")
	if err != nil {
		t.Fatalf("GET /_dashboard error: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /_dashboard, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "Webhook Replay Proxy") {
		t.Errorf("dashboard HTML missing title text")
	}

	// 2. GET /api/events
	res, err = http.Get(testHTTP.URL + "/api/events")
	if err != nil {
		t.Fatalf("GET /api/events error: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /api/events, got %d", res.StatusCode)
	}
	var eventsList []map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&eventsList); err != nil {
		t.Fatalf("decoding /api/events: %v", err)
	}
	_ = res.Body.Close()
	if len(eventsList) != 1 || eventsList[0]["id"] != "evt_test_ui" {
		t.Errorf("expected evt_test_ui in events list, got: %v", eventsList)
	}
}
