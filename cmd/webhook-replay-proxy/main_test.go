package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/store"
)

func TestPrintVersion(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printVersion()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "webhook-replay-proxy") {
		t.Errorf("version output missing binary name: %s", out)
	}
	if !strings.Contains(out, "Copyright (c) 2026 smford") {
		t.Errorf("version output missing copyright: %s", out)
	}
	if !strings.Contains(out, "Licensed under the MIT License") {
		t.Errorf("version output missing license: %s", out)
	}
	if !strings.Contains(out, "https://github.com/smford/webhook-replay-proxy") {
		t.Errorf("version output missing github URL: %s", out)
	}
}

func TestPrintUsage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	commands := []string{"listen", "list", "inspect", "export", "replay", "curl", "version"}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("usage output missing command: %s", cmd)
		}
	}
}

func TestArrayFlags(t *testing.T) {
	var a arrayFlags
	if err := a.Set("data.amount=100"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := a.Set("type=failed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(a) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(a))
	}
	if a.String() != "data.amount=100, type=failed" {
		t.Fatalf("expected string representation, got: %s", a.String())
	}
}

func TestRunExportAndCurlWithFixture(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cmd_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	eventID := "evt_cmd_test_01"
	_ = st.SaveEvent(&store.Event{
		ID:        eventID,
		Timestamp: time.Now().UTC(),
		Provider:  "stripe",
		Method:    "POST",
		Path:      "/webhook",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body:      []byte(`{"id":"evt_123","type":"invoice.paid"}`),
		TargetURL: "http://localhost:3000/api/webhook",
	})
	st.Close()

	// 1. Test runExport to file
	outFile := filepath.Join(t.TempDir(), "fixture.json")
	runExport([]string{eventID, "--db", dbPath, "-o", outFile})

	if _, err := os.Stat(outFile); os.IsNotExist(err) {
		t.Fatalf("expected fixture file to be created at %s", outFile)
	}

	// 2. Test runCurl using --file
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runCurl([]string{"--file", outFile, "--target", "http://localhost:4000/hook", "--secret", "whsec_test", "--corrupt-sig"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	curlOutput := buf.String()

	if !strings.Contains(curlOutput, "curl -X POST \"http://localhost:4000/hook\"") {
		t.Errorf("curl output missing target URL: %s", curlOutput)
	}
	if !strings.Contains(curlOutput, "deadbeef") {
		t.Errorf("curl output missing corrupted signature: %s", curlOutput)
	}
}
