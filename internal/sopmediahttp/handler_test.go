package sopmediahttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/sopmedia"
)

func TestUploadHandlerSerializesMultipartForAdmin(t *testing.T) {
	service := &fakeService{result: sopmedia.Result{
		Success:     true,
		MediaType:   "image",
		ObjectURL:   "https://cdn.example/welcome.png",
		AccessURL:   "https://cdn.example/welcome.png",
		Filename:    "welcome.png",
		ContentType: "image/png",
	}}
	handler := New(testGuard(t), service)

	response := performMultipartUpload(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload",
	}), " image ", "welcome.png", "image/png", []byte("image bytes"))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"media_type":"image"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.MediaType != " image " || service.request.Filename != "welcome.png" || service.request.ContentType != "image/png" || string(service.request.Content) != "image bytes" {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestUploadHandlerMapsValidationErrors(t *testing.T) {
	handler := New(testGuard(t), &fakeService{err: sopmedia.ValidationError{Err: sopmedia.ErrUnsupportedMIME, Detail: "不支持的文件类型：text/plain"}})
	response := performMultipartUpload(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload-validation",
	}), "image", "welcome.txt", "text/plain", []byte("text"))

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "不支持的文件类型") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestUploadHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(testGuard(t), &fakeService{})
	response := performMultipartUpload(handler.UploadHandler, "", "image", "welcome.png", "image/png", []byte("image bytes"))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}

	response = performMultipartUpload(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload-cs",
	}), "image", "welcome.png", "image/png", []byte("image bytes"))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("cs response = %d %s", response.Code, response.Body.String())
	}
}

func TestUploadHandlerRequiresConfiguredService(t *testing.T) {
	handler := New(testGuard(t), nil)
	response := performMultipartUpload(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload-missing-service",
	}), "image", "welcome.png", "image/png", []byte("image bytes"))

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "sop media upload service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestUploadHandlerRequiresFile(t *testing.T) {
	handler := New(testGuard(t), &fakeService{})
	response := performUploadWithoutFile(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload-missing-file",
	}))

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "file is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestUploadHandlerMapsUnexpectedServiceError(t *testing.T) {
	handler := New(testGuard(t), &fakeService{err: errors.New("boom")})
	response := performMultipartUpload(handler.UploadHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-upload-error",
	}), "image", "welcome.png", "image/png", []byte("image bytes"))

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	request sopmedia.Request
	result  sopmedia.Result
	err     error
}

func (service *fakeService) Upload(ctx context.Context, request sopmedia.Request) (sopmedia.Result, error) {
	service.request = request
	if service.err != nil {
		return sopmedia.Result{}, service.err
	}
	return service.result, nil
}

func performMultipartUpload(handler http.HandlerFunc, authorization string, mediaType string, filename string, contentType string, content []byte) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("media_type", mediaType)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, _ := writer.CreatePart(header)
	_, _ = part.Write(content)
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/media/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func performUploadWithoutFile(handler http.HandlerFunc, authorization string) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("media_type", "image")
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/media/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}
}

func signToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
