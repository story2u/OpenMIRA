// Package workbenchmodule tests candidate assembly without registering routes.
// It verifies workbench dependencies can be wired behind explicit stores while
// the default Go mux still leaves the Python route owner unchanged.
package workbenchmodule

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/workbench"
)

func TestNewRejectsMissingSessionSecret(t *testing.T) {
	_, err := New(Options{Config: config.Config{SessionJWTIssuer: "wework-cloud"}})
	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("New error = %v, want %v", err, auth.ErrMissingSecret)
	}
}

func TestNewRequiresStores(t *testing.T) {
	_, err := New(Options{Config: config.Config{SessionJWTSecret: "session-secret"}})
	if !errors.Is(err, ErrStoresRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrStoresRequired)
	}
}

func TestNewRequiresBlacklistWhenRequested(t *testing.T) {
	_, err := New(Options{
		Config:                config.Config{SessionJWTSecret: "session-secret"},
		Accounts:              moduleAccounts{},
		Projection:            moduleProjection{},
		RequireBlacklistStore: true,
	})
	if !errors.Is(err, ErrBlacklistStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrBlacklistStoreRequired)
	}
}

func TestNewBuildsUnmountedWorkbenchHandler(t *testing.T) {
	module, err := New(Options{
		Config: config.Config{
			SessionJWTSecret: "session-secret",
			SessionJWTIssuer: "wework-cloud",
		},
		Accounts: moduleAccounts{
			{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
		},
		Projection: moduleProjection{
			rows: []workbench.ProjectionRow{{"conversation_id": "conv-001", "assignee_id": "cs-001"}},
			stats: map[string]workbench.ProjectionStats{
				"all|pending":   {ConversationCount: 1, AssignedCount: 1},
				"sensitive|all": {ConversationCount: 0},
			},
		},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Service == nil || module.AccountRepository != nil || module.ProjectionRepository != nil {
		t.Fatalf("unexpected module state: service=%v account_repo=%v projection_repo=%v", module.Service, module.AccountRepository, module.ProjectionRepository)
	}

	token := signWorkbenchModuleToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/cs/workbench/bootstrap", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	module.Handler.BootstrapHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"selected_account_id":"all"`, `"projection_candidate_v1":true`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

type moduleAccounts []workbench.AccountRecord

func (accounts moduleAccounts) ListAccounts(ctx context.Context) ([]workbench.AccountRecord, error) {
	return accounts, nil
}

type moduleProjection struct {
	rows  []workbench.ProjectionRow
	stats map[string]workbench.ProjectionStats
}

func (projection moduleProjection) ListRows(ctx context.Context, query workbench.ProjectionQuery) ([]workbench.ProjectionRow, error) {
	return projection.rows, nil
}

func (projection moduleProjection) CountScoped(ctx context.Context, query workbench.ProjectionQuery) (workbench.ProjectionStats, error) {
	return projection.stats[query.ModeFilter+"|"+query.StatusFilter], nil
}

func signWorkbenchModuleToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeWorkbenchModulePart(t, header)
	encodedClaims := encodeWorkbenchModulePart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeWorkbenchModulePart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
