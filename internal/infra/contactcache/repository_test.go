package contactcache

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRepositoryReadsExternalContact(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{{
			" ent-1 ", " wm-1 ", " Alice ", " avatar ", int64(1), []byte("2"), " union ", " CEO ",
			" Corp ", " Corp Full ", []byte(`{"wechat_channels":{"nickname":"chan"}}`),
			[]byte(`[{"userid":"zhangsan"}]`), []byte(`[{"tag_id":"tag-1"}]`),
			"scan", "2026-07-01 08:00:00", "2026-07-01 08:01:00", int64(1), "sync",
			"2026-07-01 08:00:00", "2026-07-01 09:00:00",
		}}},
	}}
	repository := &Repository{DB: db}

	payload, ok, err := repository.GetExternalContact(context.Background(), " ent-1 ", " wm-1 ")
	if err != nil {
		t.Fatalf("GetExternalContact returned error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false")
	}
	if payload["enterprise_id"] != "ent-1" || payload["external_userid"] != "wm-1" || payload["type"] != 1 || payload["gender"] != 2 || payload["stale"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	profile := payload["external_profile_json"].(map[string]any)
	if profile["wechat_channels"].(map[string]any)["nickname"] != "chan" {
		t.Fatalf("external_profile_json = %#v", profile)
	}
	followUsers := payload["follow_users_json"].([]any)
	if followUsers[0].(map[string]any)["userid"] != "zhangsan" {
		t.Fatalf("follow_users_json = %#v", followUsers)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "FROM wework_external_contacts") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][0] != "ent-1" || db.args[1][1] != "wm-1" {
		t.Fatalf("args = %#v", db.args[1])
	}
}

func TestRepositoryUpsertsExternalContact(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{{}}}
	repository := &Repository{
		DB:      db,
		Dialect: "mysql",
		Now:     func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	err := repository.UpsertExternalContact(context.Background(), contactsPayload(map[string]any{
		"enterprise_id":   " ent-1 ",
		"external_userid": " wm-1 ",
		"name":            " Alice ",
		"avatar":          " avatar ",
		"type":            1,
		"gender":          "2",
		"external_profile_json": map[string]any{
			"wechat_channels": map[string]any{"nickname": "chan"},
		},
		"follow_users_json": []any{map[string]any{"userid": "zhangsan", "remark": "Alice"}},
		"tags_json":         []any{map[string]any{"tag_id": "tag-1"}},
		"source":            "callback_edit_external_contact",
	}))
	if err != nil {
		t.Fatalf("UpsertExternalContact returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "ent-1" || args[1] != "wm-1" || args[2] != "Alice" || args[5] != 2 || args[16] != 0 {
		t.Fatalf("args = %#v", args)
	}
	if !strings.Contains(args[10].(string), `"wechat_channels"`) || !strings.Contains(args[11].(string), `"userid":"zhangsan"`) {
		t.Fatalf("json args = %#v / %#v", args[10], args[11])
	}
}

func TestRepositoryListsStaleExternalContacts(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{
			{
				"ent-1", "wm-old", "Old", "", int64(1), int64(0), "", "", "", "",
				[]byte(`{}`), []byte(`[]`), []byte(`[]`), "", nil,
				"2026-07-01T09:00:00Z", int64(0), "sync", "2026-07-01T08:00:00Z", "2026-07-01T09:00:00Z",
			},
			{
				"ent-1", "wm-fresh", "Fresh", "", int64(1), int64(0), "", "", "", "",
				[]byte(`{}`), []byte(`[]`), []byte(`[]`), "", nil,
				"2026-07-02T09:00:00Z", int64(0), "sync", "2026-07-02T08:00:00Z", "2026-07-02T09:00:00Z",
			},
			{
				"ent-1", "wm-stale", "Stale", "", int64(1), int64(0), "", "", "", "",
				[]byte(`{}`), []byte(`[]`), []byte(`[]`), "", nil,
				"2026-07-02T09:00:00Z", int64(1), "sync", "2026-07-02T08:00:00Z", "2026-07-02T09:00:00Z",
			},
		}},
	}}
	repository := &Repository{
		DB:  db,
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payloads, err := repository.ListStaleExternalContacts(context.Background(), " ent-1 ", 2, 24)
	if err != nil {
		t.Fatalf("ListStaleExternalContacts returned error: %v", err)
	}
	if len(payloads) != 2 || payloads[0]["external_userid"] != "wm-old" || payloads[1]["external_userid"] != "wm-stale" {
		t.Fatalf("payloads = %#v", payloads)
	}
	if got := db.args[1]; got[0] != "ent-1" || got[1] != "ent-1" || got[2] != 8 {
		t.Fatalf("args = %#v", got)
	}
}

func TestRepositoryMarksExternalRefreshSkipped(t *testing.T) {
	db := &fakeContactDB{
		rows:    []*fakeContactRows{{}},
		results: []sql.Result{fakeSQLResult{rowsAffected: 1}},
	}
	repository := &Repository{
		DB:  db,
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	ok, err := repository.MarkExternalContactRefreshSkipped(context.Background(), " ent-1 ", " wm-1 ", " stale_refresh_skipped ")
	if err != nil {
		t.Fatalf("MarkExternalContactRefreshSkipped returned error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false")
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "UPDATE wework_external_contacts") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[2] != "stale_refresh_skipped" || args[3] != "ent-1" || args[4] != "wm-1" {
		t.Fatalf("args = %#v", args)
	}
}

func TestRepositoryReadsCorpUser(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{{
			" ent-1 ", " zhangsan ", " 张三 ", []byte(`[1,2]`), "dev", "1380000", int64(1),
			"a@example.com", "biz@example.com", "avatar", []byte("4"), []byte(`{"attrs":[{"name":"role"}]}`),
			"2026-07-01 08:01:00", []byte("0"), "sync", "2026-07-01 08:00:00", "2026-07-01 09:00:00",
		}}},
	}}
	repository := &Repository{DB: db}

	payload, ok, err := repository.GetCorpUser(context.Background(), "ent-1", "zhangsan")
	if err != nil {
		t.Fatalf("GetCorpUser returned error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false")
	}
	if payload["enterprise_id"] != "ent-1" || payload["userid"] != "zhangsan" || payload["status"] != 4 || payload["stale"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	departments := payload["department_json"].([]any)
	if departments[0].(float64) != 1 || departments[1].(float64) != 2 {
		t.Fatalf("department_json = %#v", departments)
	}
	extattr := payload["extattr_json"].(map[string]any)
	if extattr["attrs"].([]any)[0].(map[string]any)["name"] != "role" {
		t.Fatalf("extattr_json = %#v", extattr)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "FROM wework_corp_users") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][0] != "ent-1" || db.args[1][1] != "zhangsan" {
		t.Fatalf("args = %#v", db.args[1])
	}
}

func TestRepositoryUpsertsCorpUser(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{{}}}
	repository := &Repository{
		DB:      db,
		Dialect: "mysql",
		Now:     func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	err := repository.UpsertCorpUser(context.Background(), contactsPayload(map[string]any{
		"enterprise_id":   " ent-1 ",
		"userid":          " zhangsan ",
		"name":            " 张三 ",
		"department_json": []any{float64(1), float64(2)},
		"position":        " dev ",
		"mobile":          " 1380000 ",
		"gender":          "1",
		"email":           " a@example.com ",
		"biz_mail":        " biz@example.com ",
		"avatar":          " avatar ",
		"status":          int64(4),
		"extattr_json":    map[string]any{"attrs": []any{map[string]any{"name": "role"}}},
		"source":          "full_sync",
	}))
	if err != nil {
		t.Fatalf("UpsertCorpUser returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") || !strings.Contains(db.execs[0].query, "wework_corp_users") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "ent-1" || args[1] != "zhangsan" || args[2] != "张三" || args[6] != 1 || args[10] != 4 || args[13] != 0 || args[14] != "full_sync" {
		t.Fatalf("args = %#v", args)
	}
	if !strings.Contains(args[3].(string), `1`) || !strings.Contains(args[11].(string), `"attrs"`) {
		t.Fatalf("json args = %#v / %#v", args[3], args[11])
	}
}

func TestRepositoryListsStaleCorpUsers(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{
			{
				"ent-1", "dy-old", "Old", []byte(`[1]`), "", "", int64(1), "", "", "",
				int64(1), []byte(`{}`), "2026-07-01T09:00:00Z", int64(0), "sync", "2026-07-01T08:00:00Z", "2026-07-01T09:00:00Z",
			},
			{
				"ent-1", "dy-fresh", "Fresh", []byte(`[1]`), "", "", int64(1), "", "", "",
				int64(1), []byte(`{}`), "2026-07-02T09:00:00Z", int64(0), "sync", "2026-07-02T08:00:00Z", "2026-07-02T09:00:00Z",
			},
			{
				"ent-1", "dy-stale", "Stale", []byte(`[1]`), "", "", int64(1), "", "", "",
				int64(1), []byte(`{}`), "2026-07-02T09:00:00Z", int64(1), "sync", "2026-07-02T08:00:00Z", "2026-07-02T09:00:00Z",
			},
		}},
	}}
	repository := &Repository{
		DB:  db,
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payloads, err := repository.ListStaleCorpUsers(context.Background(), "", 2, 24)
	if err != nil {
		t.Fatalf("ListStaleCorpUsers returned error: %v", err)
	}
	if len(payloads) != 2 || payloads[0]["userid"] != "dy-old" || payloads[1]["userid"] != "dy-stale" {
		t.Fatalf("payloads = %#v", payloads)
	}
	if got := db.args[1]; got[0] != "" || got[1] != "" || got[2] != 8 {
		t.Fatalf("args = %#v", got)
	}
}

func TestRepositoryListsInternalUserCandidatesByNames(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{
			{" ent-1 ", " zhangsan ", " 张三 ", []byte(`[1,2]`), "dev", "avatar-a", "2026-07-01 08:01:00", "2026-07-01 09:00:00"},
			{"ent-1", "zhangsan", "张三重复", []byte(`[3]`), "ops", "avatar-b", "2026-07-01 08:02:00", "2026-07-01 09:01:00"},
			{"ent-1", "lisi", "李四", []byte(`invalid-json`), "", "", "", ""},
		}},
	}}
	repository := &Repository{DB: db}

	candidates, err := repository.ListInternalUserCandidatesByNames(context.Background(), " ent-1 ", []string{" 张三 ", "张三", "李四"}, 100)
	if err != nil {
		t.Fatalf("ListInternalUserCandidatesByNames returned error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2: %#v", len(candidates), candidates)
	}
	if candidates[0].EnterpriseID != "ent-1" || candidates[0].UserID != "zhangsan" || candidates[0].Name != "张三" || candidates[0].Position != "dev" {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if len(candidates[0].DepartmentJSON) != 2 || candidates[1].DepartmentJSON == nil {
		t.Fatalf("department_json = %#v / %#v", candidates[0].DepartmentJSON, candidates[1].DepartmentJSON)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "AND stale = 0") || !strings.Contains(db.queries[1], "name IN (?, ?)") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if got := db.args[1]; len(got) != 4 || got[0] != "ent-1" || got[1] != "张三" || got[2] != "李四" || got[3] != 50 {
		t.Fatalf("args = %#v", got)
	}
}

func TestRepositoryGetsInternalUserByUserID(t *testing.T) {
	db := &fakeContactDB{rows: []*fakeContactRows{
		{},
		{values: [][]any{{
			" ent-1 ", " zhangsan ", " 张三 ", []byte(`[1,2]`), "dev", "avatar-a", "2026-07-01 08:01:00", "2026-07-01 09:00:00",
		}}},
	}}
	repository := &Repository{DB: db}

	candidate, found, err := repository.GetInternalUserByUserID(context.Background(), " ent-1 ", " zhangsan ")
	if err != nil {
		t.Fatalf("GetInternalUserByUserID returned error: %v", err)
	}
	if !found || candidate.EnterpriseID != "ent-1" || candidate.UserID != "zhangsan" || candidate.Name != "张三" || candidate.Avatar != "avatar-a" {
		t.Fatalf("candidate=%#v found=%t", candidate, found)
	}
	if len(candidate.DepartmentJSON) != 2 {
		t.Fatalf("department_json = %#v", candidate.DepartmentJSON)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "userid = ?") || !strings.Contains(db.queries[1], "AND stale = 0") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if got := db.args[1]; len(got) != 2 || got[0] != "ent-1" || got[1] != "zhangsan" {
		t.Fatalf("args = %#v", got)
	}
}

func TestRepositoryMissingRowsAndTablesReturnNotFound(t *testing.T) {
	repository := &Repository{DB: &fakeContactDB{rows: []*fakeContactRows{{}, {}}}}
	if _, ok, err := repository.GetExternalContact(context.Background(), "ent-1", "wm-missing"); err != nil || ok {
		t.Fatalf("missing row ok=%t err=%v", ok, err)
	}
	repository = &Repository{DB: &fakeContactDB{errors: []error{errors.New("no such table")}}}
	if _, ok, err := repository.GetCorpUser(context.Background(), "ent-1", "missing"); err != nil || ok {
		t.Fatalf("missing table ok=%t err=%v", ok, err)
	}
	candidates, err := repository.ListInternalUserCandidatesByNames(context.Background(), "ent-1", []string{"张三"}, 20)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("missing table candidates=%#v err=%v", candidates, err)
	}
}

type fakeContactDB struct {
	rows    []*fakeContactRows
	errors  []error
	results []sql.Result
	queries []string
	args    [][]any
	execs   []fakeContactExec
}

type fakeContactExec struct {
	query string
	args  []any
}

func (db *fakeContactDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	index := len(db.queries) - 1
	if index < len(db.errors) && db.errors[index] != nil {
		return nil, db.errors[index]
	}
	if index < len(db.rows) {
		return db.rows[index], nil
	}
	return &fakeContactRows{}, nil
}

func (db *fakeContactDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeContactExec{query: query, args: append([]any{}, args...)})
	index := len(db.execs) - 1
	if index < len(db.results) && db.results[index] != nil {
		return db.results[index], nil
	}
	return fakeSQLResult{}, nil
}

func contactsPayload(values map[string]any) map[string]any {
	return values
}

type fakeContactRows struct {
	values [][]any
	index  int
}

func (rows *fakeContactRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeContactRows) Scan(dest ...any) error {
	values := rows.values[rows.index-1]
	for index := range dest {
		ptr := dest[index].(*any)
		*ptr = values[index]
	}
	return nil
}

func (rows *fakeContactRows) Close() error {
	return nil
}

func (rows *fakeContactRows) Err() error {
	return nil
}

type fakeSQLResult struct {
	rowsAffected int64
}

func (result fakeSQLResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeSQLResult) RowsAffected() (int64, error) {
	return result.rowsAffected, nil
}
