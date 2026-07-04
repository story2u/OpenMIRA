package contactidentitymaster

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/contactidentity"
)

func TestRepositoryUpsertFromContactProfileWritesMasterAndScopedIndex(t *testing.T) {
	db := &fakeIdentityDB{rows: []*fakeIdentityRows{
		{},
		{},
		{},
	}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	err := repository.UpsertFromContactProfile(context.Background(), contactidentity.ProfileUpsert{
		EnterpriseID:          "ent-1",
		SenderID:              "WMExternal123",
		SenderName:            "Deep Memory",
		SenderRemark:          "Scoped Remark",
		SenderAvatar:          "https://example.com/avatar.png",
		ScopeWeWorkUserID:     "WJ0011",
		ProfileVerifiedSource: "edit_external_contact_callback",
		ProfileVerifiedAt:     "2026-07-02T18:30:00+08:00",
	})
	if err != nil {
		t.Fatalf("UpsertFromContactProfile returned error: %v", err)
	}
	if len(db.queries) != 3 {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.execs) != 4 {
		t.Fatalf("execs = %#v", db.execs)
	}
	upsert := db.execs[0]
	if !strings.Contains(upsert.query, "INSERT INTO contact_identity_master") || !strings.Contains(upsert.query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("upsert SQL = %s", upsert.query)
	}
	if upsert.args[0] != "ent-1" || upsert.args[1] != "WMExternal123" || upsert.args[2] != "ready" {
		t.Fatalf("identity args = %#v", upsert.args[:5])
	}
	if upsert.args[3] != "Deep Memory" || upsert.args[4] != "" || upsert.args[5] != "Deep Memory" {
		t.Fatalf("display args = %#v", upsert.args[3:6])
	}
	if upsert.args[7] != "wework_contact_nickname" || upsert.args[8] != 1 || upsert.args[11] != 0 {
		t.Fatalf("state args = %#v", upsert.args[7:12])
	}
	extraJSON := upsert.args[13].(string)
	if !strings.Contains(extraJSON, `"scoped_profiles"`) || !strings.Contains(extraJSON, `"Scoped Remark"`) {
		t.Fatalf("extra_json = %s", extraJSON)
	}
	if !strings.Contains(db.execs[1].query, "DELETE FROM contact_identity_scoped_display_index") {
		t.Fatalf("delete SQL = %s", db.execs[1].query)
	}
	if db.execs[2].args[1] != "wj0011" || db.execs[2].args[4] != "Scoped Remark" {
		t.Fatalf("index args = %#v", db.execs[2].args)
	}
	if db.execs[3].args[4] != "scoped remark" {
		t.Fatalf("lowercase index args = %#v", db.execs[3].args)
	}
}

func TestRepositoryUsesPostgresJSONCast(t *testing.T) {
	db := &fakeIdentityDB{rows: []*fakeIdentityRows{{}, {}, {}}}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	err := repository.UpsertFromContactProfile(context.Background(), contactidentity.ProfileUpsert{
		EnterpriseID: "ent-1",
		SenderID:     "ext-1",
		SenderName:   "Nick",
	})
	if err != nil {
		t.Fatalf("UpsertFromContactProfile returned error: %v", err)
	}
	if !strings.Contains(db.execs[0].query, "?::jsonb") || !strings.Contains(db.execs[0].query, "ON CONFLICT(enterprise_id, sender_id)") {
		t.Fatalf("postgres upsert SQL = %s", db.execs[0].query)
	}
	if db.execs[0].args[14] != "2026-07-02T18:00:00+08:00" {
		t.Fatalf("postgres updated_at = %#v", db.execs[0].args[14])
	}
}

func TestRepositorySkipsWhenIdentityTableMissing(t *testing.T) {
	db := &fakeIdentityDB{errors: []error{sql.ErrNoRows}}
	repository := &Repository{DB: db}

	err := repository.UpsertFromContactProfile(context.Background(), contactidentity.ProfileUpsert{
		EnterpriseID: "ent-1",
		SenderID:     "ext-1",
		SenderName:   "Nick",
	})
	if err != nil {
		t.Fatalf("UpsertFromContactProfile returned error: %v", err)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestRepositoryResolveIdentityParsesExtraJSON(t *testing.T) {
	db := &fakeIdentityDB{rows: []*fakeIdentityRows{
		{},
		{values: [][]any{{
			"ent-1", "ext-1", "ready", "Nick", "", "Nick", "avatar", "wework_contact_nickname", int64(2),
			"2026-07-02 18:00:00", "2026-07-02 18:00:00", int64(0), nil,
			[]byte(`{"scoped_profiles":{"dy1":{"remark_name":"Scoped"}}}`),
		}}},
	}}
	repository := &Repository{DB: db}

	record, ok, err := repository.ResolveIdentity(context.Background(), "ent-1", "ext-1")
	if err != nil {
		t.Fatalf("ResolveIdentity returned error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if record.EnterpriseID != "ent-1" || record.SenderID != "ext-1" || record.SourceVersion != 2 || record.NeedsRefresh {
		t.Fatalf("record = %+v", record)
	}
	profile := contactidentity.ScopedProfile(record, "dy1")
	if profile["remark_name"] != "Scoped" {
		t.Fatalf("scoped profile = %+v", profile)
	}
}

func TestRepositoryMarksScopedRPASafeSearchName(t *testing.T) {
	db := &fakeIdentityDB{rows: []*fakeIdentityRows{
		{},
		{values: [][]any{{
			"ent-1", "ext-1", "partial", "", "", "Alice", "", "fallback", int64(2),
			nil, nil, int64(1), nil, []byte(`{}`),
		}}},
		{},
	}}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	err := repository.MarkScopedRPASafeSearchName(context.Background(), contactidentity.RPASafeMark{
		EnterpriseID:   "ent-1",
		SenderID:       "ext-1",
		WeWorkUserID:   "dy-1",
		BusinessRemark: "Alice",
		SafeSearchName: "Alice#QWE",
		SafeCode:       "QWE",
		SenderName:     "Alice",
	})
	if err != nil {
		t.Fatalf("MarkScopedRPASafeSearchName returned error: %v", err)
	}
	if len(db.execs) != 4 {
		t.Fatalf("execs = %#v", db.execs)
	}
	upsert := db.execs[0]
	if upsert.args[2] != "ready" || upsert.args[8] != 3 || upsert.args[11] != 0 {
		t.Fatalf("upsert state args = %#v", upsert.args)
	}
	extraJSON := upsert.args[13].(string)
	for _, fragment := range []string{`"rpa_safe_search_name":"Alice#QWE"`, `"rpa_safe_business_remark":"Alice"`, `"rpa_safe_name_status":"synced"`} {
		if !strings.Contains(extraJSON, fragment) {
			t.Fatalf("extra_json missing %s: %s", fragment, extraJSON)
		}
	}
	if db.execs[2].args[1] != "dy1" || db.execs[2].args[4] != "Alice#QWE" {
		t.Fatalf("index args = %#v", db.execs[2].args)
	}
}

func TestRepositoryDetectsScopedDisplayAmbiguity(t *testing.T) {
	db := &fakeIdentityDB{rows: []*fakeIdentityRows{
		{},
		{values: [][]any{{"ext-1"}, {"ext-2"}}},
	}}
	repository := &Repository{DB: db}

	ambiguous, err := repository.IsScopedDisplayAmbiguous(context.Background(), "ent-1", "dy-1", "Alice", "ext-1")
	if err != nil {
		t.Fatalf("IsScopedDisplayAmbiguous returned error: %v", err)
	}
	if !ambiguous {
		t.Fatal("ambiguous = false, want true")
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "FROM contact_identity_scoped_display_index") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][1] != "dy1" || db.args[1][2] == "" || db.args[1][3] != "Alice" {
		t.Fatalf("args = %#v", db.args[1])
	}
}

type fakeIdentityDB struct {
	queries []string
	args    [][]any
	execs   []fakeIdentityExec
	rows    []*fakeIdentityRows
	errors  []error
}

type fakeIdentityExec struct {
	query string
	args  []any
}

func (db *fakeIdentityDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, append([]any{}, args...))
	if len(db.errors) > 0 {
		err := db.errors[0]
		db.errors = db.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(db.rows) == 0 {
		return &fakeIdentityRows{}, nil
	}
	rows := db.rows[0]
	db.rows = db.rows[1:]
	return rows, nil
}

func (db *fakeIdentityDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeIdentityExec{query: query, args: append([]any{}, args...)})
	return fakeIdentityResult(1), nil
}

type fakeIdentityRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeIdentityRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeIdentityRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	values := rows.values[rows.index]
	rows.index++
	for index := range dest {
		if index >= len(values) {
			break
		}
		if pointer, ok := dest[index].(*any); ok {
			*pointer = values[index]
		}
	}
	return rows.err
}

func (rows *fakeIdentityRows) Close() error {
	return nil
}

func (rows *fakeIdentityRows) Err() error {
	return rows.err
}

type fakeIdentityResult int64

func (result fakeIdentityResult) LastInsertId() (int64, error) {
	return int64(result), nil
}

func (result fakeIdentityResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
