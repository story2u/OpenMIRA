package voicetranscription

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTAccessTokenProviderRequestsAndCachesToken(t *testing.T) {
	privateKey := mustRSAKey(t)
	privatePEM := encodePKCS8PrivateKey(t, privateKey)
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	requests := 0
	var gotAssertion string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		gotAssertion = strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if err := json.NewDecoder(request.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode token request: %v", err)
		}
		_, _ = writer.Write([]byte(`{"access_token":"access-1","expires_in":120}`))
	}))
	defer server.Close()

	provider := NewJWTAccessTokenProvider(JWTAccessTokenConfig{
		BaseURL:        server.URL + "/v1/workflow/run",
		ClientID:       "client-1",
		PublicKeyID:    "kid-1",
		PrivateKeyPEM:  privatePEM,
		AccessTokenTTL: time.Hour,
	})
	provider.Now = func() time.Time { return now }
	provider.Random = strings.NewReader(strings.Repeat("a", 64))

	token, err := provider.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken returned error: %v", err)
	}
	if token != "access-1" {
		t.Fatalf("token = %q", token)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
	if gotPayload["duration_seconds"] != float64(3600) || gotPayload["grant_type"] != cozeJWTGrantType {
		t.Fatalf("token payload = %#v", gotPayload)
	}
	assertionPayload := verifyAssertion(t, gotAssertion, &privateKey.PublicKey)
	if assertionPayload["iss"] != "client-1" || assertionPayload["aud"] != strings.TrimPrefix(server.URL, "http://") {
		t.Fatalf("assertion payload = %#v", assertionPayload)
	}
	if intNumber(assertionPayload["iat"]) != int(now.Unix()) || intNumber(assertionPayload["exp"]) != int(now.Unix()+3600) {
		t.Fatalf("assertion times = %#v", assertionPayload)
	}

	token, err = provider.AccessToken(context.Background())
	if err != nil || token != "access-1" {
		t.Fatalf("cached AccessToken token=%q err=%v", token, err)
	}
	if requests != 1 {
		t.Fatalf("cache miss requests = %d", requests)
	}
}

func TestJWTAccessTokenProviderClassifiesRetryableHTTPStatus(t *testing.T) {
	privatePEM := encodePKCS8PrivateKey(t, mustRSAKey(t))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "too many", http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider := NewJWTAccessTokenProvider(JWTAccessTokenConfig{
		BaseURL:       server.URL + "/v1/workflow/run",
		ClientID:      "client-1",
		PublicKeyID:   "kid-1",
		PrivateKeyPEM: privatePEM,
	})

	_, err := provider.AccessToken(context.Background())
	var retryable RetryableError
	if err == nil || !errors.As(err, &retryable) || !strings.Contains(retryable.RawResponseJSON, "too many") {
		t.Fatalf("err = %#v", err)
	}
}

func TestJWTAccessTokenProviderRejectsInvalidPrivateKey(t *testing.T) {
	provider := NewJWTAccessTokenProvider(JWTAccessTokenConfig{
		BaseURL:       "https://api.coze.cn/v1/workflow/run",
		ClientID:      "client-1",
		PublicKeyID:   "kid-1",
		PrivateKeyPEM: "bad",
	})
	_, err := provider.BuildJWTAssertion()
	var terminal TerminalError
	if err == nil || !errors.As(err, &terminal) {
		t.Fatalf("err = %#v", err)
	}
}

func TestJWTAccessTokenConfigDerivesAudienceAndTokenURL(t *testing.T) {
	config := JWTAccessTokenConfig{BaseURL: "https://api.coze.cn/v1/workflow/run"}
	audience, err := config.Audience()
	if err != nil || audience != "api.coze.cn" {
		t.Fatalf("audience=%q err=%v", audience, err)
	}
	tokenURL, err := config.TokenURL()
	if err != nil || tokenURL != "https://api.coze.cn/api/permission/oauth2/token" {
		t.Fatalf("tokenURL=%q err=%v", tokenURL, err)
	}
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}

func encodePKCS8PrivateKey(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
}

func verifyAssertion(t *testing.T, assertion string, publicKey *rsa.PublicKey) map[string]any {
	t.Helper()
	segments := strings.Split(assertion, ".")
	if len(segments) != 3 {
		t.Fatalf("assertion segment count = %d", len(segments))
	}
	headerData, err := base64.RawURLEncoding.DecodeString(segments[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerData, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "kid-1" {
		t.Fatalf("header = %#v", header)
	}
	payloadData, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadData, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sum := sha256.Sum256([]byte(segments[0] + "." + segments[1]))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, sum[:], signature); err != nil {
		t.Fatalf("VerifyPKCS1v15: %v", err)
	}
	if strings.Count(strings.TrimSpace(textValue(payload["jti"])), ":") != 2 {
		t.Fatalf("jti = %q", payload["jti"])
	}
	return payload
}
