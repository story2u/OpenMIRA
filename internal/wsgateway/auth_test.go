package wsgateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestAuthenticatorAcceptsSessionAgentAndLegacyBypass(t *testing.T) {
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Unix(100, 0).UTC() }
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "jwt-ws"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	authenticator := Authenticator{SessionVerifier: &verifier, AgentToken: "agent-token"}

	if err := authenticator.Authenticate(context.Background(), issued.Token, ""); err != nil {
		t.Fatalf("session auth returned error: %v", err)
	}
	if err := authenticator.Authenticate(context.Background(), "invalid", "agent-token"); err != nil {
		t.Fatalf("agent auth returned error: %v", err)
	}
	if err := (Authenticator{AllowLegacy: true}).Authenticate(context.Background(), "", ""); err != nil {
		t.Fatalf("legacy auth returned error: %v", err)
	}
}

func TestAuthenticatorRejectsMissingCredentials(t *testing.T) {
	err := (Authenticator{AgentToken: "agent-token"}).Authenticate(context.Background(), "", "wrong")
	if !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("error = %v, want %v", err, ErrAuthenticationRequired)
	}
}
