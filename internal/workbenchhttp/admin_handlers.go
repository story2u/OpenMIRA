// Package workbenchhttp keeps management candidates beside the CS workbench
// handlers. These adapters preserve legacy auth and response shapes while route
// mounting remains opt-in during the migration.
package workbenchhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"wework-go/internal/workbench"
)

// AccountsListHandler serializes /api/v1/accounts.
func (handler Handler) AccountsListHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountsList == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench accounts list service is not configured")
		return
	}
	payload, err := handler.AccountsList.AccountsList(r.Context(), workbench.NewAccountsListRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountUpsertHandler serializes POST /api/v1/accounts.
func (handler Handler) AccountUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountManageWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account manage write service is not configured")
		return
	}
	var body workbench.AccountUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AccountManageWrite.UpsertAccount(r.Context(), workbench.NewAccountUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountDeleteHandler serializes DELETE /api/v1/accounts/{account_id}.
func (handler Handler) AccountDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountManageWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account manage write service is not configured")
		return
	}
	payload, err := handler.AccountManageWrite.DeleteAccount(r.Context(), workbench.NewAccountDeleteRequest(r.PathValue("account_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountBatchUpsertHandler serializes POST /api/v1/accounts/batch.
func (handler Handler) AccountBatchUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountManageWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account manage write service is not configured")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeServiceError(w, workbench.ErrAccountBatchFileRequired)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "csv decode failed")
		return
	}
	payload, err := handler.AccountManageWrite.BatchUpsertAccounts(r.Context(), workbench.NewAccountBatchUpsertRequest(header.Filename, content, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountAIEnabledHandler serializes POST /api/v1/accounts/{account_id}/ai-enabled.
func (handler Handler) AccountAIEnabledHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountAIWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account ai write service is not configured")
		return
	}
	var body workbench.AccountAIEnabledBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AccountAIWrite.ToggleAccountAIEnabled(r.Context(), workbench.NewAccountAIEnabledRequest(r.PathValue("account_id"), body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountAssignHandler serializes POST /api/v1/accounts/{account_id}/assign.
func (handler Handler) AccountAssignHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountAssignWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account assign write service is not configured")
		return
	}
	var body workbench.AccountAssignBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AccountAssignWrite.AssignAccount(r.Context(), workbench.NewAccountAssignRequest(r.PathValue("account_id"), body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountUnassignHandler serializes POST /api/v1/accounts/{account_id}/unassign.
func (handler Handler) AccountUnassignHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountAssignWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account assign write service is not configured")
		return
	}
	payload, err := handler.AccountAssignWrite.UnassignAccount(r.Context(), workbench.NewAccountUnassignRequest(r.PathValue("account_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationAIHandler serializes POST /api/v1/conversations/{conversation_id}/ai-auto-reply.
func (handler Handler) ConversationAIHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ConversationAI == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversation ai service is not configured")
		return
	}
	var body workbench.ConversationAIBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.ConversationAI.ToggleConversationAI(r.Context(), workbench.NewConversationAIRequest(r.PathValue("conversation_id"), body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationAIBulkHandler serializes POST /api/v1/conversations/ai-auto-reply/bulk.
func (handler Handler) ConversationAIBulkHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ConversationAI == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversation ai service is not configured")
		return
	}
	var body workbench.ConversationAIBulkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.ConversationAI.ToggleConversationAIBulk(r.Context(), workbench.NewConversationAIBulkRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationReadHandler serializes POST /api/v1/conversations/{conversation_id}/read.
func (handler Handler) ConversationReadHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ConversationRead == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversation read service is not configured")
		return
	}
	payload, err := handler.ConversationRead.MarkConversationRead(r.Context(), workbench.NewConversationReadRequest(r.PathValue("conversation_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CustomerProfileHandler serializes PATCH /api/v1/conversations/{conversation_id}/customer-profile.
func (handler Handler) CustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CustomerProfile == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench customer profile service is not configured")
		return
	}
	var body workbench.CustomerProfileUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.CustomerProfile.UpdateConversationCustomerProfile(r.Context(), workbench.NewCustomerProfileUpdateRequest(r.PathValue("conversation_id"), body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ContactProfileResolveHandler serializes POST /api/v1/conversations/{conversation_id}/contact-profile/resolve.
func (handler Handler) ContactProfileResolveHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ContactResolve == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench contact profile resolve service is not configured")
		return
	}
	payload, err := handler.ContactResolve.ResolveConversationContactProfile(r.Context(), workbench.NewContactProfileResolveRequest(r.PathValue("conversation_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ContactProfileRefreshHandler serializes POST /api/v1/conversations/{conversation_id}/contact-profile/refresh.
func (handler Handler) ContactProfileRefreshHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ContactRefresh == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench contact profile refresh service is not configured")
		return
	}
	payload, err := handler.ContactRefresh.RefreshConversationContactProfile(r.Context(), workbench.NewContactProfileRefreshRequest(r.PathValue("conversation_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationTransferHandler serializes POST /api/v1/conversations/{conversation_id}/transfer.
func (handler Handler) ConversationTransferHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ConversationTransfer == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversation transfer service is not configured")
		return
	}
	var body workbench.ConversationTransferBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.ConversationTransfer.TransferConversation(r.Context(), workbench.NewConversationTransferRequest(r.PathValue("conversation_id"), body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CSUsersListHandler serializes /api/v1/cs-users.
func (handler Handler) CSUsersListHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CSUsersList == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench cs users list service is not configured")
		return
	}
	payload, err := handler.CSUsersList.CSUsersList(r.Context(), workbench.NewCSUsersListRequest(r.URL.Query().Get("keyword"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CSUsersStatusHandler serializes /api/v1/cs-users/status.
func (handler Handler) CSUsersStatusHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CSUsersStatus == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench cs users status service is not configured")
		return
	}
	payload, err := handler.CSUsersStatus.CSUsersStatus(r.Context(), workbench.NewCSUsersStatusRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CSUserUpsertHandler serializes POST /api/v1/cs-users.
func (handler Handler) CSUserUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CSUsersWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench cs users write service is not configured")
		return
	}
	var body workbench.CSUserUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.CSUsersWrite.UpsertCSUser(r.Context(), workbench.NewCSUserUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CSUserDeleteHandler serializes DELETE /api/v1/cs-users/{assignee_id}.
func (handler Handler) CSUserDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CSUsersWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench cs users write service is not configured")
		return
	}
	payload, err := handler.CSUsersWrite.DeleteCSUser(r.Context(), workbench.NewCSUserDeleteRequest(r.PathValue("assignee_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentConfigHandler serializes /api/v1/admin/assignment-config.
func (handler Handler) AssignmentConfigHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentCfg == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment config service is not configured")
		return
	}
	payload, err := handler.AssignmentCfg.AssignmentConfig(r.Context(), workbench.NewAssignmentConfigRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentConfigWriteHandler serializes POST /api/v1/admin/assignment-config.
func (handler Handler) AssignmentConfigWriteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentCfgWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment config write service is not configured")
		return
	}
	var body workbench.AssignmentConfigUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	payload, err := handler.AssignmentCfgWrite.UpdateAssignmentConfig(r.Context(), workbench.NewAssignmentConfigUpdateRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentWorkloadsHandler serializes /api/v1/assignments/workloads.
func (handler Handler) AssignmentWorkloadsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentLoad == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment workloads service is not configured")
		return
	}
	payload, err := handler.AssignmentLoad.AssignmentWorkloads(r.Context(), workbench.NewAssignmentWorkloadsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentsListHandler serializes /api/v1/assignments.
func (handler Handler) AssignmentsListHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentRead == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment reads service is not configured")
		return
	}
	request, err := workbench.NewAssignmentsListRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.AssignmentRead.AssignmentsList(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentDetailHandler serializes /api/v1/assignments/{conversation_id}.
func (handler Handler) AssignmentDetailHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentRead == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment reads service is not configured")
		return
	}
	payload, err := handler.AssignmentRead.AssignmentDetail(r.Context(), workbench.NewAssignmentDetailRequest(r.PathValue("conversation_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentClaimHandler serializes POST /api/v1/assignments/claim.
func (handler Handler) AssignmentClaimHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment write service is not configured")
		return
	}
	var body workbench.AssignmentClaimBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AssignmentWrite.ClaimAssignment(r.Context(), workbench.NewAssignmentClaimRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentReleaseHandler serializes POST /api/v1/assignments/release.
func (handler Handler) AssignmentReleaseHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment write service is not configured")
		return
	}
	var body workbench.AssignmentReleaseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AssignmentWrite.ReleaseAssignment(r.Context(), workbench.NewAssignmentReleaseRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentPurgeAllHandler serializes POST /api/v1/assignments/purge-all.
func (handler Handler) AssignmentPurgeAllHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentPurge == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment purge service is not configured")
		return
	}
	payload, err := handler.AssignmentPurge.PurgeAssignments(r.Context(), workbench.NewAssignmentPurgeRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AssignmentAutoAssignHandler serializes POST /api/v1/assignments/auto-assign.
func (handler Handler) AssignmentAutoAssignHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AssignmentAuto == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench assignment auto assign service is not configured")
		return
	}
	var body workbench.AssignmentAutoAssignBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	payload, err := handler.AssignmentAuto.AutoAssignAssignments(r.Context(), workbench.NewAssignmentAutoAssignRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AuditLogsHandler serializes /api/v1/admin/audit-logs.
func (handler Handler) AuditLogsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AuditLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench audit log service is not configured")
		return
	}
	payload, err := handler.AuditLogs.AuditLogs(r.Context(), workbench.NewAuditLogsRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SystemLogsHandler serializes /api/v1/admin/system-logs.
func (handler Handler) SystemLogsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SystemLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench system logs service is not configured")
		return
	}
	request, err := workbench.NewSystemLogsRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.SystemLogs.SystemLogs(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ObservabilityDashboardHandler serializes /api/v1/admin/observability/dashboard.
func (handler Handler) ObservabilityDashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Observability == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench observability dashboard service is not configured")
		return
	}
	request, err := workbench.NewObservabilityDashboardRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.Observability.ObservabilityDashboard(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// Stage6HealthHandler serializes /healthz/stage6.
func (handler Handler) Stage6HealthHandler(w http.ResponseWriter, r *http.Request) {
	_, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Stage6Health == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stage6 health service is not configured")
		return
	}
	payload, err := handler.Stage6Health.Stage6Status(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticDeviceMapHandler serializes /api/v1/admin/diagnostic/device-map.
func (handler Handler) DiagnosticDeviceMapHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Diagnostic == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic service is not configured")
		return
	}
	payload, err := handler.Diagnostic.DiagnosticDeviceMap(r.Context(), workbench.NewDiagnosticDeviceMapRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticOrphanConversationsHandler serializes /api/v1/admin/diagnostic/orphan-conversations.
func (handler Handler) DiagnosticOrphanConversationsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.OrphanConvs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic orphan conversations service is not configured")
		return
	}
	payload, err := handler.OrphanConvs.DiagnosticOrphanConversations(r.Context(), workbench.NewDiagnosticOrphanConversationsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticForkedConversationsHandler serializes /api/v1/admin/diagnostic/forked-conversations.
func (handler Handler) DiagnosticForkedConversationsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ForkedConvs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic forked conversations service is not configured")
		return
	}
	payload, err := handler.ForkedConvs.DiagnosticForkedConversations(r.Context(), workbench.NewDiagnosticForkedConversationsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticDirtyContactsHandler serializes /api/v1/admin/diagnostic/dirty-contacts.
func (handler Handler) DiagnosticDirtyContactsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.DirtyContacts == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic dirty contacts service is not configured")
		return
	}
	request, err := workbench.NewDiagnosticDirtyContactsRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.DirtyContacts.DiagnosticDirtyContacts(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticArchiveSyncStatusHandler serializes /api/v1/admin/diagnostic/archive-sync-status.
func (handler Handler) DiagnosticArchiveSyncStatusHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ArchiveSync == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic archive sync status service is not configured")
		return
	}
	payload, err := handler.ArchiveSync.DiagnosticArchiveSyncStatus(r.Context(), workbench.NewDiagnosticArchiveSyncStatusRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticArchiveMissingOutboxCheckHandler serializes /api/v1/admin/diagnostic/archive-missing-message-outbox/check.
func (handler Handler) DiagnosticArchiveMissingOutboxCheckHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.MissingOutbox == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic archive missing outbox check service is not configured")
		return
	}
	var body workbench.ArchiveMissingOutboxCheckBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	request, err := workbench.NewArchiveMissingOutboxCheckRequest(body, session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.MissingOutbox.DiagnosticArchiveMissingOutboxCheck(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticArchiveMissingOutboxReplayHandler serializes /api/v1/admin/diagnostic/archive-missing-message-outbox/replay.
func (handler Handler) DiagnosticArchiveMissingOutboxReplayHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.MissingOutboxReplay == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic archive missing outbox replay service is not configured")
		return
	}
	var body workbench.ArchiveMissingOutboxReplayBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	request, err := workbench.NewArchiveMissingOutboxReplayRequest(body, session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.MissingOutboxReplay.DiagnosticArchiveMissingOutboxReplay(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// DiagnosticHistoricalTimezoneCutoverHandler serializes /api/v1/admin/diagnostic/historical-timezone-cutover.
func (handler Handler) DiagnosticHistoricalTimezoneCutoverHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.HistoricalTimezone == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench diagnostic historical timezone cutover service is not configured")
		return
	}
	var body workbench.HistoricalTimezoneCutoverBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid historical timezone cutover payload")
		return
	}
	request, err := workbench.NewHistoricalTimezoneCutoverRequest(body, session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.HistoricalTimezone.DiagnosticHistoricalTimezoneCutover(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SensitiveWordsHandler serializes /api/v1/admin/sensitive-words.
func (handler Handler) SensitiveWordsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SensitiveWords == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sensitive words service is not configured")
		return
	}
	payload, err := handler.SensitiveWords.SensitiveWords(r.Context(), workbench.NewSensitiveWordsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SensitiveWordUpsertHandler serializes POST /api/v1/admin/sensitive-words.
func (handler Handler) SensitiveWordUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SensitiveWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sensitive words write service is not configured")
		return
	}
	var body workbench.SensitiveWordUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.SensitiveWrite.UpsertSensitiveWord(r.Context(), workbench.NewSensitiveWordUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SensitiveWordDeleteHandler serializes DELETE /api/v1/admin/sensitive-words/{word_id}.
func (handler Handler) SensitiveWordDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SensitiveWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sensitive words write service is not configured")
		return
	}
	payload, err := handler.SensitiveWrite.DeleteSensitiveWord(r.Context(), workbench.NewSensitiveWordDeleteRequest(r.PathValue("word_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ReplyScriptsHandler serializes /api/v1/admin/scripts.
func (handler Handler) ReplyScriptsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ReplyScripts == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench reply scripts service is not configured")
		return
	}
	payload, err := handler.ReplyScripts.ReplyScripts(r.Context(), workbench.NewReplyScriptsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ReplyScriptUpsertHandler serializes POST /api/v1/admin/scripts.
func (handler Handler) ReplyScriptUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ReplyScriptWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench reply scripts write service is not configured")
		return
	}
	var body workbench.ReplyScriptUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.ReplyScriptWrite.UpsertReplyScript(r.Context(), workbench.NewReplyScriptUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ReplyScriptDeleteHandler serializes DELETE /api/v1/admin/scripts/{script_id}.
func (handler Handler) ReplyScriptDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ReplyScriptWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench reply scripts write service is not configured")
		return
	}
	payload, err := handler.ReplyScriptWrite.DeleteReplyScript(r.Context(), workbench.NewReplyScriptDeleteRequest(r.PathValue("script_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AIConfigHandler serializes /api/v1/admin/ai-config.
func (handler Handler) AIConfigHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AIConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench ai config service is not configured")
		return
	}
	payload, err := handler.AIConfig.AIConfig(r.Context(), workbench.NewAIConfigRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AIConfigWriteHandler serializes POST /api/v1/admin/ai-config.
func (handler Handler) AIConfigWriteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AIConfigWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench ai config write service is not configured")
		return
	}
	var body workbench.AIConfigUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AIConfigWrite.UpdateAIConfig(r.Context(), workbench.NewAIConfigUpdateRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AIConfigTestHandler serializes POST /api/v1/admin/ai-config/test.
func (handler Handler) AIConfigTestHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AIConfigTest == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench ai config test service is not configured")
		return
	}
	var body workbench.AIConfigTestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.AIConfigTest.TestAIConfig(r.Context(), workbench.NewAIConfigTestRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AIReplyLogsHandler serializes /api/v1/admin/ai-config/reply-logs.
func (handler Handler) AIReplyLogsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AIReplyLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench ai reply logs service is not configured")
		return
	}
	request, err := workbench.NewAIReplyLogsRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.AIReplyLogs.AIReplyLogs(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPFlowsHandler serializes /api/v1/admin/sop/flows.
func (handler Handler) SOPFlowsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPFlows == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop flows service is not configured")
		return
	}
	payload, err := handler.SOPFlows.SOPFlows(r.Context(), workbench.NewSOPFlowsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPFlowUpsertHandler serializes POST /api/v1/admin/sop/flows.
func (handler Handler) SOPFlowUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPFlowsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop flows write service is not configured")
		return
	}
	var body workbench.SOPFlowUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.SOPFlowsWrite.UpsertSOPFlow(r.Context(), workbench.NewSOPFlowUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPFlowDeleteHandler serializes DELETE /api/v1/admin/sop/flows/{flow_id}.
func (handler Handler) SOPFlowDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPFlowsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop flows write service is not configured")
		return
	}
	payload, err := handler.SOPFlowsWrite.DeleteSOPFlow(r.Context(), workbench.NewSOPFlowDeleteRequest(r.PathValue("flow_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPPoliciesHandler serializes /api/v1/admin/sop/policies.
func (handler Handler) SOPPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPPolicies == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop policies service is not configured")
		return
	}
	payload, err := handler.SOPPolicies.SOPPolicies(r.Context(), workbench.NewSOPPoliciesRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPPolicyUpsertHandler serializes POST /api/v1/admin/sop/policies.
func (handler Handler) SOPPolicyUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPPoliciesWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop policies write service is not configured")
		return
	}
	var body workbench.SOPPolicyUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.SOPPoliciesWrite.UpsertSOPPolicy(r.Context(), workbench.NewSOPPolicyUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPPolicyDeleteHandler serializes DELETE /api/v1/admin/sop/policies/{policy_id}.
func (handler Handler) SOPPolicyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPPoliciesWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop policies write service is not configured")
		return
	}
	payload, err := handler.SOPPoliciesWrite.DeleteSOPPolicy(r.Context(), workbench.NewSOPPolicyDeleteRequest(r.PathValue("policy_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPAnalyticsStageStatsHandler serializes /api/v1/admin/sop/analytics/stage-stats.
func (handler Handler) SOPAnalyticsStageStatsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPAnalytics == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop analytics service is not configured")
		return
	}
	payload, err := handler.SOPAnalytics.SOPAnalyticsStageStats(r.Context(), workbench.NewSOPStageStatsRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPAnalyticsFactsHandler serializes /api/v1/admin/sop/analytics/facts.
func (handler Handler) SOPAnalyticsFactsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPAnalytics == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop analytics service is not configured")
		return
	}
	request, err := workbench.NewSOPFactsRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.SOPAnalytics.SOPAnalyticsFacts(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPDispatchTasksHandler serializes /api/v1/admin/sop/dispatch-tasks.
func (handler Handler) SOPDispatchTasksHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPDispatchTasks == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop dispatch tasks service is not configured")
		return
	}
	request, err := workbench.NewSOPDispatchTasksRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.SOPDispatchTasks.SOPDispatchTasks(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SOPDispatchTasksResendHandler serializes /api/v1/admin/sop/dispatch-tasks/resend.
func (handler Handler) SOPDispatchTasksResendHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SOPDispatchResend == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench sop dispatch resend service is not configured")
		return
	}
	var body workbench.SOPDispatchResendBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	request, err := workbench.NewSOPDispatchResendRequest(body, session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.SOPDispatchResend.SOPDispatchTasksResend(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDocsHandler serializes /api/v1/admin/knowledge/documents.
func (handler Handler) KnowledgeDocsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDocs == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs service is not configured")
		return
	}
	payload, err := handler.KnowledgeDocs.KnowledgeDocs(r.Context(), workbench.NewKnowledgeDocsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDocUploadHandler serializes POST /api/v1/admin/knowledge/documents.
func (handler Handler) KnowledgeDocUploadHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDocsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs write service is not configured")
		return
	}
	filename, content, err := readKnowledgeMultipartFile(r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.KnowledgeDocsWrite.UploadKnowledgeDoc(r.Context(), workbench.NewKnowledgeDocUploadRequest(filename, content, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDocUpdateHandler serializes PUT /api/v1/admin/knowledge/documents/{doc_id}.
func (handler Handler) KnowledgeDocUpdateHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDocsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs write service is not configured")
		return
	}
	filename, content, err := readKnowledgeMultipartFile(r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.KnowledgeDocsWrite.UpdateKnowledgeDoc(r.Context(), workbench.NewKnowledgeDocUpdateRequest(r.PathValue("doc_id"), filename, content, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDocDeleteHandler serializes DELETE /api/v1/admin/knowledge/documents/{doc_id}.
func (handler Handler) KnowledgeDocDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDocsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs write service is not configured")
		return
	}
	payload, err := handler.KnowledgeDocsWrite.DeleteKnowledgeDoc(r.Context(), workbench.NewKnowledgeDocDeleteRequest(r.PathValue("doc_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDocReindexHandler serializes POST /api/v1/admin/knowledge/documents/{doc_id}/reindex.
func (handler Handler) KnowledgeDocReindexHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDocsWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs write service is not configured")
		return
	}
	payload, err := handler.KnowledgeDocsWrite.ReindexKnowledgeDoc(r.Context(), workbench.NewKnowledgeDocReindexRequest(r.PathValue("doc_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AdminKnowledgeSearchHandler serializes POST /api/v1/admin/knowledge/search.
func (handler Handler) AdminKnowledgeSearchHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeSearch == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge search service is not configured")
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.KnowledgeSearch.SearchKnowledge(r.Context(), workbench.NewKnowledgeSearchRequest(body.Query, "query", session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeDialogueHandler serializes POST /api/v1/admin/ai-config/test-dialogue.
func (handler Handler) KnowledgeDialogueHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeDialogue == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge dialogue service is not configured")
		return
	}
	var body workbench.KnowledgeDialogueBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.KnowledgeDialogue.KnowledgeDialogue(r.Context(), workbench.NewKnowledgeDialogueRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// KnowledgeSearchHandler serializes GET /api/v1/knowledge/search.
func (handler Handler) KnowledgeSearchHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.KnowledgeSearch == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge search service is not configured")
		return
	}
	payload, err := handler.KnowledgeSearch.SearchKnowledge(r.Context(), workbench.NewKnowledgeSearchRequest(r.URL.Query().Get("q"), "q", session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func readKnowledgeMultipartFile(r *http.Request) (string, []byte, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, workbench.ErrKnowledgeDocFileRequired
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return "", nil, err
	}
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	return filename, content, nil
}

// EnterprisesHandler serializes /api/v1/admin/enterprises.
func (handler Handler) EnterprisesHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Enterprises == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench enterprises service is not configured")
		return
	}
	payload, err := handler.Enterprises.Enterprises(r.Context(), workbench.NewEnterprisesRequest(queryBool(r, "with_secrets"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// EnterpriseUpsertHandler serializes POST /api/v1/admin/enterprises.
func (handler Handler) EnterpriseUpsertHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.EnterpriseWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench enterprise write service is not configured")
		return
	}
	var body workbench.EnterpriseUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.EnterpriseWrite.UpsertEnterprise(r.Context(), workbench.NewEnterpriseUpsertRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// EnterpriseDeleteHandler serializes DELETE /api/v1/admin/enterprises/{enterprise_id}.
func (handler Handler) EnterpriseDeleteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.EnterpriseWrite == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench enterprise write service is not configured")
		return
	}
	payload, err := handler.EnterpriseWrite.DeleteEnterprise(r.Context(), workbench.NewEnterpriseDeleteRequest(r.PathValue("enterprise_id"), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatsOverviewHandler serializes /api/v1/admin/stats/overview.
func (handler Handler) StatsOverviewHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsOverview == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats overview service is not configured")
		return
	}
	payload, err := handler.StatsOverview.StatsOverview(r.Context(), workbench.NewStatsOverviewRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func queryBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// StatsTrendHandler serializes /api/v1/admin/stats/trend.
func (handler Handler) StatsTrendHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsTrend == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats trend service is not configured")
		return
	}
	request, err := workbench.NewStatsTrendRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.StatsTrend.StatsTrend(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatsAgentsHandler serializes /api/v1/admin/stats/agents.
func (handler Handler) StatsAgentsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsAgents == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats agents service is not configured")
		return
	}
	payload, err := handler.StatsAgents.StatsAgents(r.Context(), workbench.NewStatsAgentsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatsAIReplyOverviewHandler serializes /api/v1/admin/stats/ai-replies/overview.
func (handler Handler) StatsAIReplyOverviewHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsAIReply == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats ai reply overview service is not configured")
		return
	}
	request, err := workbench.NewStatsAIReplyOverviewRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.StatsAIReply.StatsAIReplyOverview(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatsAIReplyTrendHandler serializes /api/v1/admin/stats/ai-replies/trend.
func (handler Handler) StatsAIReplyTrendHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsAITrend == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats ai reply trend service is not configured")
		return
	}
	request, err := workbench.NewStatsAIReplyTrendRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.StatsAITrend.StatsAIReplyTrend(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatsAIReplyBreakdownHandler serializes /api/v1/admin/stats/ai-replies/breakdown.
func (handler Handler) StatsAIReplyBreakdownHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.StatsBreakdown == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench stats ai reply breakdown service is not configured")
		return
	}
	request, err := workbench.NewStatsAIReplyBreakdownRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.StatsBreakdown.StatsAIReplyBreakdown(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
