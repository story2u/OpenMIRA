package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/aioutreach"
	"wework-go/internal/aioutreachhttp"
	"wework-go/internal/config"
)

// TestNewWithModulesCanMountAIOutreachCandidate keeps platform-agent outreach opt-in.
func TestNewWithModulesCanMountAIOutreachCandidate(t *testing.T) {
	outreachHandler := aioutreachhttp.New(fakeAIOutreachService{}, "agent-token")
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		AIOutreach:          &outreachHandler,
		AIOutreachCandidate: true,
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/platform-agent/ai-outreach/conversation?wechat=agent-a&customer_id=customer-1", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, request, "/api/v1/platform-agent/ai-outreach/conversation", http.StatusOK, `"conversation_id":"conv-1"`)
	send := httptest.NewRequest(http.MethodPost, "/api/v1/platform-agent/ai-outreach/send", strings.NewReader(`{"corp_id":"corp-a","customer_id":"customer-1","wechat":"agent-a","plan_id":"plan-1","task_id":"task-1","reply_messages":[{"type":"text","content":"hello"}]}`))
	send.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, send, "/api/v1/platform-agent/ai-outreach/send", http.StatusOK, `"send_status":"accepted"`)

	routes := RoutesWithModules(Modules{
		AIOutreach:          &outreachHandler,
		AIOutreachCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	getRoute := routes[len(routes)-2]
	postRoute := routes[len(routes)-1]
	if getRoute.Path != "/api/v1/platform-agent/ai-outreach/conversation" || getRoute.Method != http.MethodGet || getRoute.Phase != "phase10-ai-outreach-candidate" {
		t.Fatalf("unexpected ai outreach conversation route metadata: %+v", getRoute)
	}
	if postRoute.Path != "/api/v1/platform-agent/ai-outreach/send" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase10-ai-outreach-candidate" {
		t.Fatalf("unexpected ai outreach send route metadata: %+v", postRoute)
	}
}

type fakeAIOutreachService struct{}

func (fakeAIOutreachService) QueryConversation(ctx context.Context, request aioutreach.ConversationRequest) (aioutreach.ConversationResponse, error) {
	return aioutreach.ConversationResponse{ConversationID: "conv-1"}, nil
}

func (fakeAIOutreachService) Send(ctx context.Context, request aioutreach.SendRequest) (aioutreach.SendResponse, error) {
	return aioutreach.SendResponse{SendStatus: "accepted", ConversationID: "conv-1", SystemTaskIDs: []string{"task-1"}}, nil
}
