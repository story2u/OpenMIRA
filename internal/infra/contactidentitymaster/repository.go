// Package contactidentitymaster writes the legacy contact identity master tables.
package contactidentitymaster

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/contactidentity"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

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
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository updates contact_identity_master and its scoped display index.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// UpsertFromContactProfile applies a contact profile update to the legacy identity master.
func (repository *Repository) UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error {
	if repository.DB == nil {
		return fmt.Errorf("contact identity master database is not configured")
	}
	exists, err := repository.tableExists(ctx, "contact_identity_master")
	if err != nil || !exists {
		return err
	}
	input.Now = repository.now()
	existing, found, err := repository.get(ctx, input.EnterpriseID, input.SenderID)
	if err != nil {
		return err
	}
	var existingPointer *contactidentity.Record
	if found {
		existingPointer = &existing
	}
	record, ok := contactidentity.BuildProfileUpsert(input, existingPointer)
	if !ok {
		return nil
	}
	return repository.persist(ctx, record)
}

// ResolveIdentity reads one normalized identity master record.
func (repository *Repository) ResolveIdentity(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error) {
	if repository.DB == nil {
		return contactidentity.Record{}, false, fmt.Errorf("contact identity master database is not configured")
	}
	exists, err := repository.tableExists(ctx, "contact_identity_master")
	if err != nil || !exists {
		return contactidentity.Record{}, false, err
	}
	return repository.get(ctx, enterpriseID, senderID)
}

// MarkScopedRPASafeSearchName stores synced RPA safe-search metadata locally.
func (repository *Repository) MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error {
	if repository.DB == nil {
		return fmt.Errorf("contact identity master database is not configured")
	}
	exists, err := repository.tableExists(ctx, "contact_identity_master")
	if err != nil || !exists {
		return err
	}
	input.Now = repository.now()
	existing, err := repository.resolveOrMissing(ctx, input.EnterpriseID, input.SenderID)
	if err != nil {
		return err
	}
	record, err := contactidentity.MarkScopedRPASafeSearchName(existing, input)
	if err != nil {
		return err
	}
	return repository.persist(ctx, record)
}

// ClearScopedRPASafeSearchName removes managed RPA safe-search metadata locally.
func (repository *Repository) ClearScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeClear) error {
	if repository.DB == nil {
		return fmt.Errorf("contact identity master database is not configured")
	}
	exists, err := repository.tableExists(ctx, "contact_identity_master")
	if err != nil || !exists {
		return err
	}
	input.Now = repository.now()
	existing, err := repository.resolveOrMissing(ctx, input.EnterpriseID, input.SenderID)
	if err != nil {
		return err
	}
	record, err := contactidentity.ClearScopedRPASafeSearchName(existing, input)
	if err != nil {
		return err
	}
	return repository.persist(ctx, record)
}

// IsScopedDisplayAmbiguous returns whether a display name maps to another scoped sender.
func (repository *Repository) IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("contact identity master database is not configured")
	}
	normalizedEnterpriseID := strings.TrimSpace(enterpriseID)
	normalizedWeWorkUserID := contactidentity.NormalizeScopeWeWorkUserID(weworkUserID)
	normalizedDisplayName := strings.TrimSpace(displayName)
	normalizedSenderID := strings.ToLower(strings.TrimSpace(senderID))
	if normalizedEnterpriseID == "" || normalizedWeWorkUserID == "" || normalizedDisplayName == "" {
		return false, nil
	}
	exists, err := repository.tableExists(ctx, "contact_identity_scoped_display_index")
	if err != nil || !exists {
		return false, err
	}
	rows, err := repository.DB.QueryContext(ctx, `
SELECT sender_id
FROM contact_identity_scoped_display_index
WHERE enterprise_id = ?
  AND wework_user_id = ?
  AND display_name_key = ?
  AND display_name = ?
LIMIT ?`,
		normalizedEnterpriseID,
		normalizedWeWorkUserID,
		contactidentity.HashIndexValue(normalizedDisplayName),
		normalizedDisplayName,
		5,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var matchedSenderID any
		if err := rows.Scan(&matchedSenderID); err != nil {
			return false, err
		}
		if strings.ToLower(strings.TrimSpace(stringFromDB(matchedSenderID))) != normalizedSenderID {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (repository *Repository) resolveOrMissing(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, error) {
	record, found, err := repository.get(ctx, enterpriseID, senderID)
	if err != nil {
		return contactidentity.Record{}, err
	}
	if found {
		return record, nil
	}
	return contactidentity.Record{
		EnterpriseID:   strings.TrimSpace(enterpriseID),
		SenderID:       strings.TrimSpace(senderID),
		IdentityStatus: "missing",
		SourcePriority: "fallback",
		NeedsRefresh:   true,
		ExtraJSON:      map[string]any{},
	}, nil
}

func (repository *Repository) persist(ctx context.Context, record contactidentity.Record) error {
	if _, err := repository.DB.ExecContext(ctx, repository.upsertSQL(), repository.serialize(record)...); err != nil {
		return err
	}
	indexExists, err := repository.tableExists(ctx, "contact_identity_scoped_display_index")
	if err != nil || !indexExists {
		return err
	}
	return repository.replaceScopedDisplayIndex(ctx, record)
}

func (repository *Repository) get(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error) {
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    sender_id,
    identity_status,
    display_name,
    remark_name,
    nickname,
    avatar_url,
    source_priority,
    source_version,
    last_synced_at,
    last_verified_at,
    needs_refresh,
    profile_error,
    extra_json
FROM contact_identity_master
WHERE enterprise_id = ? AND sender_id = ?
LIMIT 1`, strings.TrimSpace(enterpriseID), strings.TrimSpace(senderID))
	if err != nil {
		return contactidentity.Record{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return contactidentity.Record{}, false, rows.Err()
	}
	var enterprise any
	var sender any
	var status any
	var displayName any
	var remarkName any
	var nickname any
	var avatarURL any
	var sourcePriority any
	var sourceVersion any
	var lastSyncedAt any
	var lastVerifiedAt any
	var needsRefresh any
	var profileError any
	var extraJSON any
	if err := rows.Scan(&enterprise, &sender, &status, &displayName, &remarkName, &nickname, &avatarURL, &sourcePriority, &sourceVersion, &lastSyncedAt, &lastVerifiedAt, &needsRefresh, &profileError, &extraJSON); err != nil {
		return contactidentity.Record{}, false, err
	}
	return contactidentity.Record{
		EnterpriseID:   stringFromDB(enterprise),
		SenderID:       stringFromDB(sender),
		IdentityStatus: defaultText(stringFromDB(status), "missing"),
		DisplayName:    stringFromDB(displayName),
		RemarkName:     stringFromDB(remarkName),
		Nickname:       stringFromDB(nickname),
		AvatarURL:      stringFromDB(avatarURL),
		SourcePriority: defaultText(stringFromDB(sourcePriority), "fallback"),
		SourceVersion:  intFromDB(sourceVersion),
		LastSyncedAt:   stringFromDB(lastSyncedAt),
		LastVerifiedAt: stringFromDB(lastVerifiedAt),
		NeedsRefresh:   boolFromDB(needsRefresh),
		ProfileError:   stringFromDB(profileError),
		ExtraJSON:      jsonObjectFromDB(extraJSON),
	}, true, rows.Err()
}

func (repository *Repository) replaceScopedDisplayIndex(ctx context.Context, record contactidentity.Record) error {
	rows := contactidentity.ScopedDisplayRows(record)
	senderIDKey := contactidentity.HashIndexValue(record.SenderID)
	if senderIDKey == "" {
		return nil
	}
	if _, err := repository.DB.ExecContext(ctx, `
DELETE FROM contact_identity_scoped_display_index
WHERE enterprise_id = ? AND sender_id_key = ?`, strings.TrimSpace(record.EnterpriseID), senderIDKey); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	query := repository.scopedDisplayUpsertSQL()
	for _, row := range rows {
		if _, err := repository.DB.ExecContext(ctx, query,
			row.EnterpriseID,
			row.WeWorkUserID,
			row.DisplayNameKey,
			row.SenderIDKey,
			row.DisplayName,
			row.SenderID,
			repository.dbNowParam(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) serialize(record contactidentity.Record) []any {
	return []any{
		strings.TrimSpace(record.EnterpriseID),
		strings.TrimSpace(record.SenderID),
		defaultText(record.IdentityStatus, "missing"),
		strings.TrimSpace(record.DisplayName),
		strings.TrimSpace(record.RemarkName),
		strings.TrimSpace(record.Nickname),
		strings.TrimSpace(record.AvatarURL),
		defaultText(record.SourcePriority, "fallback"),
		record.SourceVersion,
		repository.dbNullableTimeText(record.LastSyncedAt),
		repository.dbNullableTimeText(record.LastVerifiedAt),
		boolInt(record.NeedsRefresh),
		nullableText(record.ProfileError),
		jsonString(record.ExtraJSON),
		repository.dbNowParam(),
	}
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO contact_identity_master (
    enterprise_id, sender_id, identity_status, display_name, remark_name, nickname,
    avatar_url, source_priority, source_version, last_synced_at, last_verified_at,
    needs_refresh, profile_error, extra_json, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    identity_status = VALUES(identity_status),
    display_name = VALUES(display_name),
    remark_name = VALUES(remark_name),
    nickname = VALUES(nickname),
    avatar_url = VALUES(avatar_url),
    source_priority = VALUES(source_priority),
    source_version = VALUES(source_version),
    last_synced_at = VALUES(last_synced_at),
    last_verified_at = VALUES(last_verified_at),
    needs_refresh = VALUES(needs_refresh),
    profile_error = VALUES(profile_error),
    extra_json = VALUES(extra_json),
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO contact_identity_master (
    enterprise_id, sender_id, identity_status, display_name, remark_name, nickname,
    avatar_url, source_priority, source_version, last_synced_at, last_verified_at,
    needs_refresh, profile_error, extra_json, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?)
ON CONFLICT(enterprise_id, sender_id) DO UPDATE SET
    identity_status = EXCLUDED.identity_status,
    display_name = EXCLUDED.display_name,
    remark_name = EXCLUDED.remark_name,
    nickname = EXCLUDED.nickname,
    avatar_url = EXCLUDED.avatar_url,
    source_priority = EXCLUDED.source_priority,
    source_version = EXCLUDED.source_version,
    last_synced_at = EXCLUDED.last_synced_at,
    last_verified_at = EXCLUDED.last_verified_at,
    needs_refresh = EXCLUDED.needs_refresh,
    profile_error = EXCLUDED.profile_error,
    extra_json = EXCLUDED.extra_json,
    updated_at = EXCLUDED.updated_at`
}

func (repository *Repository) scopedDisplayUpsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO contact_identity_scoped_display_index (
    enterprise_id, wework_user_id, display_name_key, sender_id_key,
    display_name, sender_id, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    display_name = VALUES(display_name),
    sender_id = VALUES(sender_id),
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO contact_identity_scoped_display_index (
    enterprise_id, wework_user_id, display_name_key, sender_id_key,
    display_name, sender_id, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(enterprise_id, wework_user_id, display_name_key, sender_id_key) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    sender_id = EXCLUDED.sender_id,
    updated_at = EXCLUDED.updated_at`
}

func (repository *Repository) tableExists(ctx context.Context, table string) (bool, error) {
	if strings.TrimSpace(table) == "" {
		return false, fmt.Errorf("table name is required")
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT 1 FROM "+table+" WHERE 1 = 0")
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	return true, rows.Err()
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbNullableTimeText(value string) any {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	if parsed := parseDBTimeString(text); !parsed.IsZero() {
		return repository.dbTimeParam(parsed)
	}
	return text
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format(time.RFC3339)
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func jsonString(value map[string]any) string {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func jsonObjectFromDB(value any) map[string]any {
	text := stringFromDB(value)
	if text == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func parseDBTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case []byte:
		return parseIntText(string(typed))
	case string:
		return parseIntText(typed)
	default:
		return parseIntText(fmt.Sprint(typed))
	}
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	default:
		return false
	}
}

func parseIntText(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return fallback
}

func nullableText(value string) any {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("contact identity master database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("contact identity master database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
