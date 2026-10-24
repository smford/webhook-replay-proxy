package main

import (
	"fmt"
	"testing"
)

func TestVersionVariables(t *testing.T) {
	if version == "" {
		t.Errorf("version string should not be empty")
	}
	versionOutput := fmt.Sprintf("webhook-replay-proxy version %s (commit: %s, built: %s)", version, commit, date)
	if len(versionOutput) == 0 {
		t.Errorf("versionOutput should not be empty")
	}
}
