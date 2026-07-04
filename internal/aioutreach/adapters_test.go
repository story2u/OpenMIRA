package aioutreach

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/messages"
	"wework-go/internal/workbench"
)

func TestProjectionConversationStoreLoadsProjectionRow(t *testing.T) {
	projection := &fakeProjectionStore{rows: []workbench.ProjectionRow{{
		"conversation_id":   "conv-1",
		"tenant_id":         "ent-a",
		"wework_user_id":    "DY-1801",
		"sender_id":         "external-1",
		"sender_name":       "Customer A",
		"conversation_name": "Customer A Chat",
	}}}

	conversation, ok, err := (ProjectionConversationStore{Projection: projection}).GetConversation(context.Background(), " conv-1 ")
	if err != nil {
		t.Fatalf("GetConversation returned error: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false")
	}
	if len(projection.query.ConversationIDs) != 1 || projection.query.ConversationIDs[0] != "conv-1" || projection.query.Limit != 1 {
		t.Fatalf("query = %#v", projection.query)
	}
	if conversation.ConversationID != "conv-1" || conversation.TenantID != "ent-a" || conversation.WeWorkUserID != "DY-1801" || conversation.SenderID != "external-1" || conversation.SenderName != "Customer A" || conversation.ConversationName != "Customer A Chat" {
		t.Fatalf("conversation = %#v", conversation)
	}
}

func TestProjectionConversationStoreReturnsNotFoundForBlankOrEmptyRows(t *testing.T) {
	projection := &fakeProjectionStore{}

	_, ok, err := (ProjectionConversationStore{Projection: projection}).GetConversation(context.Background(), " ")
	if err != nil {
		t.Fatalf("blank GetConversation returned error: %v", err)
	}
	if ok || projection.called {
		t.Fatalf("blank lookup ok=%t called=%t", ok, projection.called)
	}
	_, ok, err = (ProjectionConversationStore{Projection: projection}).GetConversation(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("empty GetConversation returned error: %v", err)
	}
	if ok {
		t.Fatalf("empty rows ok = true")
	}
}

func TestMessageListStoreUsesMessagesQuery(t *testing.T) {
	messageStore := &fakeMessagesStore{page: messages.Page{Records: []messages.Record{{TraceID: "trace-1"}}}}

	records, err := (MessageListStore{Store: messageStore}).ListLatestMessages(context.Background(), " conv-1 ", 999)
	if err != nil {
		t.Fatalf("ListLatestMessages returned error: %v", err)
	}
	if messageStore.query.ConversationID != "conv-1" || messageStore.query.Limit != maxConversationLimit {
		t.Fatalf("query = %#v", messageStore.query)
	}
	if len(records) != 1 || records[0].TraceID != "trace-1" {
		t.Fatalf("records = %#v", records)
	}
}

func TestEnterpriseCorpStoreMapsArchivePullEnterprise(t *testing.T) {
	enterprises := &fakeArchivePullEnterpriseStore{record: &enterprisestore.ArchivePullEnterprise{EnterpriseID: "ent-a", CorpID: " corp-a "}}

	corpID, ok, err := (EnterpriseCorpStore{Store: enterprises}).GetCorpID(context.Background(), " ent-a ")
	if err != nil {
		t.Fatalf("GetCorpID returned error: %v", err)
	}
	if !ok || corpID != "corp-a" || enterprises.enterpriseID != "ent-a" {
		t.Fatalf("corpID=%q ok=%t enterpriseID=%q", corpID, ok, enterprises.enterpriseID)
	}
}

func TestEnterpriseCorpStoreReturnsErrorsAndMissingRows(t *testing.T) {
	expected := errors.New("db down")
	_, ok, err := (EnterpriseCorpStore{Store: &fakeArchivePullEnterpriseStore{err: expected}}).GetCorpID(context.Background(), "ent-a")
	if !errors.Is(err, expected) || ok {
		t.Fatalf("error=%v ok=%t", err, ok)
	}
	corpID, ok, err := (EnterpriseCorpStore{Store: &fakeArchivePullEnterpriseStore{}}).GetCorpID(context.Background(), "ent-a")
	if err != nil || ok || corpID != "" {
		t.Fatalf("corpID=%q ok=%t error=%v", corpID, ok, err)
	}
}

type fakeProjectionStore struct {
	rows   []workbench.ProjectionRow
	query  workbench.ProjectionQuery
	called bool
	err    error
}

func (store *fakeProjectionStore) ListRows(ctx context.Context, query workbench.ProjectionQuery) ([]workbench.ProjectionRow, error) {
	store.called = true
	store.query = query
	if store.err != nil {
		return nil, store.err
	}
	return append([]workbench.ProjectionRow(nil), store.rows...), nil
}

type fakeMessagesStore struct {
	page  messages.Page
	query messages.Query
	err   error
}

func (store *fakeMessagesStore) List(ctx context.Context, query messages.Query) (messages.Page, error) {
	store.query = query
	if store.err != nil {
		return messages.Page{}, store.err
	}
	return store.page, nil
}

type fakeArchivePullEnterpriseStore struct {
	record       *enterprisestore.ArchivePullEnterprise
	enterpriseID string
	err          error
}

func (store *fakeArchivePullEnterpriseStore) GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error) {
	store.enterpriseID = enterpriseID
	if store.err != nil {
		return nil, store.err
	}
	return store.record, nil
}
