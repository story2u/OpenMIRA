// Package enterprisestore tests reads against the legacy enterprises table.
package enterprisestore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGetArchiveReconcileEnterpriseReadsLegacyColumns(t *testing.T) {
	db := &fakeDB{
		row: fakeRow{values: []any{
			int64(1),
			[]byte(" self_decrypt "),
			" device_primary ",
			" https://archive.example/pull ",
			[]byte(" corp-secret "),
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.GetArchiveReconcileEnterprise(context.Background(), " ent-1 ")
	if err != nil {
		t.Fatalf("GetArchiveReconcileEnterprise returned error: %v", err)
	}
	if enterprise == nil {
		t.Fatal("enterprise = nil")
	}
	if !enterprise.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if enterprise.ArchiveSource != "self_decrypt" ||
		enterprise.IncomingPrimaryMode != "device_primary" ||
		enterprise.ArchivePullURL != "https://archive.example/pull" ||
		enterprise.CorpSecret != "corp-secret" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if db.query != archiveReconcileEnterpriseSQL || len(db.args) != 1 || db.args[0] != "ent-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestGetArchivePullEnterpriseReadsRunnerFields(t *testing.T) {
	db := &fakeDB{
		row: fakeRow{values: []any{
			" ent-1 ",
			int64(1),
			[]byte(" corp-1 "),
			" self_decrypt ",
			" self_decrypt ",
			" https://archive.example/pull ",
			" token-1 ",
			" corp-secret ",
			" private-key ",
			" v1 ",
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.GetArchivePullEnterprise(context.Background(), " ent-1 ")
	if err != nil {
		t.Fatalf("GetArchivePullEnterprise returned error: %v", err)
	}
	if enterprise == nil {
		t.Fatal("enterprise = nil")
	}
	if enterprise.EnterpriseID != "ent-1" ||
		!enterprise.Enabled ||
		enterprise.CorpID != "corp-1" ||
		enterprise.ArchiveMode != "self_decrypt" ||
		enterprise.ArchiveSource != "self_decrypt" ||
		enterprise.ArchivePullURL != "https://archive.example/pull" ||
		enterprise.ArchivePullToken != "token-1" ||
		enterprise.CorpSecret != "corp-secret" ||
		enterprise.PrivateKeyPEM != "private-key" ||
		enterprise.PrivateKeyVersion != "v1" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if !strings.Contains(db.query, archivePullEnterpriseColumns) || !strings.Contains(db.query, "WHERE enterprise_id = ?") || db.args[0] != "ent-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestGetEnterpriseReadsFullStatusPayloadFields(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{
		row: fakeRow{values: []any{
			" ent-1 ",
			" corp-1 ",
			" Corp One ",
			" archive_primary ",
			" self_decrypt ",
			" self_decrypt ",
			" https://archive.example/pull ",
			" pull-token ",
			" https://archive.example/media ",
			" media-token ",
			" corp-secret ",
			" contact-secret ",
			" external-secret ",
			" private-key ",
			" v1 ",
			" callback-token ",
			" callback-aes ",
			int64(1),
			" primary ",
			"2026-07-01 18:00:00",
			createdAt,
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.GetEnterprise(context.Background(), " ent-1 ")
	if err != nil {
		t.Fatalf("GetEnterprise returned error: %v", err)
	}
	if enterprise == nil {
		t.Fatal("enterprise = nil")
	}
	if enterprise.EnterpriseID != "ent-1" ||
		enterprise.CorpID != "corp-1" ||
		enterprise.Name != "Corp One" ||
		enterprise.IncomingPrimaryMode != "archive_primary" ||
		enterprise.ArchivePullToken != "pull-token" ||
		enterprise.MediaPullURL != "https://archive.example/media" ||
		enterprise.ContactSecret != "contact-secret" ||
		enterprise.ExternalContactSecret != "external-secret" ||
		enterprise.ArchiveEventCallbackToken != "callback-token" ||
		enterprise.ArchiveEventCallbackAESKey != "callback-aes" ||
		!enterprise.Enabled ||
		enterprise.Remark != "primary" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if enterprise.CreatedAt != time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) || enterprise.UpdatedAt != createdAt {
		t.Fatalf("times created=%s updated=%s", enterprise.CreatedAt, enterprise.UpdatedAt)
	}
	if !strings.Contains(db.query, enterpriseColumns) || !strings.Contains(db.query, "WHERE enterprise_id = ?") || db.args[0] != "ent-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestGetEnterpriseHandlesMissingRowsAndBlankID(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	enterprise, err := repository.GetEnterprise(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetEnterprise returned error: %v", err)
	}
	if enterprise != nil {
		t.Fatalf("enterprise = %#v, want nil", enterprise)
	}

	db := &fakeDB{}
	repository = &Repository{DB: db}
	enterprise, err = repository.GetEnterprise(context.Background(), " ")
	if err != nil {
		t.Fatalf("GetEnterprise blank returned error: %v", err)
	}
	if enterprise != nil || db.query != "" {
		t.Fatalf("enterprise=%#v query=%q", enterprise, db.query)
	}
}

func TestListEnterprisesReadsFullRecords(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{
			" ent-2 ",
			" corp-2 ",
			" Corp Two ",
			" device_primary ",
			" self_decrypt ",
			" official ",
			" https://archive.example/pull ",
			" pull-token ",
			" https://archive.example/media ",
			" media-token ",
			" corp-secret ",
			" contact-secret ",
			" external-secret ",
			" private-key ",
			" v2 ",
			" callback-token ",
			" callback-aes ",
			int64(1),
			" remark ",
			"2026-07-01 18:00:00",
			"2026-07-01 19:00:00",
		},
	}}}
	repository := &Repository{DB: db}

	enterprises, err := repository.ListEnterprises(context.Background())
	if err != nil {
		t.Fatalf("ListEnterprises returned error: %v", err)
	}
	if len(enterprises) != 1 {
		t.Fatalf("len(enterprises) = %d", len(enterprises))
	}
	enterprise := enterprises[0]
	if enterprise.EnterpriseID != "ent-2" ||
		enterprise.CorpID != "corp-2" ||
		enterprise.Name != "Corp Two" ||
		enterprise.IncomingPrimaryMode != "device_primary" ||
		enterprise.ArchiveSource != "official" ||
		enterprise.ArchivePullToken != "pull-token" ||
		enterprise.MediaPullToken != "media-token" ||
		enterprise.CorpSecret != "corp-secret" ||
		enterprise.ArchiveEventCallbackAESKey != "callback-aes" ||
		!enterprise.Enabled ||
		enterprise.Remark != "remark" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if enterprise.CreatedAt != time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("created_at = %s", enterprise.CreatedAt)
	}
	if !strings.Contains(db.query, enterpriseColumns) || !strings.Contains(db.query, "ORDER BY created_at DESC") {
		t.Fatalf("query = %q", db.query)
	}
}

func TestUpsertEnterpriseUpdatesExistingRecord(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: enterpriseRowValues("ent-1", "corp-old", "Old Corp", "archive_primary", "self_decrypt", "self_decrypt", "", "old-token", "", "", "", "", "", "", "", "", "", int64(1), "", "2026-07-01 18:00:00", "2026-07-01 18:30:00")},
			{values: enterpriseRowValues("ent-1", "corp-1", "Corp One", "device_primary", "self_decrypt", "official", "https://archive.example/pull", "pull-token", "", "", "corp-secret", "", "", "", "", "", "", int64(0), "remark", "2026-07-01 18:00:00", now)},
		},
		result: fakeResult{affected: 1},
	}
	repository := &Repository{DB: db, Now: func() time.Time { return now }}

	enterprise, err := repository.UpsertEnterprise(context.Background(), EnterpriseUpsertCommand{
		EnterpriseID:        " ent-1 ",
		CorpID:              " corp-1 ",
		Name:                " Corp One ",
		IncomingPrimaryMode: " device_primary ",
		ArchiveMode:         " self_decrypt ",
		ArchiveSource:       " official ",
		ArchivePullURL:      " https://archive.example/pull ",
		ArchivePullToken:    " pull-token ",
		CorpSecret:          " corp-secret ",
		Enabled:             false,
		Remark:              " remark ",
	})
	if err != nil {
		t.Fatalf("UpsertEnterprise returned error: %v", err)
	}
	if enterprise.Name != "Corp One" || enterprise.ArchiveSource != "official" || enterprise.Enabled {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if !strings.Contains(db.execQuery, "UPDATE enterprises SET") || db.execArgs[0] != "corp-1" || db.execArgs[19] != "ent-1" {
		t.Fatalf("exec query=%q args=%#v", db.execQuery, db.execArgs)
	}
}

func TestUpsertEnterpriseInsertsNewRecord(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rowQueue: []fakeRow{
			{err: sql.ErrNoRows},
			{values: enterpriseRowValues("ent-new", "corp-1", "Corp One", "archive_primary", "self_decrypt", "self_decrypt", "", "", "", "", "", "", "", "", "", "", "", int64(1), "", now, now)},
		},
		result: fakeResult{affected: 1},
	}
	repository := &Repository{DB: db, Now: func() time.Time { return now }}

	enterprise, err := repository.UpsertEnterprise(context.Background(), EnterpriseUpsertCommand{
		EnterpriseID: "ent-new",
		CorpID:       "corp-1",
		Name:         "Corp One",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("UpsertEnterprise returned error: %v", err)
	}
	if enterprise.EnterpriseID != "ent-new" || enterprise.IncomingPrimaryMode != "archive_primary" || enterprise.ArchiveMode != "self_decrypt" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if !strings.Contains(db.execQuery, "INSERT INTO enterprises") || db.execArgs[0] != "ent-new" || db.execArgs[3] != "archive_primary" || db.execArgs[4] != "self_decrypt" {
		t.Fatalf("exec query=%q args=%#v", db.execQuery, db.execArgs)
	}
}

func TestDeleteEnterpriseUsesRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	deleted, err := (&Repository{DB: db}).DeleteEnterprise(context.Background(), " ent-1 ")
	if err != nil {
		t.Fatalf("DeleteEnterprise returned error: %v", err)
	}
	if !deleted || !strings.Contains(db.execQuery, "DELETE FROM enterprises") || db.execArgs[0] != "ent-1" {
		t.Fatalf("deleted=%t query=%q args=%#v", deleted, db.execQuery, db.execArgs)
	}
}

func TestGetArchivePullEnterpriseHandlesMissingRowsAndBlankID(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	enterprise, err := repository.GetArchivePullEnterprise(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetArchivePullEnterprise returned error: %v", err)
	}
	if enterprise != nil {
		t.Fatalf("enterprise = %#v, want nil", enterprise)
	}

	db := &fakeDB{}
	repository = &Repository{DB: db}
	enterprise, err = repository.GetArchivePullEnterprise(context.Background(), " ")
	if err != nil {
		t.Fatalf("GetArchivePullEnterprise blank returned error: %v", err)
	}
	if enterprise != nil || db.query != "" {
		t.Fatalf("enterprise=%#v query=%q", enterprise, db.query)
	}
}

func TestListEnabledArchivePullEnterprisesReadsRows(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"ent-2", int64(1), "corp-2", "self_decrypt", "provider", "", "", "secret-2", "", ""},
		{"ent-1", int64(1), "corp-1", "self_decrypt", "self_decrypt", "https://archive.example/pull", "token", "", "pem", "v1"},
	}}}
	repository := &Repository{DB: db}

	enterprises, err := repository.ListEnabledArchivePullEnterprises(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledArchivePullEnterprises returned error: %v", err)
	}
	if len(enterprises) != 2 || enterprises[0].EnterpriseID != "ent-2" || enterprises[1].ArchivePullURL != "https://archive.example/pull" {
		t.Fatalf("enterprises = %#v", enterprises)
	}
	if !strings.Contains(db.query, "WHERE enabled = 1 ORDER BY created_at DESC") {
		t.Fatalf("query = %q", db.query)
	}
}

func TestResolveArchiveCallbackEnterpriseMatchesIDOrCorp(t *testing.T) {
	db := &fakeDB{
		row: fakeRow{values: []any{
			" ent-1 ",
			int64(1),
			" corp-1 ",
			" official ",
			" token-1 ",
			" aes-1 ",
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.ResolveArchiveCallbackEnterprise(context.Background(), " corp-1 ")
	if err != nil {
		t.Fatalf("ResolveArchiveCallbackEnterprise returned error: %v", err)
	}
	if enterprise == nil || enterprise.EnterpriseID != "ent-1" || enterprise.CorpID != "corp-1" || enterprise.ArchiveSource != "official" || enterprise.CallbackToken != "token-1" || enterprise.CallbackAESKey != "aes-1" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if !strings.Contains(db.query, "WHERE enterprise_id = ? OR corp_id = ?") || len(db.args) != 3 || db.args[0] != "corp-1" || db.args[2] != "corp-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestResolveArchiveCallbackEnterpriseFallsBackToSingleEnabledCandidate(t *testing.T) {
	db := &fakeDB{
		row: fakeRow{err: sql.ErrNoRows},
		rows: &fakeRows{values: [][]any{
			{"ent-1", int64(1), "corp-1", "self_decrypt", "token-1", "aes-1"},
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.ResolveArchiveCallbackEnterprise(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ResolveArchiveCallbackEnterprise returned error: %v", err)
	}
	if enterprise == nil || enterprise.EnterpriseID != "ent-1" {
		t.Fatalf("enterprise = %#v", enterprise)
	}
	if !strings.Contains(db.query, "archive_event_callback_token") || !strings.Contains(db.query, "ORDER BY created_at DESC") {
		t.Fatalf("fallback query = %q", db.query)
	}
}

func TestResolveArchiveCallbackEnterpriseReturnsNilForAmbiguousFallback(t *testing.T) {
	db := &fakeDB{
		row: fakeRow{err: sql.ErrNoRows},
		rows: &fakeRows{values: [][]any{
			{"ent-2", int64(1), "corp-2", "self_decrypt", "token-2", "aes-2"},
			{"ent-1", int64(1), "corp-1", "self_decrypt", "token-1", "aes-1"},
		}},
	}
	repository := &Repository{DB: db}

	enterprise, err := repository.ResolveArchiveCallbackEnterprise(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ResolveArchiveCallbackEnterprise returned error: %v", err)
	}
	if enterprise != nil {
		t.Fatalf("enterprise = %#v, want nil ambiguous fallback", enterprise)
	}
}

func TestListArchiveCallbackEnterprisesReadsConfiguredRows(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"ent-2", int64(1), "corp-2", "official", "token-2", "aes-2"},
		{"ent-1", int64(1), "corp-1", "self_decrypt", "token-1", "aes-1"},
	}}}
	repository := &Repository{DB: db}

	enterprises, err := repository.ListArchiveCallbackEnterprises(context.Background())
	if err != nil {
		t.Fatalf("ListArchiveCallbackEnterprises returned error: %v", err)
	}
	if len(enterprises) != 2 || enterprises[0].EnterpriseID != "ent-2" || enterprises[1].CallbackAESKey != "aes-1" {
		t.Fatalf("enterprises = %#v", enterprises)
	}
	if !strings.Contains(db.query, "archive_event_callback_token") || !strings.Contains(db.query, "archive_event_callback_aes_key") {
		t.Fatalf("query = %q", db.query)
	}
}

func TestGetArchiveReconcileEnterpriseHandlesMissingRows(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	enterprise, err := repository.GetArchiveReconcileEnterprise(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetArchiveReconcileEnterprise returned error: %v", err)
	}
	if enterprise != nil {
		t.Fatalf("enterprise = %#v, want nil", enterprise)
	}
}

func TestGetArchiveReconcileEnterpriseSkipsBlankID(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	enterprise, err := repository.GetArchiveReconcileEnterprise(context.Background(), " ")
	if err != nil {
		t.Fatalf("GetArchiveReconcileEnterprise returned error: %v", err)
	}
	if enterprise != nil {
		t.Fatalf("enterprise = %#v, want nil", enterprise)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no query", db.query)
	}
}

func TestGetArchiveReconcileEnterpriseMapsDisabledValues(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{name: "integer_zero", value: 0},
		{name: "int64_zero", value: int64(0)},
		{name: "byte_false", value: []byte("false")},
		{name: "string_off", value: "off"},
		{name: "nil", value: nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repository := &Repository{DB: &fakeDB{row: fakeRow{values: []any{tt.value, "", "", "", ""}}}}

			enterprise, err := repository.GetArchiveReconcileEnterprise(context.Background(), "ent-1")
			if err != nil {
				t.Fatalf("GetArchiveReconcileEnterprise returned error: %v", err)
			}
			if enterprise == nil {
				t.Fatal("enterprise = nil")
			}
			if enterprise.Enabled {
				t.Fatalf("Enabled = true for value %#v", tt.value)
			}
		})
	}
}

func TestGetArchiveReconcileEnterpriseReturnsStoreErrors(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: errors.New("db down")}}}

	_, err := repository.GetArchiveReconcileEnterprise(context.Background(), "ent-1")
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("error = %v, want db down", err)
	}
}

func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil)

	_, err := repository.GetArchiveReconcileEnterprise(context.Background(), "ent-1")
	if err == nil || !strings.Contains(err.Error(), "sql db is nil") {
		t.Fatalf("error = %v, want nil sql db error", err)
	}
}

type fakeDB struct {
	row       fakeRow
	rowQueue  []fakeRow
	rows      *fakeRows
	result    fakeResult
	query     string
	args      []any
	execQuery string
	execArgs  []any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.args = args
	if len(db.rowQueue) > 0 {
		row := db.rowQueue[0]
		db.rowQueue = db.rowQueue[1:]
		return row
	}
	return db.row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	return db.result, nil
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index, value := range row.values {
		target := dest[index].(*any)
		*target = value
	}
	return nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

type fakeResult struct {
	affected int64
}

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return result.affected, nil
}

func enterpriseRowValues(values ...any) []any {
	return values
}
