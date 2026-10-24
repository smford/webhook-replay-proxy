package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/store"
)

// Fixture represents an exported webhook event suitable for version-controlled test suites.
type Fixture struct {
	ID            string              `json:"id"`
	Timestamp     time.Time           `json:"timestamp"`
	Provider      string              `json:"provider"`
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Query         string              `json:"query,omitempty"`
	Headers       map[string][]string `json:"headers"`
	Payload       json.RawMessage     `json:"payload"`
	TargetURL     string              `json:"target_url,omitempty"`
	ForwardStatus int                 `json:"forward_status,omitempty"`
	ResponseBody  string              `json:"response_body,omitempty"`
	LatencyMs     int64               `json:"latency_ms,omitempty"`
}

// ExportFixture converts a store.Event into pretty-printed JSON fixture bytes.
func ExportFixture(event *store.Event) ([]byte, error) {
	if event == nil {
		return nil, fmt.Errorf("event cannot be nil")
	}

	payloadRaw := json.RawMessage(event.Body)
	// Validate if it is valid JSON; if not, wrap as raw string
	if !json.Valid(event.Body) {
		encoded, err := json.Marshal(string(event.Body))
		if err != nil {
			return nil, fmt.Errorf("encoding raw body: %w", err)
		}
		payloadRaw = json.RawMessage(encoded)
	}

	f := Fixture{
		ID:            event.ID,
		Timestamp:     event.Timestamp,
		Provider:      event.Provider,
		Method:        event.Method,
		Path:          event.Path,
		Query:         event.Query,
		Headers:       event.Headers,
		Payload:       payloadRaw,
		TargetURL:     event.TargetURL,
		ForwardStatus: event.ForwardStatus,
		ResponseBody:  event.ResponseBody,
		LatencyMs:     event.LatencyMs,
	}

	return json.MarshalIndent(f, "", "  ")
}

// LoadFixture reads and parses a JSON fixture file from disk into a store.Event.
func LoadFixture(filePath string) (*store.Event, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading fixture file %s: %w", filePath, err)
	}

	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing fixture JSON: %w", err)
	}

	bodyBytes := []byte(f.Payload)
	// If the payload was encoded as a quoted JSON string, unquote it
	var strVal string
	if len(bodyBytes) > 0 && bodyBytes[0] == '"' && json.Unmarshal(bodyBytes, &strVal) == nil {
		bodyBytes = []byte(strVal)
	}

	method := f.Method
	if method == "" {
		method = "POST"
	}

	headers := f.Headers
	if headers == nil {
		headers = make(map[string][]string)
	}

	return &store.Event{
		ID:            f.ID,
		Timestamp:     f.Timestamp,
		Provider:      f.Provider,
		Method:        method,
		Path:          f.Path,
		Query:         f.Query,
		Headers:       headers,
		Body:          bodyBytes,
		TargetURL:     f.TargetURL,
		ForwardStatus: f.ForwardStatus,
		ResponseBody:  f.ResponseBody,
		LatencyMs:     f.LatencyMs,
	}, nil
}
