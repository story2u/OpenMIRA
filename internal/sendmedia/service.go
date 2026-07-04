// Package sendmedia builds legacy /send/image|video|voice|file task responses.
package sendmedia

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

const MaxUploadBytes = 50 * 1024 * 1024

type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
	KindVoice Kind = "voice"
	KindFile  Kind = "file"
)

var (
	ErrInvalidRequest     = errors.New("invalid send media request")
	ErrTaskServiceMissing = errors.New("send media task service is not configured")
	ErrUploadMissing      = errors.New("send media upload service is not configured")
	ErrUnsupportedType    = errors.New("unsupported media type")
	ErrUploadTooLarge     = errors.New("upload file is too large")
)

// TaskCreator stores one durable SDK task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// Uploader stores media bytes and returns an object URL.
type Uploader interface {
	UploadArchiveMedia(ctx context.Context, input archivemedia.UploadInput) (string, error)
}

// AuditLogWriter appends legacy audit_logs rows.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// Service uploads media and creates one SDK send task.
type Service struct {
	Tasks       TaskCreator
	Uploader    Uploader
	Targets     sendtarget.Resolver
	AuditLogs   AuditLogWriter
	DeviceGuard sendguard.DeviceOnlineGuard
	Limiter     sendguard.Limiter
	AccessURL   func(taskID string, objectURL string) string
	Now         func() time.Time
	NewID       func(prefix string) string
}

// Request mirrors the multipart fields shared by legacy media send routes.
type Request struct {
	Kind             Kind
	DeviceID         string
	Username         string
	TargetUsername   string
	Aliases          string
	AgentID          string
	ConversationID   string
	SenderID         string
	OrganizationName string
	Source           string
	FileName         string
	ContentType      string
	Content          []byte
	VoiceDurationSec int
	Operator         string
}

// Send uploads media bytes and creates one accepted SDK task.
func (service Service) Send(ctx context.Context, request Request) (map[string]any, error) {
	if service.Tasks == nil {
		return nil, ErrTaskServiceMissing
	}
	if service.Uploader == nil {
		return nil, ErrUploadMissing
	}
	normalized, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return nil, err
	}
	if err := service.checkRateLimit(normalized.DeviceID); err != nil {
		return nil, err
	}
	normalized, err = service.resolveTarget(ctx, normalized)
	if err != nil {
		return nil, err
	}
	objectURL, err := service.Uploader.UploadArchiveMedia(ctx, archivemedia.UploadInput{
		EnterpriseID: "manual-send",
		SDKFileID:    normalized.sdkFileID(),
		PayloadJSON:  normalized.uploadPayloadJSON(),
		Content:      normalized.Content,
	})
	if err != nil {
		return nil, err
	}
	mediaURL := strings.TrimSpace(objectURL)
	if service.AccessURL != nil {
		if accessURL := strings.TrimSpace(service.AccessURL("manual-send", objectURL)); accessURL != "" {
			mediaURL = accessURL
		}
	}
	now := service.now()
	traceID := service.newID("trace-")
	record, err := service.Tasks.Create(ctx, tasks.CreateRequest{
		TaskID:    service.newID("task-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  normalized.taskType(),
		Payload:   normalized.payload(mediaURL),
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	service.recordRateLimit(normalized.DeviceID)
	service.recordAudit(ctx, normalized)
	response := map[string]any{
		"success": taskAccepted(record.Status),
		"task":    record,
	}
	if len(normalized.ContactProfileUpdate) > 0 {
		response["contact_profile_update"] = normalized.ContactProfileUpdate
	}
	return response, nil
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
}

func (service Service) checkRateLimit(deviceID string) error {
	if service.Limiter == nil {
		return nil
	}
	allowed, reason := service.Limiter.Check(deviceID)
	if !allowed {
		return sendguard.RateLimitError{Reason: reason}
	}
	return nil
}

func (service Service) recordRateLimit(deviceID string) {
	if service.Limiter != nil {
		service.Limiter.Record(deviceID)
	}
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest) {
	if service.AuditLogs == nil {
		return
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "send",
		Detail:     fmt.Sprintf("发送%s: device_id=%s, username=%s, file=%s", request.Kind, request.DeviceID, request.Username, defaultText(request.FileName, "-")),
	})
}

type normalizedRequest struct {
	Kind                 Kind
	DeviceID             string
	Username             string
	ReceiverName         string
	Receiver             string
	Aliases              string
	AgentID              string
	ConversationID       string
	SenderID             string
	OrganizationName     string
	Source               string
	FileName             string
	ContentType          string
	Content              []byte
	VoiceDurationSec     int
	Operator             string
	ContactProfileUpdate map[string]any
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	kind := normalizeKind(request.Kind)
	normalized := normalizedRequest{
		Kind:             kind,
		DeviceID:         strings.TrimSpace(request.DeviceID),
		Username:         strings.TrimSpace(request.Username),
		ReceiverName:     strings.TrimSpace(request.Username),
		Receiver:         buildReceiver(request.TargetUsername, request.Username),
		Aliases:          strings.TrimSpace(request.Aliases),
		AgentID:          strings.TrimSpace(request.AgentID),
		ConversationID:   strings.TrimSpace(request.ConversationID),
		SenderID:         strings.TrimSpace(request.SenderID),
		OrganizationName: strings.TrimSpace(request.OrganizationName),
		Source:           normalizeSource(request.Source),
		FileName:         strings.TrimSpace(request.FileName),
		ContentType:      normalizeContentType(request.ContentType),
		Content:          request.Content,
		VoiceDurationSec: request.VoiceDurationSec,
		Operator:         strings.TrimSpace(request.Operator),
	}
	if normalized.Kind == "" {
		return normalizedRequest{}, invalid("media kind is required")
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	if normalized.Username == "" {
		return normalizedRequest{}, invalid("username is required")
	}
	if normalized.Receiver == "" {
		return normalizedRequest{}, invalid("target_username is required")
	}
	if len(normalized.Content) == 0 {
		return normalizedRequest{}, invalid("file content is required")
	}
	if len(normalized.Content) > MaxUploadBytes {
		return normalizedRequest{}, errors.Join(ErrUploadTooLarge, errors.New("file exceeds 50MB"))
	}
	if err := validateFileType(normalized.Kind, normalized.FileName, normalized.ContentType); err != nil {
		return normalizedRequest{}, err
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	if normalized.Aliases == normalized.Receiver {
		normalized.Aliases = ""
	}
	if normalized.VoiceDurationSec < 0 {
		normalized.VoiceDurationSec = 0
	}
	return normalized, nil
}

func (service Service) resolveTarget(ctx context.Context, request normalizedRequest) (normalizedRequest, error) {
	if service.Targets == nil {
		return request, nil
	}
	target, err := service.Targets.ResolveSendTarget(ctx, sendtarget.Request{
		ConversationID:     request.ConversationID,
		DeviceID:           request.DeviceID,
		FallbackReceiver:   request.Receiver,
		FallbackAliases:    request.Aliases,
		FallbackSenderName: request.ReceiverName,
		FallbackSenderID:   request.SenderID,
		PreferRPASafeName:  true,
	})
	if err != nil {
		return normalizedRequest{}, err
	}
	if strings.TrimSpace(target.Receiver) != "" {
		request.Receiver = strings.TrimSpace(target.Receiver)
	}
	request.Aliases = strings.TrimSpace(target.Aliases)
	if strings.EqualFold(request.Aliases, request.Receiver) {
		request.Aliases = ""
	}
	if strings.TrimSpace(target.SenderName) != "" {
		request.ReceiverName = strings.TrimSpace(target.SenderName)
	}
	if strings.TrimSpace(target.ConversationID) != "" {
		request.ConversationID = strings.TrimSpace(target.ConversationID)
	}
	if strings.TrimSpace(target.SenderID) != "" {
		request.SenderID = strings.TrimSpace(target.SenderID)
	}
	if len(target.ContactProfileUpdate) > 0 {
		request.ContactProfileUpdate = target.ContactProfileUpdate
	}
	return request, nil
}

func (request normalizedRequest) taskType() string {
	return "send_" + string(request.Kind)
}

func (request normalizedRequest) mediaMIME() string {
	if request.ContentType != "" {
		return request.ContentType
	}
	switch request.Kind {
	case KindImage:
		return "image/*"
	case KindVideo:
		return "video/*"
	case KindVoice:
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}

func (request normalizedRequest) payload(mediaURL string) map[string]any {
	payload := map[string]any{
		"username":      request.Username,
		"receiver":      request.Receiver,
		"receiver_name": request.ReceiverName,
		"media_url":     mediaURL,
		"media_mime":    request.mediaMIME(),
		"queue":         "fast",
	}
	if request.Aliases != "" {
		payload["aliases"] = request.Aliases
	}
	if request.ConversationID != "" {
		payload["conversation_id"] = request.ConversationID
		payload["session_id"] = request.ConversationID
	}
	if request.SenderID != "" {
		payload["sender_id"] = request.SenderID
	}
	switch request.Kind {
	case KindFile:
		payload["filename"] = defaultText(request.FileName, "file.bin")
	case KindVoice:
		payload["filename"] = defaultText(request.FileName, "voice.webm")
		payload["voice_duration_sec"] = request.VoiceDurationSec
	}
	return payload
}

func (request normalizedRequest) sdkFileID() string {
	sum := sha256.Sum256(request.Content)
	return "manual-send-" + hex.EncodeToString(sum[:])[:24]
}

func (request normalizedRequest) uploadPayloadJSON() string {
	payload := map[string]any{"decrypted": map[string]any{"msgtype": string(request.Kind)}}
	if request.Kind == KindFile && request.FileName != "" {
		payload["decrypted"].(map[string]any)["file"] = map[string]any{"filename": request.FileName}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func normalizeKind(kind Kind) Kind {
	switch Kind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case KindImage:
		return KindImage
	case KindVideo:
		return KindVideo
	case KindVoice:
		return KindVoice
	case KindFile:
		return KindFile
	default:
		return ""
	}
}

func buildReceiver(targetUsername string, username string) string {
	if value := strings.TrimSpace(targetUsername); value != "" {
		return value
	}
	return strings.TrimSpace(username)
}

func normalizeSource(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "cloud-web", "cloud-backend", "system":
		return normalized
	default:
		return "cloud-web"
	}
}

func normalizeContentType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
}

func validateFileType(kind Kind, filename string, contentType string) error {
	if blockedExtension(filename) {
		return errors.Join(ErrUnsupportedType, errors.New("blocked file extension"))
	}
	if contentType == "" {
		return nil
	}
	var allowed map[string]bool
	switch kind {
	case KindImage:
		allowed = imageTypes
	case KindVideo:
		allowed = videoTypes
	case KindVoice:
		allowed = audioTypes
	default:
		return nil
	}
	if !allowed[contentType] {
		return errors.Join(ErrUnsupportedType, errors.New("unsupported content type"))
	}
	return nil
}

func blockedExtension(filename string) bool {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	return blockedExtensions[extension]
}

var imageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"image/bmp":  true,
}

var videoTypes = map[string]bool{
	"video/mp4":       true,
	"video/mpeg":      true,
	"video/quicktime": true,
	"video/x-msvideo": true,
	"video/webm":      true,
}

var audioTypes = map[string]bool{
	"audio/aac":   true,
	"audio/amr":   true,
	"audio/mp4":   true,
	"audio/mpeg":  true,
	"audio/ogg":   true,
	"audio/wav":   true,
	"audio/webm":  true,
	"audio/x-m4a": true,
	"audio/x-wav": true,
}

var blockedExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".ps1": true, ".sh": true, ".bash": true,
	".dll": true, ".so": true, ".dylib": true, ".js": true, ".vbs": true, ".wsf": true,
	".hta": true, ".php": true, ".asp": true, ".aspx": true, ".jsp": true, ".py": true,
	".pyc": true, ".pyo": true, ".jar": true, ".class": true, ".scr": true, ".pif": true,
	".com": true,
}

func invalid(message string) error {
	return errors.Join(ErrInvalidRequest, errors.New(message))
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func taskAccepted(status tasks.Status) bool {
	switch status {
	case tasks.StatusAccepted, tasks.StatusRunning, tasks.StatusSuccess:
		return true
	default:
		return false
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) newID(prefix string) string {
	if service.NewID != nil {
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}
