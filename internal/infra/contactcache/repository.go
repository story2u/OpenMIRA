// Package contactcache reads legacy WeWork contact cache tables.
package contactcache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/contacts"
	"wework-go/internal/weworkuserinfo"
)

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

// Repository reads cached contact rows from legacy tables.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := ""
	if len(dialect) > 0 {
		resolvedDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// GetExternalContact returns one row from wework_external_contacts.
func (repository *Repository) GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error) {
	if repository.DB == nil {
		return nil, false, fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_external_contacts")
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    external_userid,
    name,
    avatar,
    type,
    gender,
    unionid,
    position,
    corp_name,
    corp_full_name,
    external_profile_json,
    follow_users_json,
    tags_json,
    add_way,
    add_time,
    synced_at,
    stale,
    source,
    created_at,
    updated_at
FROM wework_external_contacts
WHERE enterprise_id = ? AND external_userid = ?
LIMIT 1`, strings.TrimSpace(enterpriseID), strings.TrimSpace(externalUserID))
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var enterprise any
	var external any
	var name any
	var avatar any
	var contactType any
	var gender any
	var unionID any
	var position any
	var corpName any
	var corpFullName any
	var externalProfileJSON any
	var followUsersJSON any
	var tagsJSON any
	var addWay any
	var addTime any
	var syncedAt any
	var stale any
	var source any
	var createdAt any
	var updatedAt any
	if err := rows.Scan(&enterprise, &external, &name, &avatar, &contactType, &gender, &unionID, &position, &corpName, &corpFullName, &externalProfileJSON, &followUsersJSON, &tagsJSON, &addWay, &addTime, &syncedAt, &stale, &source, &createdAt, &updatedAt); err != nil {
		return nil, false, err
	}
	return contacts.Payload{
		"enterprise_id":         stringFromDB(enterprise),
		"external_userid":       stringFromDB(external),
		"name":                  stringFromDB(name),
		"avatar":                stringFromDB(avatar),
		"type":                  intFromDB(contactType),
		"gender":                intFromDB(gender),
		"unionid":               stringFromDB(unionID),
		"position":              stringFromDB(position),
		"corp_name":             stringFromDB(corpName),
		"corp_full_name":        stringFromDB(corpFullName),
		"external_profile_json": jsonObjectFromDB(externalProfileJSON),
		"follow_users_json":     jsonArrayFromDB(followUsersJSON),
		"tags_json":             jsonArrayFromDB(tagsJSON),
		"add_way":               stringFromDB(addWay),
		"add_time":              nilIfBlank(addTime),
		"synced_at":             nilIfBlank(syncedAt),
		"stale":                 boolFromDB(stale),
		"source":                stringFromDB(source),
		"created_at":            nilIfBlank(createdAt),
		"updated_at":            nilIfBlank(updatedAt),
	}, true, rows.Err()
}

// UpsertExternalContact stores one externalcontact/get payload in the local cache.
func (repository *Repository) UpsertExternalContact(ctx context.Context, payload contacts.Payload) error {
	if repository.DB == nil {
		return fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_external_contacts")
	if err != nil || !exists {
		return err
	}
	values, ok := repository.serializeExternalContact(payload)
	if !ok {
		return fmt.Errorf("enterprise_id and external_userid are required")
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertExternalContactSQL(), values...)
	return err
}

// ListStaleExternalContacts returns external contacts marked stale or older than maxAgeHours.
func (repository *Repository) ListStaleExternalContacts(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]contacts.Payload, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_external_contacts")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []contacts.Payload{}, nil
	}
	normalizedLimit := positiveLimit(limit)
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    external_userid,
    name,
    avatar,
    type,
    gender,
    unionid,
    position,
    corp_name,
    corp_full_name,
    external_profile_json,
    follow_users_json,
    tags_json,
    add_way,
    add_time,
    synced_at,
    stale,
    source,
    created_at,
    updated_at
FROM wework_external_contacts
WHERE (? = '' OR enterprise_id = ?)
ORDER BY updated_at ASC
LIMIT ?`, strings.TrimSpace(enterpriseID), strings.TrimSpace(enterpriseID), normalizedLimit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threshold := repository.now().Add(-time.Duration(positiveMaxAgeHours(maxAgeHours)) * time.Hour)
	results := make([]contacts.Payload, 0, normalizedLimit)
	for rows.Next() {
		payload, err := scanExternalContactPayload(rows)
		if err != nil {
			return nil, err
		}
		if isStalePayload(payload, threshold) {
			results = append(results, payload)
		}
		if len(results) >= normalizedLimit {
			break
		}
	}
	return results, rows.Err()
}

// MarkExternalContactRefreshSkipped clears stale state for a non-retryable external contact.
func (repository *Repository) MarkExternalContactRefreshSkipped(ctx context.Context, enterpriseID string, externalUserID string, source string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("contact cache database is not configured")
	}
	normalizedEnterpriseID := strings.TrimSpace(enterpriseID)
	normalizedExternalUserID := strings.TrimSpace(externalUserID)
	if normalizedEnterpriseID == "" || normalizedExternalUserID == "" {
		return false, nil
	}
	exists, err := repository.tableExists(ctx, "wework_external_contacts")
	if err != nil || !exists {
		return false, err
	}
	now := repository.now().Format(time.RFC3339Nano)
	result, err := repository.DB.ExecContext(ctx, `
UPDATE wework_external_contacts
SET synced_at = ?, updated_at = ?, stale = 0, source = ?
WHERE enterprise_id = ? AND external_userid = ?`,
		now,
		now,
		defaultString(source, "stale_refresh_skipped"),
		normalizedEnterpriseID,
		normalizedExternalUserID,
	)
	if err != nil || result == nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// UpsertCorpUser stores one user/get payload in the local corp-user cache.
func (repository *Repository) UpsertCorpUser(ctx context.Context, payload contacts.Payload) error {
	if repository.DB == nil {
		return fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_corp_users")
	if err != nil || !exists {
		return err
	}
	values, ok := repository.serializeCorpUser(payload)
	if !ok {
		return fmt.Errorf("enterprise_id and userid are required")
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertCorpUserSQL(), values...)
	return err
}

// ListStaleCorpUsers returns internal users marked stale or older than maxAgeHours.
func (repository *Repository) ListStaleCorpUsers(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]contacts.Payload, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_corp_users")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []contacts.Payload{}, nil
	}
	normalizedLimit := positiveLimit(limit)
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    userid,
    name,
    department_json,
    position,
    mobile,
    gender,
    email,
    biz_mail,
    avatar,
    status,
    extattr_json,
    synced_at,
    stale,
    source,
    created_at,
    updated_at
FROM wework_corp_users
WHERE (? = '' OR enterprise_id = ?)
ORDER BY updated_at ASC
LIMIT ?`, strings.TrimSpace(enterpriseID), strings.TrimSpace(enterpriseID), normalizedLimit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threshold := repository.now().Add(-time.Duration(positiveMaxAgeHours(maxAgeHours)) * time.Hour)
	results := make([]contacts.Payload, 0, normalizedLimit)
	for rows.Next() {
		payload, err := scanCorpUserPayload(rows)
		if err != nil {
			return nil, err
		}
		if isStalePayload(payload, threshold) {
			results = append(results, payload)
		}
		if len(results) >= normalizedLimit {
			break
		}
	}
	return results, rows.Err()
}

// GetCorpUser returns one row from wework_corp_users.
func (repository *Repository) GetCorpUser(ctx context.Context, enterpriseID string, userID string) (contacts.Payload, bool, error) {
	if repository.DB == nil {
		return nil, false, fmt.Errorf("contact cache database is not configured")
	}
	exists, err := repository.tableExists(ctx, "wework_corp_users")
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    userid,
    name,
    department_json,
    position,
    mobile,
    gender,
    email,
    biz_mail,
    avatar,
    status,
    extattr_json,
    synced_at,
    stale,
    source,
    created_at,
    updated_at
FROM wework_corp_users
WHERE enterprise_id = ? AND userid = ?
LIMIT 1`, strings.TrimSpace(enterpriseID), strings.TrimSpace(userID))
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var enterprise any
	var user any
	var name any
	var departments any
	var position any
	var mobile any
	var gender any
	var email any
	var bizMail any
	var avatar any
	var status any
	var extattr any
	var syncedAt any
	var stale any
	var source any
	var createdAt any
	var updatedAt any
	if err := rows.Scan(&enterprise, &user, &name, &departments, &position, &mobile, &gender, &email, &bizMail, &avatar, &status, &extattr, &syncedAt, &stale, &source, &createdAt, &updatedAt); err != nil {
		return nil, false, err
	}
	return contacts.Payload{
		"enterprise_id":   stringFromDB(enterprise),
		"userid":          stringFromDB(user),
		"name":            stringFromDB(name),
		"department_json": jsonArrayFromDB(departments),
		"position":        stringFromDB(position),
		"mobile":          stringFromDB(mobile),
		"gender":          intFromDB(gender),
		"email":           stringFromDB(email),
		"biz_mail":        stringFromDB(bizMail),
		"avatar":          stringFromDB(avatar),
		"status":          intFromDB(status),
		"extattr_json":    jsonObjectFromDB(extattr),
		"synced_at":       nilIfBlank(syncedAt),
		"stale":           boolFromDB(stale),
		"source":          stringFromDB(source),
		"created_at":      nilIfBlank(createdAt),
		"updated_at":      nilIfBlank(updatedAt),
	}, true, rows.Err()
}

// ListInternalUserCandidatesByNames returns local corp users for manual userid selection.
func (repository *Repository) ListInternalUserCandidatesByNames(ctx context.Context, enterpriseID string, names []string, limit int) ([]weworkuserinfo.InternalUserCandidate, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("contact cache database is not configured")
	}
	normalizedEnterpriseID := strings.TrimSpace(enterpriseID)
	normalizedNames := normalizeNames(names)
	if normalizedEnterpriseID == "" || len(normalizedNames) == 0 {
		return []weworkuserinfo.InternalUserCandidate{}, nil
	}
	exists, err := repository.tableExists(ctx, "wework_corp_users")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []weworkuserinfo.InternalUserCandidate{}, nil
	}
	query := `
SELECT
    enterprise_id,
    userid,
    name,
    department_json,
    position,
    avatar,
    synced_at,
    updated_at
FROM wework_corp_users
WHERE enterprise_id = ?
  AND stale = 0
  AND name IN (` + placeholders(len(normalizedNames)) + `)
ORDER BY updated_at DESC, userid ASC
LIMIT ?`
	args := make([]any, 0, len(normalizedNames)+2)
	args = append(args, normalizedEnterpriseID)
	for _, name := range normalizedNames {
		args = append(args, name)
	}
	args = append(args, clampLimit(limit))
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]weworkuserinfo.InternalUserCandidate, 0)
	seenUserIDs := make(map[string]bool)
	for rows.Next() {
		var ent any
		var userID any
		var name any
		var departments any
		var position any
		var avatar any
		var syncedAt any
		var updatedAt any
		if err := rows.Scan(&ent, &userID, &name, &departments, &position, &avatar, &syncedAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedUserID := stringFromDB(userID)
		if normalizedUserID == "" || seenUserIDs[normalizedUserID] {
			continue
		}
		seenUserIDs[normalizedUserID] = true
		results = append(results, weworkuserinfo.InternalUserCandidate{
			EnterpriseID:   stringFromDB(ent),
			UserID:         normalizedUserID,
			Name:           stringFromDB(name),
			DepartmentJSON: jsonArrayFromDB(departments),
			Position:       stringFromDB(position),
			Avatar:         stringFromDB(avatar),
			SyncedAt:       stringFromDB(syncedAt),
			UpdatedAt:      stringFromDB(updatedAt),
		})
	}
	return results, rows.Err()
}

// GetInternalUserByUserID returns one local corp user for manual userid repair.
func (repository *Repository) GetInternalUserByUserID(ctx context.Context, enterpriseID string, userID string) (weworkuserinfo.InternalUserCandidate, bool, error) {
	if repository.DB == nil {
		return weworkuserinfo.InternalUserCandidate{}, false, fmt.Errorf("contact cache database is not configured")
	}
	normalizedEnterpriseID := strings.TrimSpace(enterpriseID)
	normalizedUserID := strings.TrimSpace(userID)
	if normalizedEnterpriseID == "" || normalizedUserID == "" {
		return weworkuserinfo.InternalUserCandidate{}, false, nil
	}
	exists, err := repository.tableExists(ctx, "wework_corp_users")
	if err != nil {
		return weworkuserinfo.InternalUserCandidate{}, false, err
	}
	if !exists {
		return weworkuserinfo.InternalUserCandidate{}, false, nil
	}
	rows, err := repository.DB.QueryContext(ctx, `
SELECT
    enterprise_id,
    userid,
    name,
    department_json,
    position,
    avatar,
    synced_at,
    updated_at
FROM wework_corp_users
WHERE enterprise_id = ?
  AND userid = ?
  AND stale = 0
LIMIT 1`, normalizedEnterpriseID, normalizedUserID)
	if err != nil {
		return weworkuserinfo.InternalUserCandidate{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return weworkuserinfo.InternalUserCandidate{}, false, rows.Err()
	}
	var ent any
	var userid any
	var name any
	var departments any
	var position any
	var avatar any
	var syncedAt any
	var updatedAt any
	if err := rows.Scan(&ent, &userid, &name, &departments, &position, &avatar, &syncedAt, &updatedAt); err != nil {
		return weworkuserinfo.InternalUserCandidate{}, false, err
	}
	candidate := weworkuserinfo.InternalUserCandidate{
		EnterpriseID:   stringFromDB(ent),
		UserID:         stringFromDB(userid),
		Name:           stringFromDB(name),
		DepartmentJSON: jsonArrayFromDB(departments),
		Position:       stringFromDB(position),
		Avatar:         stringFromDB(avatar),
		SyncedAt:       stringFromDB(syncedAt),
		UpdatedAt:      stringFromDB(updatedAt),
	}
	if strings.TrimSpace(candidate.UserID) == "" {
		return weworkuserinfo.InternalUserCandidate{}, false, rows.Err()
	}
	return candidate, true, rows.Err()
}

func (repository *Repository) serializeExternalContact(payload contacts.Payload) ([]any, bool) {
	enterpriseID := strings.TrimSpace(stringValue(payload["enterprise_id"]))
	externalUserID := strings.TrimSpace(stringValue(payload["external_userid"]))
	if enterpriseID == "" || externalUserID == "" {
		return nil, false
	}
	now := repository.now().Format(time.RFC3339Nano)
	syncedAt := defaultString(stringValue(payload["synced_at"]), now)
	createdAt := defaultString(stringValue(payload["created_at"]), now)
	updatedAt := defaultString(stringValue(payload["updated_at"]), now)
	return []any{
		enterpriseID,
		externalUserID,
		strings.TrimSpace(stringValue(payload["name"])),
		strings.TrimSpace(stringValue(payload["avatar"])),
		intFromDB(payload["type"]),
		intFromDB(payload["gender"]),
		strings.TrimSpace(stringValue(payload["unionid"])),
		strings.TrimSpace(stringValue(payload["position"])),
		strings.TrimSpace(stringValue(payload["corp_name"])),
		strings.TrimSpace(stringValue(payload["corp_full_name"])),
		jsonText(payload["external_profile_json"], map[string]any{}),
		jsonText(payload["follow_users_json"], []any{}),
		jsonText(payload["tags_json"], []any{}),
		strings.TrimSpace(stringValue(payload["add_way"])),
		nilIfBlank(payload["add_time"]),
		syncedAt,
		boolInt(payload["stale"]),
		defaultString(stringValue(payload["source"]), "sync"),
		createdAt,
		updatedAt,
	}, true
}

func (repository *Repository) serializeCorpUser(payload contacts.Payload) ([]any, bool) {
	enterpriseID := strings.TrimSpace(stringValue(payload["enterprise_id"]))
	userID := strings.TrimSpace(stringValue(payload["userid"]))
	if enterpriseID == "" || userID == "" {
		return nil, false
	}
	now := repository.now().Format(time.RFC3339Nano)
	syncedAt := defaultString(stringValue(payload["synced_at"]), now)
	createdAt := defaultString(stringValue(payload["created_at"]), now)
	updatedAt := defaultString(stringValue(payload["updated_at"]), now)
	return []any{
		enterpriseID,
		userID,
		strings.TrimSpace(stringValue(payload["name"])),
		jsonText(payload["department_json"], []any{}),
		strings.TrimSpace(stringValue(payload["position"])),
		strings.TrimSpace(stringValue(payload["mobile"])),
		intFromDB(payload["gender"]),
		strings.TrimSpace(stringValue(payload["email"])),
		strings.TrimSpace(stringValue(payload["biz_mail"])),
		strings.TrimSpace(stringValue(payload["avatar"])),
		intFromDB(payload["status"]),
		jsonText(payload["extattr_json"], map[string]any{}),
		syncedAt,
		boolInt(payload["stale"]),
		defaultString(stringValue(payload["source"]), "sync"),
		createdAt,
		updatedAt,
	}, true
}

func scanExternalContactPayload(rows RowsScanner) (contacts.Payload, error) {
	var enterprise any
	var external any
	var name any
	var avatar any
	var contactType any
	var gender any
	var unionID any
	var position any
	var corpName any
	var corpFullName any
	var externalProfileJSON any
	var followUsersJSON any
	var tagsJSON any
	var addWay any
	var addTime any
	var syncedAt any
	var stale any
	var source any
	var createdAt any
	var updatedAt any
	if err := rows.Scan(&enterprise, &external, &name, &avatar, &contactType, &gender, &unionID, &position, &corpName, &corpFullName, &externalProfileJSON, &followUsersJSON, &tagsJSON, &addWay, &addTime, &syncedAt, &stale, &source, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return contacts.Payload{
		"enterprise_id":         stringFromDB(enterprise),
		"external_userid":       stringFromDB(external),
		"name":                  stringFromDB(name),
		"avatar":                stringFromDB(avatar),
		"type":                  intFromDB(contactType),
		"gender":                intFromDB(gender),
		"unionid":               stringFromDB(unionID),
		"position":              stringFromDB(position),
		"corp_name":             stringFromDB(corpName),
		"corp_full_name":        stringFromDB(corpFullName),
		"external_profile_json": jsonObjectFromDB(externalProfileJSON),
		"follow_users_json":     jsonArrayFromDB(followUsersJSON),
		"tags_json":             jsonArrayFromDB(tagsJSON),
		"add_way":               stringFromDB(addWay),
		"add_time":              nilIfBlank(addTime),
		"synced_at":             nilIfBlank(syncedAt),
		"stale":                 boolFromDB(stale),
		"source":                stringFromDB(source),
		"created_at":            nilIfBlank(createdAt),
		"updated_at":            nilIfBlank(updatedAt),
	}, nil
}

func scanCorpUserPayload(rows RowsScanner) (contacts.Payload, error) {
	var enterprise any
	var user any
	var name any
	var departments any
	var position any
	var mobile any
	var gender any
	var email any
	var bizMail any
	var avatar any
	var status any
	var extattr any
	var syncedAt any
	var stale any
	var source any
	var createdAt any
	var updatedAt any
	if err := rows.Scan(&enterprise, &user, &name, &departments, &position, &mobile, &gender, &email, &bizMail, &avatar, &status, &extattr, &syncedAt, &stale, &source, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return contacts.Payload{
		"enterprise_id":   stringFromDB(enterprise),
		"userid":          stringFromDB(user),
		"name":            stringFromDB(name),
		"department_json": jsonArrayFromDB(departments),
		"position":        stringFromDB(position),
		"mobile":          stringFromDB(mobile),
		"gender":          intFromDB(gender),
		"email":           stringFromDB(email),
		"biz_mail":        stringFromDB(bizMail),
		"avatar":          stringFromDB(avatar),
		"status":          intFromDB(status),
		"extattr_json":    jsonObjectFromDB(extattr),
		"synced_at":       nilIfBlank(syncedAt),
		"stale":           boolFromDB(stale),
		"source":          stringFromDB(source),
		"created_at":      nilIfBlank(createdAt),
		"updated_at":      nilIfBlank(updatedAt),
	}, nil
}

func isStalePayload(payload contacts.Payload, threshold time.Time) bool {
	if boolFromDB(payload["stale"]) {
		return true
	}
	syncedAt := contacts.FirstEventTime(payload["synced_at"])
	return syncedAt.IsZero() || !syncedAt.After(threshold.UTC())
}

func (repository *Repository) upsertExternalContactSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") || strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgresql") {
		return `
INSERT INTO wework_external_contacts (
    enterprise_id, external_userid, name, avatar, type, gender, unionid, position,
    corp_name, corp_full_name, external_profile_json, follow_users_json, tags_json,
    add_way, add_time, synced_at, stale, source, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(enterprise_id, external_userid) DO UPDATE SET
    name = EXCLUDED.name,
    avatar = EXCLUDED.avatar,
    type = EXCLUDED.type,
    gender = EXCLUDED.gender,
    unionid = EXCLUDED.unionid,
    position = EXCLUDED.position,
    corp_name = EXCLUDED.corp_name,
    corp_full_name = EXCLUDED.corp_full_name,
    external_profile_json = EXCLUDED.external_profile_json,
    follow_users_json = EXCLUDED.follow_users_json,
    tags_json = EXCLUDED.tags_json,
    add_way = EXCLUDED.add_way,
    add_time = EXCLUDED.add_time,
    synced_at = EXCLUDED.synced_at,
    stale = EXCLUDED.stale,
    source = EXCLUDED.source,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO wework_external_contacts (
    enterprise_id, external_userid, name, avatar, type, gender, unionid, position,
    corp_name, corp_full_name, external_profile_json, follow_users_json, tags_json,
    add_way, add_time, synced_at, stale, source, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    name = VALUES(name),
    avatar = VALUES(avatar),
    type = VALUES(type),
    gender = VALUES(gender),
    unionid = VALUES(unionid),
    position = VALUES(position),
    corp_name = VALUES(corp_name),
    corp_full_name = VALUES(corp_full_name),
    external_profile_json = VALUES(external_profile_json),
    follow_users_json = VALUES(follow_users_json),
    tags_json = VALUES(tags_json),
    add_way = VALUES(add_way),
    add_time = VALUES(add_time),
    synced_at = VALUES(synced_at),
    stale = VALUES(stale),
    source = VALUES(source),
    updated_at = VALUES(updated_at)`
}

func (repository *Repository) upsertCorpUserSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") || strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgresql") {
		return `
INSERT INTO wework_corp_users (
    enterprise_id, userid, name, department_json, position, mobile, gender, email,
    biz_mail, avatar, status, extattr_json, synced_at, stale, source, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(enterprise_id, userid) DO UPDATE SET
    name = EXCLUDED.name,
    department_json = EXCLUDED.department_json,
    position = EXCLUDED.position,
    mobile = EXCLUDED.mobile,
    gender = EXCLUDED.gender,
    email = EXCLUDED.email,
    biz_mail = EXCLUDED.biz_mail,
    avatar = EXCLUDED.avatar,
    status = EXCLUDED.status,
    extattr_json = EXCLUDED.extattr_json,
    synced_at = EXCLUDED.synced_at,
    stale = EXCLUDED.stale,
    source = EXCLUDED.source,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO wework_corp_users (
    enterprise_id, userid, name, department_json, position, mobile, gender, email,
    biz_mail, avatar, status, extattr_json, synced_at, stale, source, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    name = VALUES(name),
    department_json = VALUES(department_json),
    position = VALUES(position),
    mobile = VALUES(mobile),
    gender = VALUES(gender),
    email = VALUES(email),
    biz_mail = VALUES(biz_mail),
    avatar = VALUES(avatar),
    status = VALUES(status),
    extattr_json = VALUES(extattr_json),
    synced_at = VALUES(synced_at),
    stale = VALUES(stale),
    source = VALUES(source),
    updated_at = VALUES(updated_at)`
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
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

func normalizeNames(names []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		normalized = append(normalized, name)
	}
	return normalized
}

func placeholders(count int) string {
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ", ")
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func positiveLimit(limit int) int {
	if limit <= 0 {
		return 1
	}
	return limit
}

func positiveMaxAgeHours(hours int) int {
	if hours <= 0 {
		return 24
	}
	return hours
}

func jsonText(value any, fallback any) string {
	if value == nil {
		value = fallback
	}
	data, err := json.Marshal(value)
	if err != nil {
		data, _ = json.Marshal(fallback)
	}
	return string(data)
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func boolInt(value any) int {
	if boolFromDB(value) {
		return 1
	}
	return 0
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
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

func jsonArrayFromDB(value any) []any {
	text := stringFromDB(value)
	if text == "" {
		return []any{}
	}
	var parsed []any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return []any{}
	}
	return parsed
}

func nilIfBlank(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		text := strings.TrimSpace(string(typed))
		if text == "" {
			return nil
		}
		return text
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return text
	default:
		return typed
	}
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

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseIntText(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("contact cache database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("contact cache database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
