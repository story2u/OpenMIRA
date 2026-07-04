// Package enterprisestore adapts the legacy enterprises table for Go workers.
// The repository is intentionally read-only; Python still owns enterprise
// writes and schema lifecycle during the migration.
package enterprisestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/archivereconcile"
)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads enterprise archive reconcile fields from enterprises.
type Repository struct {
	DB  Queryer
	Now func() time.Time
}

// ArchivePullEnterprise captures the enterprise fields needed by archive pull runners.
type ArchivePullEnterprise struct {
	EnterpriseID      string
	Enabled           bool
	CorpID            string
	ArchiveMode       string
	ArchiveSource     string
	ArchivePullURL    string
	ArchivePullToken  string
	CorpSecret        string
	PrivateKeyPEM     string
	PrivateKeyVersion string
}

// EnterpriseRecord mirrors the legacy EnterpriseRecord API payload fields.
type EnterpriseRecord struct {
	EnterpriseID               string
	CorpID                     string
	Name                       string
	IncomingPrimaryMode        string
	ArchiveMode                string
	ArchiveSource              string
	ArchivePullURL             string
	ArchivePullToken           string
	MediaPullURL               string
	MediaPullToken             string
	CorpSecret                 string
	ContactSecret              string
	ExternalContactSecret      string
	PrivateKeyPEM              string
	PrivateKeyVersion          string
	ArchiveEventCallbackToken  string
	ArchiveEventCallbackAESKey string
	Enabled                    bool
	Remark                     string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// EnterpriseUpsertCommand carries admin-managed enterprise write fields.
type EnterpriseUpsertCommand struct {
	EnterpriseID               string
	CorpID                     string
	Name                       string
	IncomingPrimaryMode        string
	ArchiveMode                string
	ArchiveSource              string
	ArchivePullURL             string
	ArchivePullToken           string
	MediaPullURL               string
	MediaPullToken             string
	CorpSecret                 string
	ContactSecret              string
	ExternalContactSecret      string
	PrivateKeyPEM              string
	PrivateKeyVersion          string
	ArchiveEventCallbackToken  string
	ArchiveEventCallbackAESKey string
	Enabled                    bool
	Remark                     string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// GetEnterprise returns one full enterprise record by enterprise_id.
func (repository *Repository) GetEnterprise(ctx context.Context, enterpriseID string) (*EnterpriseRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	key := strings.TrimSpace(enterpriseID)
	if key == "" {
		return nil, nil
	}
	record, err := scanEnterpriseRecord(repository.DB.QueryRowContext(ctx, enterpriseSQL("WHERE enterprise_id = ?"), key))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ListEnterprises returns all legacy enterprise records newest first.
func (repository *Repository) ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, enterpriseSQL("ORDER BY created_at DESC"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []EnterpriseRecord{}
	for rows.Next() {
		record, err := scanEnterpriseRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// UpsertEnterprise creates or updates an admin-managed enterprise record.
func (repository *Repository) UpsertEnterprise(ctx context.Context, command EnterpriseUpsertCommand) (EnterpriseRecord, error) {
	if repository.DB == nil {
		return EnterpriseRecord{}, fmt.Errorf("enterprise store database is not configured")
	}
	key := strings.TrimSpace(command.EnterpriseID)
	if key == "" {
		return EnterpriseRecord{}, fmt.Errorf("enterprise_id is required")
	}
	now := repository.now()
	existing, err := repository.GetEnterprise(ctx, key)
	if err != nil {
		return EnterpriseRecord{}, err
	}
	if existing != nil {
		_, err = repository.DB.ExecContext(ctx, enterpriseUpdateSQL,
			strings.TrimSpace(command.CorpID),
			strings.TrimSpace(command.Name),
			defaultString(strings.TrimSpace(command.IncomingPrimaryMode), "archive_primary"),
			defaultString(strings.TrimSpace(command.ArchiveMode), "self_decrypt"),
			defaultString(strings.TrimSpace(command.ArchiveSource), "self_decrypt"),
			strings.TrimSpace(command.ArchivePullURL),
			strings.TrimSpace(command.ArchivePullToken),
			strings.TrimSpace(command.MediaPullURL),
			strings.TrimSpace(command.MediaPullToken),
			strings.TrimSpace(command.CorpSecret),
			strings.TrimSpace(command.ContactSecret),
			strings.TrimSpace(command.ExternalContactSecret),
			strings.TrimSpace(command.PrivateKeyPEM),
			strings.TrimSpace(command.PrivateKeyVersion),
			strings.TrimSpace(command.ArchiveEventCallbackToken),
			strings.TrimSpace(command.ArchiveEventCallbackAESKey),
			boolInt(command.Enabled),
			strings.TrimSpace(command.Remark),
			now,
			key,
		)
	} else {
		_, err = repository.DB.ExecContext(ctx, enterpriseInsertSQL,
			key,
			strings.TrimSpace(command.CorpID),
			strings.TrimSpace(command.Name),
			defaultString(strings.TrimSpace(command.IncomingPrimaryMode), "archive_primary"),
			defaultString(strings.TrimSpace(command.ArchiveMode), "self_decrypt"),
			defaultString(strings.TrimSpace(command.ArchiveSource), "self_decrypt"),
			strings.TrimSpace(command.ArchivePullURL),
			strings.TrimSpace(command.ArchivePullToken),
			strings.TrimSpace(command.MediaPullURL),
			strings.TrimSpace(command.MediaPullToken),
			strings.TrimSpace(command.CorpSecret),
			strings.TrimSpace(command.ContactSecret),
			strings.TrimSpace(command.ExternalContactSecret),
			strings.TrimSpace(command.PrivateKeyPEM),
			strings.TrimSpace(command.PrivateKeyVersion),
			strings.TrimSpace(command.ArchiveEventCallbackToken),
			strings.TrimSpace(command.ArchiveEventCallbackAESKey),
			boolInt(command.Enabled),
			strings.TrimSpace(command.Remark),
			now,
			now,
		)
	}
	if err != nil {
		return EnterpriseRecord{}, err
	}
	record, err := repository.GetEnterprise(ctx, key)
	if err != nil {
		return EnterpriseRecord{}, err
	}
	if record == nil {
		return EnterpriseRecord{}, fmt.Errorf("enterprise upsert failed")
	}
	return *record, nil
}

// DeleteEnterprise removes an admin-managed enterprise record.
func (repository *Repository) DeleteEnterprise(ctx context.Context, enterpriseID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("enterprise store database is not configured")
	}
	key := strings.TrimSpace(enterpriseID)
	if key == "" {
		return false, nil
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM enterprises WHERE enterprise_id = ?", key)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// GetArchiveReconcileEnterprise returns the fields needed for incoming/archive decisions.
func (repository *Repository) GetArchiveReconcileEnterprise(ctx context.Context, enterpriseID string) (*archivereconcile.Enterprise, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	key := strings.TrimSpace(enterpriseID)
	if key == "" {
		return nil, nil
	}

	var (
		enabled             any
		archiveSource       any
		incomingPrimaryMode any
		archivePullURL      any
		corpSecret          any
	)
	err := repository.DB.QueryRowContext(ctx, archiveReconcileEnterpriseSQL, key).Scan(
		&enabled,
		&archiveSource,
		&incomingPrimaryMode,
		&archivePullURL,
		&corpSecret,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &archivereconcile.Enterprise{
		Enabled:             boolFromDB(enabled),
		ArchiveSource:       stringFromDB(archiveSource),
		IncomingPrimaryMode: stringFromDB(incomingPrimaryMode),
		ArchivePullURL:      stringFromDB(archivePullURL),
		CorpSecret:          stringFromDB(corpSecret),
	}, nil
}

const archiveReconcileEnterpriseSQL = "SELECT enabled, archive_source, incoming_primary_mode, archive_pull_url, corp_secret FROM enterprises WHERE enterprise_id = ?"

// GetArchivePullEnterprise returns archive pull configuration for one enterprise.
func (repository *Repository) GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*ArchivePullEnterprise, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	key := strings.TrimSpace(enterpriseID)
	if key == "" {
		return nil, nil
	}
	record, err := scanArchivePullEnterprise(repository.DB.QueryRowContext(ctx, archivePullEnterpriseSQL("WHERE enterprise_id = ?"), key))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ListEnabledArchivePullEnterprises returns enabled enterprises for archive pull loops.
func (repository *Repository) ListEnabledArchivePullEnterprises(ctx context.Context) ([]ArchivePullEnterprise, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, archivePullEnterpriseSQL("WHERE enabled = 1 ORDER BY created_at DESC"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []ArchivePullEnterprise{}
	for rows.Next() {
		record, err := scanArchivePullEnterprise(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

const archivePullEnterpriseColumns = "enterprise_id, enabled, corp_id, archive_mode, archive_source, archive_pull_url, archive_pull_token, corp_secret, private_key_pem, private_key_version"

func archivePullEnterpriseSQL(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix != "" {
		suffix = " " + suffix
	}
	return "SELECT " + archivePullEnterpriseColumns + " FROM enterprises" + suffix
}

func scanArchivePullEnterprise(row RowScanner) (ArchivePullEnterprise, error) {
	var (
		enterpriseID      any
		enabled           any
		corpID            any
		archiveMode       any
		archiveSource     any
		archivePullURL    any
		archivePullToken  any
		corpSecret        any
		privateKeyPEM     any
		privateKeyVersion any
	)
	if err := row.Scan(
		&enterpriseID,
		&enabled,
		&corpID,
		&archiveMode,
		&archiveSource,
		&archivePullURL,
		&archivePullToken,
		&corpSecret,
		&privateKeyPEM,
		&privateKeyVersion,
	); err != nil {
		return ArchivePullEnterprise{}, err
	}
	return ArchivePullEnterprise{
		EnterpriseID:      stringFromDB(enterpriseID),
		Enabled:           boolFromDB(enabled),
		CorpID:            stringFromDB(corpID),
		ArchiveMode:       stringFromDB(archiveMode),
		ArchiveSource:     stringFromDB(archiveSource),
		ArchivePullURL:    stringFromDB(archivePullURL),
		ArchivePullToken:  stringFromDB(archivePullToken),
		CorpSecret:        stringFromDB(corpSecret),
		PrivateKeyPEM:     stringFromDB(privateKeyPEM),
		PrivateKeyVersion: stringFromDB(privateKeyVersion),
	}, nil
}

const enterpriseColumns = "enterprise_id, corp_id, name, incoming_primary_mode, archive_mode, archive_source, archive_pull_url, archive_pull_token, media_pull_url, media_pull_token, corp_secret, contact_secret, external_contact_secret, private_key_pem, private_key_version, archive_event_callback_token, archive_event_callback_aes_key, enabled, remark, created_at, updated_at"

const enterpriseInsertSQL = "INSERT INTO enterprises (enterprise_id, corp_id, name, incoming_primary_mode, archive_mode, archive_source, archive_pull_url, archive_pull_token, media_pull_url, media_pull_token, corp_secret, contact_secret, external_contact_secret, private_key_pem, private_key_version, archive_event_callback_token, archive_event_callback_aes_key, enabled, remark, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

const enterpriseUpdateSQL = "UPDATE enterprises SET corp_id = ?, name = ?, incoming_primary_mode = ?, archive_mode = ?, archive_source = ?, archive_pull_url = ?, archive_pull_token = ?, media_pull_url = ?, media_pull_token = ?, corp_secret = ?, contact_secret = ?, external_contact_secret = ?, private_key_pem = ?, private_key_version = ?, archive_event_callback_token = ?, archive_event_callback_aes_key = ?, enabled = ?, remark = ?, updated_at = ? WHERE enterprise_id = ?"

func enterpriseSQL(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix != "" {
		suffix = " " + suffix
	}
	return "SELECT " + enterpriseColumns + " FROM enterprises" + suffix
}

func scanEnterpriseRecord(row RowScanner) (EnterpriseRecord, error) {
	var (
		enterpriseID              any
		corpID                    any
		name                      any
		incomingPrimaryMode       any
		archiveMode               any
		archiveSource             any
		archivePullURL            any
		archivePullToken          any
		mediaPullURL              any
		mediaPullToken            any
		corpSecret                any
		contactSecret             any
		externalContactSecret     any
		privateKeyPEM             any
		privateKeyVersion         any
		archiveEventCallbackToken any
		archiveEventCallbackAES   any
		enabled                   any
		remark                    any
		createdAt                 any
		updatedAt                 any
	)
	if err := row.Scan(
		&enterpriseID,
		&corpID,
		&name,
		&incomingPrimaryMode,
		&archiveMode,
		&archiveSource,
		&archivePullURL,
		&archivePullToken,
		&mediaPullURL,
		&mediaPullToken,
		&corpSecret,
		&contactSecret,
		&externalContactSecret,
		&privateKeyPEM,
		&privateKeyVersion,
		&archiveEventCallbackToken,
		&archiveEventCallbackAES,
		&enabled,
		&remark,
		&createdAt,
		&updatedAt,
	); err != nil {
		return EnterpriseRecord{}, err
	}
	return EnterpriseRecord{
		EnterpriseID:               stringFromDB(enterpriseID),
		CorpID:                     stringFromDB(corpID),
		Name:                       stringFromDB(name),
		IncomingPrimaryMode:        stringFromDB(incomingPrimaryMode),
		ArchiveMode:                stringFromDB(archiveMode),
		ArchiveSource:              stringFromDB(archiveSource),
		ArchivePullURL:             stringFromDB(archivePullURL),
		ArchivePullToken:           stringFromDB(archivePullToken),
		MediaPullURL:               stringFromDB(mediaPullURL),
		MediaPullToken:             stringFromDB(mediaPullToken),
		CorpSecret:                 stringFromDB(corpSecret),
		ContactSecret:              stringFromDB(contactSecret),
		ExternalContactSecret:      stringFromDB(externalContactSecret),
		PrivateKeyPEM:              stringFromDB(privateKeyPEM),
		PrivateKeyVersion:          stringFromDB(privateKeyVersion),
		ArchiveEventCallbackToken:  stringFromDB(archiveEventCallbackToken),
		ArchiveEventCallbackAESKey: stringFromDB(archiveEventCallbackAES),
		Enabled:                    boolFromDB(enabled),
		Remark:                     stringFromDB(remark),
		CreatedAt:                  timeFromDB(createdAt),
		UpdatedAt:                  timeFromDB(updatedAt),
	}, nil
}

// ResolveArchiveCallbackEnterprise mirrors Python resolve_enterprise_for_callback.
func (repository *Repository) ResolveArchiveCallbackEnterprise(ctx context.Context, key string) (*archivecallback.Enterprise, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	normalizedKey := strings.TrimSpace(key)
	if normalizedKey != "" {
		record, err := scanArchiveCallbackEnterprise(repository.DB.QueryRowContext(ctx, archiveCallbackEnterpriseSQL("WHERE enterprise_id = ? OR corp_id = ? ORDER BY CASE WHEN enterprise_id = ? THEN 0 ELSE 1 END LIMIT 1"), normalizedKey, normalizedKey, normalizedKey))
		if err == nil {
			return &record, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}
	records, err := repository.ListArchiveCallbackEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	if len(records) == 1 {
		return &records[0], nil
	}
	return nil, nil
}

// ListArchiveCallbackEnterprises returns enabled callback candidates with configured secrets.
func (repository *Repository) ListArchiveCallbackEnterprises(ctx context.Context) ([]archivecallback.Enterprise, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("enterprise store database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, archiveCallbackEnterpriseSQL("WHERE enabled = 1 AND COALESCE(archive_event_callback_token, '') <> '' AND COALESCE(archive_event_callback_aes_key, '') <> '' ORDER BY created_at DESC"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []archivecallback.Enterprise{}
	for rows.Next() {
		record, err := scanArchiveCallbackEnterprise(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

const archiveCallbackEnterpriseColumns = "enterprise_id, enabled, corp_id, archive_source, archive_event_callback_token, archive_event_callback_aes_key"

func archiveCallbackEnterpriseSQL(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix != "" {
		suffix = " " + suffix
	}
	return "SELECT " + archiveCallbackEnterpriseColumns + " FROM enterprises" + suffix
}

func scanArchiveCallbackEnterprise(row RowScanner) (archivecallback.Enterprise, error) {
	var (
		enterpriseID   any
		enabled        any
		corpID         any
		archiveSource  any
		callbackToken  any
		callbackAESKey any
	)
	if err := row.Scan(
		&enterpriseID,
		&enabled,
		&corpID,
		&archiveSource,
		&callbackToken,
		&callbackAESKey,
	); err != nil {
		return archivecallback.Enterprise{}, err
	}
	return archivecallback.Enterprise{
		EnterpriseID:   stringFromDB(enterpriseID),
		Enabled:        boolFromDB(enabled),
		CorpID:         stringFromDB(corpID),
		ArchiveSource:  stringFromDB(archiveSource),
		CallbackToken:  stringFromDB(callbackToken),
		CallbackAESKey: stringFromDB(callbackAESKey),
	}, nil
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case []byte:
		return boolFromString(string(typed))
	case string:
		return boolFromString(typed)
	default:
		return boolFromString(fmt.Sprint(typed))
	}
}

func boolFromString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseTimeString(string(typed))
	case string:
		return parseTimeString(typed)
	default:
		return time.Time{}
	}
}

func parseTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, time.FixedZone("Asia/Shanghai", 8*60*60)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is nil")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
