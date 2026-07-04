package archivemedia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPUploaderUploadsMultipartAndReadsObjectURL(t *testing.T) {
	var gotEnterprise string
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
				gotEnterprise = string(data)
			case "sdk_file_id":
				gotSDKFileID = string(data)
			case "file":
				gotFilename = part.FileName()
				gotContentType = part.Header.Get("Content-Type")
				gotContent = data
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object_url": "http://object-storage:9102/objects/ent-1/file.png"})
	}))
	defer server.Close()

	objectURL, err := (HTTPUploader{UploadURL: server.URL, UploadToken: "upload-token"}).UploadArchiveMedia(context.Background(), UploadInput{
		EnterpriseID: "ent-1",
		SDKFileID:    "sdk-1",
		PayloadJSON:  `{"decrypted":{"msgtype":"image"}}`,
		Content:      []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
	})
	if err != nil {
		t.Fatalf("UploadArchiveMedia returned error: %v", err)
	}
	if objectURL != "http://object-storage:9102/objects/ent-1/file.png" {
		t.Fatalf("objectURL = %q", objectURL)
	}
	if gotEnterprise != "ent-1" || gotSDKFileID != "sdk-1" || gotFilename != "sdk-1.png" || gotContentType != "image/png" || len(gotContent) == 0 {
		t.Fatalf("multipart enterprise=%q sdk=%q filename=%q contentType=%q bytes=%d", gotEnterprise, gotSDKFileID, gotFilename, gotContentType, len(gotContent))
	}
}

func TestHTTPUploaderUsesConfiguredUploadMeta(t *testing.T) {
	var gotFilename string
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if part.FormName() == "file" {
				gotFilename = part.FileName()
				gotContentType = part.Header.Get("Content-Type")
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object_url": "https://cdn.example/custom-name.png"})
	}))
	defer server.Close()

	_, err := (HTTPUploader{UploadURL: server.URL}).UploadArchiveMedia(context.Background(), UploadInput{
		EnterpriseID: "ent-1",
		SDKFileID:    "sdk-1",
		Filename:     "custom-name.png",
		ContentType:  "image/png",
		PayloadJSON:  `{"decrypted":{"msgtype":"file","filename":"legacy.bin"}}`,
		Content:      []byte("plain bytes"),
	})
	if err != nil {
		t.Fatalf("UploadArchiveMedia returned error: %v", err)
	}
	if gotFilename != "custom-name.png" || gotContentType != "image/png" {
		t.Fatalf("filename=%q contentType=%q", gotFilename, gotContentType)
	}
}

func TestHTTPUploaderReturnsStatusDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad token"}`))
	}))
	defer server.Close()

	_, err := (HTTPUploader{UploadURL: server.URL}).UploadArchiveMedia(context.Background(), UploadInput{SDKFileID: "sdk-1", Content: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "status=401 detail=bad token") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPUploaderDeletesArchiveMediaViaDerivedEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/delete" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer upload-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotPath = r.Form.Get("object_path")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
	}))
	defer server.Close()

	deleted, err := (HTTPUploader{UploadURL: server.URL + "/internal/upload", UploadToken: "upload-token"}).DeleteArchiveMedia(context.Background(), "http://object-storage:9102/objects/ent-1/file.png?token=x")
	if err != nil {
		t.Fatalf("DeleteArchiveMedia returned error: %v", err)
	}
	if !deleted || gotPath != "ent-1/file.png" {
		t.Fatalf("deleted=%t object_path=%q", deleted, gotPath)
	}
}

func TestHTTPUploaderDeleteSkipsNonObjectURL(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	deleted, err := (HTTPUploader{DeleteURL: server.URL + "/internal/delete"}).DeleteArchiveMedia(context.Background(), "https://cloud.example/media/file.png")
	if err != nil {
		t.Fatalf("DeleteArchiveMedia returned error: %v", err)
	}
	if deleted || called {
		t.Fatalf("deleted=%t called=%t", deleted, called)
	}
}

func TestResolveUploadMetaSniffsArchivePayload(t *testing.T) {
	tests := []struct {
		name        string
		payloadJSON string
		content     []byte
		wantName    string
		wantType    string
	}{
		{
			name:        "voice amr",
			payloadJSON: `{"decrypted":{"msgtype":"voice"}}`,
			content:     []byte("#!AMR\nvoice"),
			wantName:    "sdk-1.amr",
			wantType:    "audio/amr",
		},
		{
			name:        "file keeps original name",
			payloadJSON: `{"decrypted":{"msgtype":"file","file":{"filename":"report.pdf"}}}`,
			content:     []byte("%PDF"),
			wantName:    "report.pdf",
			wantType:    "application/pdf",
		},
		{
			name:        "video mp4",
			payloadJSON: `{"decrypted":{"msgtype":"video"}}`,
			content:     []byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'},
			wantName:    "sdk-1.mp4",
			wantType:    "video/mp4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, contentType := ResolveUploadMeta("sdk-1", tt.payloadJSON, tt.content)
			if filename != tt.wantName || contentType != tt.wantType {
				t.Fatalf("meta = %q %q", filename, contentType)
			}
		})
	}
}

func TestEscapeMultipartFilenameEscapesQuotes(t *testing.T) {
	if got := escapeMultipartFilename(`a"b.bin`); got != `a\"b.bin` {
		t.Fatalf("escaped = %q", got)
	}
}
