package sopplatform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceTestConnectionUsesHEADAndUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s, want HEAD", r.Method)
		}
		if r.Header.Get("User-Agent") != DefaultUserAgent {
			t.Fatalf("user agent = %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := (Service{Client: server.Client()}).TestConnection(context.Background(), Request{TaskURL: " " + server.URL + " "})

	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if !result.Success || result.Message != "连接成功 (HTTP 204)" {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceTestConnectionMapsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	result, err := (Service{Client: server.Client()}).TestConnection(context.Background(), Request{TaskURL: server.URL})

	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if result.Success || result.Message != "HTTP 错误: 404" {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceTestConnectionRequiresTaskURL(t *testing.T) {
	_, err := (Service{}).TestConnection(context.Background(), Request{})

	if !errors.Is(err, ErrTaskURLRequired) {
		t.Fatalf("error = %v, want task_url required", err)
	}
}

func TestServiceTestConnectionReturnsTrimmedConnectionError(t *testing.T) {
	longError := strings.Repeat("x", 120)
	result, err := (Service{Client: failingDoer{err: errors.New(longError)}}).TestConnection(context.Background(), Request{TaskURL: "https://example.test/task"})

	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if result.Success || !strings.HasPrefix(result.Message, "连接错误: ") || len(strings.TrimPrefix(result.Message, "连接错误: ")) != 100 {
		t.Fatalf("result = %+v", result)
	}
}

type failingDoer struct {
	err error
}

func (doer failingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, doer.err
}
