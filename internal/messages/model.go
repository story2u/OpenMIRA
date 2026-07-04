package messages

import (
	"context"
	"strings"
	"time"
)

// Store is the read model needed by the conversation message service.
type Store interface {
	List(ctx context.Context, query Query) (Page, error)
}

// Query describes the bounded message page read shape.
type Query struct {
	ConversationID string
	Limit          int
	Offset         int
	After          *Cursor
	Before         *Cursor
}

// Cursor is the legacy timestamp/message/trace keyset cursor.
type Cursor struct {
	Timestamp time.Time
	MessageID *int64
	TraceID   string
	Raw       string
}

// Page is the storage-neutral result for one conversation message page.
type Page struct {
	Records []Record
	Total   int
	HasMore bool
}

// Record mirrors the base messages table plus message_revoke_states overlay.
type Record struct {
	MessageID                   *int64
	TraceID                     string
	ArchiveMsgID                string
	TenantID                    string
	ConversationID              string
	DeviceID                    string
	SenderID                    string
	SenderName                  string
	SenderAvatar                string
	SenderRemark                string
	Content                     string
	MsgType                     string
	Direction                   string
	MessageOrigin               string
	TaskID                      string
	SendStatus                  string
	SendError                   string
	RevokeStatus                string
	RevokeTaskID                string
	RevokeError                 string
	RevokedAt                   *time.Time
	Timestamp                   time.Time
	CreatedAt                   time.Time
	DisplayName                 string
	AvatarURL                   string
	ArchiveSeq                  *int64
	ArchiveMsgtime              *int64
	ArchiveTypeRaw              string
	MediaURL                    string
	MediaReady                  bool
	MediaStatus                 string
	MediaTaskID                 string
	FileName                    string
	MediaFingerprint            string
	MediaSizeBytes              int64
	VoiceDurationSec            int
	VoiceText                   string
	VoiceTranscriptionStatus    string
	VoiceTranscriptionError     string
	VoiceTranscriptionExecuteID string
}

// Normalized clamps query values before a store uses them.
func (query Query) Normalized() Query {
	query.ConversationID = strings.TrimSpace(query.ConversationID)
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return query
}
