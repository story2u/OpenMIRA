package sopmedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
)

func TestUploadStoresSOPMediaAndBuildsSignedPreview(t *testing.T) {
	uploader := &fakeUploader{objectURL: "/objects/sop-config/welcome.png"}
	service := Service{
		Uploader: uploader,
		Access: archivemedia.AccessURLBuilder{
			SigningKey: "media-secret",
			TokenTTL:   time.Minute,
			Now: func() time.Time {
				return time.Unix(1000, 0).UTC()
			},
		},
		RandomSuffix: suffixes("a1b2c3d4", "feedface"),
	}
	content := []byte("image bytes")

	result, err := service.Upload(context.Background(), Request{
		MediaType:   " Image ",
		Filename:    "welcome.png",
		ContentType: "image/png",
		Content:     content,
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

	sum := sha256.Sum256(content)
	wantSDKFileID := "sop-image-" + hex.EncodeToString(sum[:])[:24] + "-a1b2c3d4"
	if uploader.input.EnterpriseID != "sop-config" || uploader.input.SDKFileID != wantSDKFileID || uploader.input.Filename != "welcome.png" || uploader.input.ContentType != "image/png" || string(uploader.input.Content) != string(content) {
		t.Fatalf("upload input = %+v", uploader.input)
	}
	if !result.Success || result.MediaType != "image" || result.ObjectURL != uploader.objectURL || result.Filename != "welcome.png" || result.ContentType != "image/png" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.HasPrefix(result.AccessURL, "/api/v1/archive/media/objects/sop-config/welcome.png?token=") {
		t.Fatalf("access url = %s", result.AccessURL)
	}
}

func TestUploadKeepsRemotePreviewURLRaw(t *testing.T) {
	service := Service{
		Uploader:     &fakeUploader{objectURL: "https://cdn.example/media/welcome.png"},
		RandomSuffix: suffixes("a1b2c3d4", "feedface"),
	}

	result, err := service.Upload(context.Background(), Request{
		MediaType: "image",
		Content:   []byte("image bytes"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.AccessURL != "https://cdn.example/media/welcome.png" {
		t.Fatalf("access url = %q", result.AccessURL)
	}
}

func TestUploadBuildsLocalPreviewURLWithPythonEscaping(t *testing.T) {
	service := Service{
		Uploader:     &fakeUploader{objectURL: "local://sop/media/welcome image.png"},
		RandomSuffix: suffixes("a1b2c3d4", "feedface"),
	}

	result, err := service.Upload(context.Background(), Request{
		MediaType:   "video",
		Filename:    "",
		ContentType: "",
		Content:     []byte("video bytes"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.AccessURL != "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fmedia%2Fwelcome%20image.png" {
		t.Fatalf("access url = %q", result.AccessURL)
	}
	if serviceUploader, ok := service.Uploader.(*fakeUploader); ok {
		if serviceUploader.input.Filename != "video.bin" || serviceUploader.input.ContentType != "video/*" {
			t.Fatalf("default upload meta = filename %q content_type %q", serviceUploader.input.Filename, serviceUploader.input.ContentType)
		}
	}
}

func TestUploadValidatesLegacyMediaFields(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr error
		want    string
	}{
		{
			name:    "invalid media type",
			request: Request{MediaType: "audio", Filename: "clip.mp3", ContentType: "audio/mpeg", Content: []byte("x")},
			wantErr: ErrInvalidMediaType,
			want:    "media_type must be image or video",
		},
		{
			name:    "blocked extension",
			request: Request{MediaType: "image", Filename: "bad.sh", ContentType: "image/png", Content: []byte("x")},
			wantErr: ErrBlockedExtension,
			want:    "不允许上传该类型文件：.sh",
		},
		{
			name:    "unsupported mime",
			request: Request{MediaType: "image", Filename: "bad.png", ContentType: "text/plain", Content: []byte("x")},
			wantErr: ErrUnsupportedMIME,
			want:    "不支持的文件类型：text/plain",
		},
		{
			name:    "empty content",
			request: Request{MediaType: "image", Filename: "empty.png", ContentType: "image/png"},
			wantErr: ErrContentEmpty,
			want:    "upload content is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (Service{Uploader: &fakeUploader{objectURL: "https://cdn.example/media.png"}}).Upload(context.Background(), tt.request)
			if !errors.Is(err, tt.wantErr) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %v containing %q", err, tt.wantErr, tt.want)
			}
		})
	}
}

func TestUploadRequiresConfiguredUploader(t *testing.T) {
	_, err := (Service{}).Upload(context.Background(), Request{
		MediaType:   "image",
		Filename:    "welcome.png",
		ContentType: "image/png",
		Content:     []byte("x"),
	})
	if !errors.Is(err, ErrUploaderMissing) {
		t.Fatalf("error = %v", err)
	}
}

func TestUploadMapsEmptyObjectURL(t *testing.T) {
	_, err := (Service{Uploader: &fakeUploader{objectURL: "  "}}).Upload(context.Background(), Request{
		MediaType:   "image",
		Filename:    "welcome.png",
		ContentType: "image/png",
		Content:     []byte("x"),
	})
	if !errors.Is(err, ErrObjectURLMissing) {
		t.Fatalf("error = %v", err)
	}
}

type fakeUploader struct {
	input     archivemedia.UploadInput
	objectURL string
	err       error
}

func (uploader *fakeUploader) UploadArchiveMedia(ctx context.Context, input archivemedia.UploadInput) (string, error) {
	uploader.input = input
	if uploader.err != nil {
		return "", uploader.err
	}
	return uploader.objectURL, nil
}

func suffixes(values ...string) func() string {
	index := 0
	return func() string {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
