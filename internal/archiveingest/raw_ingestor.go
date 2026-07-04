package archiveingest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archiveraw"
	"wework-go/internal/outbox"
)

// RawStore is the archive_raw_messages boundary used by RawBatchIngestor.
type RawStore interface {
	UpsertRawMessage(ctx context.Context, input archiveraw.UpsertInput) (bool, *archiveraw.Record, error)
	MarkDecryptStarted(ctx context.Context, enterpriseID string, source string, archiveMsgID string, startedAt *time.Time) (*archiveraw.Record, error)
	MarkDecryptFinished(ctx context.Context, enterpriseID string, source string, archiveMsgID string, finishedAt *time.Time) (*archiveraw.Record, error)
}

// MediaTaskStore is the archive_media_tasks enqueue boundary used by RawBatchIngestor.
type MediaTaskStore interface {
	EnqueueMany(ctx context.Context, inputs []archivemediatask.EnqueueInput) ([]archivemediatask.EnqueueResult, error)
}

// MessageStore is the durable messages/conversations write boundary.
type MessageStore interface {
	AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error)
}

// OutboxEnqueuer is the durable outbox append boundary.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// RawBatchIngestor persists staged archive batches into archive_raw_messages.
type RawBatchIngestor struct {
	Raw        RawStore
	MediaTasks MediaTaskStore
	Messages   MessageStore
	Outbox     OutboxEnqueuer
	Now        func() time.Time
}

// IngestArchiveBatch mirrors the raw archive write portion of Python ingest_batch.
func (ingestor RawBatchIngestor) IngestArchiveBatch(ctx context.Context, request BatchRequest) (Result, error) {
	if ingestor.Raw == nil {
		return Result{}, fmt.Errorf("archive raw store is not configured")
	}
	enterpriseID := normalizeEnterpriseID(request.EnterpriseID)
	source := normalizeSource(request.Source)
	startedAt := ingestor.now()
	mediaTasks := []archivemediatask.EnqueueInput{}
	outboxEvents := []outbox.EventEnvelope{}
	inserted := 0
	deduplicated := 0
	conversationIDs := map[string]struct{}{}
	for index, message := range request.Messages {
		archiveMessage, err := NormalizeMessagePayload(enterpriseID, source, message, startedAt)
		if err != nil {
			return Result{}, fmt.Errorf("archive message %d: %w", index, err)
		}
		input := rawInputFromArchiveMessage(archiveMessage)
		msgID := archiveMessage.ArchiveMsgID
		input.SkipRecordReload = true
		if _, _, err := ingestor.Raw.UpsertRawMessage(ctx, input); err != nil {
			return Result{}, err
		}
		if _, err := ingestor.Raw.MarkDecryptStarted(ctx, enterpriseID, source, msgID, &startedAt); err != nil {
			return Result{}, err
		}
		if ingestor.Messages != nil {
			storedMessage := incomingmodel.NormalizeIncomingMessage(incomingMessageFromArchive(archiveMessage), 0, archiveMessage.Timestamp)
			messageCreated, conversation, err := ingestor.Messages.AddIncomingMessage(ctx, storedMessage)
			if err != nil {
				return Result{}, err
			}
			if _, err := ingestor.Raw.MarkDecryptFinished(ctx, enterpriseID, source, msgID, &startedAt); err != nil {
				return Result{}, err
			}
			if messageCreated {
				inserted++
			} else {
				deduplicated++
			}
			if conversationID := strings.TrimSpace(conversation.ConversationID); conversationID != "" {
				conversationIDs[conversationID] = struct{}{}
			}
			outboxEvents = append(outboxEvents, archiveOutboxEventsForMessage(archiveMessage, storedMessage, conversation, messageCreated, startedAt)...)
		}
		if input.SDKFileID != "" && ingestor.MediaTasks != nil {
			payloadJSON, err := jsonString(input.RawJSON)
			if err != nil {
				return Result{}, err
			}
			mediaTasks = append(mediaTasks, archivemediatask.EnqueueInput{
				EnterpriseID: enterpriseID,
				Source:       source,
				ArchiveMsgID: msgID,
				SDKFileID:    input.SDKFileID,
				PayloadJSON:  payloadJSON,
			})
		}
	}
	if len(outboxEvents) > 0 && ingestor.Outbox != nil {
		if _, err := ingestor.Outbox.EnqueueMany(ctx, outboxEvents); err != nil {
			return Result{}, err
		}
	}
	if len(mediaTasks) > 0 {
		if _, err := ingestor.MediaTasks.EnqueueMany(ctx, mediaTasks); err != nil {
			return Result{}, err
		}
	}
	return Result{
		EnterpriseID:    enterpriseID,
		Source:          source,
		Total:           len(request.Messages),
		Inserted:        inserted,
		Deduplicated:    deduplicated,
		Cursor:          request.Cursor,
		ConversationIDs: sortedKeys(conversationIDs),
	}, nil
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func jsonString(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func (ingestor RawBatchIngestor) now() time.Time {
	if ingestor.Now == nil {
		return time.Now().UTC()
	}
	return ingestor.Now().UTC()
}

func rawInputFromArchiveMessage(archiveMessage ArchiveMessage) archiveraw.UpsertInput {
	return archiveraw.UpsertInput{
		EnterpriseID:     archiveMessage.EnterpriseID,
		Source:           archiveMessage.Source,
		ArchiveMsgID:     archiveMessage.ArchiveMsgID,
		Seq:              archiveMessage.Seq,
		Action:           archiveMessage.Action,
		FromID:           archiveMessage.FromID,
		ToList:           archiveMessage.ToList,
		RoomID:           firstTextValue(archiveMessage.RoomID, archiveMessage.ConversationName),
		MsgTypeRaw:       archiveMessage.MsgTypeRaw,
		SDKFileID:        archiveMessage.SDKFileID,
		RawJSON:          archiveMessage.RawJSON,
		SkipRecordReload: true,
	}
}

func incomingMessageFromArchive(message ArchiveMessage) incomingmodel.IncomingMessage {
	deviceID := defaultTextValue(message.DeviceID, "enterprise:"+message.EnterpriseID)
	weworkUserID := extractArchiveWeWorkUserID(deviceID)
	senderID := archiveConversationSenderID(message)
	conversationName := firstTextValue(message.ConversationName, message.RoomID, senderID)
	return incomingmodel.IncomingMessage{
		TenantID:         message.EnterpriseID,
		ArchiveMsgID:     message.ArchiveMsgID,
		WeWorkUserID:     weworkUserID,
		ExternalUserID:   archiveExternalUserID(message, senderID),
		RoomID:           message.RoomID,
		ConversationType: archiveConversationType(message.RoomID),
		DeviceID:         deviceID,
		SenderID:         senderID,
		SenderName:       defaultTextValue(message.SenderName, senderID),
		SenderAvatar:     message.SenderAvatar,
		SenderRemark:     message.SenderRemark,
		Content:          message.Content,
		MsgType:          message.MsgType,
		ConversationName: conversationName,
		Timestamp:        message.Timestamp,
		TraceID:          message.TraceID,
		MessageOrigin:    "archive_history",
		Direction:        message.Direction,
	}
}

func archiveConversationSenderID(message ArchiveMessage) string {
	if message.ExternalUserID != "" {
		return message.ExternalUserID
	}
	if strings.EqualFold(message.Direction, incomingmodel.DirectionOutgoing) {
		for _, item := range message.ToList {
			if looksExternalUserID(item) {
				return item
			}
		}
	}
	return message.SenderID
}

func archiveExternalUserID(message ArchiveMessage, senderID string) string {
	if message.ExternalUserID != "" {
		return message.ExternalUserID
	}
	if looksExternalUserID(senderID) {
		return senderID
	}
	for _, item := range message.ToList {
		if looksExternalUserID(item) {
			return item
		}
	}
	return ""
}

func archiveConversationType(roomID string) string {
	if strings.TrimSpace(roomID) != "" {
		return "room"
	}
	return "single"
}

func extractArchiveWeWorkUserID(deviceID string) string {
	scope, raw, ok := strings.Cut(strings.TrimSpace(deviceID), ":")
	if !ok {
		return ""
	}
	if scope != "archive_user" && scope != "wework" {
		return ""
	}
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "-", "")
}

func archiveOutboxEventsForMessage(message ArchiveMessage, stored incomingmodel.IncomingMessage, conversation incomingmodel.ConversationSnapshot, messageCreated bool, occurredAt time.Time) []outbox.EventEnvelope {
	if !messageCreated {
		return nil
	}
	payload := BuildArchiveMessageOutboxPayload(message, stored, conversation, messageCreated)
	if strings.EqualFold(stored.Direction, incomingmodel.DirectionIncoming) {
		return []outbox.EventEnvelope{
			BuildArchiveConversationMessageReceivedEvent(message.EnterpriseID, stored.TraceID, payload, occurredAt),
		}
	}
	return []outbox.EventEnvelope{
		BuildArchiveMessageIngestedEvent(message.EnterpriseID, stored.TraceID, payload, occurredAt),
	}
}

func looksExternalUserID(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(text, "wo") || strings.HasPrefix(text, "wm") || strings.HasPrefix(text, "external_")
}

func fallbackRawJSON(message map[string]any, archiveMsgID string) map[string]any {
	return map[string]any{
		"archive_msgid":     archiveMsgID,
		"timestamp":         textValue(message["timestamp"]),
		"trace_id":          textValue(message["trace_id"]),
		"device_id":         textValue(message["device_id"]),
		"sender_id":         textValue(message["sender_id"]),
		"sender_name":       textValue(message["sender_name"]),
		"conversation_name": textValue(message["conversation_name"]),
		"content":           textValue(message["content"]),
		"msg_type":          textValue(message["msg_type"]),
		"direction":         textValue(message["direction"]),
	}
}

func objectMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return cloneMap(typed)
	case map[string]string:
		output := make(map[string]any, len(typed))
		for key, item := range typed {
			output[key] = item
		}
		return output
	case json.RawMessage:
		var output map[string]any
		if err := json.Unmarshal(typed, &output); err == nil {
			return output
		}
	case []byte:
		var output map[string]any
		if err := json.Unmarshal(typed, &output); err == nil {
			return output
		}
	case string:
		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &output); err == nil {
			return output
		}
	}
	return nil
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case []string:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	case []any:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := textValue(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case map[string]any:
		output := []string{}
		for _, item := range typed {
			if text := textValue(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	}
	return []string{}
}

func firstTextValue(values ...any) string {
	for _, value := range values {
		if text := textValue(value); text != "" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case sql.NullString:
		if !typed.Valid {
			return ""
		}
		return strings.TrimSpace(typed.String)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case []byte:
		return parseInt64String(string(typed))
	case string:
		return parseInt64String(typed)
	default:
		return 0
	}
}

func parseInt64String(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
