package messages

import (
	"context"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceListBuildsLegacyEnvelope(t *testing.T) {
	messageID := int64(42)
	timestamp := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{page: Page{
		Records: []Record{{
			MessageID:      &messageID,
			TraceID:        "coze-auto-reply-babc123-seq2",
			ConversationID: "conv-001",
			SenderID:       "external-001",
			SenderName:     "客户一",
			Content:        "hello",
			MsgType:        "text",
			Direction:      "incoming",
			MessageOrigin:  "system_task",
			Timestamp:      timestamp,
			CreatedAt:      timestamp,
		}},
		Total:   2,
		HasMore: true,
	}}
	service := Service{Store: store}

	payload, err := service.List(context.Background(), Request{
		Session:        auth.Session{AssigneeID: "cs-001"},
		ConversationID: "conv-001",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if payload["total"] != 2 || payload["has_more"] != true || payload["first_cursor"] != "1782727200000:42:coze-auto-reply-babc123-seq2" {
		t.Fatalf("payload envelope = %+v", payload)
	}
	rows := payload["messages"].([]map[string]any)
	if rows[0]["source"] != "system" || rows[0]["sub_source"] != "coze_auto_reply" || rows[0]["coze_reply_order"] != 2 {
		t.Fatalf("serialized row = %+v", rows[0])
	}
	if store.query.ConversationID != "conv-001" || store.query.Limit != 20 {
		t.Fatalf("query = %+v", store.query)
	}
}

func TestServiceListUsesBeforeCursorBeforeAfterCursor(t *testing.T) {
	store := &fakeStore{}
	service := Service{Store: store}

	_, err := service.List(context.Background(), Request{
		ConversationID: "conv-001",
		Limit:          10,
		AfterCursor:    "1782698400000:1:after",
		BeforeCursor:   "1782698300000:2:before",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if store.query.Before == nil || store.query.Before.TraceID != "before" || store.query.After != nil {
		t.Fatalf("query = %+v", store.query)
	}
}

func TestServiceListRequiresStore(t *testing.T) {
	_, err := (Service{}).List(context.Background(), Request{ConversationID: "conv-001"})
	if err != ErrStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStoreUnavailable)
	}
}

type fakeStore struct {
	page  Page
	query Query
	err   error
}

func (store *fakeStore) List(ctx context.Context, query Query) (Page, error) {
	store.query = query
	if store.err != nil {
		return Page{}, store.err
	}
	return store.page, nil
}
