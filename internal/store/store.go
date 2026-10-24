package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Event represents a recorded webhook HTTP request and target response.
type Event struct {
	ID            string              `json:"id"`
	Timestamp     time.Time           `json:"timestamp"`
	Provider      string              `json:"provider"`
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Query         string              `json:"query"`
	Headers       map[string][]string `json:"headers"`
	Body          []byte              `json:"body"`
	TargetURL     string              `json:"target_url"`
	ForwardStatus int                 `json:"forward_status"`
	ResponseBody  string              `json:"response_body"`
	LatencyMs     int64               `json:"latency_ms"`
}

// ReplayRecord tracks a replayed webhook execution.
type ReplayRecord struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Timestamp time.Time `json:"timestamp"`
	TargetURL string    `json:"target_url"`
	Status    int       `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error"`
}

// Store handles persistent SQLite storage for recorded webhooks.
type Store struct {
	db *sql.DB
}

// Open creates or connects to a pure-Go SQLite database and applies migrations.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database at %s: %w", dbPath, err)
	}

	// Set connection pool limits appropriate for SQLite
	db.SetMaxOpenConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		provider TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		query TEXT NOT NULL,
		headers TEXT NOT NULL,
		body BLOB NOT NULL,
		target_url TEXT NOT NULL,
		forward_status INTEGER NOT NULL,
		response_body TEXT,
		latency_ms INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC);

	CREATE TABLE IF NOT EXISTS replays (
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		target_url TEXT NOT NULL,
		status INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL,
		success INTEGER NOT NULL,
		error TEXT,
		FOREIGN KEY(event_id) REFERENCES events(id)
	);
	`

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("executing sqlite schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying SQLite database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveEvent persists a recorded webhook event.
func (s *Store) SaveEvent(e *Event) error {
	headerJSON, err := json.Marshal(e.Headers)
	if err != nil {
		return fmt.Errorf("serializing headers: %w", err)
	}

	query := `
	INSERT INTO events (id, timestamp, provider, method, path, query, headers, body, target_url, forward_status, response_body, latency_ms)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = s.db.Exec(query,
		e.ID,
		e.Timestamp.UTC().Format(time.RFC3339Nano),
		e.Provider,
		e.Method,
		e.Path,
		e.Query,
		string(headerJSON),
		e.Body,
		e.TargetURL,
		e.ForwardStatus,
		e.ResponseBody,
		e.LatencyMs,
	)
	if err != nil {
		return fmt.Errorf("saving event %s: %w", e.ID, err)
	}
	return nil
}

// GetEvent retrieves a recorded event by its unique ID.
func (s *Store) GetEvent(id string) (*Event, error) {
	query := `
	SELECT id, timestamp, provider, method, path, query, headers, body, target_url, forward_status, response_body, latency_ms
	FROM events WHERE id = ?
	`
	row := s.db.QueryRow(query, id)

	var (
		e           Event
		tsStr       string
		headersJSON string
	)

	err := row.Scan(
		&e.ID,
		&tsStr,
		&e.Provider,
		&e.Method,
		&e.Path,
		&e.Query,
		&headersJSON,
		&e.Body,
		&e.TargetURL,
		&e.ForwardStatus,
		&e.ResponseBody,
		&e.LatencyMs,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("event not found: %s", id)
		}
		return nil, fmt.Errorf("querying event %s: %w", id, err)
	}

	e.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
	if err := json.Unmarshal([]byte(headersJSON), &e.Headers); err != nil {
		e.Headers = make(map[string][]string)
	}

	return &e, nil
}

// ListEvents lists up to limit recent events ordered by descending timestamp.
func (s *Store) ListEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
	SELECT id, timestamp, provider, method, path, query, headers, body, target_url, forward_status, response_body, latency_ms
	FROM events ORDER BY timestamp DESC LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var (
			e           Event
			tsStr       string
			headersJSON string
		)
		err := rows.Scan(
			&e.ID,
			&tsStr,
			&e.Provider,
			&e.Method,
			&e.Path,
			&e.Query,
			&headersJSON,
			&e.Body,
			&e.TargetURL,
			&e.ForwardStatus,
			&e.ResponseBody,
			&e.LatencyMs,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
		_ = json.Unmarshal([]byte(headersJSON), &e.Headers)
		events = append(events, e)
	}

	return events, nil
}

// RecordReplay stores an entry in the replays history table.
func (s *Store) RecordReplay(r *ReplayRecord) error {
	query := `
	INSERT INTO replays (id, event_id, timestamp, target_url, status, latency_ms, success, error)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	successInt := 0
	if r.Success {
		successInt = 1
	}

	_, err := s.db.Exec(query,
		r.ID,
		r.EventID,
		r.Timestamp.UTC().Format(time.RFC3339Nano),
		r.TargetURL,
		r.Status,
		r.LatencyMs,
		successInt,
		r.Error,
	)
	if err != nil {
		return fmt.Errorf("recording replay %s: %w", r.ID, err)
	}
	return nil
}
