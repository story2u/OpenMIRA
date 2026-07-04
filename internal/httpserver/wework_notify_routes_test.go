package httpserver

import (
	"net/http"
	"testing"

	"wework-go/internal/config"
	"wework-go/internal/weworknotifyhttp"
)

// TestNewWithModulesCanMountWeWorkNotifyCandidate keeps notify callback opt-in.
func TestNewWithModulesCanMountWeWorkNotifyCandidate(t *testing.T) {
	notifyHandler := weworknotifyhttp.New(nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		WeWorkNotify:                  &notifyHandler,
		WeWorkNotifyCallbackCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/notify/event/ent-1?msg_signature=sig&timestamp=123&nonce=n&echostr=e", http.StatusServiceUnavailable, "wework notify service is not configured")
	assertPostStatus(t, handler, "/api/v1/notify/event/ent-1?msg_signature=sig&timestamp=123&nonce=n", http.StatusServiceUnavailable, "wework notify service is not configured")

	routes := RoutesWithModules(Modules{
		WeWorkNotify:                  &notifyHandler,
		WeWorkNotifyCallbackCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	getRoute := routes[len(routes)-2]
	postRoute := routes[len(routes)-1]
	if getRoute.Path != "/api/v1/notify/event/{enterprise_id}" || getRoute.Method != http.MethodGet || getRoute.Phase != "phase11-wework-notify-callback-candidate" {
		t.Fatalf("unexpected notify callback GET route metadata: %+v", getRoute)
	}
	if postRoute.Path != "/api/v1/notify/event/{enterprise_id}" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase11-wework-notify-callback-candidate" {
		t.Fatalf("unexpected notify callback POST route metadata: %+v", postRoute)
	}
}
