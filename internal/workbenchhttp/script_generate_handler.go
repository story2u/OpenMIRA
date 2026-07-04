package workbenchhttp

import (
	"encoding/json"
	"net/http"

	"wework-go/internal/workbench"
)

// ScriptGenerateHandler serializes POST /api/v1/scripts/generate.
func (handler Handler) ScriptGenerateHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ScriptGenerate == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench script generation service is not configured")
		return
	}
	var body workbench.ScriptGenerateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.ScriptGenerate.GenerateScript(r.Context(), workbench.NewScriptGenerateRequest(body, session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
