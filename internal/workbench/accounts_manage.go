package workbench

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"wework-go/internal/auth"
)

var (
	// ErrAccountNameRequired preserves FastAPI's account_name validation.
	ErrAccountNameRequired       = errors.New("account_name is required")
	ErrAccountBatchFileRequired  = errors.New("file is required")
	ErrAccountBatchCSVOnly       = errors.New("only .csv file is supported")
	ErrAccountBatchCSVEmpty      = errors.New("csv is empty")
	ErrAccountBatchCSVDecode     = errors.New("csv decode failed")
	ErrAccountBatchHeaderMissing = errors.New("csv header is required")
)

// AccountUpsertBody is the JSON input for POST /accounts.
type AccountUpsertBody struct {
	AccountID           string `json:"account_id"`
	AccountName         string `json:"account_name"`
	AgentID             string `json:"agent_id"`
	DeviceID            string `json:"device_id"`
	WeWorkUserID        string `json:"wework_user_id"`
	EnterpriseID        string `json:"enterprise_id"`
	SOPFlowID           string `json:"sop_flow_id"`
	SOPEnabled          *bool  `json:"sop_enabled"`
	SOPReplyWindowStart string `json:"sop_reply_window_start"`
	SOPReplyWindowEnd   string `json:"sop_reply_window_end"`
	AIEnabled           *bool  `json:"ai_enabled"`
	AIModel             string `json:"ai_model"`
	KnowledgeTag        string `json:"knowledge_tag"`
}

// AccountUpsertRequest carries the legacy account create/update request.
type AccountUpsertRequest struct {
	Session auth.Session
	Command AccountUpsertCommand
}

// AccountDeleteRequest carries the legacy account delete request.
type AccountDeleteRequest struct {
	Session   auth.Session
	AccountID string
}

// AccountBatchUpsertRequest carries a CSV account import request.
type AccountBatchUpsertRequest struct {
	Session  auth.Session
	Filename string
	Content  []byte
}

// AccountUpsertCommand is the repository-level account upsert mutation.
type AccountUpsertCommand struct {
	AccountID           string
	AccountName         string
	AgentID             string
	DeviceID            string
	WeWorkUserID        string
	EnterpriseID        string
	SOPFlowID           string
	SOPEnabled          *bool
	SOPReplyWindowStart string
	SOPReplyWindowEnd   string
	AIEnabled           *bool
	AIModel             string
	KnowledgeTag        string
}

// NewAccountUpsertRequest normalizes account create/update input.
func NewAccountUpsertRequest(body AccountUpsertBody, session auth.Session) AccountUpsertRequest {
	return AccountUpsertRequest{
		Session: session,
		Command: AccountUpsertCommand{
			AccountID:           strings.TrimSpace(body.AccountID),
			AccountName:         strings.TrimSpace(body.AccountName),
			AgentID:             strings.TrimSpace(body.AgentID),
			DeviceID:            strings.TrimSpace(body.DeviceID),
			WeWorkUserID:        strings.TrimSpace(body.WeWorkUserID),
			EnterpriseID:        strings.TrimSpace(body.EnterpriseID),
			SOPFlowID:           strings.TrimSpace(body.SOPFlowID),
			SOPEnabled:          body.SOPEnabled,
			SOPReplyWindowStart: strings.TrimSpace(body.SOPReplyWindowStart),
			SOPReplyWindowEnd:   strings.TrimSpace(body.SOPReplyWindowEnd),
			AIEnabled:           body.AIEnabled,
			AIModel:             strings.TrimSpace(body.AIModel),
			KnowledgeTag:        strings.TrimSpace(body.KnowledgeTag),
		},
	}
}

// NewAccountDeleteRequest normalizes account delete input.
func NewAccountDeleteRequest(accountID string, session auth.Session) AccountDeleteRequest {
	return AccountDeleteRequest{Session: session, AccountID: strings.TrimSpace(accountID)}
}

// NewAccountBatchUpsertRequest normalizes account CSV import input.
func NewAccountBatchUpsertRequest(filename string, content []byte, session auth.Session) AccountBatchUpsertRequest {
	copied := append([]byte(nil), content...)
	return AccountBatchUpsertRequest{Session: session, Filename: strings.TrimSpace(filename), Content: copied}
}

// UpsertAccount handles POST /api/v1/accounts.
func (service Service) UpsertAccount(ctx context.Context, request AccountUpsertRequest) (Payload, error) {
	store := service.accountManageWriteStore()
	if store == nil {
		return nil, ErrAccountStoreUnavailable
	}
	command := request.Command
	if strings.TrimSpace(command.AccountName) == "" {
		return nil, ErrAccountNameRequired
	}
	if strings.TrimSpace(command.AccountID) == "" {
		command.AccountID = newAccountID()
	}
	account, err := store.UpsertAccount(ctx, command)
	if err != nil {
		return nil, err
	}
	if service.AccountEvents != nil {
		if err := service.AccountEvents.Publish(ctx, "devices", "account.updated", "account.changed", map[string]any(accountRecordFullPayload(account))); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAccountAudit(ctx, request.Session, fmt.Sprintf("创建/更新账号: %s", strings.TrimSpace(command.AccountName)))
	return Payload{"success": true, "account": accountRecordFullPayload(account)}, nil
}

// DeleteAccount handles DELETE /api/v1/accounts/{account_id}.
func (service Service) DeleteAccount(ctx context.Context, request AccountDeleteRequest) (Payload, error) {
	store := service.accountManageWriteStore()
	if store == nil {
		return nil, ErrAccountStoreUnavailable
	}
	accountID := strings.TrimSpace(request.AccountID)
	deleted, err := store.DeleteAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if deleted {
		if service.AccountEvents != nil {
			if err := service.AccountEvents.Publish(ctx, "devices", "account.deleted", "account.changed", map[string]any{"account_id": accountID}); err != nil {
				return nil, err
			}
		}
		service.invalidateAllReadModelNamespaces(ctx)
		service.recordAccountAudit(ctx, request.Session, fmt.Sprintf("删除账号: %s", accountID))
	}
	return Payload{"success": deleted}, nil
}

// BatchUpsertAccounts handles POST /api/v1/accounts/batch.
func (service Service) BatchUpsertAccounts(ctx context.Context, request AccountBatchUpsertRequest) (Payload, error) {
	store := service.accountManageWriteStore()
	if store == nil {
		return nil, ErrAccountStoreUnavailable
	}
	filename := strings.TrimSpace(request.Filename)
	if filename == "" {
		return nil, ErrAccountBatchFileRequired
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".csv") {
		return nil, ErrAccountBatchCSVOnly
	}
	if len(request.Content) == 0 {
		return nil, ErrAccountBatchCSVEmpty
	}
	if !utf8.Valid(request.Content) {
		return nil, ErrAccountBatchCSVDecode
	}
	commands, err := accountBatchCommands(request.Content)
	if err != nil {
		return nil, err
	}
	accounts := make([]ProjectionRow, 0, len(commands))
	for _, command := range commands {
		if strings.TrimSpace(command.AccountID) == "" {
			command.AccountID = newAccountID()
		}
		account, err := store.UpsertAccount(ctx, command)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, accountRecordFullPayload(account))
	}
	if service.AccountEvents != nil {
		if err := service.AccountEvents.Publish(ctx, "devices", "account.batch_imported", "account.changed", map[string]any{"count": len(accounts)}); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAccountAudit(ctx, request.Session, fmt.Sprintf("批量导入账号: %d 条", len(accounts)))
	return Payload{"success": true, "count": len(accounts), "accounts": accounts}, nil
}

func (service Service) accountManageWriteStore() AccountManageWriteStore {
	if store, ok := service.Accounts.(AccountManageWriteStore); ok {
		return store
	}
	if store, ok := service.AccountAIWriteStore.(AccountManageWriteStore); ok {
		return store
	}
	return nil
}

func accountBatchCommands(content []byte) ([]AccountUpsertCommand, error) {
	text := strings.TrimPrefix(string(content), "\ufeff")
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil || len(headers) == 0 {
		return nil, ErrAccountBatchHeaderMissing
	}
	headerIndex := make(map[string]int, len(headers))
	for index, header := range headers {
		key := strings.TrimSpace(header)
		if key != "" {
			headerIndex[key] = index
		}
	}
	if len(headerIndex) == 0 {
		return nil, ErrAccountBatchHeaderMissing
	}
	commands := make([]AccountUpsertCommand, 0)
	for {
		row, err := reader.Read()
		if errors.Is(err, csv.ErrFieldCount) {
			return nil, err
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		accountName := csvCell(row, headerIndex, "account_name")
		if accountName == "" {
			continue
		}
		commands = append(commands, AccountUpsertCommand{
			AccountName:         accountName,
			AgentID:             csvCell(row, headerIndex, "agent_id"),
			DeviceID:            csvCell(row, headerIndex, "device_id"),
			WeWorkUserID:        csvCell(row, headerIndex, "wework_user_id"),
			EnterpriseID:        csvCell(row, headerIndex, "enterprise_id"),
			SOPFlowID:           csvCell(row, headerIndex, "sop_flow_id"),
			SOPEnabled:          csvBoolCell(row, headerIndex, "sop_enabled"),
			SOPReplyWindowStart: csvCell(row, headerIndex, "sop_reply_window_start"),
			SOPReplyWindowEnd:   csvCell(row, headerIndex, "sop_reply_window_end"),
			AIEnabled:           csvBoolCell(row, headerIndex, "ai_enabled"),
			AIModel:             csvCell(row, headerIndex, "ai_model"),
			KnowledgeTag:        csvCell(row, headerIndex, "knowledge_tag"),
		})
	}
	return commands, nil
}

func csvCell(row []string, headerIndex map[string]int, key string) string {
	index, ok := headerIndex[key]
	if !ok || index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func csvBoolCell(row []string, headerIndex map[string]int, key string) *bool {
	raw := strings.ToLower(csvCell(row, headerIndex, key))
	if raw == "" {
		return nil
	}
	value := raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	return &value
}

func newAccountID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "acc-" + strings.ReplaceAll(fmt.Sprintf("%p", &bytes), "0x", "")
	}
	return "acc-" + hex.EncodeToString(bytes[:])
}
