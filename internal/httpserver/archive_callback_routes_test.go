package httpserver

import (
	"net/http"
	"testing"

	"wework-go/internal/archivecallbackhttp"
	"wework-go/internal/config"
)

// TestNewWithModulesCanMountArchiveCallbackCandidate keeps callback opt-in.
func TestNewWithModulesCanMountArchiveCallbackCandidate(t *testing.T) {
	callbackHandler := archivecallbackhttp.New(nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		ArchiveCallback:          &callbackHandler,
		ArchiveCallbackCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/archive/callback/ent-1?msg_signature=sig&timestamp=123&nonce=n&echostr=e", http.StatusServiceUnavailable, "archive callback service is not configured")
	assertPostStatus(t, handler, "/api/v1/archive/callback/ent-1?msg_signature=sig&timestamp=123&nonce=n", http.StatusServiceUnavailable, "archive callback service is not configured")

	routes := RoutesWithModules(Modules{
		ArchiveCallback:          &callbackHandler,
		ArchiveCallbackCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	getRoute := routes[len(routes)-2]
	postRoute := routes[len(routes)-1]
	if getRoute.Path != "/api/v1/archive/callback/{enterprise_id}" || getRoute.Method != http.MethodGet || getRoute.Phase != "phase9-archive-callback-candidate" {
		t.Fatalf("unexpected archive callback GET route metadata: %+v", getRoute)
	}
	if postRoute.Path != "/api/v1/archive/callback/{enterprise_id}" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase9-archive-callback-candidate" {
		t.Fatalf("unexpected archive callback POST route metadata: %+v", postRoute)
	}
}

// TestNewWithModulesCanMountArchiveCallbackReceiptsCandidate keeps the static monitor route opt-in.
func TestNewWithModulesCanMountArchiveCallbackReceiptsCandidate(t *testing.T) {
	callbackHandler := archivecallbackhttp.New(nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		ArchiveCallback:          &callbackHandler,
		ArchiveCallbackCandidate: true,
		ArchiveCallbackReceipts:  true,
	})

	assertStatus(t, handler, "/api/v1/archive/callback/receipts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		ArchiveCallback:          &callbackHandler,
		ArchiveCallbackCandidate: true,
		ArchiveCallbackReceipts:  true,
	})
	if len(routes) != 7 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 7", len(routes))
	}
	receiptRoute := routes[len(routes)-3]
	if receiptRoute.Path != "/api/v1/archive/callback/receipts" || receiptRoute.Method != http.MethodGet || receiptRoute.Phase != "phase9-archive-callback-candidate" {
		t.Fatalf("unexpected archive callback receipts route metadata: %+v", receiptRoute)
	}
}
