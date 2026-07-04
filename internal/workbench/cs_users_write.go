package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"wework-go/internal/auth"
)

var (
	ErrCSUserAssigneeIDRequired   = errors.New("assignee_id is required")
	ErrCSUserAssigneeNameRequired = errors.New("assignee_name is required")
	ErrCSUserInvalidRole          = errors.New("role must be one of admin/supervisor/cs")
	ErrCSUserPasswordTooShort     = errors.New("密码长度不得少于6位")
)

// CSUserConflictError maps duplicate user constraints to HTTP 409.
type CSUserConflictError struct {
	Detail string
}

func (err CSUserConflictError) Error() string {
	return err.Detail
}

// CSUserCommand carries one customer-service user upsert command.
type CSUserCommand struct {
	AssigneeID   string
	AssigneeName string
	Role         string
	Enabled      bool
	AIEnabled    bool
	MaxSessions  int
	CreateOnly   bool
	Password     string
}

// CSUserUpsertBody is the JSON input for POST /cs-users.
type CSUserUpsertBody struct {
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
	Role         string `json:"role"`
	Enabled      *bool  `json:"enabled"`
	AIEnabled    *bool  `json:"ai_enabled"`
	MaxSessions  int    `json:"max_sessions"`
	CreateOnly   bool   `json:"create_only"`
	Password     string `json:"password"`
}

// CSUserUpsertRequest carries the legacy POST request body.
type CSUserUpsertRequest struct {
	Session auth.Session
	Command CSUserCommand
}

// CSUserDeleteRequest carries the legacy DELETE path parameter.
type CSUserDeleteRequest struct {
	Session    auth.Session
	AssigneeID string
}

// NewCSUserUpsertRequest normalizes the upsert body boundary.
func NewCSUserUpsertRequest(body CSUserUpsertBody, session auth.Session) CSUserUpsertRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	aiEnabled := false
	if body.AIEnabled != nil {
		aiEnabled = *body.AIEnabled
	}
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "cs"
	}
	return CSUserUpsertRequest{
		Session: session,
		Command: CSUserCommand{
			AssigneeID:   strings.TrimSpace(body.AssigneeID),
			AssigneeName: strings.TrimSpace(body.AssigneeName),
			Role:         role,
			Enabled:      enabled,
			AIEnabled:    aiEnabled,
			MaxSessions:  maxInt(0, body.MaxSessions),
			CreateOnly:   body.CreateOnly,
			Password:     strings.TrimSpace(body.Password),
		},
	}
}

// NewCSUserDeleteRequest normalizes the delete path parameter.
func NewCSUserDeleteRequest(assigneeID string, session auth.Session) CSUserDeleteRequest {
	return CSUserDeleteRequest{Session: session, AssigneeID: strings.TrimSpace(assigneeID)}
}

// UpsertCSUser handles POST /api/v1/cs-users.
func (service Service) UpsertCSUser(ctx context.Context, request CSUserUpsertRequest) (Payload, error) {
	store := service.csUserWriteStore()
	if store == nil || service.CSUsers == nil {
		return nil, ErrCSUserStoreUnavailable
	}
	command := normalizeCSUserCommand(request.Command)
	if err := validateCSUserCommand(command); err != nil {
		return nil, err
	}
	if command.CreateOnly {
		if _, ok, err := store.GetCSUser(ctx, command.AssigneeID); err != nil {
			return nil, err
		} else if ok {
			return nil, CSUserConflictError{Detail: fmt.Sprintf("客服ID已存在：%s", command.AssigneeID)}
		}
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if strings.TrimSpace(user.AssigneeID) != command.AssigneeID && strings.EqualFold(strings.TrimSpace(user.AssigneeName), command.AssigneeName) {
			return nil, CSUserConflictError{Detail: fmt.Sprintf("客服名称已存在：%s", command.AssigneeName)}
		}
	}
	user, err := store.UpsertCSUser(ctx, command)
	if err != nil {
		return nil, err
	}
	if service.CSUserEvents != nil {
		if err := service.CSUserEvents.Publish(ctx, "devices", "cs.user.updated", "cs.user", map[string]any{"assignee_id": user.AssigneeID, "assignee_name": user.AssigneeName}); err != nil {
			return nil, err
		}
	}
	if service.AuditLogWriter != nil {
		detail := fmt.Sprintf("创建/更新客服账号: %s(%s), role=%s, enabled=%t, ai_enabled=%t", command.AssigneeName, command.AssigneeID, command.Role, command.Enabled, command.AIEnabled)
		if command.Password != "" {
			detail += " [重置密码]"
		}
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "cs_user", Detail: detail}); err != nil {
			return nil, err
		}
	}
	currentSessions := 0
	if store := service.assignmentCountStore(); store != nil {
		if counts, err := store.CountByAssigneeIDs(ctx, []string{user.AssigneeID}, ""); err == nil {
			currentSessions = counts[strings.TrimSpace(user.AssigneeID)]
		}
	}
	return Payload{"success": true, "user": csUserRecordPayload(user, currentSessions, service.now().UTC().Add(-5*time.Minute))}, nil
}

// DeleteCSUser handles DELETE /api/v1/cs-users/{assignee_id}.
func (service Service) DeleteCSUser(ctx context.Context, request CSUserDeleteRequest) (Payload, error) {
	store := service.csUserWriteStore()
	if store == nil {
		return nil, ErrCSUserStoreUnavailable
	}
	assigneeID := strings.TrimSpace(request.AssigneeID)
	deleted, err := store.DeleteCSUser(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	if deleted && service.CSUserEvents != nil {
		if err := service.CSUserEvents.Publish(ctx, "devices", "cs.user.deleted", "cs.user", map[string]any{"assignee_id": assigneeID}); err != nil {
			return nil, err
		}
	}
	if deleted && service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "cs_user", Detail: fmt.Sprintf("删除客服账号: %s", assigneeID)}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": deleted}, nil
}

func normalizeCSUserCommand(command CSUserCommand) CSUserCommand {
	command.AssigneeID = strings.TrimSpace(command.AssigneeID)
	command.AssigneeName = strings.TrimSpace(command.AssigneeName)
	command.Role = strings.TrimSpace(command.Role)
	if command.Role == "" {
		command.Role = "cs"
	}
	command.Password = strings.TrimSpace(command.Password)
	command.MaxSessions = maxInt(0, command.MaxSessions)
	return command
}

func validateCSUserCommand(command CSUserCommand) error {
	if command.AssigneeID == "" {
		return ErrCSUserAssigneeIDRequired
	}
	if command.AssigneeName == "" {
		return ErrCSUserAssigneeNameRequired
	}
	switch command.Role {
	case "admin", "supervisor", "cs":
	default:
		return ErrCSUserInvalidRole
	}
	if command.Password != "" && utf8.RuneCountInString(command.Password) < 6 {
		return ErrCSUserPasswordTooShort
	}
	return nil
}

func (service Service) csUserWriteStore() CSUserWriteStore {
	if service.CSUserWriteStore != nil {
		return service.CSUserWriteStore
	}
	if store, ok := service.CSUsers.(CSUserWriteStore); ok {
		return store
	}
	return nil
}
