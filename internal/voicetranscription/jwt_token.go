package voicetranscription

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const cozeJWTGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

// JWTAccessTokenConfig describes Coze service OAuth JWT bearer credentials.
type JWTAccessTokenConfig struct {
	BaseURL        string
	ClientID       string
	PublicKeyID    string
	PrivateKeyPEM  string
	AccessTokenTTL time.Duration
	Timeout        time.Duration
}

// JWTAccessTokenProvider exchanges signed JWT assertions for Coze access tokens.
type JWTAccessTokenProvider struct {
	Config     JWTAccessTokenConfig
	HTTPClient *http.Client
	Now        func() time.Time
	Random     io.Reader

	mu            sync.Mutex
	cachedKey     string
	cachedToken   string
	cachedExpires int64
}

// NewJWTAccessTokenProvider returns a reusable provider with in-process caching.
func NewJWTAccessTokenProvider(config JWTAccessTokenConfig) *JWTAccessTokenProvider {
	return &JWTAccessTokenProvider{Config: config}
}

// AccessToken returns a cached token or refreshes it with a signed JWT assertion.
func (provider *JWTAccessTokenProvider) AccessToken(ctx context.Context) (string, error) {
	cacheKey := provider.Config.CacheKey()
	if provider.isCacheValid(cacheKey) {
		return provider.cachedToken, nil
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.isCacheValid(cacheKey) {
		return provider.cachedToken, nil
	}
	token, expiresAt, err := provider.requestAccessToken(ctx)
	if err != nil {
		return "", err
	}
	provider.cachedKey = cacheKey
	provider.cachedToken = token
	provider.cachedExpires = expiresAt
	return token, nil
}

// Audience returns the JWT aud claim derived from the Coze API host.
func (config JWTAccessTokenConfig) Audience() (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", TerminalError{Message: "coze jwt base_url is invalid"}
	}
	return parsed.Host, nil
}

// TokenURL returns the OAuth token endpoint derived from the workflow endpoint.
func (config JWTAccessTokenConfig) TokenURL() (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", TerminalError{Message: "coze jwt base_url is invalid"}
	}
	return parsed.Scheme + "://" + parsed.Host + "/api/permission/oauth2/token", nil
}

// CacheKey returns a stable hash of all credential material that affects tokens.
func (config JWTAccessTokenConfig) CacheKey() string {
	ttlSeconds := int(config.accessTokenTTL().Seconds())
	raw := strings.Join([]string{
		strings.TrimSpace(config.BaseURL),
		strings.TrimSpace(config.ClientID),
		strings.TrimSpace(config.PublicKeyID),
		strings.TrimSpace(config.PrivateKeyPEM),
		fmt.Sprint(ttlSeconds),
	}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (provider *JWTAccessTokenProvider) requestAccessToken(ctx context.Context) (string, int64, error) {
	assertion, err := provider.BuildJWTAssertion()
	if err != nil {
		return "", 0, err
	}
	tokenURL, err := provider.Config.TokenURL()
	if err != nil {
		return "", 0, err
	}
	body, err := json.Marshal(map[string]any{
		"duration_seconds": int(provider.Config.accessTokenTTL().Seconds()),
		"grant_type":       cozeJWTGrantType,
	})
	if err != nil {
		return "", 0, TerminalError{Message: err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", 0, TerminalError{Message: err.Error()}
	}
	request.Header.Set("Authorization", "Bearer "+assertion)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.httpClient().Do(request)
	if err != nil {
		return "", 0, RetryableError{Message: "coze jwt token request failed: " + err.Error()}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", 0, RetryableError{Message: "coze jwt token response read failed: " + err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = fmt.Sprintf("http status=%d", response.StatusCode)
		}
		if _, ok := retryableHTTPStatuses[response.StatusCode]; ok {
			return "", 0, RetryableError{Message: message, RawResponseJSON: message}
		}
		return "", 0, TerminalError{Message: message, RawResponseJSON: message}
	}
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return "", 0, TerminalError{
			Message:         "coze jwt token response json decode failed: " + err.Error(),
			RawResponseJSON: strings.TrimSpace(string(responseBody)),
		}
	}
	accessToken := strings.TrimSpace(textValue(payload["access_token"]))
	if accessToken == "" {
		return "", 0, TerminalError{
			Message:         "coze jwt token response missing access_token",
			RawResponseJSON: compactJSON(payload),
		}
	}
	return accessToken, provider.resolveExpiresAt(payload["expires_in"]), nil
}

// BuildJWTAssertion signs the Coze JWT bearer assertion.
func (provider *JWTAccessTokenProvider) BuildJWTAssertion() (string, error) {
	config := provider.Config
	audience, err := config.Audience()
	if err != nil {
		return "", err
	}
	privateKey, err := parseRSAPrivateKey(config.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	now := provider.now().Unix()
	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": strings.TrimSpace(config.PublicKeyID),
	}
	payload := map[string]any{
		"iss": strings.TrimSpace(config.ClientID),
		"aud": audience,
		"iat": now,
		"exp": now + 3600,
		"jti": provider.jti(now),
	}
	headerSegment := base64URLJSON(header)
	payloadSegment := base64URLJSON(payload)
	signingInput := []byte(headerSegment + "." + payloadSegment)
	sum := sha256.Sum256(signingInput)
	signature, err := rsa.SignPKCS1v15(provider.random(), privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", TerminalError{Message: "coze jwt assertion sign failed: " + err.Error()}
	}
	return headerSegment + "." + payloadSegment + "." + base64URL(signature), nil
}

func parseRSAPrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(privateKeyPEM)))
	if block == nil {
		return nil, TerminalError{Message: "coze jwt private_key_pem is invalid: missing PEM block"}
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if key, ok := parsed.(*rsa.PrivateKey); ok {
			return key, nil
		}
		return nil, TerminalError{Message: "coze jwt private_key_pem must be an RSA private key"}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, TerminalError{Message: "coze jwt private_key_pem is invalid"}
}

func (provider *JWTAccessTokenProvider) isCacheValid(cacheKey string) bool {
	now := provider.now().Unix()
	return provider.cachedKey != "" &&
		provider.cachedKey == cacheKey &&
		provider.cachedToken != "" &&
		now < provider.cachedExpires-60
}

func (provider *JWTAccessTokenProvider) resolveExpiresAt(raw any) int64 {
	now := provider.now().Unix()
	expires := int64(intNumber(raw))
	if expires > now+300 {
		return expires
	}
	if expires > 0 {
		return now + expires
	}
	return now + int64(provider.Config.accessTokenTTL().Seconds())
}

func (config JWTAccessTokenConfig) accessTokenTTL() time.Duration {
	ttl := config.AccessTokenTTL
	if ttl < time.Minute {
		ttl = time.Hour
	}
	if ttl > 86399*time.Second {
		ttl = 86399 * time.Second
	}
	return ttl
}

func (provider *JWTAccessTokenProvider) httpClient() *http.Client {
	if provider.HTTPClient != nil {
		return provider.HTTPClient
	}
	timeout := provider.Config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func (provider *JWTAccessTokenProvider) now() time.Time {
	if provider.Now == nil {
		return time.Now().UTC()
	}
	return provider.Now().UTC()
}

func (provider *JWTAccessTokenProvider) random() io.Reader {
	if provider.Random == nil {
		return rand.Reader
	}
	return provider.Random
}

func (provider *JWTAccessTokenProvider) jti(now int64) string {
	var first [8]byte
	var second [8]byte
	_, _ = io.ReadFull(provider.random(), first[:])
	_, _ = io.ReadFull(provider.random(), second[:])
	return fmt.Sprintf("%d:%s:%s", now, hex.EncodeToString(first[:]), hex.EncodeToString(second[:]))
}

func base64URLJSON(value map[string]any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return base64URL(data)
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
