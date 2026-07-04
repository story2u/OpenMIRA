package voicetranscriptionhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/voicetranscription"
)

func TestRetryHandlerRequiresAllowedRole(t *testing.T) {
	handler := New(auth.Guard{}, &fakeRetryService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/voice-transcriptions/retry", strings.NewReader(`{"archive_msgid":"am-1"}`))

	handler.RetryHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRetryHandlerReturnsManualRetryPayload(t *testing.T) {
	service := fakeRetryService{response: voicetranscription.ManualRetryResponse{
		Accepted:                    true,
		EnterpriseID:                "ent-1",
		ArchiveMsgID:                "am-1",
		TaskID:                      "vtt-1",
		Status:                      voicetranscription.StatusSuccess,
		VoiceTranscriptionStatus:    voicetranscription.StatusSuccess,
		VoiceText:                   "转写文本",
		VoiceTranscriptionExecuteID: "exec-1",
	}}
	handler := New(auth.Guard{Verifier: testVerifier(t)}, &service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/voice-transcriptions/retry", strings.NewReader(`{"enterprise_id":"ent-1","archive_msgid":"am-1"}`))
	request.Header.Set("Authorization", "Bearer "+issueRetryToken(t, "cs"))

	handler.RetryHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"voice_text":"转写文本"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.ArchiveMsgID != "am-1" || service.request.EnterpriseID != "ent-1" {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestRetryHandlerMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "missing archive", err: voicetranscription.ErrArchiveMsgIDRequired, wantStatus: http.StatusBadRequest, wantBody: "archive_msgid is required"},
		{name: "not configured", err: voicetranscription.ErrManualRetryNotConfigured, wantStatus: http.StatusServiceUnavailable, wantBody: "voice transcription is not configured"},
		{name: "not found", err: voicetranscription.ErrVoiceTranscriptionNotFound, wantStatus: http.StatusNotFound, wantBody: "voice transcription task not found"},
		{name: "already succeeded", err: voicetranscription.ErrVoiceTranscriptionSucceeded, wantStatus: http.StatusConflict, wantBody: "voice transcription already succeeded"},
		{name: "media not found", err: voicetranscription.ErrArchiveVoiceMediaNotFound, wantStatus: http.StatusNotFound, wantBody: "archive voice media task not found"},
		{name: "enterprise not found", err: voicetranscription.ErrArchiveVoiceEnterpriseNotFound, wantStatus: http.StatusNotFound, wantBody: "archive voice enterprise not found"},
		{name: "message not found", err: voicetranscription.ErrArchiveVoiceMessageNotFound, wantStatus: http.StatusNotFound, wantBody: "archive voice message not found"},
		{name: "message not voice", err: voicetranscription.ErrArchiveMessageNotVoice, wantStatus: http.StatusConflict, wantBody: "archive message is not voice"},
		{name: "media not ready", err: voicetranscription.ErrArchiveVoiceMediaNotReady, wantStatus: http.StatusBadGateway, wantBody: "archive voice media object is not ready"},
		{name: "status cannot retry", err: voicetranscription.StatusCannotRetryError{Status: "cancelled"}, wantStatus: http.StatusConflict, wantBody: "voice transcription status cannot retry: cancelled"},
		{name: "execute failed", err: wrapRetryExecuteError(errors.New("coze down")), wantStatus: http.StatusBadGateway, wantBody: "voice transcription retry failed: coze down"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := fakeRetryService{err: testCase.err}
			handler := New(auth.Guard{Verifier: testVerifier(t)}, &service)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/voice-transcriptions/retry", strings.NewReader(`{"archive_msgid":"am-1"}`))
			request.Header.Set("Authorization", "Bearer "+issueRetryToken(t, "admin"))

			handler.RetryHandler(response, request)

			if response.Code != testCase.wantStatus || !strings.Contains(response.Body.String(), testCase.wantBody) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func wrapRetryExecuteError(cause error) error {
	return retryOperationForTest{kind: voicetranscription.ErrManualRetryExecuteFailed, cause: cause}
}

type retryOperationForTest struct {
	kind  error
	cause error
}

func (err retryOperationForTest) Error() string {
	return err.cause.Error()
}

func (err retryOperationForTest) Unwrap() error {
	return err.kind
}

type fakeRetryService struct {
	request  voicetranscription.ManualRetryRequest
	response voicetranscription.ManualRetryResponse
	err      error
}

func (service *fakeRetryService) RetryArchiveVoiceTranscription(ctx context.Context, request voicetranscription.ManualRetryRequest) (voicetranscription.ManualRetryResponse, error) {
	service.request = request
	if service.err != nil {
		return voicetranscription.ManualRetryResponse{}, service.err
	}
	return service.response, nil
}

func testVerifier(t *testing.T) auth.Verifier {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return verifier
}

func issueRetryToken(t *testing.T, role string) string {
	t.Helper()
	verifier := testVerifier(t)
	issuedAt := time.Now().UTC().Add(-time.Minute)
	verifier.Now = func() time.Time { return issuedAt }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID: "cs-001",
		Role:       role,
		TTL:        time.Hour,
		JTI:        "jwt-voice-retry-" + role,
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}
