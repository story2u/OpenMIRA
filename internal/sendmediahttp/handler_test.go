package sendmediahttp

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendmedia"
)

func TestImageHandlerRequiresBearer(t *testing.T) {
	handler := New(auth.Guard{}, fakeService{})
	response := httptest.NewRecorder()
	request := multipartRequest(t, "/send/image", map[string]string{"device_id": "device-1"}, "file", "image.png", "image/png", []byte("png"))

	handler.ImageHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestImageHandlerSerializesMultipart(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	service := &recordingService{payload: map[string]any{"success": true, "task": map[string]any{"task_id": "task-1"}}}
	handler := New(guard, service)
	request := multipartRequest(t, "/send/image", map[string]string{
		"device_id":       "device-1",
		"username":        "Alice",
		"target_username": "Bob",
		"conversation_id": "conv-1",
		"sender_id":       "sender-1",
		"source":          "system",
	}, "file", "image.png", "image/png", []byte("png"))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.ImageHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.request.Kind != sendmedia.KindImage || service.request.DeviceID != "device-1" || service.request.Username != "Alice" || service.request.TargetUsername != "Bob" || service.request.FileName != "image.png" || service.request.ContentType != "image/png" || string(service.request.Content) != "png" || service.request.Operator != "user-1" {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestVoiceHandlerParsesDuration(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &recordingService{payload: map[string]any{"success": true}}
	handler := New(guard, service)
	request := multipartRequest(t, "/send/voice", map[string]string{
		"device_id":          "device-1",
		"username":           "Alice",
		"voice_duration_sec": "12",
	}, "file", "voice.webm", "audio/webm", []byte("voice"))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.VoiceHandler(response, request)

	if response.Code != http.StatusOK || service.request.Kind != sendmedia.KindVoice || service.request.VoiceDurationSec != 12 {
		t.Fatalf("status=%d body=%s request=%#v", response.Code, response.Body.String(), service.request)
	}
}

func TestMediaHandlerMapsServiceErrors(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "offline", err: sendguard.DeviceOfflineError{}, status: http.StatusConflict},
		{name: "invalid", err: sendmedia.ErrInvalidRequest, status: http.StatusUnprocessableEntity},
		{name: "unsupported", err: sendmedia.ErrUnsupportedType, status: http.StatusBadRequest},
		{name: "missing upload", err: sendmedia.ErrUploadMissing, status: http.StatusServiceUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := New(guard, fakeService{err: tc.err})
			request := multipartRequest(t, "/send/file", map[string]string{"device_id": "device-1", "username": "Alice"}, "file", "a.txt", "text/plain", []byte("x"))
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()

			handler.FileHandler(response, request)

			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type fakeService struct {
	err error
}

func (service fakeService) Send(ctx context.Context, request sendmedia.Request) (map[string]any, error) {
	_ = ctx
	_ = request
	if service.err != nil {
		return nil, service.err
	}
	return map[string]any{"success": true}, nil
}

type recordingService struct {
	payload map[string]any
	request sendmedia.Request
}

func (service *recordingService) Send(ctx context.Context, request sendmedia.Request) (map[string]any, error) {
	_ = ctx
	service.request = request
	return service.payload, nil
}

func multipartRequest(t *testing.T, path string, fields map[string]string, fileField string, filename string, contentType string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField error: %v", err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fileField+`"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("part write error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer close error: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "send-media-" + role})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
