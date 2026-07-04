package main

import "testing"

// TestHeaderFlagsParseRepeatedHeaders keeps live golden auth inputs reusable.
func TestHeaderFlagsParseRepeatedHeaders(t *testing.T) {
	var shared headerFlags
	if err := shared.Set("Authorization=Bearer token.with=equals"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	var endpoint headerFlags
	if err := endpoint.Set("X-Trace = phase2-session"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	headers, err := mergeHeaders(shared, endpoint)
	if err != nil {
		t.Fatalf("mergeHeaders returned error: %v", err)
	}
	if headers["Authorization"] != "Bearer token.with=equals" {
		t.Fatalf("Authorization header = %q", headers["Authorization"])
	}
	if headers["X-Trace"] != "phase2-session" {
		t.Fatalf("X-Trace header = %q", headers["X-Trace"])
	}
}

// TestHeaderFlagsRejectInvalidHeader avoids silent unauthenticated live diffs.
func TestHeaderFlagsRejectInvalidHeader(t *testing.T) {
	var headers headerFlags
	if err := headers.Set("Authorization"); err == nil {
		t.Fatal("Set accepted header without '='")
	}
	if _, err := mergeHeaders(headerFlags{"=Bearer token"}); err == nil {
		t.Fatal("mergeHeaders accepted empty key")
	}
}
