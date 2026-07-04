// JWT code mirrors the legacy Python JwtSessionService contract.
// It validates existing tokens and can issue new HS256 session tokens for later
// login/refresh route migration without changing the legacy payload shape.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrMissingSecret means SESSION_JWT_SECRET was not configured.
	ErrMissingSecret = errors.New("session jwt secret is required")
	// ErrMalformedToken means the token cannot be parsed as header.payload.signature.
	ErrMalformedToken = errors.New("session token is malformed")
	// ErrUnsupportedAlgorithm means the token header is not HS256.
	ErrUnsupportedAlgorithm = errors.New("session token algorithm is unsupported")
	// ErrInvalidSignature means the HMAC signature does not match.
	ErrInvalidSignature = errors.New("session token signature is invalid")
	// ErrInvalidIssuer means iss does not match the configured issuer.
	ErrInvalidIssuer = errors.New("session token issuer is invalid")
	// ErrMissingJTI means jti is absent, so logout/blacklist cannot work.
	ErrMissingJTI = errors.New("session token jti is missing")
	// ErrBlacklisted means jti is explicitly revoked.
	ErrBlacklisted = errors.New("session token is blacklisted")
	// ErrBlacklistUnavailable means the blacklist fact source could not be read.
	ErrBlacklistUnavailable = errors.New("session token blacklist is unavailable")
	// ErrExpired means exp is not in the future.
	ErrExpired = errors.New("session token is expired")
	// ErrMissingSubject means sub is absent, so no assignee can be resolved.
	ErrMissingSubject = errors.New("session token subject is missing")
)

// Blacklist checks whether a JWT jti has been revoked.
type Blacklist interface {
	Contains(ctx context.Context, jti string) (bool, error)
}

// Revoker records a JWT jti as revoked until its original expiry.
type Revoker interface {
	Add(ctx context.Context, jti string, expiresAt time.Time) error
}

// Verifier validates legacy HS256 session JWTs.
type Verifier struct {
	Secret    string
	Issuer    string
	Now       func() time.Time
	Blacklist Blacklist
}

// IssueOptions describes the legacy JWT claims that can be safely generated.
type IssueOptions struct {
	AssigneeID   string
	AssigneeName string
	Role         string
	TTL          time.Duration
	JTI          string
}

// IssuedSession is the Go equivalent of Python JwtSessionService.login output.
type IssuedSession struct {
	Token        string
	AssigneeID   string
	AssigneeName string
	Role         string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	JTI          string
}

// RevocableToken is the minimal signed JWT payload needed for logout.
type RevocableToken struct {
	JTI       string
	ExpiresAt time.Time
	Claims    map[string]any
}

// NewVerifier builds a verifier with the legacy default issuer.
func NewVerifier(secret string, issuer string) (Verifier, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return Verifier{}, ErrMissingSecret
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = "wework-cloud"
	}
	return Verifier{Secret: secret, Issuer: issuer, Now: time.Now}, nil
}

// Verify validates token signature, issuer, blacklist, expiry, and subject.
func (verifier Verifier) Verify(token string) (Session, error) {
	return verifier.VerifyContext(context.Background(), token)
}

// VerifyContext validates token state using ctx for blacklist lookups.
func (verifier Verifier) VerifyContext(ctx context.Context, token string) (Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	claims, err := verifier.decode(token)
	if err != nil {
		return Session{}, err
	}
	if stringClaim(claims, "iss") != verifier.issuer() {
		return Session{}, ErrInvalidIssuer
	}
	jti := stringClaim(claims, "jti")
	if jti == "" {
		return Session{}, ErrMissingJTI
	}
	if verifier.Blacklist != nil {
		blacklisted, err := verifier.Blacklist.Contains(ctx, jti)
		if err != nil {
			return Session{}, fmt.Errorf("%w: %w", ErrBlacklistUnavailable, err)
		}
		if blacklisted {
			return Session{}, ErrBlacklisted
		}
	}
	exp := int64Claim(claims, "exp")
	now := verifier.now().Unix()
	if exp <= now {
		return Session{}, ErrExpired
	}
	subject := stringClaim(claims, "sub")
	if subject == "" {
		return Session{}, ErrMissingSubject
	}
	role := stringClaim(claims, "role")
	if role == "" {
		role = "cs"
	}
	return Session{
		AssigneeID:   subject,
		AssigneeName: nullableStringClaim(claims, "name"),
		Role:         role,
		ExpiresAt:    time.Unix(exp, 0).UTC(),
		JTI:          jti,
		Claims:       claims,
	}, nil
}

// Issue signs a new legacy-compatible HS256 session JWT.
func (verifier Verifier) Issue(options IssueOptions) (IssuedSession, error) {
	if strings.TrimSpace(verifier.Secret) == "" {
		return IssuedSession{}, ErrMissingSecret
	}
	assigneeID := strings.TrimSpace(options.AssigneeID)
	if assigneeID == "" {
		return IssuedSession{}, ErrMissingSubject
	}
	role := strings.TrimSpace(options.Role)
	if role == "" {
		role = "cs"
	}
	ttl := options.TTL
	if ttl < time.Hour {
		ttl = time.Hour
	}
	now := verifier.now().UTC()
	expiresAt := now.Add(ttl)
	jti := strings.TrimSpace(options.JTI)
	if jti == "" {
		generatedJTI, err := randomJTI()
		if err != nil {
			return IssuedSession{}, err
		}
		jti = generatedJTI
	}
	claims := map[string]any{
		"iss":  verifier.issuer(),
		"sub":  assigneeID,
		"name": options.AssigneeName,
		"role": role,
		"iat":  now.Unix(),
		"exp":  expiresAt.Unix(),
		"jti":  jti,
	}
	token, err := verifier.encode(claims)
	if err != nil {
		return IssuedSession{}, err
	}
	return IssuedSession{
		Token:        token,
		AssigneeID:   assigneeID,
		AssigneeName: options.AssigneeName,
		Role:         role,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
		JTI:          jti,
	}, nil
}

// DecodeRevocableToken mirrors the legacy logout path: it validates the HS256
// signature and extracts jti/exp without requiring the token to be unexpired.
func (verifier Verifier) DecodeRevocableToken(token string) (RevocableToken, bool) {
	claims, err := verifier.decode(token)
	if err != nil {
		return RevocableToken{}, false
	}
	jti := stringClaim(claims, "jti")
	exp := int64Claim(claims, "exp")
	if jti == "" || exp <= 0 {
		return RevocableToken{}, false
	}
	return RevocableToken{JTI: jti, ExpiresAt: time.Unix(exp, 0).UTC(), Claims: claims}, true
}

func (verifier Verifier) encode(claims map[string]any) (string, error) {
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader, err := encodeJSONPart(header)
	if err != nil {
		return "", err
	}
	encodedClaims, err := encodeJSONPart(claims)
	if err != nil {
		return "", err
	}
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(verifier.Secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

func (verifier Verifier) decode(token string) (map[string]any, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, ErrMalformedToken
	}
	header, err := decodeJSONPart(parts[0])
	if err != nil {
		return nil, ErrMalformedToken
	}
	if stringClaim(header, "alg") != "HS256" {
		return nil, ErrUnsupportedAlgorithm
	}
	signingInput := parts[0] + "." + parts[1]
	expected := hmac.New(sha256.New, []byte(verifier.Secret))
	_, _ = expected.Write([]byte(signingInput))
	actual, err := decodeBase64URL(parts[2])
	if err != nil {
		return nil, ErrMalformedToken
	}
	if !hmac.Equal(expected.Sum(nil), actual) {
		return nil, ErrInvalidSignature
	}
	claims, err := decodeJSONPart(parts[1])
	if err != nil {
		return nil, ErrMalformedToken
	}
	return claims, nil
}

func encodeJSONPart(value map[string]any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeJSONPart(part string) (map[string]any, error) {
	raw, err := decodeBase64URL(part)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("json object is empty")
	}
	return value, nil
}

func decodeBase64URL(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	padding := strings.Repeat("=", (4-len(value)%4)%4)
	return base64.URLEncoding.DecodeString(value + padding)
}

func randomJTI() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "jwt-" + hex.EncodeToString(raw[:]), nil
}

func (verifier Verifier) issuer() string {
	issuer := strings.TrimSpace(verifier.Issuer)
	if issuer == "" {
		return "wework-cloud"
	}
	return issuer
}

func (verifier Verifier) now() time.Time {
	if verifier.Now == nil {
		return time.Now()
	}
	return verifier.Now()
}

func stringClaim(claims map[string]any, key string) string {
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func nullableStringClaim(claims map[string]any, key string) string {
	value := stringClaim(claims, key)
	if value == "" {
		return ""
	}
	return value
}

func int64Claim(claims map[string]any, key string) int64 {
	switch value := claims[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	case string:
		var parsed int64
		_, _ = fmt.Sscan(value, &parsed)
		return parsed
	default:
		return 0
	}
}
