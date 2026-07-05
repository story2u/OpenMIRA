// JWT tests use locally signed HS256 tokens so they stay deterministic and do
// not depend on Python runtime state. The claims mirror the legacy payload
// shape documented in Python/docs/ai/features/auth-rbac.md.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVerifierAcceptsLegacyHS256Token(t *testing.T) {
	verifier := testVerifier(t)
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss":  "im-cloud",
		"sub":  "cs-001",
		"name": "消息端一",
		"role": "cs",
		"iat":  int64(1000),
		"exp":  int64(2000),
		"jti":  "jwt-test",
	})

	session, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if session.AssigneeID != "cs-001" || session.AssigneeName != "消息端一" || session.Role != "cs" || session.JTI != "jwt-test" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if !session.ExpiresAt.Equal(time.Unix(2000, 0).UTC()) {
		t.Fatalf("expires_at = %s, want unix 2000", session.ExpiresAt)
	}
}

func TestVerifierIssuesLegacyCompatibleToken(t *testing.T) {
	verifier := testVerifier(t)

	issued, err := verifier.Issue(IssueOptions{
		AssigneeID:   "cs-001",
		AssigneeName: "消息端一",
		Role:         "cs",
		TTL:          168 * time.Hour,
		JTI:          "jwt-issued",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	session, err := verifier.Verify(issued.Token)
	if err != nil {
		t.Fatalf("Verify issued token returned error: %v", err)
	}
	if session.AssigneeID != "cs-001" || session.AssigneeName != "消息端一" || session.Role != "cs" || session.JTI != "jwt-issued" {
		t.Fatalf("unexpected issued session: %+v", session)
	}
	if issued.CreatedAt.Unix() != 1000 || issued.ExpiresAt.Unix() != 605800 {
		t.Fatalf("unexpected issue times: created=%s expires=%s", issued.CreatedAt, issued.ExpiresAt)
	}
	if int64Claim(session.Claims, "iat") != 1000 {
		t.Fatalf("iat claim = %v, want 1000", session.Claims["iat"])
	}
}

func TestVerifierIssueDefaultsRoleAndMinimumTTL(t *testing.T) {
	verifier := testVerifier(t)

	issued, err := verifier.Issue(IssueOptions{AssigneeID: "cs-001", TTL: time.Minute, JTI: "jwt-short"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	if issued.Role != "cs" {
		t.Fatalf("role = %q, want cs", issued.Role)
	}
	if issued.ExpiresAt.Unix() != 4600 {
		t.Fatalf("expires_at unix = %d, want 4600", issued.ExpiresAt.Unix())
	}
}

func TestVerifierIssueRejectsMissingSubject(t *testing.T) {
	verifier := testVerifier(t)

	_, err := verifier.Issue(IssueOptions{JTI: "jwt-missing-sub"})

	if !errors.Is(err, ErrMissingSubject) {
		t.Fatalf("Issue error = %v, want %v", err, ErrMissingSubject)
	}
}

func TestVerifierIssueRejectsMissingSecret(t *testing.T) {
	_, err := Verifier{}.Issue(IssueOptions{AssigneeID: "cs-001", JTI: "jwt-missing-secret"})

	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("Issue error = %v, want %v", err, ErrMissingSecret)
	}
}

func TestVerifierDecodesRevocableTokenWithoutExpiryOrIssuerChecks(t *testing.T) {
	verifier := testVerifier(t)
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss": "other-issuer",
		"sub": "cs-001",
		"exp": int64(999),
		"jti": "jwt-logout",
	})

	revocable, ok := verifier.DecodeRevocableToken(token)

	if !ok || revocable.JTI != "jwt-logout" || revocable.ExpiresAt.Unix() != 999 {
		t.Fatalf("revocable = %+v ok=%t", revocable, ok)
	}
	if _, ok := verifier.DecodeRevocableToken("bad-token"); ok {
		t.Fatal("malformed token decoded as revocable")
	}
}

func TestVerifierRejectsInvalidSessionTokens(t *testing.T) {
	verifier := testVerifier(t)
	validClaims := map[string]any{
		"iss": "im-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-test",
	}
	cases := []struct {
		name  string
		token string
		want  error
	}{
		{name: "malformed", token: "bad-token", want: ErrMalformedToken},
		{name: "wrong issuer", token: signTestToken(t, verifier.Secret, cloneClaims(validClaims, "iss", "other")), want: ErrInvalidIssuer},
		{name: "expired", token: signTestToken(t, verifier.Secret, cloneClaims(validClaims, "exp", int64(999))), want: ErrExpired},
		{name: "missing subject", token: signTestToken(t, verifier.Secret, cloneClaims(validClaims, "sub", "")), want: ErrMissingSubject},
		{name: "invalid signature", token: signTestToken(t, "other-secret", validClaims), want: ErrInvalidSignature},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := verifier.Verify(testCase.token)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Verify error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestVerifierRejectsBlacklistedJTI(t *testing.T) {
	verifier := testVerifier(t)
	verifier.Blacklist = blacklistMap{"jwt-test": true}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss": "im-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-test",
	})

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrBlacklisted) {
		t.Fatalf("Verify error = %v, want %v", err, ErrBlacklisted)
	}
}

func TestVerifierPropagatesBlacklistStoreErrors(t *testing.T) {
	verifier := testVerifier(t)
	storeErr := errors.New("db unavailable")
	verifier.Blacklist = failingBlacklist{err: storeErr}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss": "im-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-test",
	})

	_, err := verifier.VerifyContext(context.Background(), token)
	if !errors.Is(err, ErrBlacklistUnavailable) || !errors.Is(err, storeErr) {
		t.Fatalf("VerifyContext error = %v, want blacklist unavailable wrapping store error", err)
	}
}

func TestParseBearerTokenAndRoles(t *testing.T) {
	if got := ParseBearerToken("Bearer abc.def"); got != "abc.def" {
		t.Fatalf("ParseBearerToken = %q", got)
	}
	if got := ParseBearerToken("Basic abc"); got != "" {
		t.Fatalf("ParseBearerToken for basic auth = %q, want empty", got)
	}
	session := Session{Role: "supervisor"}
	if !session.HasRole("admin", "supervisor") {
		t.Fatal("expected supervisor role to be accepted")
	}
	if session.HasRole("cs") {
		t.Fatal("expected supervisor role to reject cs-only access")
	}
}

type blacklistMap map[string]bool

func (blacklist blacklistMap) Contains(ctx context.Context, jti string) (bool, error) {
	return blacklist[jti], nil
}

type failingBlacklist struct {
	err error
}

func (blacklist failingBlacklist) Contains(ctx context.Context, jti string) (bool, error) {
	return false, blacklist.err
}

func testVerifier(t *testing.T) Verifier {
	t.Helper()
	verifier, err := NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time {
		return time.Unix(1000, 0).UTC()
	}
	return verifier
}

func signTestToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeTestPart(t, header)
	encodedClaims := encodeTestPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeTestPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func cloneClaims(claims map[string]any, key string, value any) map[string]any {
	copied := make(map[string]any, len(claims))
	for claimKey, claimValue := range claims {
		copied[claimKey] = claimValue
	}
	copied[key] = value
	return copied
}
