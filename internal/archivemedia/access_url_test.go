package archivemedia

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildAccessURLUsesSignedObjectGateway(t *testing.T) {
	builder := AccessURLBuilder{
		BaseURL:               "https://cloud.example",
		ObjectPublicBaseURL:   "https://cloud.example/media-objects",
		PreferDirectObjectURL: true,
		SigningKey:            "media-secret",
		TokenTTL:              2 * time.Minute,
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	}

	accessURL := builder.BuildAccessURL("task-001", "http://object-storage:9102/objects/ent-a/path/image.png")

	if !strings.HasPrefix(accessURL, "https://cloud.example/media-objects/signed/objects/ent-a/path/image.png?token=") {
		t.Fatalf("access url = %s", accessURL)
	}
	token := queryToken(t, accessURL)
	payload := decodeToken(t, token)
	if payload.ResourceType != "object" || payload.ResourceID != "ent-a/path/image.png" || payload.Exp != 1120 {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Sig != expectedSignature("media-secret", "object:ent-a/path/image.png:1120") {
		t.Fatalf("signature = %s", payload.Sig)
	}
}

func TestBuildAccessURLKeepsPlainObjectPublicBase(t *testing.T) {
	builder := AccessURLBuilder{
		ObjectPublicBaseURL:   "https://cdn.example/archive",
		PreferDirectObjectURL: true,
		SigningKey:            "media-secret",
	}

	accessURL := builder.BuildAccessURL("task-001", "/objects/ent-a/file.bin")

	if accessURL != "https://cdn.example/archive/ent-a/file.bin" {
		t.Fatalf("access url = %s", accessURL)
	}
}

func TestBuildAccessURLFallsBackToTaskRoute(t *testing.T) {
	builder := AccessURLBuilder{
		BaseURL:    "https://cloud.example",
		SigningKey: "media-secret",
		TokenTTL:   time.Minute,
		Now: func() time.Time {
			return time.Unix(2000, 0).UTC()
		},
	}

	accessURL := builder.BuildAccessURL("task-002", "https://cdn.example/not-managed/file.bin")

	if !strings.HasPrefix(accessURL, "https://cloud.example/api/v1/archive/media/files/task-002?token=") {
		t.Fatalf("access url = %s", accessURL)
	}
	payload := decodeToken(t, queryToken(t, accessURL))
	if payload.ResourceType != "task" || payload.ResourceID != "task-002" || payload.Exp != 2060 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestAccessURLBuilderVerifiesTaskAndObjectTokens(t *testing.T) {
	builder := AccessURLBuilder{
		SigningKey: "media-secret",
		TokenTTL:   time.Minute,
		Now: func() time.Time {
			return time.Unix(3000, 0).UTC()
		},
	}
	taskURL := builder.BuildAccessURL("task-003", "")
	taskToken := queryToken(t, taskURL)
	if !builder.VerifyAccessToken("task-003", taskToken) {
		t.Fatalf("expected task token to verify")
	}
	if builder.VerifyObjectAccessToken("task-003", taskToken) {
		t.Fatalf("task token should not verify as object token")
	}

	objectURL := builder.BuildAccessURL("task-003", "/objects/ent-a/file.png")
	objectToken := queryToken(t, objectURL)
	if !builder.VerifyObjectAccessToken("ent-a/file.png", objectToken) {
		t.Fatalf("expected object token to verify")
	}
	if builder.VerifyAccessToken("ent-a/file.png", objectToken) {
		t.Fatalf("object token should not verify as task token")
	}
}

func TestAccessURLBuilderRejectsExpiredToken(t *testing.T) {
	now := time.Unix(4000, 0).UTC()
	builder := AccessURLBuilder{
		SigningKey: "media-secret",
		TokenTTL:   time.Minute,
		Now: func() time.Time {
			return now
		},
	}
	token := queryToken(t, builder.BuildAccessURL("task-004", ""))
	builder.Now = func() time.Time {
		return now.Add(2 * time.Minute)
	}
	if builder.VerifyAccessToken("task-004", token) {
		t.Fatalf("expired token should not verify")
	}
}

func queryToken(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("missing token in %s", rawURL)
	}
	return token
}

func decodeToken(t *testing.T, token string) tokenPayload {
	t.Helper()
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	var payload tokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal token: %v", err)
	}
	return payload
}

func expectedSignature(secret string, plain string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(plain))
	return hex.EncodeToString(mac.Sum(nil))
}
