package httpserver

import (
	"net/http"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/voicetranscriptionhttp"
)

// TestNewWithModulesCanMountArchiveVoiceRetryCandidate keeps manual retry opt-in.
func TestNewWithModulesCanMountArchiveVoiceRetryCandidate(t *testing.T) {
	voiceHandler := voicetranscriptionhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		ArchiveVoiceTranscription:  &voiceHandler,
		ArchiveVoiceRetryCandidate: true,
	})

	assertPostStatus(t, handler, "/api/v1/archive/voice-transcriptions/retry", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		ArchiveVoiceTranscription:  &voiceHandler,
		ArchiveVoiceRetryCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/voice-transcriptions/retry" || last.Method != http.MethodPost || last.Phase != "phase9-archive-voice-candidate" {
		t.Fatalf("unexpected archive voice route metadata: %+v", last)
	}
}
