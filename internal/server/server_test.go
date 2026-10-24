package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smford/webhook-replay-proxy/internal/config"
)

func TestHealthz(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewServer(cfg, "v0.1.0")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", res["status"])
	}
}

func TestVersion(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewServer(cfg, "v1.2.3")

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["version"] != "v1.2.3" {
		t.Errorf("expected version 'v1.2.3', got %q", res["version"])
	}
}

func TestRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewServer(cfg, "v0.1.0")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestWebhookIngestAndList(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Storage.MaxEvents = 2
	s := NewServer(cfg, "v0.1.0")

	// Post webhook 1
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(`{"event":"order.created"}`))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr1.Code)
	}

	// Post webhook 2
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(`{"event":"order.updated"}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr2.Code)
	}

	// List webhooks
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	listRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listRR.Code)
	}

	var listRes map[string]any
	if err := json.Unmarshal(listRR.Body.Bytes(), &listRes); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	count := int(listRes["count"].(float64))
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}
