// Enterprises expose legacy enterprise configuration in read-only form.
// Writes, associated cleanup, and archive worker side effects stay with Python
// until the enterprise management path has separate write-contract coverage.
package workbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrEnterpriseStoreUnavailable means enterprises cannot be loaded.
	ErrEnterpriseStoreUnavailable = errors.New("workbench enterprise store is unavailable")
	// ErrEnterpriseCorpIDRequired preserves FastAPI's corp_id validation.
	ErrEnterpriseCorpIDRequired = errors.New("corp_id is required")
	// ErrEnterpriseNameRequired preserves FastAPI's name validation.
	ErrEnterpriseNameRequired = errors.New("name is required")
)

// EnterpriseStore loads legacy enterprise configuration rows.
type EnterpriseStore interface {
	ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error)
}

// EnterpriseWriteStore mutates admin-managed enterprise configuration rows.
type EnterpriseWriteStore interface {
	GetEnterprise(ctx context.Context, enterpriseID string) (EnterpriseRecord, bool, error)
	UpsertEnterprise(ctx context.Context, command EnterpriseUpsertCommand) (EnterpriseRecord, error)
	DeleteEnterprise(ctx context.Context, enterpriseID string) (bool, error)
}

// EnterpriseRecord mirrors the legacy EnterpriseRecord API fields.
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

// EnterpriseUpsertBody is the JSON input for POST /api/v1/admin/enterprises.
type EnterpriseUpsertBody struct {
	EnterpriseID               string `json:"enterprise_id"`
	CorpID                     string `json:"corp_id"`
	Name                       string `json:"name"`
	IncomingPrimaryMode        string `json:"incoming_primary_mode"`
	ArchiveMode                string `json:"archive_mode"`
	ArchiveSource              string `json:"archive_source"`
	ArchivePullURL             string `json:"archive_pull_url"`
	ArchivePullToken           string `json:"archive_pull_token"`
	MediaPullURL               string `json:"media_pull_url"`
	MediaPullToken             string `json:"media_pull_token"`
	CorpSecret                 string `json:"corp_secret"`
	ContactSecret              string `json:"contact_secret"`
	ExternalContactSecret      string `json:"external_contact_secret"`
	PrivateKeyPEM              string `json:"private_key_pem"`
	PrivateKeyVersion          string `json:"private_key_version"`
	ArchiveEventCallbackToken  string `json:"archive_event_callback_token"`
	ArchiveEventCallbackAESKey string `json:"archive_event_callback_aes_key"`
	Enabled                    *bool  `json:"enabled"`
	Remark                     string `json:"remark"`
}

// EnterprisesRequest carries the authenticated management session and masking mode.
type EnterprisesRequest struct {
	WithSecrets bool
	Session     auth.Session
}

// EnterpriseUpsertRequest carries the legacy enterprise create/update request.
type EnterpriseUpsertRequest struct {
	Session auth.Session
	Command EnterpriseUpsertCommand
}

// EnterpriseDeleteRequest carries the legacy enterprise delete request.
type EnterpriseDeleteRequest struct {
	Session      auth.Session
	EnterpriseID string
}

// EnterpriseUpsertCommand is the repository-level enterprise upsert mutation.
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

// NewEnterprisesRequest normalizes the enterprise list request boundary.
func NewEnterprisesRequest(withSecrets bool, session auth.Session) EnterprisesRequest {
	return EnterprisesRequest{WithSecrets: withSecrets, Session: session}
}

// NewEnterpriseUpsertRequest normalizes enterprise create/update input.
func NewEnterpriseUpsertRequest(body EnterpriseUpsertBody, session auth.Session) EnterpriseUpsertRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	return EnterpriseUpsertRequest{
		Session: session,
		Command: EnterpriseUpsertCommand{
			EnterpriseID:               strings.TrimSpace(body.EnterpriseID),
			CorpID:                     strings.TrimSpace(body.CorpID),
			Name:                       strings.TrimSpace(body.Name),
			IncomingPrimaryMode:        normalizeEnterprisePrimaryMode(body.IncomingPrimaryMode),
			ArchiveMode:                "self_decrypt",
			ArchiveSource:              enterpriseDefaultString(strings.TrimSpace(body.ArchiveSource), "self_decrypt"),
			ArchivePullURL:             strings.TrimSpace(body.ArchivePullURL),
			ArchivePullToken:           strings.TrimSpace(body.ArchivePullToken),
			MediaPullURL:               strings.TrimSpace(body.MediaPullURL),
			MediaPullToken:             strings.TrimSpace(body.MediaPullToken),
			CorpSecret:                 strings.TrimSpace(body.CorpSecret),
			ContactSecret:              strings.TrimSpace(body.ContactSecret),
			ExternalContactSecret:      strings.TrimSpace(body.ExternalContactSecret),
			PrivateKeyPEM:              strings.TrimSpace(body.PrivateKeyPEM),
			PrivateKeyVersion:          strings.TrimSpace(body.PrivateKeyVersion),
			ArchiveEventCallbackToken:  strings.TrimSpace(body.ArchiveEventCallbackToken),
			ArchiveEventCallbackAESKey: strings.TrimSpace(body.ArchiveEventCallbackAESKey),
			Enabled:                    enabled,
			Remark:                     strings.TrimSpace(body.Remark),
		},
	}
}

// NewEnterpriseDeleteRequest normalizes enterprise delete input.
func NewEnterpriseDeleteRequest(enterpriseID string, session auth.Session) EnterpriseDeleteRequest {
	return EnterpriseDeleteRequest{Session: session, EnterpriseID: strings.TrimSpace(enterpriseID)}
}

// Enterprises builds the read-only /api/v1/admin/enterprises payload.
func (service Service) Enterprises(ctx context.Context, request EnterprisesRequest) (Payload, error) {
	if service.EnterpriseStore == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	enterprises, err := service.EnterpriseStore.ListEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"enterprises": enterprisePayload(enterprises, request.WithSecrets)}, nil
}

// UpsertEnterprise handles POST /api/v1/admin/enterprises.
func (service Service) UpsertEnterprise(ctx context.Context, request EnterpriseUpsertRequest) (Payload, error) {
	store := service.enterpriseWriteStore()
	if store == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	command := request.Command
	if strings.TrimSpace(command.CorpID) == "" {
		return nil, ErrEnterpriseCorpIDRequired
	}
	if strings.TrimSpace(command.Name) == "" {
		return nil, ErrEnterpriseNameRequired
	}
	if strings.TrimSpace(command.EnterpriseID) == "" {
		command.EnterpriseID = newEnterpriseID()
	}
	existing, ok, err := store.GetEnterprise(ctx, command.EnterpriseID)
	if err != nil {
		return nil, err
	}
	if ok {
		preserveEnterpriseSecrets(&command, existing)
	}
	enterprise, err := store.UpsertEnterprise(ctx, command)
	if err != nil {
		return nil, err
	}
	rows := enterprisePayload([]EnterpriseRecord{enterprise}, false)
	return Payload{"success": true, "enterprise": rows[0]}, nil
}

// DeleteEnterprise handles DELETE /api/v1/admin/enterprises/{enterprise_id}.
func (service Service) DeleteEnterprise(ctx context.Context, request EnterpriseDeleteRequest) (Payload, error) {
	store := service.enterpriseWriteStore()
	if store == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	deleted, err := store.DeleteEnterprise(ctx, request.EnterpriseID)
	if err != nil {
		return nil, err
	}
	return Payload{"success": deleted}, nil
}

func (service Service) enterpriseWriteStore() EnterpriseWriteStore {
	if service.EnterpriseWriteStore != nil {
		return service.EnterpriseWriteStore
	}
	if store, ok := service.EnterpriseStore.(EnterpriseWriteStore); ok {
		return store
	}
	return nil
}

// enterprisePayload serializes enterprise rows to the legacy enterprises[] shape.
func enterprisePayload(enterprises []EnterpriseRecord, withSecrets bool) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(enterprises))
	for _, enterprise := range enterprises {
		row := ProjectionRow{
			"enterprise_id":                  strings.TrimSpace(enterprise.EnterpriseID),
			"corp_id":                        strings.TrimSpace(enterprise.CorpID),
			"name":                           strings.TrimSpace(enterprise.Name),
			"incoming_primary_mode":          enterpriseDefaultString(strings.TrimSpace(enterprise.IncomingPrimaryMode), "archive_primary"),
			"archive_mode":                   enterpriseDefaultString(strings.TrimSpace(enterprise.ArchiveMode), "self_decrypt"),
			"archive_source":                 enterpriseDefaultString(strings.TrimSpace(enterprise.ArchiveSource), "self_decrypt"),
			"archive_pull_url":               strings.TrimSpace(enterprise.ArchivePullURL),
			"archive_pull_token":             strings.TrimSpace(enterprise.ArchivePullToken),
			"media_pull_url":                 strings.TrimSpace(enterprise.MediaPullURL),
			"media_pull_token":               strings.TrimSpace(enterprise.MediaPullToken),
			"corp_secret":                    strings.TrimSpace(enterprise.CorpSecret),
			"contact_secret":                 strings.TrimSpace(enterprise.ContactSecret),
			"external_contact_secret":        strings.TrimSpace(enterprise.ExternalContactSecret),
			"private_key_pem":                strings.TrimSpace(enterprise.PrivateKeyPEM),
			"private_key_version":            strings.TrimSpace(enterprise.PrivateKeyVersion),
			"archive_event_callback_token":   strings.TrimSpace(enterprise.ArchiveEventCallbackToken),
			"archive_event_callback_aes_key": strings.TrimSpace(enterprise.ArchiveEventCallbackAESKey),
			"enabled":                        enterprise.Enabled,
			"remark":                         strings.TrimSpace(enterprise.Remark),
			"created_at":                     nilIfZeroTime(enterprise.CreatedAt),
			"updated_at":                     nilIfZeroTime(enterprise.UpdatedAt),
		}
		if !withSecrets {
			maskEnterpriseSecrets(row)
		}
		payload = append(payload, row)
	}
	return payload
}

func preserveEnterpriseSecrets(command *EnterpriseUpsertCommand, existing EnterpriseRecord) {
	if strings.TrimSpace(command.ArchivePullToken) == "" {
		command.ArchivePullToken = strings.TrimSpace(existing.ArchivePullToken)
	}
	if strings.TrimSpace(command.MediaPullToken) == "" {
		command.MediaPullToken = strings.TrimSpace(existing.MediaPullToken)
	}
	if strings.TrimSpace(command.CorpSecret) == "" {
		command.CorpSecret = strings.TrimSpace(existing.CorpSecret)
	}
	if strings.TrimSpace(command.ContactSecret) == "" {
		command.ContactSecret = strings.TrimSpace(existing.ContactSecret)
	}
	if strings.TrimSpace(command.ExternalContactSecret) == "" {
		command.ExternalContactSecret = strings.TrimSpace(existing.ExternalContactSecret)
	}
	if strings.TrimSpace(command.PrivateKeyPEM) == "" {
		command.PrivateKeyPEM = strings.TrimSpace(existing.PrivateKeyPEM)
	}
	if strings.TrimSpace(command.PrivateKeyVersion) == "" {
		command.PrivateKeyVersion = strings.TrimSpace(existing.PrivateKeyVersion)
	}
	if strings.TrimSpace(command.ArchiveEventCallbackToken) == "" {
		command.ArchiveEventCallbackToken = strings.TrimSpace(existing.ArchiveEventCallbackToken)
	}
	if strings.TrimSpace(command.ArchiveEventCallbackAESKey) == "" {
		command.ArchiveEventCallbackAESKey = strings.TrimSpace(existing.ArchiveEventCallbackAESKey)
	}
}

func maskEnterpriseSecrets(row ProjectionRow) {
	secretFields := []string{
		"archive_pull_token",
		"media_pull_token",
		"corp_secret",
		"contact_secret",
		"external_contact_secret",
		"private_key_pem",
		"archive_event_callback_token",
		"archive_event_callback_aes_key",
	}
	for _, field := range secretFields {
		value := strings.TrimSpace(rowText(row, field))
		row["has_"+field] = value != ""
		row[field] = ""
	}
}

func normalizeEnterprisePrimaryMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "device_primary") {
		return "device_primary"
	}
	return "archive_primary"
}

func enterpriseDefaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func nilIfZeroTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func newEnterpriseID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "ent-" + strings.ReplaceAll(fmt.Sprintf("%p", &bytes), "0x", "")
	}
	return "ent-" + hex.EncodeToString(bytes[:])
}
