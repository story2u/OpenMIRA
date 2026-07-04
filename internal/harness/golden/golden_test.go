package golden

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareMatchesCanonicalJSON(t *testing.T) {
	reference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeader(t, r, "X-Case", "phase1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"trace_id":"python","data":{"items":[{"name":"a","updated_at":"old"}]}}`))
	}))
	defer reference.Close()

	goTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeader(t, r, "X-Case", "phase1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"updated_at":"new","name":"a"}]},"trace_id":"go","ok":true}`))
	}))
	defer goTarget.Close()

	result, err := Compare(context.Background(), nil, Endpoint{
		Name:    "reference",
		BaseURL: reference.URL,
	}, Endpoint{
		Name:    "go",
		BaseURL: goTarget.URL,
	}, Case{
		Name:    "probe",
		Path:    "/healthz",
		Headers: map[string]string{"X-Case": "phase1"},
	}, Options{
		IgnoreJSONFields: []string{"trace_id", "data.items.updated_at"},
	})
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if !result.Match {
		t.Fatalf("result.Match = false, diffs=%v", result.Diffs)
	}
}

func TestCompareReportsStatusAndBodyDrift(t *testing.T) {
	reference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer reference.Close()

	goTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer goTarget.Close()

	result, err := Compare(context.Background(), nil, Endpoint{Name: "reference", BaseURL: reference.URL}, Endpoint{Name: "go", BaseURL: goTarget.URL}, Case{Name: "probe"}, Options{})
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if result.Match {
		t.Fatal("result.Match = true, want false")
	}
	lines := strings.Join(StableDiffLines(result), "\n")
	if !strings.Contains(lines, "status: reference=200 go=503") {
		t.Fatalf("missing status drift: %s", lines)
	}
	if !strings.Contains(lines, "body: normalized response differs") {
		t.Fatalf("missing body drift: %s", lines)
	}
}

// TestCompareCanSkipBodyComparison keeps volatile probe bodies out of diffs.
func TestCompareCanSkipBodyComparison(t *testing.T) {
	reference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reference metrics"))
	}))
	defer reference.Close()

	goTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("go metrics"))
	}))
	defer goTarget.Close()

	result, err := Compare(context.Background(), nil, Endpoint{Name: "reference", BaseURL: reference.URL}, Endpoint{Name: "go", BaseURL: goTarget.URL}, Case{
		Name:            "metrics",
		Path:            "/metrics",
		SkipBodyCompare: true,
	}, Options{})
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if !result.Match {
		t.Fatalf("result.Match = false, diffs=%v", result.Diffs)
	}
}

func assertHeader(t *testing.T, request *http.Request, key string, want string) {
	t.Helper()
	if got := request.Header.Get(key); got != want {
		t.Fatalf("%s header = %q, want %q", key, got, want)
	}
}
