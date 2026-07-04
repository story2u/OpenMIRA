// Script library handlers expose CS/admin quick-reply reads without migrating
// AI generation, admin writes, or WebSocket script broadcasts.
package workbenchhttp

import (
	"net/http"

	"wework-go/internal/workbench"
)

// ScriptLibraryHandler serializes /api/v1/scripts.
func (handler Handler) ScriptLibraryHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ScriptLibrary == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench script library service is not configured")
		return
	}
	payload, err := handler.ScriptLibrary.ScriptLibrary(r.Context(), workbench.NewReplyScriptsRequest(session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
