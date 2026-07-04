// Package conversationcall builds WeCom call SDK tasks from conversation snapshots.
package conversationcall

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest       = errors.New("invalid conversation call request")
	ErrTaskServiceMissing   = errors.New("conversation call task service is not configured")
	ErrLockStoreMissing     = errors.New("conversation call lock store is not configured")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrTargetNotReady       = errors.New("contact identity is not ready")
	ErrCallSlotBusy         = errors.New("conversation call slot is busy")
)

const defaultLockTTL = 7200 * time.Second

type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

type ConversationStore interface {
	GetConversation(ctx context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error)
}

// AuditLogWriter appends legacy audit_logs rows.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

type LockStore interface {
	Reserve(ctx context.Context, key string, lock Lock, ttl time.Duration) (bool, error)
	Read(ctx context.Context, key string) (Lock, bool, error)
	Refresh(ctx context.Context, key string, lock Lock, ttl time.Duration) error
	Release(ctx context.Context, key string) error
}

type Service struct {
	Tasks         TaskCreator
	Conversations ConversationStore
	Locks         LockStore
	AuditLogs     AuditLogWriter
	DeviceGuard   sendguard.DeviceOnlineGuard
	Targets       sendtarget.Resolver
	LockTTL       time.Duration
	CachePrefix   string
	Now           func() time.Time
	NewID         func(prefix string) string
}

type Request struct {
	DeviceID      string `json:"device_id"`
	CallType      string `json:"call_type"`
	AgentID       string `json:"agent_id"`
	Source        string `json:"source"`
	ReservationID string `json:"reservation_id"`
	Operator      string `json:"-"`
}

type Lock struct {
	ReservationID  string  `json:"reservation_id"`
	ConversationID string  `json:"conversation_id"`
	DeviceID       string  `json:"device_id"`
	Operator       string  `json:"operator"`
	CallType       string  `json:"call_type"`
	Receiver       string  `json:"receiver"`
	AccountScope   string  `json:"account_scope"`
	CreatedAt      float64 `json:"created_at"`
	ExpiresAt      float64 `json:"expires_at"`
}

func (service Service) Availability(ctx context.Context, conversationID string, request Request, operator string) (map[string]any, error) {
	normalized, err := service.normalize(ctx, conversationID, request, true, false)
	if err != nil {
		return nil, err
	}
	lock, err := service.reserve(ctx, normalized, operator, request.ReservationID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"success":        true,
		"reservation_id": lock.ReservationID,
		"expires_in":     int(service.lockTTL().Seconds()),
		"call_type":      normalized.CallType,
	}, nil
}

func (service Service) ReleaseReservation(ctx context.Context, conversationID string, request Request) (map[string]any, error) {
	normalized, err := service.normalize(ctx, conversationID, request, false, false)
	if err != nil {
		return nil, err
	}
	released, err := service.release(ctx, normalized, request.ReservationID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "released": released}, nil
}

func (service Service) Call(ctx context.Context, conversationID string, request Request) (map[string]any, error) {
	normalized, err := service.normalize(ctx, conversationID, request, true, true)
	if err != nil {
		return nil, err
	}
	lock, err := service.reserve(ctx, normalized, request.Operator, request.ReservationID)
	if err != nil {
		return nil, err
	}
	taskType := "wework_voice_call"
	if normalized.CallType == "video" {
		taskType = "wework_video_call"
	}
	record, err := service.create(ctx, normalized, taskType)
	if err != nil {
		_, _ = service.release(ctx, normalized, lock.ReservationID)
		return nil, err
	}
	service.recordAudit(ctx, normalized, "call")
	return map[string]any{"success": true, "task": record, "reservation_id": lock.ReservationID}, nil
}

func (service Service) Hangup(ctx context.Context, conversationID string, request Request) (map[string]any, error) {
	normalized, err := service.normalize(ctx, conversationID, request, false, true)
	if err != nil {
		return nil, err
	}
	record, err := service.create(ctx, normalized, "wework_hangup_call")
	if err != nil {
		return nil, err
	}
	_, _ = service.release(ctx, normalized, request.ReservationID)
	service.recordAudit(ctx, normalized, "hangup")
	return map[string]any{"success": true, "task": record}, nil
}

type normalizedRequest struct {
	ConversationID string
	DeviceID       string
	CallType       string
	AgentID        string
	Source         string
	SenderID       string
	Receiver       string
	ReceiverName   string
	Aliases        string
	AccountScope   string
	Operator       string
}

func (service Service) normalize(ctx context.Context, conversationID string, request Request, requireCallType bool, requireTask bool) (normalizedRequest, error) {
	if requireTask && service.Tasks == nil {
		return normalizedRequest{}, ErrTaskServiceMissing
	}
	if service.Conversations == nil {
		return normalizedRequest{}, ErrConversationNotFound
	}
	normalized := normalizedRequest{
		ConversationID: text(conversationID),
		DeviceID:       text(request.DeviceID),
		AgentID:        text(request.AgentID),
		Source:         normalizeSource(request.Source),
		Operator:       text(request.Operator),
	}
	if normalized.ConversationID == "" {
		return normalizedRequest{}, invalid("conversation_id is required")
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	rawCallType := strings.ToLower(strings.TrimSpace(request.CallType))
	switch rawCallType {
	case "":
		if requireCallType {
			normalized.CallType = "voice"
		}
	case "voice", "video":
		normalized.CallType = rawCallType
	default:
		return normalizedRequest{}, invalid("call_type must be voice or video")
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return normalizedRequest{}, err
	}
	snapshot, ok, err := service.Conversations.GetConversation(ctx, normalized.ConversationID)
	if err != nil {
		return normalizedRequest{}, err
	}
	if !ok {
		return normalizedRequest{}, ErrConversationNotFound
	}
	normalized.SenderID = text(snapshot.SenderID)
	normalized.Receiver = firstNonBlank(snapshot.SenderRemark, snapshot.SenderName, snapshot.ConversationName, snapshot.SenderID)
	normalized.ReceiverName = firstNonBlank(snapshot.SenderName, normalized.Receiver)
	normalized.AccountScope = callAccountScope(snapshot, normalized.DeviceID)
	if aliases := firstNonBlank(snapshot.SenderRemark); aliases != "" && aliases != normalized.Receiver {
		normalized.Aliases = aliases
	}
	if normalized.Receiver == "" {
		return normalizedRequest{}, ErrTargetNotReady
	}
	return service.resolveTarget(ctx, normalized)
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
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
	})
	if err != nil {
		return normalizedRequest{}, err
	}
	if text(target.Receiver) != "" {
		request.Receiver = text(target.Receiver)
	}
	request.Aliases = text(target.Aliases)
	if strings.EqualFold(request.Aliases, request.Receiver) {
		request.Aliases = ""
	}
	if text(target.SenderName) != "" {
		request.ReceiverName = text(target.SenderName)
	}
	if text(target.SenderID) != "" {
		request.SenderID = text(target.SenderID)
	}
	if text(target.ConversationID) != "" {
		request.ConversationID = text(target.ConversationID)
	}
	if request.Receiver == "" {
		return normalizedRequest{}, ErrTargetNotReady
	}
	return request, nil
}

func (service Service) reserve(ctx context.Context, request normalizedRequest, operator string, reservationID string) (Lock, error) {
	if service.Locks == nil {
		return Lock{}, ErrLockStoreMissing
	}
	reservationID = text(reservationID)
	if reservationID == "" {
		reservationID = service.newID("reservation-")
	}
	now := service.now()
	ttl := service.lockTTL()
	lock := Lock{
		ReservationID:  reservationID,
		ConversationID: request.ConversationID,
		DeviceID:       request.DeviceID,
		Operator:       text(operator),
		CallType:       request.CallType,
		Receiver:       request.Receiver,
		AccountScope:   request.AccountScope,
		CreatedAt:      unixSeconds(now),
		ExpiresAt:      unixSeconds(now.Add(ttl)),
	}
	key := service.lockKey(request.AccountScope)
	acquired, err := service.Locks.Reserve(ctx, key, lock, ttl)
	if err != nil {
		return Lock{}, err
	}
	if acquired {
		return lock, nil
	}
	active, ok, err := service.Locks.Read(ctx, key)
	if err != nil {
		return Lock{}, err
	}
	if sameReservation(active, ok, reservationID) {
		if err := service.Locks.Refresh(ctx, key, lock, ttl); err != nil {
			return Lock{}, err
		}
		return lock, nil
	}
	return Lock{}, ErrCallSlotBusy
}

func (service Service) release(ctx context.Context, request normalizedRequest, reservationID string) (bool, error) {
	if service.Locks == nil {
		return false, ErrLockStoreMissing
	}
	key := service.lockKey(request.AccountScope)
	reservationID = text(reservationID)
	if reservationID != "" {
		active, ok, err := service.Locks.Read(ctx, key)
		if err != nil {
			return false, err
		}
		if !sameReservation(active, ok, reservationID) {
			return false, nil
		}
	}
	if err := service.Locks.Release(ctx, key); err != nil {
		return false, err
	}
	return true, nil
}

func (service Service) create(ctx context.Context, request normalizedRequest, taskType string) (tasks.Record, error) {
	now := service.now()
	traceID := service.newID("trace-call-")
	return service.Tasks.Create(ctx, tasks.CreateRequest{
		TaskID:    service.newID("task-call-"),
		Source:    request.Source,
		Target:    tasks.Target{AgentID: request.AgentID, DeviceID: request.DeviceID},
		TaskType:  taskType,
		Payload:   request.payload(),
		CreatedAt: now,
		TraceID:   &traceID,
	})
}

func (request normalizedRequest) payload() map[string]any {
	payload := map[string]any{
		"conversation_id": request.ConversationID,
		"session_id":      request.ConversationID,
		"sender_id":       request.SenderID,
		"username":        request.Receiver,
		"receiver":        request.Receiver,
		"receiver_name":   request.ReceiverName,
		"call_type":       request.CallType,
		"queue":           "fast",
	}
	if request.Aliases != "" {
		payload["aliases"] = request.Aliases
	}
	return payload
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

func callAccountScope(snapshot incomingmodel.ConversationSnapshot, deviceID string) string {
	tenantID := strings.ToLower(text(snapshot.TenantID))
	weworkUserID := strings.ToLower(text(snapshot.WeWorkUserID))
	if tenantID != "" && weworkUserID != "" {
		return "wework:" + tenantID + ":" + weworkUserID
	}
	return "device:" + strings.ToLower(text(deviceID))
}

func (service Service) lockKey(accountScope string) string {
	prefix := text(service.CachePrefix)
	if prefix == "" {
		prefix = "wework"
	}
	sum := sha1.Sum([]byte(text(accountScope)))
	return prefix + ":call:account:" + hex.EncodeToString(sum[:])
}

func (service Service) lockTTL() time.Duration {
	ttl := service.LockTTL
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	if ttl < 300*time.Second {
		return 300 * time.Second
	}
	return ttl
}

func sameReservation(lock Lock, ok bool, reservationID string) bool {
	return ok && text(lock.ReservationID) != "" && text(lock.ReservationID) == text(reservationID)
}

func unixSeconds(value time.Time) float64 {
	return float64(value.UnixNano()) / float64(time.Second)
}

func invalid(message string) error {
	return errors.Join(ErrInvalidRequest, errors.New(message))
}

func text(value string) string {
	return strings.TrimSpace(value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if normalized := text(value); normalized != "" {
			return normalized
		}
	}
	return ""
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

func (service Service) recordAudit(ctx context.Context, request normalizedRequest, action string) {
	if service.AuditLogs == nil {
		return
	}
	callType := request.CallType
	if callType == "" {
		callType = "voice"
	}
	detail := fmt.Sprintf("会话发起%s通话: conversation_id=%s, receiver=%s", callType, request.ConversationID, firstNonBlank(request.Receiver, "-"))
	if action == "hangup" {
		detail = fmt.Sprintf("会话挂断通话: conversation_id=%s, receiver=%s", request.ConversationID, firstNonBlank(request.Receiver, "-"))
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "call",
		Detail:     detail,
	})
}

type memoryLock struct {
	lock      Lock
	expiresAt time.Time
}

type MemoryLockStore struct {
	mu    sync.Mutex
	locks map[string]memoryLock
}

func NewMemoryLockStore() *MemoryLockStore {
	return &MemoryLockStore{locks: map[string]memoryLock{}}
}

func (store *MemoryLockStore) Reserve(_ context.Context, key string, lock Lock, ttl time.Duration) (bool, error) {
	if store == nil {
		return false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpiredLocked(key, time.Now().UTC())
	if _, ok := store.locks[key]; ok {
		return false, nil
	}
	store.setLocked(key, lock, ttl)
	return true, nil
}

func (store *MemoryLockStore) Read(_ context.Context, key string) (Lock, bool, error) {
	if store == nil {
		return Lock{}, false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpiredLocked(key, time.Now().UTC())
	current, ok := store.locks[key]
	return current.lock, ok, nil
}

func (store *MemoryLockStore) Refresh(_ context.Context, key string, lock Lock, ttl time.Duration) error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.setLocked(key, lock, ttl)
	return nil
}

func (store *MemoryLockStore) Release(_ context.Context, key string) error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.locks, key)
	return nil
}

func (store *MemoryLockStore) setLocked(key string, lock Lock, ttl time.Duration) {
	if store.locks == nil {
		store.locks = map[string]memoryLock{}
	}
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	store.locks[key] = memoryLock{lock: lock, expiresAt: time.Now().UTC().Add(ttl)}
}

func (store *MemoryLockStore) pruneExpiredLocked(key string, now time.Time) {
	current, ok := store.locks[key]
	if ok && !current.expiresAt.After(now) {
		delete(store.locks, key)
	}
}
