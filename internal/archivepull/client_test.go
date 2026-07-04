package archivepull

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientPullSelfDecryptPostsPayloadAndNormalizesResponse(t *testing.T) {
	cursor := "12"
	var capturedPayload map[string]any
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		capturedAuth = request.Header.Get("Authorization")
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(request.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"source":"self_decrypt","cursor":"20","messages":[{"archive_msgid":"msg-1"}]}`))
	}))
	defer server.Close()

	result, err := (Client{PullURL: server.URL, PullToken: " token-1 "}).PullSelfDecrypt(context.Background(), PullInput{
		Source:       "self_decrypt",
		Cursor:       &cursor,
		Limit:        3000,
		EnterpriseID: "ent-1",
		Mode:         "self_decrypt",
	})
	if err != nil {
		t.Fatalf("PullSelfDecrypt returned error: %v", err)
	}
	if capturedAuth != "Bearer token-1" {
		t.Fatalf("auth = %q", capturedAuth)
	}
	if capturedPayload["cursor"] != "12" || capturedPayload["limit"] != float64(2000) || capturedPayload["enterprise_id"] != "ent-1" || capturedPayload["archive_mode"] != "self_decrypt" {
		t.Fatalf("payload = %#v", capturedPayload)
	}
	if result.Source != "self_decrypt" || result.Cursor == nil || *result.Cursor != "20" || len(result.Messages) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientPullSelfDecryptReturnsStatusDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"detail":"ret=10010"}`))
	}))
	defer server.Close()

	_, err := (Client{PullURL: server.URL}).PullSelfDecrypt(context.Background(), PullInput{Source: "self_decrypt", Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "status=502 detail=ret=10010") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientPullSelfDecryptUsesInputEndpointAndToken(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		capturedAuth = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"source":"self_decrypt","cursor":"20","messages":[]}`))
	}))
	defer server.Close()

	_, err := (Client{PullURL: "https://default.invalid", PullToken: "default-token"}).PullSelfDecrypt(context.Background(), PullInput{
		Source:    "self_decrypt",
		Limit:     1,
		PullURL:   server.URL,
		PullToken: " enterprise-token ",
	})
	if err != nil {
		t.Fatalf("PullSelfDecrypt returned error: %v", err)
	}
	if capturedAuth != "Bearer enterprise-token" {
		t.Fatalf("auth = %q", capturedAuth)
	}
}

func TestClientPullSelfDecryptRequiresURL(t *testing.T) {
	_, err := (Client{}).PullSelfDecrypt(context.Background(), PullInput{Source: "self_decrypt"})
	if err == nil || !strings.Contains(err.Error(), "ARCHIVE_SELF_DECRYPT_PULL_URL is not configured") {
		t.Fatalf("error = %v", err)
	}
}
