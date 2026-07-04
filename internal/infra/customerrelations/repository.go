// Package customerrelations reads customer-member relation state for send guards.
package customerrelations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"wework-go/internal/conversationreply"
	"wework-go/internal/customerrelation"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the database/sql single-row shape used by Repository.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by reconcile queries.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Execer is the database/sql write shape used by Repository.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads customer_member_relations rows keyed by enterprise/member/customer.
type Repository struct {
	DB     Queryer
	ExecDB Execer
	Now    func() time.Time
}

// FollowUserReconcileInput describes the current externalcontact/get follow_user set.
type FollowUserReconcileInput struct {
	EnterpriseID   string
	ExternalUserID string
	FollowUserIDs  []string
	EventTime      time.Time
	Source         string
}

// FollowUserReconcileResult summarizes relation repair writes.
type FollowUserReconcileResult struct {
	EnterpriseID       string
	ExternalUserID     string
	CurrentFollowUsers int
	ActivatedRelations int
	RestoredRelations  int
	DeletedRelations   int
	IgnoredStaleEvents int
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	queryer := sqlQueryer{db: db}
	return &Repository{DB: queryer, ExecDB: queryer}
}

// GetCustomerRelation returns the latest relation row for one scoped customer.
func (repository *Repository) GetCustomerRelation(ctx context.Context, key conversationreply.CustomerRelationKey) (conversationreply.CustomerRelationSnapshot, bool, error) {
	if repository.DB == nil {
		return conversationreply.CustomerRelationSnapshot{}, false, fmt.Errorf("customer relation database is not configured")
	}
	normalized := normalizeKey(key)
	if normalized.EnterpriseID == "" || normalized.WeWorkUserID == "" || normalized.ExternalUserID == "" {
		return conversationreply.CustomerRelationSnapshot{}, false, nil
	}
	var status any
	var deletedAt any
	var restoredAt any
	err := repository.DB.QueryRowContext(ctx, `
SELECT relation_status, deleted_at, restored_at
FROM customer_member_relations
WHERE enterprise_id = ? AND wework_user_id = ? AND external_userid = ?
LIMIT 1`,
		normalized.EnterpriseID,
		normalized.WeWorkUserID,
		normalized.ExternalUserID,
	).Scan(&status, &deletedAt, &restoredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return conversationreply.CustomerRelationSnapshot{}, false, nil
	}
	if err != nil {
		return conversationreply.CustomerRelationSnapshot{}, false, err
	}
	relationStatus := strings.ToLower(stringFromDB(status))
	return conversationreply.CustomerRelationSnapshot{
		Status:               relationStatus,
		DeletedCurrentMember: relationStatus == "deleted_by_customer",
		DeletedAt:            formatBeijingAPIISO(deletedAt),
		RestoredAt:           formatBeijingAPIISO(restoredAt),
	}, true, nil
}

// UpsertEvent applies a callback relation event with stale-event protection.
func (repository *Repository) UpsertEvent(ctx context.Context, event customerrelation.Event) (customerrelation.RelationRow, error) {
	if repository.DB == nil || repository.ExecDB == nil {
		return customerrelation.RelationRow{}, fmt.Errorf("customer relation database is not configured")
	}
	normalized := customerrelation.Event{
		EnterpriseID:   strings.TrimSpace(event.EnterpriseID),
		WeWorkUserID:   normalizeRelationWeWorkUserID(event.WeWorkUserID),
		ExternalUserID: normalizeRelationExternalUserID(event.ExternalUserID),
		EventType:      strings.TrimSpace(event.EventType),
		ChangeType:     strings.TrimSpace(event.ChangeType),
		EventTime:      event.EventTime.UTC(),
		RawEventHash:   strings.TrimSpace(event.RawEventHash),
		Source:         defaultText(event.Source, "callback"),
	}
	if normalized.EnterpriseID == "" || normalized.WeWorkUserID == "" || normalized.ExternalUserID == "" {
		return customerrelation.RelationRow{}, fmt.Errorf("enterprise_id, wework_user_id and external_userid are required")
	}
	if normalized.EventTime.IsZero() {
		normalized.EventTime = repository.now()
	}
	nextStatus, deletedAt, restoredAt, lastSeenAt, err := nextRelationState(normalized.ChangeType, normalized.EventTime)
	if err != nil {
		return customerrelation.RelationRow{}, err
	}
	current, found, err := repository.loadCurrent(ctx, normalized.EnterpriseID, normalized.WeWorkUserID, normalized.ExternalUserID)
	if err != nil {
		return customerrelation.RelationRow{}, err
	}
	if found && current.LastEventAt != nil && normalized.EventTime.Before(current.LastEventAt.UTC()) {
		current.StateChanged = false
		current.IgnoredStale = true
		return current, nil
	}
	stateChanged := !found || strings.TrimSpace(current.RelationStatus) != nextStatus
	relationFirstAdd := !found && normalized.ChangeType == customerrelation.ChangeTypeAddExternalContact
	now := repository.now()
	if !found {
		if _, err := repository.ExecDB.ExecContext(ctx, `
INSERT INTO customer_member_relations (
	enterprise_id, wework_user_id, external_userid, relation_status, source,
	event_type, change_type, deleted_at, restored_at, last_seen_at,
	last_event_at, raw_event_hash, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			normalized.EnterpriseID,
			normalized.WeWorkUserID,
			normalized.ExternalUserID,
			nextStatus,
			normalized.Source,
			normalized.EventType,
			normalized.ChangeType,
			timeOrNil(deletedAt),
			timeOrNil(restoredAt),
			timeOrNil(lastSeenAt),
			normalized.EventTime,
			normalized.RawEventHash,
			now,
			now,
		); err != nil {
			return customerrelation.RelationRow{}, err
		}
	} else {
		if _, err := repository.ExecDB.ExecContext(ctx, `
UPDATE customer_member_relations
SET relation_status = ?,
	source = ?,
	event_type = ?,
	change_type = ?,
	deleted_at = ?,
	restored_at = ?,
	last_seen_at = ?,
	last_event_at = ?,
	raw_event_hash = ?,
	updated_at = ?
WHERE enterprise_id = ? AND wework_user_id = ? AND external_userid = ?`,
			nextStatus,
			normalized.Source,
			normalized.EventType,
			normalized.ChangeType,
			timeOrNil(deletedAt),
			timeOrNil(restoredAt),
			timeOrNil(lastSeenAt),
			normalized.EventTime,
			normalized.RawEventHash,
			now,
			normalized.EnterpriseID,
			normalized.WeWorkUserID,
			normalized.ExternalUserID,
		); err != nil {
			return customerrelation.RelationRow{}, err
		}
	}
	return customerrelation.RelationRow{
		EnterpriseID:     normalized.EnterpriseID,
		WeWorkUserID:     normalized.WeWorkUserID,
		ExternalUserID:   normalized.ExternalUserID,
		RelationStatus:   nextStatus,
		Source:           normalized.Source,
		EventType:        normalized.EventType,
		ChangeType:       normalized.ChangeType,
		DeletedAt:        deletedAt,
		RestoredAt:       restoredAt,
		LastSeenAt:       lastSeenAt,
		LastEventAt:      &normalized.EventTime,
		RawEventHash:     normalized.RawEventHash,
		StateChanged:     stateChanged,
		IgnoredStale:     false,
		RelationFirstAdd: relationFirstAdd,
	}, nil
}

// ReconcileExternalContactFollowUsers repairs relation rows from externalcontact/get follow_user data.
func (repository *Repository) ReconcileExternalContactFollowUsers(ctx context.Context, input FollowUserReconcileInput) (FollowUserReconcileResult, error) {
	if repository.DB == nil || repository.ExecDB == nil {
		return FollowUserReconcileResult{}, fmt.Errorf("customer relation database is not configured")
	}
	enterpriseID := strings.TrimSpace(input.EnterpriseID)
	externalUserID := normalizeRelationExternalUserID(input.ExternalUserID)
	if enterpriseID == "" || externalUserID == "" {
		return FollowUserReconcileResult{}, fmt.Errorf("enterprise_id and external_userid are required")
	}
	currentFollowUserIDs := normalizeFollowUserIDs(input.FollowUserIDs)
	activeUserIDs, err := repository.listActiveFollowUsers(ctx, enterpriseID, externalUserID)
	if err != nil {
		return FollowUserReconcileResult{}, err
	}
	currentSet := map[string]bool{}
	for _, userID := range currentFollowUserIDs {
		currentSet[userID] = true
	}
	deletedUserIDs := make([]string, 0)
	for _, userID := range activeUserIDs {
		if !currentSet[userID] {
			deletedUserIDs = append(deletedUserIDs, userID)
		}
	}
	sort.Strings(deletedUserIDs)

	result := FollowUserReconcileResult{
		EnterpriseID:       enterpriseID,
		ExternalUserID:     externalUserID,
		CurrentFollowUsers: len(currentFollowUserIDs),
	}
	eventTime := input.EventTime
	if eventTime.IsZero() {
		eventTime = repository.now()
	}
	source := defaultText(input.Source, "external_contact_sync_reconcile")
	for _, userID := range currentFollowUserIDs {
		row, err := repository.UpsertEvent(ctx, customerrelation.Event{
			EnterpriseID:   enterpriseID,
			WeWorkUserID:   userID,
			ExternalUserID: externalUserID,
			EventType:      "external_contact_sync",
			ChangeType:     customerrelation.ChangeTypeAddExternalContact,
			EventTime:      eventTime,
			Source:         source,
		})
		if err != nil {
			return FollowUserReconcileResult{}, err
		}
		if row.IgnoredStale {
			result.IgnoredStaleEvents++
			continue
		}
		if row.RelationFirstAdd {
			result.ActivatedRelations++
		} else if row.StateChanged {
			result.RestoredRelations++
		}
	}
	for _, userID := range deletedUserIDs {
		row, err := repository.UpsertEvent(ctx, customerrelation.Event{
			EnterpriseID:   enterpriseID,
			WeWorkUserID:   userID,
			ExternalUserID: externalUserID,
			EventType:      "external_contact_sync",
			ChangeType:     customerrelation.ChangeTypeDelFollowUser,
			EventTime:      eventTime,
			Source:         source,
		})
		if err != nil {
			return FollowUserReconcileResult{}, err
		}
		if row.IgnoredStale {
			result.IgnoredStaleEvents++
			continue
		}
		if row.StateChanged {
			result.DeletedRelations++
		}
	}
	return result, nil
}

func (repository *Repository) listActiveFollowUsers(ctx context.Context, enterpriseID string, externalUserID string) ([]string, error) {
	rows, err := repository.DB.QueryContext(ctx, `
SELECT wework_user_id
FROM customer_member_relations
WHERE enterprise_id = ? AND external_userid = ? AND relation_status = ?`,
		enterpriseID,
		externalUserID,
		customerrelation.RelationStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	userIDs := make([]string, 0)
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		userID := normalizeRelationWeWorkUserID(stringFromDB(raw))
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	return userIDs, rows.Err()
}

func (repository *Repository) loadCurrent(ctx context.Context, enterpriseID string, weworkUserID string, externalUserID string) (customerrelation.RelationRow, bool, error) {
	var status any
	var source any
	var eventType any
	var changeType any
	var deletedAt any
	var restoredAt any
	var lastSeenAt any
	var lastEventAt any
	var rawEventHash any
	err := repository.DB.QueryRowContext(ctx, `
SELECT relation_status, source, event_type, change_type, deleted_at, restored_at, last_seen_at, last_event_at, raw_event_hash
FROM customer_member_relations
WHERE enterprise_id = ? AND wework_user_id = ? AND external_userid = ?
LIMIT 1`,
		enterpriseID,
		weworkUserID,
		externalUserID,
	).Scan(&status, &source, &eventType, &changeType, &deletedAt, &restoredAt, &lastSeenAt, &lastEventAt, &rawEventHash)
	if errors.Is(err, sql.ErrNoRows) {
		return customerrelation.RelationRow{}, false, nil
	}
	if err != nil {
		return customerrelation.RelationRow{}, false, err
	}
	return customerrelation.RelationRow{
		EnterpriseID:   enterpriseID,
		WeWorkUserID:   weworkUserID,
		ExternalUserID: externalUserID,
		RelationStatus: stringFromDB(status),
		Source:         stringFromDB(source),
		EventType:      stringFromDB(eventType),
		ChangeType:     stringFromDB(changeType),
		DeletedAt:      optionalTime(deletedAt),
		RestoredAt:     optionalTime(restoredAt),
		LastSeenAt:     optionalTime(lastSeenAt),
		LastEventAt:    optionalTime(lastEventAt),
		RawEventHash:   stringFromDB(rawEventHash),
	}, true, nil
}

func normalizeKey(key conversationreply.CustomerRelationKey) conversationreply.CustomerRelationKey {
	return conversationreply.CustomerRelationKey{
		EnterpriseID:   strings.TrimSpace(key.EnterpriseID),
		WeWorkUserID:   normalizeRelationWeWorkUserID(key.WeWorkUserID),
		ExternalUserID: normalizeRelationExternalUserID(key.ExternalUserID),
	}
}

func normalizeRelationWeWorkUserID(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func normalizeRelationExternalUserID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFollowUserIDs(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		userID := normalizeRelationWeWorkUserID(value)
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		normalized = append(normalized, userID)
	}
	sort.Strings(normalized)
	return normalized
}

func nextRelationState(changeType string, eventTime time.Time) (string, *time.Time, *time.Time, *time.Time, error) {
	switch strings.TrimSpace(changeType) {
	case customerrelation.ChangeTypeDelExternalContact, customerrelation.ChangeTypeDelFollowUser:
		eventTime = eventTime.UTC()
		return customerrelation.RelationStatusDeletedByCustomer, &eventTime, nil, nil, nil
	case customerrelation.ChangeTypeAddExternalContact:
		eventTime = eventTime.UTC()
		return customerrelation.RelationStatusActive, nil, &eventTime, &eventTime, nil
	default:
		return "", nil, nil, nil, fmt.Errorf("%w: %s", customerrelation.ErrUnsupportedRelationChangeType, changeType)
	}
}

func timeOrNil(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func optionalTime(value any) *time.Time {
	parsed, ok := parseDBTime(value)
	if !ok {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case sql.NullString:
		if !typed.Valid {
			return ""
		}
		return strings.TrimSpace(typed.String)
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func formatBeijingAPIISO(value any) string {
	parsed, ok := parseDBTime(value)
	if !ok {
		return ""
	}
	return parsed.In(beijingLocation).Format(time.RFC3339)
}

func parseDBTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		if typed.IsZero() {
			return time.Time{}, false
		}
		return typed, true
	case sql.NullTime:
		if !typed.Valid || typed.Time.IsZero() {
			return time.Time{}, false
		}
		return typed.Time, true
	case sql.NullString:
		if !typed.Valid {
			return time.Time{}, false
		}
		return parseDBTimeString(typed.String)
	case []byte:
		return parseDBTimeString(string(typed))
	case string:
		return parseDBTimeString(typed)
	default:
		return parseDBTimeString(fmt.Sprint(typed))
	}
}

func parseDBTimeString(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed, true
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("customer relation database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}
