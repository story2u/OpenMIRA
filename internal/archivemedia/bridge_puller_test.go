package archivemedia

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPBridgePullerPullsChunkStreamUntilFinished(t *testing.T) {
	requests := []map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, payload)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("a")) + `","outindexbuf":"idx-2","is_finish":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("b")) + `","outindexbuf":"idx-final","is_finish":true}`))
	}))
	defer server.Close()

	result, err := (HTTPBridgePuller{PullURL: server.URL, PullToken: "token-1"}).PullArchiveMedia(context.Background(), PullInput{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		ArchiveMsgID: "am-1",
		SDKFileID:    "sdk-1",
	})
	if err != nil {
		t.Fatalf("PullArchiveMedia returned error: %v", err)
	}
	if !result.IsFinish || string(result.Content) != "ab" || result.NextIndex != "idx-final" {
		t.Fatalf("result = %#v", result)
	}
	if len(requests) != 2 || requests[0]["sdk_file_id"] != "sdk-1" || requests[0]["enterprise_id"] != "ent-1" || requests[1]["index_buf"] != "idx-2" {
		t.Fatalf("requests = %#v", requests)
	}
	if result.Response["assembled_bytes"] != 2 || result.Response["mode"] != "chunk_stream" {
		t.Fatalf("assembled response = %#v", result.Response)
	}
}

func TestHTTPBridgePullerAcceptsOneShotBase64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"file_base64":"` + base64.StdEncoding.EncodeToString([]byte("whole")) + `","is_finish":false}`))
	}))
	defer server.Close()

	result, err := (HTTPBridgePuller{PullURL: server.URL}).PullArchiveMedia(context.Background(), PullInput{SDKFileID: "sdk-1"})
	if err != nil {
		t.Fatalf("PullArchiveMedia returned error: %v", err)
	}
	if !result.IsFinish || string(result.Content) != "whole" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPBridgePullerRejectsNonAdvancingCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("a")) + `","outindexbuf":"same","is_finish":false}`))
	}))
	defer server.Close()

	_, err := (HTTPBridgePuller{PullURL: server.URL}).PullArchiveMedia(context.Background(), PullInput{SDKFileID: "sdk-1", IndexBuf: "same"})
	if err == nil || !strings.Contains(err.Error(), "cursor did not advance") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPBridgePullerReturnsStatusDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"bridge down"}`))
	}))
	defer server.Close()

	_, err := (HTTPBridgePuller{PullURL: server.URL}).PullArchiveMedia(context.Background(), PullInput{SDKFileID: "sdk-1"})
	if err == nil || !strings.Contains(err.Error(), "status=502 detail=bridge down") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeMediaPullResponseClassifiesPayloads(t *testing.T) {
	normalized, err := NormalizeMediaPullResponse(map[string]any{
		"download_url": "https://cdn/file.bin",
		"data_base64":  base64.StdEncoding.EncodeToString([]byte("chunk")),
		"is_finish":    "1",
	})
	if err != nil {
		t.Fatalf("NormalizeMediaPullResponse returned error: %v", err)
	}
	if normalized.DownloadURL != "https://cdn/file.bin" || len(normalized.ChunkBytes) != 0 || !normalized.IsFinish {
		t.Fatalf("normalized = %#v", normalized)
	}
}
