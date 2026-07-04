package platformproxyfacts

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/platformproxy"
	"wework-go/internal/sendtarget"
)

func TestResolverResolveSendTargetReadsProjection(t *testing.T) {
	db := &fakeDB{
		rows: &fakeRows{
			columns: []string{"conversation_id", "sender_id", "sender_remark", "sender_name", "account_wework_user_id"},
			values:  [][]any{{"conv-1", "external-1", "VIP 客户", "客户昵称", "dy-1"}},
		},
	}
	resolver := NewResolver(db)

	target, err := resolver.ResolveSendTarget(context.Background(), platformproxy.SendTargetRequest{
		ConversationID:   "conv-1",
		FallbackReceiver: "旧名字",
		FallbackAliases:  "旧别名",
		FallbackSenderID: "external-1",
	})
	if err != nil {
		t.Fatalf("ResolveSendTarget returned error: %v", err)
	}
	if target.Receiver != "VIP 客户" || target.Aliases != "" || target.ConversationID != "conv-1" || target.SenderID != "external-1" {
		t.Fatalf("target = %+v, want projection receiver without scoped aliases", target)
	}
	if !strings.Contains(db.query, "conversation_overview_projection") || db.args[0] != "conv-1" {
		t.Fatalf("query = %q args=%v, want projection lookup", db.query, db.args)
	}
}

func TestResolverResolveSendTargetFallsBackWithoutConversation(t *testing.T) {
	db := &fakeDB{}
	resolver := NewResolver(db)

	target, err := resolver.ResolveSendTarget(context.Background(), platformproxy.SendTargetRequest{
		FallbackReceiver: "客户A",
		FallbackAliases:  "客户别名",
		FallbackSenderID: "external-1",
	})
	if err != nil {
		t.Fatalf("ResolveSendTarget returned error: %v", err)
	}
	if target.Receiver != "客户A" || target.Aliases != "客户别名" || target.SenderID != "external-1" {
		t.Fatalf("target = %+v, want fallback target", target)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no DB read without conversation_id", db.query)
	}
}

func TestResolverResolveSendTargetRefreshesStaleScopedProfile(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rows: &fakeRows{
			columns: []string{"conversation_id", "sender_id", "sender_remark", "sender_name", "account_wework_user_id", "identity_profile_verified_source", "identity_profile_verified_at"},
			values:  [][]any{{"conv-1", "wm-external-1", "旧备注", "旧昵称", "dy-1", "manual_edit", now.Add(-48 * time.Hour).Format(time.RFC3339)}},
		},
	}
	profiles := &fakeContactProfileResolver{payload: map[string]any{
		"conversation_id": "conv-1",
		"sender_id":       "wm-external-1",
		"profile": map[string]any{
			"sender_remark": "新备注",
			"sender_name":   "新昵称",
		},
	}}
	resolver := NewResolver(db)
	resolver.ContactProfiles = profiles
	resolver.Now = func() time.Time { return now }

	target, err := resolver.ResolveSendTarget(context.Background(), platformproxy.SendTargetRequest{
		ConversationID:   "conv-1",
		FallbackReceiver: "旧名字",
		FallbackSenderID: "wm-external-1",
	})
	if err != nil {
		t.Fatalf("ResolveSendTarget returned error: %v", err)
	}
	if profiles.conversationID != "conv-1" {
		t.Fatalf("profile resolver conversation_id = %q", profiles.conversationID)
	}
	if target.Receiver != "新备注" || target.SenderName != "新昵称" || target.ContactProfileUpdate["conversation_id"] != "conv-1" {
		t.Fatalf("target = %+v, want refreshed contact profile target", target)
	}
}

func TestResolverResolveSendTargetSkipsFreshScopedProfileRefresh(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rows: &fakeRows{
			columns: []string{"conversation_id", "sender_id", "sender_remark", "sender_name", "account_wework_user_id", "identity_profile_verified_source", "identity_profile_verified_at"},
			values:  [][]any{{"conv-1", "wm-external-1", "当前备注", "当前昵称", "dy-1", "contact_profile_resolve", now.Add(-time.Hour).Format(time.RFC3339)}},
		},
	}
	profiles := &fakeContactProfileResolver{payload: map[string]any{"conversation_id": "conv-1"}}
	resolver := NewResolver(db)
	resolver.ContactProfiles = profiles
	resolver.Now = func() time.Time { return now }

	target, err := resolver.ResolveSendTarget(context.Background(), platformproxy.SendTargetRequest{
		ConversationID:   "conv-1",
		FallbackReceiver: "旧名字",
		FallbackSenderID: "wm-external-1",
	})
	if err != nil {
		t.Fatalf("ResolveSendTarget returned error: %v", err)
	}
	if profiles.conversationID != "" {
		t.Fatalf("profile resolver should not be called for fresh profile, got %q", profiles.conversationID)
	}
	if target.Receiver != "当前备注" || len(target.ContactProfileUpdate) != 0 {
		t.Fatalf("target = %+v, want projection target without refresh payload", target)
	}
}

func TestResolverResolveSendTargetReturnsConflictWhenStaleRefreshFails(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rows: &fakeRows{
			columns: []string{"conversation_id", "sender_id", "sender_remark", "sender_name", "account_wework_user_id", "identity_profile_verified_source", "identity_profile_verified_at"},
			values:  [][]any{{"conv-1", "wm-external-1", "旧备注", "旧昵称", "dy-1", "manual_edit", now.Add(-48 * time.Hour).Format(time.RFC3339)}},
		},
	}
	wantErr := errors.New("remote lookup failed")
	resolver := NewResolver(db)
	resolver.ContactProfiles = &fakeContactProfileResolver{err: wantErr}
	resolver.Now = func() time.Time { return now }

	_, err := resolver.ResolveSendTarget(context.Background(), platformproxy.SendTargetRequest{
		ConversationID:   "conv-1",
		FallbackReceiver: "旧名字",
		FallbackSenderID: "wm-external-1",
	})
	var contactErr sendtarget.ContactIdentityError
	if !errors.As(err, &contactErr) || !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want contact identity wrapping refresh error", err)
	}
}

func TestResolverResolveSidebarEntityReadsLoginSession(t *testing.T) {
	db := &fakeDB{
		rows: &fakeRows{
			columns: []string{"device_id", "organization_name"},
			values:  [][]any{{"device-1", "黛伊科技"}},
		},
	}
	resolver := NewResolver(db)

	entity, err := resolver.ResolveSidebarEntity(context.Background(), platformproxy.SidebarEntityRequest{DeviceID: "sdk:device-1"})
	if err != nil {
		t.Fatalf("ResolveSidebarEntity returned error: %v", err)
	}
	if entity.Entity != "黛伊科技" || entity.RawDeviceID != "sdk:device-1" || entity.ResolvedDeviceID != "device-1" || entity.OrganizationNameSource != "login_service.organization_name" {
		t.Fatalf("entity = %+v, want login-session organization", entity)
	}
	if !strings.Contains(db.query, "wework_login_sessions") || db.args[0] != "device-1" {
		t.Fatalf("query = %q args=%v, want stripped sdk device lookup", db.query, db.args)
	}
}

func TestResolverResolveSidebarEntityPrefersRequestOrganization(t *testing.T) {
	db := &fakeDB{}
	resolver := NewResolver(db)

	entity, err := resolver.ResolveSidebarEntity(context.Background(), platformproxy.SidebarEntityRequest{
		DeviceID:         "device-1",
		OrganizationName: "会话企业",
	})
	if err != nil {
		t.Fatalf("ResolveSidebarEntity returned error: %v", err)
	}
	if entity.Entity != "会话企业" || entity.OrganizationNameSource != "conversation.organization_name" {
		t.Fatalf("entity = %+v, want explicit organization", entity)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no DB read for explicit organization", db.query)
	}
}

type fakeDB struct {
	query string
	args  []any
	rows  *fakeRows
	err   error
}

type fakeContactProfileResolver struct {
	conversationID string
	payload        map[string]any
	err            error
}

func (resolver *fakeContactProfileResolver) ResolveConversationContactProfile(_ context.Context, conversationID string) (map[string]any, error) {
	resolver.conversationID = conversationID
	if resolver.err != nil {
		return nil, resolver.err
	}
	return resolver.payload, nil
}

func (db *fakeDB) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if db.err != nil {
		return nil, db.err
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows.clone(), nil
}

type fakeRows struct {
	columns []string
	values  [][]any
	index   int
	err     error
}

func (rows *fakeRows) clone() *fakeRows {
	if rows == nil {
		return &fakeRows{}
	}
	return &fakeRows{columns: rows.columns, values: rows.values, err: rows.err, index: 0}
}

func (rows *fakeRows) Columns() ([]string, error) {
	return rows.columns, nil
}

func (rows *fakeRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.values) {
		return sql.ErrNoRows
	}
	values := rows.values[rows.index-1]
	for index := range dest {
		if index >= len(values) {
			break
		}
		pointer, ok := dest[index].(*any)
		if !ok {
			continue
		}
		*pointer = values[index]
	}
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}
