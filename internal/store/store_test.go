package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_webhooks.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	event := &Event{
		ID:            "evt_test_1",
		Timestamp:     now,
		Provider:      "stripe",
		Method:        "POST",
		Path:          "/webhooks/stripe",
		Query:         "live=false",
		Headers:       map[string][]string{"Stripe-Signature": {"t=123,v1=abc"}},
		Body:          []byte(`{"event":"test"}`),
		TargetURL:     "http://localhost:3000/webhooks",
		ForwardStatus: 200,
		ResponseBody:  `{"received":true}`,
		LatencyMs:     42,
	}

	if err := s.SaveEvent(event); err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	fetched, err := s.GetEvent("evt_test_1")
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}

	if fetched.ID != event.ID {
		t.Errorf("expected ID %s, got %s", event.ID, fetched.ID)
	}
	if fetched.Provider != "stripe" {
		t.Errorf("expected provider stripe, got %s", fetched.Provider)
	}
	if fetched.ForwardStatus != 200 {
		t.Errorf("expected forward status 200, got %d", fetched.ForwardStatus)
	}
	if fetched.Headers["Stripe-Signature"][0] != "t=123,v1=abc" {
		t.Errorf("expected Stripe-Signature header preserved")
	}

	list, err := s.ListEvents(10)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 event in list, got %d", len(list))
	}

	// Test RecordReplay
	replay := &ReplayRecord{
		ID:        "rep_1",
		EventID:   "evt_test_1",
		Timestamp: time.Now().UTC(),
		TargetURL: "http://localhost:3000/webhooks",
		Status:    200,
		LatencyMs: 25,
		Success:   true,
	}
	if err := s.RecordReplay(replay); err != nil {
		t.Fatalf("RecordReplay failed: %v", err)
	}
}
