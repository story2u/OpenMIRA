package archivecoldstorage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPObjectFinalizerUploadsLocalParquetArtifact(t *testing.T) {
	var gotEnterpriseID string
	var gotSDKFileID string
	var gotFilename string
	var gotContentType string
	var gotContent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upload-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "enterprise_id":
				gotEnterpriseID = string(data)
			case "sdk_file_id":
				gotSDKFileID = string(data)
			case "file":
				gotFilename = part.FileName()
				gotContentType = part.Header.Get("Content-Type")
				gotContent = data
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object_url": "http://object-storage:9102/objects/tenant-a/messages.parquet"})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "messages.parquet")
	if err := os.WriteFile(path, []byte("parquet-bytes"), 0o644); err != nil {
		t.Fatalf("write parquet fixture: %v", err)
	}
	result, err := (HTTPObjectFinalizer{
		UploadURL:   server.URL,
		UploadToken: "upload-token",
		Timeout:     time.Second,
	}).FinalizeObject(context.Background(), ObjectFinalizeInput{
		EnterpriseID:  " tenant-a ",
		SDKFileID:     " encrypted_messages_2026_01 ",
		LocalFilePath: path,
	})
	if err != nil {
		t.Fatalf("FinalizeObject returned error: %v", err)
	}
	if result.Backend != "http_uploader" || result.ObjectURL != "http://object-storage:9102/objects/tenant-a/messages.parquet" {
		t.Fatalf("result = %#v", result)
	}
	if gotEnterpriseID != "tenant-a" || gotSDKFileID != "encrypted_messages_2026_01" || gotFilename != "messages.parquet" || gotContentType != "application/octet-stream" || string(gotContent) != "parquet-bytes" {
		t.Fatalf("multipart enterprise=%q sdk=%q filename=%q contentType=%q content=%q", gotEnterpriseID, gotSDKFileID, gotFilename, gotContentType, string(gotContent))
	}
}

func TestHTTPObjectFinalizerSkipsMissingLocalFile(t *testing.T) {
	result, err := (HTTPObjectFinalizer{UploadURL: "https://objects.example/upload"}).FinalizeObject(context.Background(), ObjectFinalizeInput{
		LocalFilePath: filepath.Join(t.TempDir(), "missing.parquet"),
	})
	if err != nil {
		t.Fatalf("FinalizeObject returned error: %v", err)
	}
	if result.Backend != "http_uploader" || result.ObjectURL != "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPObjectFinalizerRequiresUploadURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.parquet")
	if err := os.WriteFile(path, []byte("parquet-bytes"), 0o644); err != nil {
		t.Fatalf("write parquet fixture: %v", err)
	}
	_, err := (HTTPObjectFinalizer{}).FinalizeObject(context.Background(), ObjectFinalizeInput{LocalFilePath: path})
	if err == nil || !strings.Contains(err.Error(), "ARCHIVE_MEDIA_OBJECT_UPLOAD_URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPObjectFinalizerReturnsStatusDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad token"}`))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "messages.parquet")
	if err := os.WriteFile(path, []byte("parquet-bytes"), 0o644); err != nil {
		t.Fatalf("write parquet fixture: %v", err)
	}

	_, err := (HTTPObjectFinalizer{UploadURL: server.URL}).FinalizeObject(context.Background(), ObjectFinalizeInput{LocalFilePath: path})
	if err == nil || !strings.Contains(err.Error(), "status=401 detail=bad token") {
		t.Fatalf("error = %v", err)
	}
}
