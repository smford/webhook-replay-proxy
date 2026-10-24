package proxy

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AssertReplay validates replay results against expected status codes and latency limits.
func AssertReplay(results []ReplayResult, assertStatus string, maxLatency time.Duration) error {
	if len(results) == 0 {
		return fmt.Errorf("assertion failed: no replay results to evaluate")
	}

	// 1. Validate max latency if specified
	if maxLatency > 0 {
		limitMs := maxLatency.Milliseconds()
		for _, res := range results {
			if res.LatencyMs > limitMs {
				return fmt.Errorf("latency assertion failed: request %d took %dms (max allowed: %s)",
					res.Index, res.LatencyMs, maxLatency)
			}
		}
	}

	// 2. Validate status code distribution if specified
	if assertStatus == "" {
		return nil
	}

	// Tally actual status codes
	actualCounts := make(map[int]int)
	for _, res := range results {
		actualCounts[res.StatusCode]++
	}

	parts := strings.Split(assertStatus, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if !strings.Contains(part, ":") {
			// Single code format (e.g. "200"): all responses must match this code
			expectedCode, err := strconv.Atoi(part)
			if err != nil {
				return fmt.Errorf("invalid assert-status format %q: expected status code integer", part)
			}
			for _, res := range results {
				if res.StatusCode != expectedCode {
					return fmt.Errorf("status assertion failed: expected all responses to have HTTP %d, but request %d returned %d",
						expectedCode, res.Index, res.StatusCode)
				}
			}
			return nil
		}

		// Count format (e.g. "200:1" or "409:9")
		kv := strings.SplitN(part, ":", 2)
		code, err := strconv.Atoi(strings.TrimSpace(kv[0]))
		if err != nil {
			return fmt.Errorf("invalid status code in assert-status %q: %w", kv[0], err)
		}
		expectedCount, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			return fmt.Errorf("invalid count in assert-status %q: %w", kv[1], err)
		}

		actual := actualCounts[code]
		if actual != expectedCount {
			return fmt.Errorf("status count assertion failed for HTTP %d: expected %d, got %d (actual distribution: %v)",
				code, expectedCount, actual, formatDistribution(actualCounts))
		}
	}

	return nil
}

func formatDistribution(counts map[int]int) string {
	var parts []string
	for code, count := range counts {
		parts = append(parts, fmt.Sprintf("%d:%d", code, count))
	}
	return strings.Join(parts, ", ")
}
