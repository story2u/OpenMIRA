// Package archiveraw adapts the legacy archive_raw_messages table.
package archiveraw

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by list queries.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Record mirrors Python ArchiveRawMessageRecord.
type Record struct {
	RecordID          string
	EnterpriseID      string
	Source            string
	ArchiveMsgID      string
	Seq               int64
	Action            string
	FromID            string
	ToList            string
	RoomID            string
	MsgTypeRaw        string
	SDKFileID         string
	RawJSON           string
	DecryptStartedAt  *time.Time
	DecryptFinishedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// UpsertInput describes one raw archive row write.
type UpsertInput struct {
	EnterpriseID      string
	Source            string
	ArchiveMsgID      string
	Seq               int64
	Action            string
	FromID            string
	ToList            []string
	RoomID            string
	MsgTypeRaw        string
	SDKFileID         string
	RawJSON           map[string]any
	DecryptStartedAt  *time.Time
	DecryptFinishedAt *time.Time
	SkipRecordReload  bool
}

// Repository reads and writes archive_raw_messages rows.
type Repository struct {
	DB           Queryer
	Dialect      string
	Now          func() time.Time
	NextRecordID func() string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// UpsertRawMessage upserts one raw archive message by enterprise/source/archive_msgid.
func (repository *Repository) UpsertRawMessage(ctx context.Context, input UpsertInput) (bool, *Record, error) {
	if repository.DB == nil {
		return false, nil, fmt.Errorf("archive raw database is not configured")
	}
	ent := normalizeEnterpriseID(input.EnterpriseID)
	source := normalizeSource(input.Source)
	msgID := strings.TrimSpace(input.ArchiveMsgID)
	if msgID == "" {
		return false, nil, fmt.Errorf("archive_msgid is required")
	}
	_, found, err := repository.recordIDByIdentity(ctx, ent, source, msgID)
	if err != nil {
		return false, nil, err
	}
	toList := input.ToList
	if toList == nil {
		toList = []string{}
	}
	toListJSON, err := jsonString(toList)
	if err != nil {
		return false, nil, err
	}
	rawPayload := input.RawJSON
	if rawPayload == nil {
		rawPayload = map[string]any{}
	}
	rawJSON, err := jsonString(rawPayload)
	if err != nil {
		return false, nil, err
	}
	now := repository.dbNowParam()
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		repository.nextRecordID(),
		ent,
		source,
		msgID,
		maxInt64(0, input.Seq),
		strings.TrimSpace(input.Action),
		strings.TrimSpace(input.FromID),
		toListJSON,
		strings.TrimSpace(input.RoomID),
		strings.TrimSpace(input.MsgTypeRaw),
		strings.TrimSpace(input.SDKFileID),
		rawJSON,
		repository.dbNullableTimeParam(input.DecryptStartedAt),
		repository.dbNullableTimeParam(input.DecryptFinishedAt),
		now,
		now,
	)
	if err != nil {
		return false, nil, err
	}
	created := !found
	if input.SkipRecordReload {
		return created, nil, nil
	}
	record, err := repository.GetByIdentity(ctx, ent, source, msgID)
	if err != nil {
		return created, nil, err
	}
	if record == nil {
		return created, nil, fmt.Errorf("archive raw upsert failed")
	}
	return created, record, nil
}

// GetByIdentity returns one raw archive message by unique identity.
func (repository *Repository) GetByIdentity(ctx context.Context, enterpriseID string, source string, archiveMsgID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive raw database is not configured")
	}
	ent := normalizeEnterpriseID(enterpriseID)
	src := normalizeSource(source)
	msgID := strings.TrimSpace(archiveMsgID)
	if msgID == "" {
		return nil, nil
	}
	row := repository.DB.QueryRowContext(ctx, "SELECT "+recordColumnSQL("")+" FROM archive_raw_messages WHERE enterprise_id = ? AND source = ? AND archive_msgid = ?", ent, src, msgID)
	record, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// ListByArchiveMsgIDs returns raw archive records for archive messages, newest first.
func (repository *Repository) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive raw database is not configured")
	}
	ids := normalizeArchiveMsgIDs(archiveMsgIDs)
	if len(ids) == 0 {
		return []Record{}, nil
	}
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	query := "SELECT " + recordColumnSQL("") + " FROM archive_raw_messages WHERE archive_msgid IN (" + placeholders(len(ids)) + ")"
	if ent := strings.TrimSpace(enterpriseID); ent != "" {
		query += " AND enterprise_id = ?"
		args = append(args, ent)
	}
	query += " ORDER BY updated_at DESC, created_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// MarkDecryptStarted records the first decrypt start timestamp.
func (repository *Repository) MarkDecryptStarted(ctx context.Context, enterpriseID string, source string, archiveMsgID string, startedAt *time.Time) (*Record, error) {
	return repository.markDecryptTime(ctx, "decrypt_started_at", enterpriseID, source, archiveMsgID, startedAt)
}

// MarkDecryptFinished records the first decrypt finished timestamp.
func (repository *Repository) MarkDecryptFinished(ctx context.Context, enterpriseID string, source string, archiveMsgID string, finishedAt *time.Time) (*Record, error) {
	return repository.markDecryptTime(ctx, "decrypt_finished_at", enterpriseID, source, archiveMsgID, finishedAt)
}

// LatestSeq returns the highest raw archive sequence for one scope.
func (repository *Repository) LatestSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive raw database is not configured")
	}
	var seq any
	err := repository.DB.QueryRowContext(ctx, repository.latestSeqSQL(), normalizeEnterpriseID(enterpriseID), normalizeSource(source)).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return intValue(seq), nil
}

// PruneBefore deletes old raw archive rows by created_at.
func (repository *Repository) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive raw database is not configured")
	}
	if batchSize < 1 {
		batchSize = 1
	}
	rows, err := repository.DB.QueryContext(ctx, selectPruneRecordIDsSQL, repository.dbTimeParam(cutoff), batchSize)
	if err != nil {
		return 0, err
	}
	recordIDs := make([]string, 0, batchSize)
	for rows.Next() {
		var recordID any
		if err := rows.Scan(&recordID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if value := strings.TrimSpace(textValue(recordID)); value != "" {
			recordIDs = append(recordIDs, value)
		}
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if len(recordIDs) == 0 {
		return 0, nil
	}
	_, err = repository.DB.ExecContext(ctx, "DELETE FROM archive_raw_messages WHERE record_id IN ("+placeholders(len(recordIDs))+")", stringsToAny(recordIDs)...)
	if err != nil {
		return 0, err
	}
	return len(recordIDs), nil
}

func (repository *Repository) markDecryptTime(ctx context.Context, column string, enterpriseID string, source string, archiveMsgID string, value *time.Time) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive raw database is not configured")
	}
	if column != "decrypt_started_at" && column != "decrypt_finished_at" {
		return nil, fmt.Errorf("unsupported decrypt timestamp column")
	}
	ent := normalizeEnterpriseID(enterpriseID)
	src := normalizeSource(source)
	msgID := strings.TrimSpace(archiveMsgID)
	if msgID == "" {
		return nil, fmt.Errorf("archive_msgid is required")
	}
	at := repository.now()
	if value != nil {
		at = value.UTC()
	}
	_, err := repository.DB.ExecContext(ctx, `
UPDATE archive_raw_messages
SET `+column+` = COALESCE(`+column+`, ?),
    updated_at = ?
WHERE enterprise_id = ? AND source = ? AND archive_msgid = ?`,
		repository.dbTimeParam(at),
		repository.dbNowParam(),
		ent,
		src,
		msgID,
	)
	if err != nil {
		return nil, err
	}
	return repository.GetByIdentity(ctx, ent, src, msgID)
}

func (repository *Repository) recordIDByIdentity(ctx context.Context, enterpriseID string, source string, archiveMsgID string) (string, bool, error) {
	var recordID any
	err := repository.DB.QueryRowContext(ctx, "SELECT record_id FROM archive_raw_messages WHERE enterprise_id = ? AND source = ? AND archive_msgid = ? LIMIT 1", enterpriseID, source, archiveMsgID).Scan(&recordID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(textValue(recordID)), true, nil
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO archive_raw_messages (
    record_id, enterprise_id, source, archive_msgid, seq, action,
    from_id, to_list, room_id, msg_type_raw, sdk_file_id, raw_json,
    decrypt_started_at, decrypt_finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    seq = VALUES(seq),
    action = VALUES(action),
    from_id = VALUES(from_id),
    to_list = VALUES(to_list),
    room_id = VALUES(room_id),
    msg_type_raw = VALUES(msg_type_raw),
    sdk_file_id = VALUES(sdk_file_id),
    raw_json = VALUES(raw_json),
    decrypt_started_at = COALESCE(decrypt_started_at, VALUES(decrypt_started_at)),
    decrypt_finished_at = COALESCE(decrypt_finished_at, VALUES(decrypt_finished_at)),
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO archive_raw_messages (
    record_id, enterprise_id, source, archive_msgid, seq, action,
    from_id, to_list, room_id, msg_type_raw, sdk_file_id, raw_json,
    decrypt_started_at, decrypt_finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(enterprise_id, source, archive_msgid) DO UPDATE SET
    seq=excluded.seq,
    action=excluded.action,
    from_id=excluded.from_id,
    to_list=excluded.to_list,
    room_id=excluded.room_id,
    msg_type_raw=excluded.msg_type_raw,
    sdk_file_id=excluded.sdk_file_id,
    raw_json=excluded.raw_json,
    decrypt_started_at=COALESCE(decrypt_started_at, excluded.decrypt_started_at),
    decrypt_finished_at=COALESCE(decrypt_finished_at, excluded.decrypt_finished_at),
    updated_at=excluded.updated_at`
}

func (repository *Repository) latestSeqSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
SELECT seq
FROM archive_raw_messages FORCE INDEX (idx_archive_raw_ent_source_seq)
WHERE enterprise_id = ? AND source = ?
ORDER BY seq DESC
LIMIT 1`
	}
	return `
SELECT seq
FROM archive_raw_messages
WHERE enterprise_id = ? AND source = ?
ORDER BY seq DESC
LIMIT 1`
}

const selectPruneRecordIDsSQL = `
SELECT record_id
FROM archive_raw_messages
WHERE created_at < ?
ORDER BY created_at ASC, record_id ASC
LIMIT ?`

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbNullableTimeParam(value *time.Time) any {
	if value == nil {
		return nil
	}
	return repository.dbTimeParam(*value)
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05-07:00")
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func (repository *Repository) nextRecordID() string {
	if repository.NextRecordID != nil {
		if value := strings.TrimSpace(repository.NextRecordID()); value != "" {
			return value
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("ar-%d", repository.now().UnixNano())
	}
	return "ar-" + hex.EncodeToString(random)
}

func recordColumnSQL(prefix string) string {
	columns := []string{
		"record_id",
		"enterprise_id",
		"source",
		"archive_msgid",
		"seq",
		"action",
		"from_id",
		"to_list",
		"room_id",
		"msg_type_raw",
		"sdk_file_id",
		"raw_json",
		"decrypt_started_at",
		"decrypt_finished_at",
		"created_at",
		"updated_at",
	}
	qualified := make([]string, 0, len(columns))
	for _, column := range columns {
		qualified = append(qualified, prefix+column)
	}
	return strings.Join(qualified, ", ")
}

func scanRecord(row RowScanner) (Record, error) {
	var recordID any
	var enterpriseID any
	var source any
	var archiveMsgID any
	var seq any
	var action any
	var fromID any
	var toList any
	var roomID any
	var msgTypeRaw any
	var sdkFileID any
	var rawJSON any
	var decryptStartedAt any
	var decryptFinishedAt any
	var createdAt any
	var updatedAt any
	if err := row.Scan(
		&recordID,
		&enterpriseID,
		&source,
		&archiveMsgID,
		&seq,
		&action,
		&fromID,
		&toList,
		&roomID,
		&msgTypeRaw,
		&sdkFileID,
		&rawJSON,
		&decryptStartedAt,
		&decryptFinishedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Record{}, err
	}
	return Record{
		RecordID:          strings.TrimSpace(textValue(recordID)),
		EnterpriseID:      normalizeEnterpriseID(textValue(enterpriseID)),
		Source:            normalizeSource(textValue(source)),
		ArchiveMsgID:      strings.TrimSpace(textValue(archiveMsgID)),
		Seq:               intValue(seq),
		Action:            strings.TrimSpace(textValue(action)),
		FromID:            strings.TrimSpace(textValue(fromID)),
		ToList:            textValue(toList),
		RoomID:            strings.TrimSpace(textValue(roomID)),
		MsgTypeRaw:        strings.TrimSpace(textValue(msgTypeRaw)),
		SDKFileID:         strings.TrimSpace(textValue(sdkFileID)),
		RawJSON:           textValue(rawJSON),
		DecryptStartedAt:  nullableDBTime(decryptStartedAt),
		DecryptFinishedAt: nullableDBTime(decryptFinishedAt),
		CreatedAt:         parseDBTime(createdAt),
		UpdatedAt:         parseDBTime(updatedAt),
	}, nil
}

func scanRecords(rows RowsScanner) ([]Record, error) {
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
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

func normalizeArchiveMsgIDs(values []string) []string {
	seen := map[string]struct{}{}
	output := []string{}
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		output = append(output, text)
	}
	return output
}

func placeholders(count int) string {
	if count < 1 {
		return ""
	}
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ", ")
}

func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func jsonString(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func nullableDBTime(value any) *time.Time {
	if value == nil {
		return nil
	}
	if typed, ok := value.(sql.NullTime); ok {
		if !typed.Valid {
			return nil
		}
		parsed := parseDBTime(typed.Time)
		return &parsed
	}
	parsed := parseDBTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func parseDBTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseDBTimeString(string(typed))
	case string:
		return parseDBTimeString(typed)
	default:
		return time.Time{}
	}
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

func normalizeEnterpriseID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func normalizeSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "self_decrypt"
	}
	return value
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case sql.NullString:
		if !typed.Valid {
			return ""
		}
		return typed.String
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		value, _ := typed.Int64()
		return value
	case []byte:
		return parseIntString(string(typed))
	case string:
		return parseIntString(typed)
	default:
		return 0
	}
}

func parseIntString(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive raw database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive raw database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("archive raw database is not configured")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
