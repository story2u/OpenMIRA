package archivehttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/archiveadmin"
	"wework-go/internal/archivecontacts"
	"wework-go/internal/archiveeventnotify"
	"wework-go/internal/archiveingest"
	"wework-go/internal/archiveintegration"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesdk"
	"wework-go/internal/archivesync"
	"wework-go/internal/auth"
)

func TestStatusHandlerSerializesPayload(t *testing.T) {
	service := &fakeArchiveService{statusPayload: archiveadmin.Payload{"enterprise_id": "ent-1", "cursor": "42"}}
	handler := New(testGuard(t), service)
	response := perform(handler.StatusHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-status",
	}), "/api/v1/archive/status?enterprise_id=ent-1&source=official")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cursor":"42"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.statusRequest.EnterpriseID != "ent-1" || service.statusRequest.Source != "official" {
		t.Fatalf("status request = %#v", service.statusRequest)
	}
}

func TestCursorHandlerSerializesPayload(t *testing.T) {
	service := &fakeArchiveService{cursorPayload: archiveadmin.Payload{"enterprise_id": "ent-1", "source": "self_decrypt", "cursor": nil}}
	handler := New(testGuard(t), service)
	response := perform(handler.CursorHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-cursor",
	}), "/api/v1/archive/cursor?enterprise_id=ent-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cursor":null`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.cursorRequest.EnterpriseID != "ent-1" {
		t.Fatalf("cursor request = %#v", service.cursorRequest)
	}
}

func TestMediaTasksHandlerSerializesPayloadWithoutBearer(t *testing.T) {
	service := &fakeArchiveService{mediaTasksPayload: archiveadmin.Payload{"tasks": []archiveadmin.Payload{{"task_id": "task-1"}}}}
	handler := New(testGuard(t), service)
	response := perform(handler.MediaTasksHandler, "", "/api/v1/archive/media/tasks?enterprise_id=ent-1&source=self_decrypt&status=success&limit=0")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"task_id":"task-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.mediaTasksRequest.EnterpriseID != "ent-1" || service.mediaTasksRequest.Source != "self_decrypt" || service.mediaTasksRequest.Status != "success" || service.mediaTasksRequest.Limit != 1 {
		t.Fatalf("media tasks request = %#v", service.mediaTasksRequest)
	}
}

func TestMediaTasksHandlerRejectsInvalidLimit(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	response := perform(handler.MediaTasksHandler, "", "/api/v1/archive/media/tasks?limit=abc")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestOfficialCheckHandlerSerializesPayload(t *testing.T) {
	service := &fakeArchiveService{officialPayload: archiveadmin.Payload{"accepted": true, "enterprise_id": "ent-1"}}
	handler := New(testGuard(t), service)
	handler.BackendBaseURL = "https://cloud.example/"
	response := performPostBody(handler.OfficialCheckHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-official-check",
	}), "/api/v1/archive/official/check", `{"enterprise_id":"ent-1"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enterprise_id":"ent-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.officialRequest.EnterpriseID != "ent-1" || service.officialRequest.BaseURL != "https://cloud.example" {
		t.Fatalf("official request = %#v", service.officialRequest)
	}
}

func TestOfficialCheckHandlerFallsBackToRequestBaseURL(t *testing.T) {
	service := &fakeArchiveService{officialPayload: archiveadmin.Payload{"accepted": true}}
	handler := New(testGuard(t), service)
	response := performPostBodyWithHeader(handler.OfficialCheckHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-official-base",
	}), "/api/v1/archive/official/check", `{"enterprise_id":"ent-1"}`, map[string]string{
		"X-Forwarded-Proto": "https",
		"X-Forwarded-Host":  "ops.example",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.officialRequest.BaseURL != "https://ops.example" {
		t.Fatalf("base url = %q", service.officialRequest.BaseURL)
	}
}

func TestOfficialCheckHandlerMapsValidationErrors(t *testing.T) {
	service := &fakeArchiveService{officialErr: archiveadmin.ErrOfficialEnterpriseIDRequired}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.OfficialCheckHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-official-validation",
	}), "/api/v1/archive/official/check", `{}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "enterprise_id is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIntegrationTestHandlerSerializesPayload(t *testing.T) {
	service := &fakeArchiveService{integrationPayload: archiveintegration.Payload{
		"passed":        false,
		"enterprise_id": "ent-1",
		"steps":         []archiveintegration.Payload{{"name": "配置检查", "status": "failed"}},
	}}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.IntegrationTestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-integration-test",
	}), "/api/v1/archive/integration/test", `{"enterprise_id":"ent-1","source":"official","pull_limit":25,"sync_limit":125,"contact_limit":75,"media_limit":10}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enterprise_id":"ent-1"`) || !strings.Contains(response.Body.String(), `"passed":false`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.integrationRequest.EnterpriseID != "ent-1" ||
		service.integrationRequest.Source != "official" ||
		service.integrationRequest.PullLimit != 25 ||
		service.integrationRequest.SyncLimit != 125 ||
		service.integrationRequest.ContactLimit != 75 ||
		service.integrationRequest.MediaLimit != 10 {
		t.Fatalf("integration request = %#v", service.integrationRequest)
	}
}

func TestIntegrationTestHandlerMapsValidationErrors(t *testing.T) {
	service := &fakeArchiveService{integrationErr: archiveintegration.ErrEnterpriseIDRequired}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.IntegrationTestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-integration-validation",
	}), "/api/v1/archive/integration/test", `{}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "enterprise_id is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMessagesBatchHandlerSerializesPayloadWithAgentToken(t *testing.T) {
	cursor := "42"
	service := &fakeArchiveService{batchResult: archiveingest.Result{
		EnterpriseID:    "ent-1",
		Source:          "self_decrypt",
		Total:           2,
		Inserted:        1,
		Deduplicated:    1,
		Cursor:          &cursor,
		ConversationIDs: []string{"conv-1"},
	}}
	handler := New(testGuard(t), service)
	handler.AgentToken = "agent-token"
	response := performPostBodyWithHeader(handler.MessagesBatchHandler, "", "/api/v1/archive/messages/batch", `{"enterprise_id":"ent-1","source":"self_decrypt","cursor":"42","messages":[{"archive_msgid":"am-1"},{"archive_msgid":"am-2"}]}`, map[string]string{
		"X-Agent-Token": "agent-token",
	})

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) || !strings.Contains(response.Body.String(), `"inserted":1`) || !strings.Contains(response.Body.String(), `"deduplicated":1`) || !strings.Contains(response.Body.String(), `"conversation_ids":["conv-1"]`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.batchRequest.EnterpriseID != "ent-1" || service.batchRequest.Source != "self_decrypt" || service.batchRequest.Cursor == nil || *service.batchRequest.Cursor != "42" || len(service.batchRequest.Messages) != 2 {
		t.Fatalf("batch request = %#v", service.batchRequest)
	}
}

func TestMessagesBatchHandlerAcceptsSessionJWT(t *testing.T) {
	service := &fakeArchiveService{batchResult: archiveingest.Result{EnterpriseID: "ent-1", Source: "self_decrypt", Total: 1}}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.MessagesBatchHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "agent-supervisor",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-batch",
	}), "/api/v1/archive/messages/batch", `{"enterprise_id":"ent-1","messages":[{"archive_msgid":"am-1"}]}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMessagesBatchHandlerRequiresAgentOrSession(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	handler.AgentToken = "agent-token"
	response := performPostBody(handler.MessagesBatchHandler, "", "/api/v1/archive/messages/batch", `{"messages":[]}`)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMessagesBatchHandlerAllowsLegacyAgentAuth(t *testing.T) {
	service := &fakeArchiveService{batchResult: archiveingest.Result{EnterpriseID: "default", Source: "self_decrypt"}}
	handler := New(testGuard(t), service)
	handler.AllowLegacyAgentAuth = true
	response := performPostBody(handler.MessagesBatchHandler, "", "/api/v1/archive/messages/batch", `{"messages":[]}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"conversation_ids":[]`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMessagesBatchHandlerHonorsArchiveIngestDisabled(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	handler.AllowLegacyAgentAuth = true
	handler.ArchiveIngestDisabled = true
	response := performPostBody(handler.MessagesBatchHandler, "", "/api/v1/archive/messages/batch", `{"messages":[]}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive ingest disabled") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMessagesBatchHandlerMapsValidationError(t *testing.T) {
	service := &fakeArchiveService{batchErr: errors.New("archive message 0: archive_msgid is required")}
	handler := New(testGuard(t), service)
	handler.AllowLegacyAgentAuth = true
	response := performPostBody(handler.MessagesBatchHandler, "", "/api/v1/archive/messages/batch", `{"messages":[{}]}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "archive_msgid is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSyncRunHandlerSerializesProcessedStagedTask(t *testing.T) {
	cursor := "20"
	service := &fakeArchiveService{
		syncResult: archivesync.Result{
			EnterpriseID: "ent-1",
			Source:       "self_decrypt",
			Cursor:       &cursor,
			PulledTotal:  2,
			StagedTaskID: "ait-1",
		},
		processResult: &archiveingest.Result{
			EnterpriseID:    "ent-1",
			Source:          "self_decrypt",
			Total:           2,
			Inserted:        1,
			Deduplicated:    1,
			Cursor:          &cursor,
			ConversationIDs: []string{"conv-1"},
		},
	}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.SyncRunHandler, "", "/api/v1/archive/sync/run", `{"enterprise_id":"ent-1","source":"self_decrypt","cursor":"10","limit":2,"wework_user_id":"zhangsan"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pulled_total":2`) || !strings.Contains(response.Body.String(), `"matched_total":2`) || !strings.Contains(response.Body.String(), `"inserted":1`) || !strings.Contains(response.Body.String(), `"conversation_ids":["conv-1"]`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.syncRequest.EnterpriseID != "ent-1" || service.syncRequest.Source != "self_decrypt" || service.syncRequest.Cursor == nil || *service.syncRequest.Cursor != "10" || service.syncRequest.Limit != 2 || service.syncRequest.WeWorkUserID != "zhangsan" || service.syncRequest.TriggerReason != "manual" {
		t.Fatalf("sync request = %#v", service.syncRequest)
	}
	if service.processTaskID != "ait-1" {
		t.Fatalf("process task id = %q", service.processTaskID)
	}
}

func TestSyncRunHandlerHonorsArchiveIngestDisabled(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	handler.ArchiveIngestDisabled = true
	response := performPostBody(handler.SyncRunHandler, "", "/api/v1/archive/sync/run", `{}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive ingest disabled") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSyncRunHandlerMapsDefaultPullURLMissing(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{syncErr: errors.New("ARCHIVE_SELF_DECRYPT_PULL_URL is not configured")})
	response := performPostBody(handler.SyncRunHandler, "", "/api/v1/archive/sync/run", `{}`)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "ARCHIVE_SELF_DECRYPT_PULL_URL is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSyncRunHandlerMapsSkippedEnterprise(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{syncResult: archivesync.Result{EnterpriseID: "ent-missing", Source: "self_decrypt", Skipped: true, SkipReason: "enterprise_missing"}})
	response := performPostBody(handler.SyncRunHandler, "", "/api/v1/archive/sync/run", `{"enterprise_id":"ent-missing"}`)

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "enterprise not found: ent-missing") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSyncRunHandlerMapsStagedTaskProcessError(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{
		syncResult: archivesync.Result{EnterpriseID: "ent-1", Source: "self_decrypt", PulledTotal: 1, StagedTaskID: "ait-1"},
		processErr: errors.New("process down"),
	})
	response := performPostBody(handler.SyncRunHandler, "", "/api/v1/archive/sync/run", `{}`)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "archive sync failed: process down") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMediaSyncRunHandlerSerializesPayloadWithoutBearer(t *testing.T) {
	service := &fakeArchiveService{mediaRunResult: archivemedia.RunResult{EnterpriseID: "ent-1", Source: "self_decrypt", Total: 2, Success: 1, Pending: 1}}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.MediaSyncRunHandler, "", "/api/v1/archive/media/sync/run", `{"enterprise_id":"ent-1","source":"self_decrypt","limit":7}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":1`) || !strings.Contains(response.Body.String(), `"pending":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.mediaRunEnterpriseID != "ent-1" || service.mediaRunSource != "self_decrypt" || service.mediaRunLimit != 7 {
		t.Fatalf("media run request = %q/%q limit=%d", service.mediaRunEnterpriseID, service.mediaRunSource, service.mediaRunLimit)
	}
}

func TestMediaSyncRunHandlerReturnsBadGatewayOnFailedTasks(t *testing.T) {
	service := &fakeArchiveService{mediaRunResult: archivemedia.RunResult{EnterpriseID: "ent-1", Source: "self_decrypt", Total: 1, Failed: 1}}
	handler := New(testGuard(t), service)
	response := performPostBody(handler.MediaSyncRunHandler, "", "/api/v1/archive/media/sync/run", `{}`)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"message":"archive media sync failed"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMediaTaskPrepareHandlerSerializesPayloadForCS(t *testing.T) {
	service := &fakeArchiveService{mediaTaskResult: archivemedia.RunResult{EnterpriseID: "ent-1", Source: "self_decrypt", Total: 1, Success: 1}}
	handler := New(testGuard(t), service)
	response := performPostPathValue(handler.MediaTaskPrepareHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-media-prepare",
	}), "/api/v1/archive/media/tasks/task-1/prepare", "task_id", "task-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"task_id":"task-1"`) || !strings.Contains(response.Body.String(), `"success":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.mediaTaskID != "task-1" {
		t.Fatalf("media task id = %q", service.mediaTaskID)
	}
}

func TestMediaTaskPrepareHandlerMapsNotFound(t *testing.T) {
	service := &fakeArchiveService{mediaTaskErr: archivemedia.ErrMediaTaskNotFound}
	handler := New(testGuard(t), service)
	response := performPostPathValue(handler.MediaTaskPrepareHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-media-prepare-not-found",
	}), "/api/v1/archive/media/tasks/missing/prepare", "task_id", "missing")

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "media task not found") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMediaFileHandlerStreamsDownload(t *testing.T) {
	service := &fakeArchiveService{download: archivemedia.DownloadResponse{
		Body:          io.NopCloser(strings.NewReader("media-bytes")),
		ContentType:   "image/png",
		Filename:      "sdk-1.png",
		ContentLength: int64(len("media-bytes")),
	}}
	handler := New(testGuard(t), service)
	response := performGetPathValue(handler.MediaFileHandler, "/api/v1/archive/media/files/task-1?token=t", "task_id", "task-1")

	if response.Code != http.StatusOK || response.Body.String() != "media-bytes" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "image/png" || response.Header().Get("Cache-Control") != "private, max-age=86400" {
		t.Fatalf("headers = %#v", response.Header())
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "sdk-1.png") {
		t.Fatalf("content disposition = %q", response.Header().Get("Content-Disposition"))
	}
	if service.downloadTaskID != "task-1" || service.downloadToken != "t" {
		t.Fatalf("download request task=%q token=%q", service.downloadTaskID, service.downloadToken)
	}
}

func TestMediaObjectHandlerMapsTokenErrors(t *testing.T) {
	service := &fakeArchiveService{downloadErr: archivemedia.ErrMediaAccessTokenRequired}
	handler := New(testGuard(t), service)
	response := performGetPathValue(handler.MediaObjectHandler, "/api/v1/archive/media/objects/ent-1/file.png", "object_path", "ent-1/file.png")

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "token is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSOPLocalMediaHandlerServesLocalFileForAdmin(t *testing.T) {
	service := &fakeArchiveService{download: archivemedia.DownloadResponse{
		Body:          io.NopCloser(strings.NewReader("image-bytes")),
		ContentType:   "image/png",
		Filename:      "welcome.png",
		ContentLength: int64(len("image-bytes")),
	}}
	handler := New(testGuard(t), service)
	response := perform(handler.SOPLocalMediaHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-local",
	}), "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fwelcome.png")

	if response.Code != http.StatusOK || response.Body.String() != "image-bytes" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "image/png" || !strings.Contains(response.Header().Get("Content-Disposition"), "inline") || !strings.Contains(response.Header().Get("Content-Disposition"), "welcome.png") {
		t.Fatalf("headers = %#v", response.Header())
	}
	if service.downloadLocalObjectURL != "local://sop/welcome.png" {
		t.Fatalf("local object url = %q", service.downloadLocalObjectURL)
	}
}

func TestSOPLocalMediaHandlerRejectsNonLocalURL(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	response := perform(handler.SOPLocalMediaHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-local-non-local",
	}), "/api/v1/admin/sop/media/local?object_url=https%3A%2F%2Fexample.test%2Fimage.png")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "object_url must be a local media url") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSOPLocalMediaHandlerMapsMissingFile(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{downloadErr: archivemedia.ErrMediaLocalFileNotFound})
	response := perform(handler.SOPLocalMediaHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-media-local-missing",
	}), "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fmissing.png")

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "local media file not found") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSOPLocalMediaHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	response := perform(handler.SOPLocalMediaHandler, "", "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fwelcome.png")

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventNotifyHandlerSerializesPayloadWithBridgeToken(t *testing.T) {
	service := &fakeArchiveService{eventNotifyResult: archiveeventnotify.Result{
		Accepted:     true,
		Running:      false,
		TriggerID:    "archive-trigger-test",
		EnterpriseID: "ent-1",
		Event:        "message.new",
		Vendor:       "bridge-a",
	}}
	handler := New(testGuard(t), service)
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.EventNotifyHandler, "Bearer bridge-token", "/api/v1/archive/events/notify", `{"enterprise_id":"ent-1","source":"official","cursor":"42","limit":77,"event":"message.new","vendor":"bridge-a","payload":{"msgid":"m-1"}}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) || !strings.Contains(response.Body.String(), `"trigger_id":"archive-trigger-test"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.eventNotifyRequest.EnterpriseID != "ent-1" || service.eventNotifyRequest.Source != "official" || service.eventNotifyRequest.Cursor != "42" || service.eventNotifyRequest.Limit != 77 || service.eventNotifyRequest.Event != "message.new" || service.eventNotifyRequest.Vendor != "bridge-a" {
		t.Fatalf("event notify request = %#v", service.eventNotifyRequest)
	}
}

func TestEventNotifyHandlerAcceptsRawBridgeToken(t *testing.T) {
	service := &fakeArchiveService{eventNotifyResult: archiveeventnotify.Result{Accepted: true, TriggerID: "archive-trigger-raw", EnterpriseID: "default", Event: "message.new"}}
	handler := New(testGuard(t), service)
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.EventNotifyHandler, "bridge-token", "/api/v1/archive/events/notify", `{}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"trigger_id":"archive-trigger-raw"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventNotifyHandlerRejectsInvalidBridgeToken(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.EventNotifyHandler, "Bearer wrong-token", "/api/v1/archive/events/notify", `{}`)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "archive bridge token invalid") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSDKPullHandlerSerializesPayloadWithBridgeToken(t *testing.T) {
	cursor := "42"
	service := &fakeArchiveService{sdkPullPayload: archivesdk.Payload{"cursor": "43", "messages": []any{map[string]any{"archive_msgid": "am-1"}}}}
	handler := New(testGuard(t), service)
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.SDKPullHandler, "Bearer bridge-token", "/api/v1/archive/sdk/pull", `{"enterprise_id":"ent-1","source":"official","cursor":"42","limit":25,"corp_id":"corp-1","corp_secret":"secret-1","private_key_pem":"pem","private_key_version":"v2"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cursor":"43"`) || !strings.Contains(response.Body.String(), `"archive_msgid":"am-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.sdkPullRequest.EnterpriseID != "ent-1" ||
		service.sdkPullRequest.Source != "official" ||
		service.sdkPullRequest.Cursor == nil ||
		*service.sdkPullRequest.Cursor != cursor ||
		service.sdkPullRequest.Limit != 25 ||
		service.sdkPullRequest.CorpID != "corp-1" ||
		service.sdkPullRequest.CorpSecret != "secret-1" ||
		service.sdkPullRequest.PrivateKeyPEM != "pem" ||
		service.sdkPullRequest.PrivateKeyVersion != "v2" {
		t.Fatalf("sdk pull request = %#v", service.sdkPullRequest)
	}
}

func TestSDKMediaPullHandlerSerializesPayloadAndErrors(t *testing.T) {
	service := &fakeArchiveService{sdkMediaPullPayload: archivesdk.Payload{"data_base64": "AA==", "is_finish": true}}
	handler := New(testGuard(t), service)
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.SDKMediaPullHandler, "bridge-token", "/api/v1/archive/sdk/media/pull", `{"enterprise_id":"ent-1","source":"official","sdk_file_id":"file-1","index_buf":"idx","corp_id":"corp-1","corp_secret":"secret-1"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data_base64":"AA=="`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.sdkMediaPullRequest.EnterpriseID != "ent-1" ||
		service.sdkMediaPullRequest.Source != "official" ||
		service.sdkMediaPullRequest.SDKFileID != "file-1" ||
		service.sdkMediaPullRequest.IndexBuf != "idx" ||
		service.sdkMediaPullRequest.CorpID != "corp-1" ||
		service.sdkMediaPullRequest.CorpSecret != "secret-1" {
		t.Fatalf("sdk media pull request = %#v", service.sdkMediaPullRequest)
	}

	service = &fakeArchiveService{sdkMediaPullErr: archivesdk.ErrSDKFileIDRequired}
	handler = New(testGuard(t), service)
	response = performPostBody(handler.SDKMediaPullHandler, "", "/api/v1/archive/sdk/media/pull", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "sdk_file_id is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSDKPullHandlerRejectsInvalidBridgeToken(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	handler.BridgeToken = "bridge-token"

	response := performPostBody(handler.SDKPullHandler, "Bearer wrong-token", "/api/v1/archive/sdk/pull", `{}`)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "archive bridge token invalid") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSDKPullHandlerMapsBridgeFailures(t *testing.T) {
	service := &fakeArchiveService{sdkPullErr: archivesdk.ErrBridgeUnavailable}
	handler := New(testGuard(t), service)

	response := performPostBody(handler.SDKPullHandler, "", "/api/v1/archive/sdk/pull", `{}`)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "sdk pull failed: official sdk bridge is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersRequireAdminOrSupervisor(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	response := perform(handler.StatusHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-status-cs",
	}), "/api/v1/archive/status")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersReturnServiceUnavailableWhenUnconfigured(t *testing.T) {
	handler := New(testGuard(t), nil)
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-missing-service",
	})
	response := perform(handler.StatusHandler, token, "/api/v1/archive/status")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive status service is not configured") {
		t.Fatalf("status response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.CursorHandler, token, "/api/v1/archive/cursor")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive cursor service is not configured") {
		t.Fatalf("cursor response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.MediaTasksHandler, "", "/api/v1/archive/media/tasks")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive media tasks service is not configured") {
		t.Fatalf("media tasks response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.MediaSyncRunHandler, "", "/api/v1/archive/media/sync/run", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive media service is not configured") {
		t.Fatalf("media sync response = %d %s", response.Code, response.Body.String())
	}
	response = performPostPathValue(handler.MediaTaskPrepareHandler, token, "/api/v1/archive/media/tasks/task-1/prepare", "task_id", "task-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive media service is not configured") {
		t.Fatalf("media prepare response = %d %s", response.Code, response.Body.String())
	}
	response = performGetPathValue(handler.MediaFileHandler, "/api/v1/archive/media/files/task-1?token=t", "task_id", "task-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive media download service is not configured") {
		t.Fatalf("media file response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.EventNotifyHandler, "", "/api/v1/archive/events/notify", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive event notify service is not configured") {
		t.Fatalf("event notify response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.OfficialCheckHandler, token, "/api/v1/archive/official/check", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive official check service is not configured") {
		t.Fatalf("official check response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.IntegrationTestHandler, token, "/api/v1/archive/integration/test", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive integration test service is not configured") {
		t.Fatalf("integration test response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.ContactsSyncHandler, token, "/api/v1/archive/contacts/sync", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive contacts sync service is not configured") {
		t.Fatalf("contacts sync response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.SDKPullHandler, "", "/api/v1/archive/sdk/pull", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive sdk bridge service is not configured") {
		t.Fatalf("sdk pull response = %d %s", response.Code, response.Body.String())
	}
	response = performPostBody(handler.SDKMediaPullHandler, "", "/api/v1/archive/sdk/media/pull", `{}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "archive sdk bridge service is not configured") {
		t.Fatalf("sdk media pull response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersRequireBearer(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	response := perform(handler.StatusHandler, "", "/api/v1/archive/status")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestContactsSyncHandlerSerializesPayload(t *testing.T) {
	service := &fakeArchiveService{contactsSyncPayload: archivecontacts.Payload{
		"enterprise_id": "ent-1",
		"total":         1,
		"profiles":      []archivecontacts.Payload{{"sender_id": "wm-1"}},
	}}
	handler := New(testGuard(t), service)
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-contacts-sync",
	})

	response := performPostBody(handler.ContactsSyncHandler, token, "/api/v1/archive/contacts/sync", `{"enterprise_id":"ent-1","sender_ids":["wm-1"],"force_refresh":true,"limit":7}`)

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["accepted"] != true || payload["enterprise_id"] != "ent-1" || payload["total"].(float64) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if service.contactsSyncRequest.EnterpriseID != "ent-1" || service.contactsSyncRequest.Limit != 7 || !service.contactsSyncRequest.ForceRefresh || len(service.contactsSyncRequest.SenderIDs) != 1 {
		t.Fatalf("request = %#v", service.contactsSyncRequest)
	}
}

func TestContactsSyncHandlerDefaultsBody(t *testing.T) {
	service := &fakeArchiveService{contactsSyncPayload: archivecontacts.Payload{"profiles": []archivecontacts.Payload{}}}
	handler := New(testGuard(t), service)
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-contacts-sync-default",
	})

	response := performPostBody(handler.ContactsSyncHandler, token, "/api/v1/archive/contacts/sync", `{}`)

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.contactsSyncRequest.EnterpriseID != archivecontacts.DefaultEnterpriseID || service.contactsSyncRequest.Limit != archivecontacts.DefaultLimit {
		t.Fatalf("request = %#v", service.contactsSyncRequest)
	}
}

func TestContactsSyncHandlerRejectsInvalidJSON(t *testing.T) {
	handler := New(testGuard(t), &fakeArchiveService{})
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-archive-contacts-sync-invalid",
	})

	response := performPostBody(handler.ContactsSyncHandler, token, "/api/v1/archive/contacts/sync", `{`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid archive contacts sync payload") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeArchiveService struct {
	statusPayload          archiveadmin.Payload
	statusRequest          archiveadmin.StatusRequest
	statusErr              error
	cursorPayload          archiveadmin.Payload
	cursorRequest          archiveadmin.CursorRequest
	cursorErr              error
	mediaTasksPayload      archiveadmin.Payload
	mediaTasksRequest      archiveadmin.MediaTasksRequest
	mediaTasksErr          error
	mediaRunResult         archivemedia.RunResult
	mediaRunErr            error
	mediaRunEnterpriseID   string
	mediaRunSource         string
	mediaRunLimit          int
	mediaTaskResult        archivemedia.RunResult
	mediaTaskErr           error
	mediaTaskID            string
	download               archivemedia.DownloadResponse
	downloadErr            error
	downloadTaskID         string
	downloadObjectPath     string
	downloadLocalObjectURL string
	downloadToken          string
	eventNotifyResult      archiveeventnotify.Result
	eventNotifyErr         error
	eventNotifyRequest     archiveeventnotify.Request
	officialPayload        archiveadmin.Payload
	officialErr            error
	officialRequest        archiveadmin.OfficialCheckRequest
	integrationPayload     archiveintegration.Payload
	integrationRequest     archiveintegration.Request
	integrationErr         error
	batchResult            archiveingest.Result
	batchErr               error
	batchRequest           archiveingest.BatchRequest
	syncResult             archivesync.Result
	syncErr                error
	syncRequest            archivesync.Request
	contactsSyncPayload    archivecontacts.Payload
	contactsSyncRequest    archivecontacts.Request
	contactsSyncErr        error
	sdkPullPayload         archivesdk.Payload
	sdkPullRequest         archivesdk.PullRequest
	sdkPullErr             error
	sdkMediaPullPayload    archivesdk.Payload
	sdkMediaPullRequest    archivesdk.MediaPullRequest
	sdkMediaPullErr        error
	processResult          *archiveingest.Result
	processErr             error
	processTaskID          string
}

func (service *fakeArchiveService) Status(ctx context.Context, request archiveadmin.StatusRequest) (archiveadmin.Payload, error) {
	service.statusRequest = request
	if service.statusErr != nil {
		return nil, service.statusErr
	}
	return service.statusPayload, nil
}

func (service *fakeArchiveService) Cursor(ctx context.Context, request archiveadmin.CursorRequest) (archiveadmin.Payload, error) {
	service.cursorRequest = request
	if service.cursorErr != nil {
		return nil, service.cursorErr
	}
	return service.cursorPayload, nil
}

func (service *fakeArchiveService) MediaTasks(ctx context.Context, request archiveadmin.MediaTasksRequest) (archiveadmin.Payload, error) {
	service.mediaTasksRequest = request
	if service.mediaTasksErr != nil {
		return nil, service.mediaTasksErr
	}
	return service.mediaTasksPayload, nil
}

func (service *fakeArchiveService) RunOnce(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
	service.mediaRunEnterpriseID = enterpriseID
	service.mediaRunSource = source
	if service.mediaRunErr != nil {
		return archivemedia.RunResult{}, service.mediaRunErr
	}
	return service.mediaRunResult, nil
}

func (service *fakeArchiveService) RunOnceWithLimit(ctx context.Context, enterpriseID string, source string, limit int) (archivemedia.RunResult, error) {
	service.mediaRunLimit = limit
	return service.RunOnce(ctx, enterpriseID, source)
}

func (service *fakeArchiveService) RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error) {
	service.mediaTaskID = taskID
	if service.mediaTaskErr != nil {
		return archivemedia.RunResult{}, service.mediaTaskErr
	}
	return service.mediaTaskResult, nil
}

func (service *fakeArchiveService) DownloadTask(ctx context.Context, taskID string, token string) (archivemedia.DownloadResponse, error) {
	service.downloadTaskID = taskID
	service.downloadToken = token
	if service.downloadErr != nil {
		return archivemedia.DownloadResponse{}, service.downloadErr
	}
	return service.download, nil
}

func (service *fakeArchiveService) DownloadObject(ctx context.Context, objectPath string, token string) (archivemedia.DownloadResponse, error) {
	service.downloadObjectPath = objectPath
	service.downloadToken = token
	if service.downloadErr != nil {
		return archivemedia.DownloadResponse{}, service.downloadErr
	}
	return service.download, nil
}

func (service *fakeArchiveService) DownloadLocalObject(ctx context.Context, objectURL string) (archivemedia.DownloadResponse, error) {
	service.downloadLocalObjectURL = objectURL
	if service.downloadErr != nil {
		return archivemedia.DownloadResponse{}, service.downloadErr
	}
	return service.download, nil
}

func (service *fakeArchiveService) Notify(ctx context.Context, request archiveeventnotify.Request) (archiveeventnotify.Result, error) {
	service.eventNotifyRequest = request
	if service.eventNotifyErr != nil {
		return archiveeventnotify.Result{}, service.eventNotifyErr
	}
	return service.eventNotifyResult, nil
}

func (service *fakeArchiveService) OfficialCheck(ctx context.Context, request archiveadmin.OfficialCheckRequest) (archiveadmin.Payload, error) {
	_ = ctx
	service.officialRequest = request
	if service.officialErr != nil {
		return nil, service.officialErr
	}
	return service.officialPayload, nil
}

func (service *fakeArchiveService) Test(ctx context.Context, request archiveintegration.Request) (archiveintegration.Payload, error) {
	_ = ctx
	service.integrationRequest = request
	if service.integrationErr != nil {
		return nil, service.integrationErr
	}
	return service.integrationPayload, nil
}

func (service *fakeArchiveService) IngestArchiveBatch(ctx context.Context, request archiveingest.BatchRequest) (archiveingest.Result, error) {
	_ = ctx
	service.batchRequest = request
	if service.batchErr != nil {
		return archiveingest.Result{}, service.batchErr
	}
	return service.batchResult, nil
}

func (service *fakeArchiveService) RunArchiveSyncOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error) {
	_ = ctx
	service.syncRequest = request
	if service.syncErr != nil {
		return archivesync.Result{}, service.syncErr
	}
	return service.syncResult, nil
}

func (service *fakeArchiveService) SyncArchiveContacts(ctx context.Context, request archivecontacts.Request) (archivecontacts.Payload, error) {
	_ = ctx
	service.contactsSyncRequest = request
	if service.contactsSyncErr != nil {
		return nil, service.contactsSyncErr
	}
	return service.contactsSyncPayload, nil
}

func (service *fakeArchiveService) Pull(ctx context.Context, request archivesdk.PullRequest) (archivesdk.Payload, error) {
	_ = ctx
	service.sdkPullRequest = request
	if service.sdkPullErr != nil {
		return nil, service.sdkPullErr
	}
	return service.sdkPullPayload, nil
}

func (service *fakeArchiveService) PullMedia(ctx context.Context, request archivesdk.MediaPullRequest) (archivesdk.Payload, error) {
	_ = ctx
	service.sdkMediaPullRequest = request
	if service.sdkMediaPullErr != nil {
		return nil, service.sdkMediaPullErr
	}
	return service.sdkMediaPullPayload, nil
}

func (service *fakeArchiveService) ProcessTask(ctx context.Context, taskID string) (*archiveingest.Result, error) {
	_ = ctx
	service.processTaskID = taskID
	if service.processErr != nil {
		return nil, service.processErr
	}
	return service.processResult, nil
}

func perform(handler http.HandlerFunc, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func performPostBody(handler http.HandlerFunc, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func performPostBodyWithHeader(handler http.HandlerFunc, authorization string, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func performPostPathValue(handler http.HandlerFunc, authorization string, target string, key string, value string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue(key, value)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func performGetPathValue(handler http.HandlerFunc, target string, key string, value string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.SetPathValue(key, value)
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
