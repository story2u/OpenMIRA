package avatarstorage

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
)

func TestPersistAvatarReferenceUploadsInlineDataURI(t *testing.T) {
	content := []byte("\x89PNG\r\n\x1a\navatar-bytes")
	uploader := &fakeAvatarUploader{objectURL: "https://objects.internal/objects/ent-1/avatar.png"}
	service := Service{Uploader: uploader}

	stored := service.PersistAvatarReference(context.Background(), " ent-1 ", " external-contact:wm-1 ", " data:image/png;base64,"+base64.StdEncoding.EncodeToString(content)+" ")

	if stored != uploader.objectURL {
		t.Fatalf("stored avatar = %q", stored)
	}
	if uploader.input.EnterpriseID != "ent-1" || uploader.input.Filename != "avatar.png" || uploader.input.ContentType != "image/png" {
		t.Fatalf("upload input = %+v", uploader.input)
	}
	if !strings.HasPrefix(uploader.input.SDKFileID, "avatar-") || string(uploader.input.Content) != string(content) {
		t.Fatalf("upload input = %+v", uploader.input)
	}
}

func TestPersistAvatarReferenceAcceptsCaseInsensitiveDataURI(t *testing.T) {
	content := []byte("\x89PNG\r\n\x1a\navatar-bytes")
	uploader := &fakeAvatarUploader{objectURL: "https://objects.internal/objects/ent-1/avatar.png"}
	service := Service{Uploader: uploader}

	stored := service.PersistAvatarReference(context.Background(), "ent-1", "avatar", "DATA:image/png;base64,"+base64.StdEncoding.EncodeToString(content))

	if stored != uploader.objectURL || uploader.input.ContentType != "image/png" {
		t.Fatalf("stored=%q upload=%+v", stored, uploader.input)
	}
}

func TestPersistAvatarReferenceUploadsRawBase64AsPNG(t *testing.T) {
	content := []byte("\x89PNG\r\n\x1a\n0123456789abcdef0123456789abcdef")
	uploader := &fakeAvatarUploader{objectURL: "local://archive_media/ent-1/avatar.png"}
	service := Service{Uploader: uploader}

	stored := service.PersistAvatarReference(context.Background(), "ent-1", "corp-user:dy1", base64.StdEncoding.EncodeToString(content))

	if stored != uploader.objectURL || uploader.input.ContentType != "image/png" || uploader.input.Filename != "avatar.png" {
		t.Fatalf("stored=%q upload=%+v", stored, uploader.input)
	}
}

func TestPersistAvatarReferenceKeepsObjectAndRemoteURLs(t *testing.T) {
	uploader := &fakeAvatarUploader{objectURL: "unexpected"}
	service := Service{Uploader: uploader}

	for _, value := range []string{
		"https://object-storage:9102/objects/ent-1/avatar.png",
		"ent-1/avatar.png",
		"https://cdn.example/avatar.png",
	} {
		if stored := service.PersistAvatarReference(context.Background(), "ent-1", "avatar", " "+value+" "); stored != value {
			t.Fatalf("stored avatar for %q = %q", value, stored)
		}
	}
	if uploader.calls != 0 {
		t.Fatalf("uploader calls = %d", uploader.calls)
	}
}

func TestPersistAvatarReferenceFallsBackToSafeDisplayWhenUploadFails(t *testing.T) {
	pngDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfallback"))
	service := Service{Uploader: &fakeAvatarUploader{err: errUploadFailed{}}}

	if stored := service.PersistAvatarReference(context.Background(), "ent-1", "avatar", pngDataURI); stored != pngDataURI {
		t.Fatalf("stored avatar = %q", stored)
	}
	invalid := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("not an image"))
	if stored := service.PersistAvatarReference(context.Background(), "ent-1", "avatar", invalid); stored != "" {
		t.Fatalf("invalid data URI stored avatar = %q", stored)
	}
}

func TestPersistAvatarReferenceConvertsDiceBearToLocalInitialsAvatar(t *testing.T) {
	service := Service{}

	stored := service.PersistAvatarReference(context.Background(), "ent-1", "avatar", "https://api.dicebear.com/7.x/initials/svg?seed=Alice")

	if !strings.HasPrefix(stored, "data:image/svg+xml;charset=utf-8,") || !strings.Contains(stored, "%3EAl%3C") {
		t.Fatalf("stored avatar = %q", stored)
	}
}

func TestResolveAvatarURLBuildsAccessURLForObjectReferences(t *testing.T) {
	service := Service{Access: archivemedia.AccessURLBuilder{
		BaseURL:               "https://cloud.example",
		PreferDirectObjectURL: false,
		SigningKey:            "test-key",
		TokenTTL:              time.Hour,
		Now:                   func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}}

	resolved := service.ResolveAvatarURL("https://object-storage:9102/objects/ent-1/avatar.png")

	if !strings.HasPrefix(resolved, "https://cloud.example/api/v1/archive/media/objects/ent-1/avatar.png?token=") {
		t.Fatalf("resolved avatar = %q", resolved)
	}
	if plain := service.ResolveAvatarURL("https://cdn.example/avatar.png"); plain != "https://cdn.example/avatar.png" {
		t.Fatalf("plain remote avatar = %q", plain)
	}
	if local := service.ResolveAvatarURL("local://archive_media/ent-1/avatar.png"); local != "" {
		t.Fatalf("missing local avatar = %q", local)
	}
}

func TestResolveAvatarURLReturnsDataURIForLocalImageObject(t *testing.T) {
	root := t.TempDir()
	content := []byte("\x89PNG\r\n\x1a\nlocal-avatar")
	path := filepath.Join(root, "archive_media", "ent-1", "avatar.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write local avatar: %v", err)
	}
	service := Service{LocalDataRoot: root}

	resolved := service.ResolveAvatarURL("local://archive_media/ent-1/avatar.png")

	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(content)
	if resolved != want {
		t.Fatalf("resolved local avatar = %q, want %q", resolved, want)
	}
}

func TestResolveAvatarURLRejectsUnsafeLocalObjectPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "avatar.png"), []byte("\x89PNG\r\n\x1a\nx"), 0o644); err != nil {
		t.Fatalf("write local avatar: %v", err)
	}
	textPath := filepath.Join(root, "archive_media", "ent-1", "avatar.txt")
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(textPath, []byte("not an image"), 0o644); err != nil {
		t.Fatalf("write local text avatar: %v", err)
	}
	service := Service{LocalDataRoot: root}

	for _, value := range []string{"local://../avatar.png", "local://archive_media/ent-1/avatar.txt"} {
		if resolved := service.ResolveAvatarURL(value); resolved != "" {
			t.Fatalf("resolved unsafe local avatar %q = %q", value, resolved)
		}
	}
}

type fakeAvatarUploader struct {
	input     archivemedia.UploadInput
	objectURL string
	err       error
	calls     int
}

func (uploader *fakeAvatarUploader) UploadArchiveMedia(ctx context.Context, input archivemedia.UploadInput) (string, error) {
	uploader.calls++
	uploader.input = input
	return uploader.objectURL, uploader.err
}

type errUploadFailed struct{}

func (errUploadFailed) Error() string { return "upload failed" }
