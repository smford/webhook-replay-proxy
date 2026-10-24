package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
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
		t.Errorf("expected joined string, got %s", a.String())
	}
}
