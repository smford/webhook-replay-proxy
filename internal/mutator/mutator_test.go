package mutator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMutateJSON(t *testing.T) {
	initial := []byte(`{
		"id": "evt_123",
		"type": "charge.succeeded",
		"data": {
			"object": {
				"amount": 2000,
				"paid": true,
				"customer": "cus_abc"
			}
		}
	}`)

	mutations := []string{
		"type=charge.failed",
		"data.object.amount=9999",
		"data.object.paid=false",
		"data.object.metadata.notes=testing",
		"data.object.currency=usd",
	}

	res, err := MutateJSON(initial, mutations)
	if err != nil {
		t.Fatalf("MutateJSON failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(res, &m); err != nil {
		t.Fatalf("Unmarshal mutated JSON failed: %v", err)
	}

	if m["type"] != "charge.failed" {
		t.Errorf("expected type charge.failed, got %v", m["type"])
	}

	dataObj := m["data"].(map[string]any)["object"].(map[string]any)
	if dataObj["amount"] != float64(9999) && dataObj["amount"] != int64(9999) {
		t.Errorf("expected amount 9999, got %v", dataObj["amount"])
	}
	if dataObj["paid"] != false {
		t.Errorf("expected paid false, got %v", dataObj["paid"])
	}
	if dataObj["currency"] != "usd" {
		t.Errorf("expected currency usd, got %v", dataObj["currency"])
	}
	metadata := dataObj["metadata"].(map[string]any)
	if metadata["notes"] != "testing" {
		t.Errorf("expected metadata.notes testing, got %v", metadata["notes"])
	}
}

func TestDiffJSON(t *testing.T) {
	orig := []byte(`{"status": "pending"}`)
	mutated := []byte(`{"status": "completed"}`)

	diff := DiffJSON(orig, mutated)
	if !strings.Contains(diff, "-   \"status\": \"pending\"") && !strings.Contains(diff, "pending") {
		t.Errorf("diff should contain pending: %s", diff)
	}
	if !strings.Contains(diff, "+   \"status\": \"completed\"") && !strings.Contains(diff, "completed") {
		t.Errorf("diff should contain completed: %s", diff)
	}
}

func TestMutateJSONInvalid(t *testing.T) {
	_, err := MutateJSON([]byte(`not json`), []string{"a=b"})
	if err == nil {
		t.Errorf("expected error with invalid JSON")
	}

	_, err = MutateJSON([]byte(`{"a": 1}`), []string{"invalid_no_equals"})
	if err == nil {
		t.Errorf("expected error with invalid mutation expression")
	}
}
