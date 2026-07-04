package archivemedia

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wework-go/internal/infra/archivemediatask"
)

func TestDownloadTaskReadsLocalArchiveMediaFile(t *testing.T) {
	dataRoot := t.TempDir()
	mediaPath := filepath.Join(dataRoot, "archive_media", "ent-1")
	if err := os.MkdirAll(mediaPath, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaPath, "image.png"), []byte("image-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	builder := testAccessBuilder()
	service := DownloadService{
		Tasks: &fakeTaskReader{record: &archivemediatask.Record{
			TaskID:    "task-1",
			SDKFileID: "sdk-1",
			ObjectURL: "local://archive_media/ent-1/image.png",
		}},
		Access:        builder,
		LocalDataRoot: dataRoot,
	}

	download, err := service.DownloadTask(context.Background(), "task-1", builder.signPayload("task", "task-1"))
	if err != nil {
		t.Fatalf("DownloadTask returned error: %v", err)
	}
	defer download.Body.Close()
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(body) != "image-bytes" || download.ContentType != "image/png" || download.Filename != "sdk-1.png" || download.ContentLength != int64(len("image-bytes")) {
		t.Fatalf("download = body %q type %q filename %q len %d", body, download.ContentType, download.Filename, download.ContentLength)
	}
}

func TestDownloadLocalObjectReadsLocalSOPMediaFile(t *testing.T) {
	dataRoot := t.TempDir()
	mediaPath := filepath.Join(dataRoot, "sop", "media")
	if err := os.MkdirAll(mediaPath, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaPath, "clip.mov"), []byte("mov-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	service := DownloadService{LocalDataRoot: dataRoot}

	download, err := service.DownloadLocalObject(context.Background(), "local://sop/media/clip.mov")
	if err != nil {
		t.Fatalf("DownloadLocalObject returned error: %v", err)
	}
	defer download.Body.Close()
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(body) != "mov-bytes" || download.ContentType != "video/quicktime" || download.Filename != "clip.mov" || download.ContentLength != int64(len("mov-bytes")) {
		t.Fatalf("download = body %q type %q filename %q len %d", body, download.ContentType, download.Filename, download.ContentLength)
	}
}

func TestDownloadLocalObjectRejectsTraversal(t *testing.T) {
	service := DownloadService{LocalDataRoot: t.TempDir()}

	_, err := service.DownloadLocalObject(context.Background(), "local://../secret.png")

	if !errors.Is(err, ErrMediaLocalFileNotFound) {
		t.Fatalf("error = %v, want local file not found", err)
	}
}

func TestDownloadTaskRejectsInvalidTokenBeforeStoreRead(t *testing.T) {
	storeErr := errors.New("store should not be read")
	service := DownloadService{Tasks: &fakeTaskReader{err: storeErr}, Access: testAccessBuilder()}

	_, err := service.DownloadTask(context.Background(), "task-1", "invalid")

	if !errors.Is(err, ErrMediaAccessTokenInvalid) {
		t.Fatalf("error = %v, want invalid token", err)
	}
}

func TestDownloadObjectProxiesObjectStorage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/objects/ent-1/file.png" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer upstream.Close()
	builder := testAccessBuilder()
	service := DownloadService{
		Access:                builder,
		ObjectInternalBaseURL: upstream.URL,
		HTTPClient:            upstream.Client(),
	}

	download, err := service.DownloadObject(context.Background(), "ent-1/file.png", builder.signPayload("object", "ent-1/file.png"))
	if err != nil {
		t.Fatalf("DownloadObject returned error: %v", err)
	}
	defer download.Body.Close()
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(body) != "png-bytes" || download.ContentType != "image/png" || download.Filename != "file.png" {
		t.Fatalf("download = body %q type %q filename %q", body, download.ContentType, download.Filename)
	}
}

func TestDownloadObjectBuildsDefaultObjectStorageURL(t *testing.T) {
	service := DownloadService{}
	if got := service.BuildObjectUpstreamURL("/ent-1/file.png"); got != DefaultObjectInternalBaseURL+"/objects/ent-1/file.png" {
		t.Fatalf("upstream url = %q", got)
	}
}

func TestExtractLocalObjectPathRejectsNonLocalURL(t *testing.T) {
	if got := ExtractLocalObjectPath("http://object-storage:9102/objects/ent-1/file.png"); got != "" {
		t.Fatalf("local path = %q", got)
	}
	if got := ExtractLocalObjectPath("local://archive_media/ent-1/file.png"); got != "archive_media/ent-1/file.png" {
		t.Fatalf("local path = %q", got)
	}
}

func testAccessBuilder() AccessURLBuilder {
	return AccessURLBuilder{
		SigningKey: "media-secret",
		TokenTTL:   time.Minute,
		Now: func() time.Time {
			return time.Unix(5000, 0).UTC()
		},
	}
}

type fakeTaskReader struct {
	record *archivemediatask.Record
	err    error
}

func (reader *fakeTaskReader) GetTask(context.Context, string) (*archivemediatask.Record, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	return reader.record, nil
}

func TestGuessDownloadMetaFallsBackToOctetStream(t *testing.T) {
	contentType, extension := GuessDownloadMeta(strings.TrimSpace(""))
	if contentType != "application/octet-stream" || extension != ".bin" {
		t.Fatalf("meta = %q %q", contentType, extension)
	}
}
