// Package archivehttp adapts archive management routes to net/http.
package archivehttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

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

type statusService interface {
	Status(ctx context.Context, request archiveadmin.StatusRequest) (archiveadmin.Payload, error)
}

type cursorService interface {
	Cursor(ctx context.Context, request archiveadmin.CursorRequest) (archiveadmin.Payload, error)
}

type mediaTasksService interface {
	MediaTasks(ctx context.Context, request archiveadmin.MediaTasksRequest) (archiveadmin.Payload, error)
}

type officialCheckService interface {
	OfficialCheck(ctx context.Context, request archiveadmin.OfficialCheckRequest) (archiveadmin.Payload, error)
}

type integrationTestService interface {
	Test(ctx context.Context, request archiveintegration.Request) (archiveintegration.Payload, error)
}

type batchIngestService interface {
	IngestArchiveBatch(ctx context.Context, request archiveingest.BatchRequest) (archiveingest.Result, error)
}

type syncRunService interface {
	RunArchiveSyncOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error)
}

type contactsSyncService interface {
	SyncArchiveContacts(ctx context.Context, request archivecontacts.Request) (archivecontacts.Payload, error)
}

type sdkBridgeService interface {
	Pull(ctx context.Context, request archivesdk.PullRequest) (archivesdk.Payload, error)
	PullMedia(ctx context.Context, request archivesdk.MediaPullRequest) (archivesdk.Payload, error)
}

type stagedTaskProcessor interface {
	ProcessTask(ctx context.Context, taskID string) (*archiveingest.Result, error)
}

type archiveSyncRunner interface {
	RunOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error)
}

// SyncRunnerAdapter adapts archivesync.Runner to the archive HTTP sync-run boundary.
type SyncRunnerAdapter struct {
	Runner archiveSyncRunner
}

// RunArchiveSyncOnce runs one archive sync pass through the wrapped runner.
func (adapter SyncRunnerAdapter) RunArchiveSyncOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error) {
	if adapter.Runner == nil {
		return archivesync.Result{}, errors.New("archive sync runner is not configured")
	}
	return adapter.Runner.RunOnce(ctx, request)
}

type mediaRunner interface {
	RunOnce(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error)
	RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error)
}

type mediaRunnerWithLimit interface {
	RunOnceWithLimit(ctx context.Context, enterpriseID string, source string, limit int) (archivemedia.RunResult, error)
}

type eventNotifyService interface {
	Notify(ctx context.Context, request archiveeventnotify.Request) (archiveeventnotify.Result, error)
}

type mediaDownloadService interface {
	DownloadTask(ctx context.Context, taskID string, token string) (archivemedia.DownloadResponse, error)
	DownloadObject(ctx context.Context, objectPath string, token string) (archivemedia.DownloadResponse, error)
	DownloadLocalObject(ctx context.Context, objectURL string) (archivemedia.DownloadResponse, error)
}

// Handler owns archive admin route serialization.
type Handler struct {
	Guard                 auth.Guard
	BridgeToken           string
	Status                statusService
	Cursor                cursorService
	MediaTasks            mediaTasksService
	Official              officialCheckService
	Integration           integrationTestService
	BatchIngest           batchIngestService
	SyncRun               syncRunService
	ContactsSync          contactsSyncService
	SDKBridge             sdkBridgeService
	SyncIngest            stagedTaskProcessor
	BackendBaseURL        string
	AgentToken            string
	AllowLegacyAgentAuth  bool
	ArchiveIngestDisabled bool
	Media                 mediaRunner
	EventNotify           eventNotifyService
	Download              mediaDownloadService
}

// New builds an archive HTTP handler.
func New(guard auth.Guard, service any) Handler {
	handler := Handler{Guard: guard}
	if status, ok := service.(statusService); ok {
		handler.Status = status
	}
	if cursor, ok := service.(cursorService); ok {
		handler.Cursor = cursor
	}
	if mediaTasks, ok := service.(mediaTasksService); ok {
		handler.MediaTasks = mediaTasks
	}
	if official, ok := service.(officialCheckService); ok {
		handler.Official = official
	}
	if integration, ok := service.(integrationTestService); ok {
		handler.Integration = integration
	}
	if batchIngest, ok := service.(batchIngestService); ok {
		handler.BatchIngest = batchIngest
	}
	if syncRun, ok := service.(syncRunService); ok {
		handler.SyncRun = syncRun
	}
	if contactsSync, ok := service.(contactsSyncService); ok {
		handler.ContactsSync = contactsSync
	}
	if sdkBridge, ok := service.(sdkBridgeService); ok {
		handler.SDKBridge = sdkBridge
	}
	if syncIngest, ok := service.(stagedTaskProcessor); ok {
		handler.SyncIngest = syncIngest
	}
	if media, ok := service.(mediaRunner); ok {
		handler.Media = media
	}
	if eventNotify, ok := service.(eventNotifyService); ok {
		handler.EventNotify = eventNotify
	}
	if download, ok := service.(mediaDownloadService); ok {
		handler.Download = download
	}
	return handler
}

// ContactsSyncHandler serializes POST /api/v1/archive/contacts/sync.
func (handler Handler) ContactsSyncHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ContactsSync == nil {
		writeError(w, http.StatusServiceUnavailable, archivecontacts.ErrContactsServiceUnavailable.Error())
		return
	}
	payload, ok := decodeArchiveContactsSyncBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive contacts sync payload")
		return
	}
	response, err := handler.ContactsSync.SyncArchiveContacts(r.Context(), archivecontacts.Request{
		EnterpriseID: payload.EnterpriseID,
		SenderIDs:    payload.SenderIDs,
		ForceRefresh: payload.ForceRefresh,
		Limit:        payload.Limit,
	})
	if err != nil {
		writeArchiveContactsSyncError(w, err)
		return
	}
	response["accepted"] = true
	writeJSON(w, http.StatusOK, response)
}

// SDKPullHandler serializes POST /api/v1/archive/sdk/pull.
func (handler Handler) SDKPullHandler(w http.ResponseWriter, r *http.Request) {
	if !verifyBridgeToken(r.Header.Get("Authorization"), handler.BridgeToken) {
		writeError(w, http.StatusUnauthorized, "archive bridge token invalid")
		return
	}
	if handler.SDKBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "archive sdk bridge service is not configured")
		return
	}
	payload, ok := decodeSDKPullBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive sdk pull payload")
		return
	}
	response, err := handler.SDKBridge.Pull(r.Context(), archivesdk.PullRequest{
		EnterpriseID:      payload.EnterpriseID,
		Source:            payload.Source,
		Cursor:            payload.Cursor,
		Limit:             payload.Limit,
		CorpID:            payload.CorpID,
		CorpSecret:        payload.CorpSecret,
		PrivateKeyPEM:     payload.PrivateKeyPEM,
		PrivateKeyVersion: payload.PrivateKeyVersion,
	})
	if err != nil {
		writeSDKPullError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// SDKMediaPullHandler serializes POST /api/v1/archive/sdk/media/pull.
func (handler Handler) SDKMediaPullHandler(w http.ResponseWriter, r *http.Request) {
	if !verifyBridgeToken(r.Header.Get("Authorization"), handler.BridgeToken) {
		writeError(w, http.StatusUnauthorized, "archive bridge token invalid")
		return
	}
	if handler.SDKBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "archive sdk bridge service is not configured")
		return
	}
	payload, ok := decodeSDKMediaPullBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive sdk media pull payload")
		return
	}
	response, err := handler.SDKBridge.PullMedia(r.Context(), archivesdk.MediaPullRequest{
		EnterpriseID: payload.EnterpriseID,
		Source:       payload.Source,
		SDKFileID:    payload.SDKFileID,
		IndexBuf:     payload.IndexBuf,
		CorpID:       payload.CorpID,
		CorpSecret:   payload.CorpSecret,
	})
	if err != nil {
		writeSDKMediaPullError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// IntegrationTestHandler serializes POST /api/v1/archive/integration/test.
func (handler Handler) IntegrationTestHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Integration == nil {
		writeError(w, http.StatusServiceUnavailable, "archive integration test service is not configured")
		return
	}
	payload, ok := decodeIntegrationTestBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive integration test payload")
		return
	}
	response, err := handler.Integration.Test(r.Context(), archiveintegration.Request{
		EnterpriseID: payload.EnterpriseID,
		Source:       payload.Source,
		PullLimit:    payload.PullLimit,
		SyncLimit:    payload.SyncLimit,
		ContactLimit: payload.ContactLimit,
		MediaLimit:   payload.MediaLimit,
	})
	if err != nil {
		writeIntegrationTestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// OfficialCheckHandler serializes POST /api/v1/archive/official/check.
func (handler Handler) OfficialCheckHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Official == nil {
		writeError(w, http.StatusServiceUnavailable, "archive official check service is not configured")
		return
	}
	payload, ok := decodeOfficialCheckBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive official check payload")
		return
	}
	response, err := handler.Official.OfficialCheck(r.Context(), archiveadmin.OfficialCheckRequest{
		EnterpriseID: payload.EnterpriseID,
		BaseURL:      handler.resolveBaseURL(r),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// MessagesBatchHandler serializes POST /api/v1/archive/messages/batch.
func (handler Handler) MessagesBatchHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentOrSession(w, r) {
		return
	}
	if handler.ArchiveIngestDisabled {
		writeError(w, http.StatusServiceUnavailable, "archive ingest disabled")
		return
	}
	if handler.BatchIngest == nil {
		writeError(w, http.StatusServiceUnavailable, "archive ingest service is not configured")
		return
	}
	payload, ok := decodeMessagesBatchBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive messages batch payload")
		return
	}
	result, err := handler.BatchIngest.IngestArchiveBatch(r.Context(), archiveingest.BatchRequest{
		EnterpriseID: payload.EnterpriseID,
		Source:       payload.Source,
		Cursor:       payload.Cursor,
		Messages:     payload.Messages,
	})
	if err != nil {
		writeArchiveBatchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, archiveBatchPayload(result))
}

// SyncRunHandler serializes POST /api/v1/archive/sync/run.
func (handler Handler) SyncRunHandler(w http.ResponseWriter, r *http.Request) {
	if handler.ArchiveIngestDisabled {
		writeError(w, http.StatusServiceUnavailable, "archive ingest disabled")
		return
	}
	if handler.SyncRun == nil {
		writeError(w, http.StatusServiceUnavailable, "archive sync service is not configured")
		return
	}
	payload, ok := decodeSyncRunBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive sync payload")
		return
	}
	result, err := handler.SyncRun.RunArchiveSyncOnce(r.Context(), archivesync.Request{
		EnterpriseID:  payload.EnterpriseID,
		Source:        payload.Source,
		Cursor:        payload.Cursor,
		Limit:         payload.Limit,
		WeWorkUserID:  payload.WeWorkUserID,
		TriggerReason: "manual",
	})
	if err != nil {
		writeArchiveSyncError(w, err)
		return
	}
	if writeArchiveSyncSkip(w, result) {
		return
	}
	var ingestResult *archiveingest.Result
	if strings.TrimSpace(result.StagedTaskID) != "" && handler.SyncIngest != nil {
		processed, err := handler.SyncIngest.ProcessTask(r.Context(), result.StagedTaskID)
		if err != nil {
			writeArchiveSyncError(w, err)
			return
		}
		ingestResult = processed
	}
	writeJSON(w, http.StatusOK, archiveSyncRunPayload(result, ingestResult, payload.WeWorkUserID))
}

// StatusHandler serializes GET /api/v1/archive/status.
func (handler Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Status == nil {
		writeError(w, http.StatusServiceUnavailable, "archive status service is not configured")
		return
	}
	query := r.URL.Query()
	payload, err := handler.Status.Status(r.Context(), archiveadmin.StatusRequest{
		EnterpriseID: query.Get("enterprise_id"),
		Source:       query.Get("source"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CursorHandler serializes GET /api/v1/archive/cursor.
func (handler Handler) CursorHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Cursor == nil {
		writeError(w, http.StatusServiceUnavailable, "archive cursor service is not configured")
		return
	}
	query := r.URL.Query()
	payload, err := handler.Cursor.Cursor(r.Context(), archiveadmin.CursorRequest{
		EnterpriseID: query.Get("enterprise_id"),
		Source:       query.Get("source"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// MediaTasksHandler serializes GET /api/v1/archive/media/tasks.
func (handler Handler) MediaTasksHandler(w http.ResponseWriter, r *http.Request) {
	if handler.MediaTasks == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media tasks service is not configured")
		return
	}
	query := r.URL.Query()
	limit, ok := queryInt(query.Get("limit"), 100)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid limit")
		return
	}
	payload, err := handler.MediaTasks.MediaTasks(r.Context(), archiveadmin.MediaTasksRequest{
		EnterpriseID: query.Get("enterprise_id"),
		Source:       query.Get("source"),
		Status:       query.Get("status"),
		Limit:        limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// MediaSyncRunHandler serializes POST /api/v1/archive/media/sync/run.
func (handler Handler) MediaSyncRunHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Media == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media service is not configured")
		return
	}
	payload, ok := decodeMediaSyncRunBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive media sync payload")
		return
	}
	result, err := runMediaOnceWithLimit(r.Context(), handler.Media, payload.EnterpriseID, payload.Source, payload.Limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, "archive media sync failed: "+err.Error())
		return
	}
	response := mediaRunPayload(result)
	if result.Failed > 0 {
		writeJSON(w, http.StatusBadGateway, map[string]any{"detail": mediaRunFailurePayload(response)})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// MediaTaskPrepareHandler serializes POST /api/v1/archive/media/tasks/{task_id}/prepare.
func (handler Handler) MediaTaskPrepareHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Media == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media service is not configured")
		return
	}
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	result, err := handler.Media.RunTask(r.Context(), taskID)
	if err != nil {
		switch {
		case errors.Is(err, archivemedia.ErrMediaTaskNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "not configured"):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		default:
			writeError(w, http.StatusBadGateway, "archive media prepare failed: "+err.Error())
		}
		return
	}
	response := mediaRunPayload(result)
	response["task_id"] = taskID
	writeJSON(w, http.StatusOK, response)
}

// MediaFileHandler serializes GET /api/v1/archive/media/files/{task_id}.
func (handler Handler) MediaFileHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Download == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media download service is not configured")
		return
	}
	download, err := handler.Download.DownloadTask(r.Context(), strings.TrimSpace(r.PathValue("task_id")), r.URL.Query().Get("token"))
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeDownload(w, download)
}

// MediaObjectHandler serializes GET /api/v1/archive/media/objects/{object_path:path}.
func (handler Handler) MediaObjectHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Download == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media download service is not configured")
		return
	}
	download, err := handler.Download.DownloadObject(r.Context(), strings.TrimSpace(r.PathValue("object_path")), r.URL.Query().Get("token"))
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeDownload(w, download)
}

// SOPLocalMediaHandler serializes GET /api/v1/admin/sop/media/local.
func (handler Handler) SOPLocalMediaHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Download == nil {
		writeError(w, http.StatusServiceUnavailable, "archive media download service is not configured")
		return
	}
	objectURL := strings.TrimSpace(r.URL.Query().Get("object_url"))
	if !strings.HasPrefix(strings.ToLower(objectURL), "local://") {
		writeError(w, http.StatusUnprocessableEntity, "object_url must be a local media url")
		return
	}
	download, err := handler.Download.DownloadLocalObject(r.Context(), objectURL)
	if err != nil {
		if errors.Is(err, archivemedia.ErrMediaLocalFileNotFound) {
			writeError(w, http.StatusNotFound, "local media file not found")
			return
		}
		writeDownloadError(w, err)
		return
	}
	writeDownload(w, download)
}

// EventNotifyHandler serializes POST /api/v1/archive/events/notify.
func (handler Handler) EventNotifyHandler(w http.ResponseWriter, r *http.Request) {
	if !verifyBridgeToken(r.Header.Get("Authorization"), handler.BridgeToken) {
		writeError(w, http.StatusUnauthorized, "archive bridge token invalid")
		return
	}
	if handler.EventNotify == nil {
		writeError(w, http.StatusServiceUnavailable, "archive event notify service is not configured")
		return
	}
	payload, ok := decodeEventNotifyBody(w, r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid archive event notify payload")
		return
	}
	result, err := handler.EventNotify.Notify(r.Context(), archiveeventnotify.Request{
		EnterpriseID: payload.EnterpriseID,
		Source:       payload.Source,
		Cursor:       payload.Cursor,
		Limit:        payload.Limit,
		Event:        payload.Event,
		Vendor:       payload.Vendor,
		Payload:      payload.Payload,
	})
	if err != nil {
		if errors.Is(err, archiveeventnotify.ErrOutboxStoreUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "archive event notify failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":      result.Accepted,
		"running":       result.Running,
		"trigger_id":    result.TriggerID,
		"enterprise_id": result.EnterpriseID,
		"event":         result.Event,
		"vendor":        result.Vendor,
	})
}

func writeDownloadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archivemedia.ErrMediaAccessTokenRequired):
		writeError(w, http.StatusUnauthorized, "token is required")
	case errors.Is(err, archivemedia.ErrMediaAccessTokenInvalid):
		writeError(w, http.StatusUnauthorized, "token invalid or expired")
	case errors.Is(err, archivemedia.ErrMediaTaskStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, archivemedia.ErrMediaTaskNotFound):
		writeError(w, http.StatusNotFound, "media task not found")
	case errors.Is(err, archivemedia.ErrMediaObjectNotFound):
		writeError(w, http.StatusNotFound, "media object not found")
	case errors.Is(err, archivemedia.ErrMediaLocalFileNotFound):
		writeError(w, http.StatusNotFound, "media local file not found")
	case errors.Is(err, archivemedia.ErrObjectNotFound):
		writeError(w, http.StatusNotFound, "object not found")
	case strings.HasPrefix(err.Error(), "media proxy failed:"):
		writeError(w, http.StatusBadGateway, err.Error())
	case strings.HasPrefix(err.Error(), "object proxy failed:"):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeDownload(w http.ResponseWriter, download archivemedia.DownloadResponse) {
	if download.Body == nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer download.Body.Close()
	contentType := strings.TrimSpace(download.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	filename := strings.TrimSpace(download.Filename)
	if filename == "" {
		filename = "media.bin"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if download.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(download.ContentLength, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, download.Body)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archiveadmin.ErrOfficialEnterpriseIDRequired):
		writeError(w, http.StatusUnprocessableEntity, archiveadmin.ErrOfficialEnterpriseIDRequired.Error())
	case errors.Is(err, archiveadmin.ErrOfficialEnterpriseNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, archiveadmin.ErrEnterpriseStoreUnavailable),
		errors.Is(err, archiveadmin.ErrCursorStoreUnavailable),
		errors.Is(err, archiveadmin.ErrMediaTaskStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeArchiveBatchError(w http.ResponseWriter, err error) {
	switch {
	case strings.Contains(err.Error(), "not configured"):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case strings.HasPrefix(err.Error(), "archive message "):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeArchiveSyncError(w http.ResponseWriter, err error) {
	switch {
	case strings.Contains(err.Error(), "ARCHIVE_SELF_DECRYPT_PULL_URL is not configured"):
		writeError(w, http.StatusBadRequest, "ARCHIVE_SELF_DECRYPT_PULL_URL is not configured")
	case strings.Contains(err.Error(), "not configured"):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusBadGateway, "archive sync failed: "+err.Error())
	}
}

func writeArchiveContactsSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archivecontacts.ErrContactsServiceUnavailable),
		errors.Is(err, archivecontacts.ErrConversationStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "archive contacts sync failed")
	}
}

func writeSDKPullError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archivesdk.ErrEnterpriseNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, archivesdk.ErrCorpCredentialsRequired):
		writeError(w, http.StatusUnprocessableEntity, archivesdk.ErrCorpCredentialsRequired.Error())
	case errors.Is(err, archivesdk.ErrEnterpriseStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, archivesdk.ErrEnterpriseStoreUnavailable.Error())
	default:
		writeError(w, http.StatusBadGateway, "sdk pull failed: "+err.Error())
	}
}

func writeSDKMediaPullError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archivesdk.ErrSDKFileIDRequired):
		writeError(w, http.StatusUnprocessableEntity, archivesdk.ErrSDKFileIDRequired.Error())
	case errors.Is(err, archivesdk.ErrEnterpriseNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, archivesdk.ErrCorpCredentialsRequired):
		writeError(w, http.StatusUnprocessableEntity, archivesdk.ErrCorpCredentialsRequired.Error())
	case errors.Is(err, archivesdk.ErrEnterpriseStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, archivesdk.ErrEnterpriseStoreUnavailable.Error())
	default:
		writeError(w, http.StatusBadGateway, "sdk media pull failed: "+err.Error())
	}
}

func writeIntegrationTestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, archiveintegration.ErrEnterpriseIDRequired):
		writeError(w, http.StatusUnprocessableEntity, archiveintegration.ErrEnterpriseIDRequired.Error())
	case errors.Is(err, archiveintegration.ErrEnterpriseNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, archiveintegration.ErrEnterpriseStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, archiveintegration.ErrEnterpriseStoreUnavailable.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeArchiveSyncSkip(w http.ResponseWriter, result archivesync.Result) bool {
	if !result.Skipped {
		return false
	}
	switch result.SkipReason {
	case "enterprise_missing":
		writeError(w, http.StatusNotFound, "enterprise not found: "+defaultText(result.EnterpriseID, archivesync.DefaultEnterpriseID))
	case "enterprise_disabled":
		writeError(w, http.StatusUnprocessableEntity, "enterprise disabled: "+defaultText(result.EnterpriseID, archivesync.DefaultEnterpriseID))
	case "self_decrypt_pull_url_missing":
		writeError(w, http.StatusUnprocessableEntity, "enterprise archive sync requires archive_pull_url: "+defaultText(result.EnterpriseID, archivesync.DefaultEnterpriseID))
	default:
		return false
	}
	return true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

type mediaSyncRunBody struct {
	EnterpriseID string `json:"enterprise_id"`
	Source       string `json:"source"`
	Limit        int    `json:"limit"`
}

type eventNotifyBody struct {
	EnterpriseID string         `json:"enterprise_id"`
	Source       string         `json:"source"`
	Cursor       string         `json:"cursor"`
	Limit        int            `json:"limit"`
	Event        string         `json:"event"`
	Vendor       string         `json:"vendor"`
	Payload      map[string]any `json:"payload"`
}

type officialCheckBody struct {
	EnterpriseID string `json:"enterprise_id"`
}

type integrationTestBody struct {
	EnterpriseID string `json:"enterprise_id"`
	Source       string `json:"source"`
	PullLimit    int    `json:"pull_limit"`
	SyncLimit    int    `json:"sync_limit"`
	ContactLimit int    `json:"contact_limit"`
	MediaLimit   int    `json:"media_limit"`
}

type sdkPullBody struct {
	EnterpriseID      string  `json:"enterprise_id"`
	Source            string  `json:"source"`
	Cursor            *string `json:"cursor"`
	Limit             int     `json:"limit"`
	CorpID            string  `json:"corp_id"`
	CorpSecret        string  `json:"corp_secret"`
	PrivateKeyPEM     string  `json:"private_key_pem"`
	PrivateKeyVersion string  `json:"private_key_version"`
}

type sdkMediaPullBody struct {
	EnterpriseID string `json:"enterprise_id"`
	Source       string `json:"source"`
	SDKFileID    string `json:"sdk_file_id"`
	IndexBuf     string `json:"index_buf"`
	CorpID       string `json:"corp_id"`
	CorpSecret   string `json:"corp_secret"`
}

type messagesBatchBody struct {
	EnterpriseID string           `json:"enterprise_id"`
	Source       string           `json:"source"`
	Cursor       *string          `json:"cursor"`
	Messages     []map[string]any `json:"messages"`
}

type syncRunBody struct {
	EnterpriseID string  `json:"enterprise_id"`
	Source       string  `json:"source"`
	Cursor       *string `json:"cursor"`
	Limit        int     `json:"limit"`
	WeWorkUserID string  `json:"wework_user_id"`
}

type archiveContactsSyncBody struct {
	EnterpriseID string   `json:"enterprise_id"`
	SenderIDs    []string `json:"sender_ids"`
	ForceRefresh bool     `json:"force_refresh"`
	Limit        int      `json:"limit"`
}

func decodeMessagesBatchBody(w http.ResponseWriter, r *http.Request) (messagesBatchBody, bool) {
	if r.Body == nil {
		return messagesBatchBody{}, false
	}
	var payload messagesBatchBody
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
	if err := decoder.Decode(&payload); err != nil {
		return messagesBatchBody{}, false
	}
	if payload.Messages == nil {
		return messagesBatchBody{}, false
	}
	return payload, true
}

func decodeSyncRunBody(w http.ResponseWriter, r *http.Request) (syncRunBody, bool) {
	var payload syncRunBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return syncRunBody{}, false
	}
	return payload, true
}

func decodeOfficialCheckBody(w http.ResponseWriter, r *http.Request) (officialCheckBody, bool) {
	var payload officialCheckBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return officialCheckBody{}, false
	}
	return payload, true
}

func decodeIntegrationTestBody(w http.ResponseWriter, r *http.Request) (integrationTestBody, bool) {
	var payload integrationTestBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return integrationTestBody{}, false
	}
	return payload, true
}

func decodeSDKPullBody(w http.ResponseWriter, r *http.Request) (sdkPullBody, bool) {
	var payload sdkPullBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return sdkPullBody{}, false
	}
	return payload, true
}

func decodeSDKMediaPullBody(w http.ResponseWriter, r *http.Request) (sdkMediaPullBody, bool) {
	var payload sdkMediaPullBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return sdkMediaPullBody{}, false
	}
	return payload, true
}

func decodeMediaSyncRunBody(w http.ResponseWriter, r *http.Request) (mediaSyncRunBody, bool) {
	var payload mediaSyncRunBody
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return mediaSyncRunBody{}, false
	}
	return payload, true
}

func decodeEventNotifyBody(w http.ResponseWriter, r *http.Request) (eventNotifyBody, bool) {
	var payload eventNotifyBody
	if r.Body == nil {
		return payload, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return eventNotifyBody{}, false
	}
	return payload, true
}

func decodeArchiveContactsSyncBody(w http.ResponseWriter, r *http.Request) (archiveContactsSyncBody, bool) {
	payload := archiveContactsSyncBody{
		EnterpriseID: archivecontacts.DefaultEnterpriseID,
		Limit:        archivecontacts.DefaultLimit,
	}
	if r.Body == nil {
		return payload, true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, true
		}
		return archiveContactsSyncBody{}, false
	}
	return payload, true
}

func runMediaOnceWithLimit(ctx context.Context, runner mediaRunner, enterpriseID string, source string, limit int) (archivemedia.RunResult, error) {
	if limited, ok := runner.(mediaRunnerWithLimit); ok {
		return limited.RunOnceWithLimit(ctx, enterpriseID, source, limit)
	}
	return runner.RunOnce(ctx, enterpriseID, source)
}

func mediaRunPayload(result archivemedia.RunResult) map[string]any {
	return map[string]any{
		"accepted":        true,
		"enterprise_id":   result.EnterpriseID,
		"source":          result.Source,
		"total":           result.Total,
		"success":         result.Success,
		"failed":          result.Failed,
		"pending":         result.Pending,
		"failure_reasons": []any{},
	}
}

func mediaRunFailurePayload(payload map[string]any) map[string]any {
	return map[string]any{
		"message":         "archive media sync failed",
		"enterprise_id":   payload["enterprise_id"],
		"source":          payload["source"],
		"total":           payload["total"],
		"success":         payload["success"],
		"failed":          payload["failed"],
		"pending":         payload["pending"],
		"failure_reasons": payload["failure_reasons"],
	}
}

func verifyBridgeToken(authorization string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	token := strings.TrimSpace(authorization)
	if token != "" {
		parts := strings.SplitN(token, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			token = strings.TrimSpace(parts[1])
		}
	}
	if token == "" || len(token) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (handler Handler) requireAgentOrSession(w http.ResponseWriter, r *http.Request) bool {
	token := auth.ParseBearerToken(r.Header.Get("Authorization"))
	if token != "" && strings.TrimSpace(handler.Guard.Verifier.Secret) != "" {
		if _, err := handler.Guard.Verifier.VerifyContext(r.Context(), token); err == nil {
			return true
		}
	}
	expectedAgentToken := strings.TrimSpace(handler.AgentToken)
	actualAgentToken := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	if expectedAgentToken != "" && actualAgentToken != "" && len(actualAgentToken) == len(expectedAgentToken) && subtle.ConstantTimeCompare([]byte(actualAgentToken), []byte(expectedAgentToken)) == 1 {
		return true
	}
	if handler.AllowLegacyAgentAuth {
		return true
	}
	writeError(w, http.StatusUnauthorized, "authentication required")
	return false
}

func archiveBatchPayload(result archiveingest.Result) map[string]any {
	conversationIDs := result.ConversationIDs
	if conversationIDs == nil {
		conversationIDs = []string{}
	}
	return map[string]any{
		"accepted":         true,
		"enterprise_id":    result.EnterpriseID,
		"source":           result.Source,
		"total":            result.Total,
		"merged":           result.Merged,
		"inserted":         result.Inserted,
		"deduplicated":     result.Deduplicated,
		"cursor":           result.Cursor,
		"conversation_ids": conversationIDs,
	}
}

func archiveSyncRunPayload(result archivesync.Result, ingestResult *archiveingest.Result, weworkUserID string) map[string]any {
	cursor := result.Cursor
	total := 0
	merged := 0
	inserted := 0
	deduplicated := 0
	conversationIDs := []string{}
	if ingestResult != nil {
		total = ingestResult.Total
		merged = ingestResult.Merged
		inserted = ingestResult.Inserted
		deduplicated = ingestResult.Deduplicated
		if ingestResult.Cursor != nil {
			cursor = ingestResult.Cursor
		}
		if ingestResult.ConversationIDs != nil {
			conversationIDs = ingestResult.ConversationIDs
		}
	}
	var userID any
	if value := strings.TrimSpace(weworkUserID); value != "" {
		userID = value
	}
	return map[string]any{
		"accepted":         true,
		"enterprise_id":    result.EnterpriseID,
		"source":           result.Source,
		"total":            total,
		"merged":           merged,
		"inserted":         inserted,
		"deduplicated":     deduplicated,
		"cursor":           cursor,
		"conversation_ids": conversationIDs,
		"pulled_total":     result.PulledTotal,
		"matched_total":    result.PulledTotal,
		"wework_user_id":   userID,
	}
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func queryInt(raw string, fallback int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	if value < 1 {
		return 1, true
	}
	if value > 1000 {
		return 1000, true
	}
	return value, true
}

func (handler Handler) resolveBaseURL(r *http.Request) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(handler.BackendBaseURL), "/"); baseURL != "" {
		return baseURL
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	if comma := strings.Index(scheme, ","); comma >= 0 {
		scheme = strings.TrimSpace(scheme[:comma])
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}
