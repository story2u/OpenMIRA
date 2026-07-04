// Package archivemedia builds legacy-compatible access URLs for archived media.
// It only signs already persisted object references; uploading, proxying, and
// object-store SDK calls remain outside the candidate message read path.
package archivemedia

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultSigningKey = "archive-media-secret"

// AccessURLBuilder mirrors Python ArchiveMediaStorageService.build_access_url.
type AccessURLBuilder struct {
	BaseURL               string
	ObjectPublicBaseURL   string
	PreferDirectObjectURL bool
	SigningKey            string
	TokenTTL              time.Duration
	Now                   func() time.Time
}

type tokenPayload struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Exp          int64  `json:"exp"`
	Sig          string `json:"sig"`
}

// BuildAccessURL returns the browser-facing media URL for a completed task.
func (builder AccessURLBuilder) BuildAccessURL(taskID string, objectURL string) string {
	objectPath := ExtractObjectPath(objectURL)
	if objectPath != "" {
		token := builder.signPayload("object", objectPath)
		query := url.Values{"token": []string{token}}.Encode()
		if builder.PreferDirectObjectURL && strings.TrimSpace(builder.ObjectPublicBaseURL) != "" {
			if builder.usesPlainObjectPublicBaseURL() {
				return joinURLPath(builder.ObjectPublicBaseURL, objectPath)
			}
			return joinURLPath(builder.signedObjectPublicBaseURL(), objectPath) + "?" + query
		}
		if builder.PreferDirectObjectURL {
			return joinURLPath("/signed/objects", objectPath) + "?" + query
		}
		if strings.TrimSpace(builder.BaseURL) != "" {
			return joinURLPath(builder.BaseURL, "api/v1/archive/media/objects/"+objectPath) + "?" + query
		}
		return joinURLPath("/api/v1/archive/media/objects", objectPath) + "?" + query
	}

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	token := builder.signPayload("task", taskID)
	query := url.Values{"token": []string{token}}.Encode()
	if strings.TrimSpace(builder.BaseURL) != "" {
		return joinURLPath(builder.BaseURL, "api/v1/archive/media/files/"+taskID) + "?" + query
	}
	return joinURLPath("/api/v1/archive/media/files", taskID) + "?" + query
}

func (builder AccessURLBuilder) signPayload(resourceType string, resourceID string) string {
	now := time.Now
	if builder.Now != nil {
		now = builder.Now
	}
	ttl := builder.TokenTTL
	if ttl < time.Minute {
		ttl = time.Hour * 24
	}
	signingKey := strings.TrimSpace(builder.SigningKey)
	if signingKey == "" {
		signingKey = defaultSigningKey
	}
	exp := now().UTC().Add(ttl).Unix()
	plain := resourceType + ":" + resourceID + ":" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(signingKey))
	_, _ = mac.Write([]byte(plain))
	payload := tokenPayload{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Exp:          exp,
		Sig:          hex.EncodeToString(mac.Sum(nil)),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// VerifyAccessToken validates a task-scoped archive media access token.
func (builder AccessURLBuilder) VerifyAccessToken(taskID string, token string) bool {
	return builder.verifyPayloadToken("task", strings.TrimSpace(taskID), token)
}

// VerifyObjectAccessToken validates an object-path-scoped archive media access token.
func (builder AccessURLBuilder) VerifyObjectAccessToken(objectPath string, token string) bool {
	return builder.verifyPayloadToken("object", strings.TrimLeft(strings.TrimSpace(objectPath), "/"), token)
}

func (builder AccessURLBuilder) verifyPayloadToken(resourceType string, resourceID string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceID) == "" {
		return false
	}
	payload, ok := decodeAccessToken(token)
	if !ok {
		return false
	}
	if payload.ResourceType != resourceType || payload.ResourceID != resourceID {
		return false
	}
	now := time.Now
	if builder.Now != nil {
		now = builder.Now
	}
	if payload.Exp <= now().UTC().Unix() {
		return false
	}
	plain := payload.ResourceType + ":" + payload.ResourceID + ":" + strconv.FormatInt(payload.Exp, 10)
	mac := hmac.New(sha256.New, []byte(builder.signingKey()))
	_, _ = mac.Write([]byte(plain))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(payload.Sig), []byte(expected))
}

func decodeAccessToken(token string) (tokenPayload, bool) {
	var data []byte
	var err error
	for _, encoding := range []*base64.Encoding{base64.URLEncoding, base64.RawURLEncoding} {
		data, err = encoding.DecodeString(token)
		if err == nil {
			break
		}
	}
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return tokenPayload{}, false
	}
	var payload tokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return tokenPayload{}, false
	}
	return payload, true
}

func (builder AccessURLBuilder) signingKey() string {
	signingKey := strings.TrimSpace(builder.SigningKey)
	if signingKey == "" {
		return defaultSigningKey
	}
	return signingKey
}

func (builder AccessURLBuilder) signedObjectPublicBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(builder.ObjectPublicBaseURL), "/")
	if base == "" {
		return "/signed/objects"
	}
	lowered := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lowered, "/signed/objects"):
		return base
	case strings.HasSuffix(lowered, "/signed"):
		return base + "/objects"
	default:
		return base + "/signed/objects"
	}
}

func (builder AccessURLBuilder) usesPlainObjectPublicBaseURL() bool {
	base := strings.TrimRight(strings.TrimSpace(builder.ObjectPublicBaseURL), "/")
	if base == "" {
		return false
	}
	lowered := strings.ToLower(base)
	return !strings.HasSuffix(lowered, "/media-objects") &&
		!strings.HasSuffix(lowered, "/signed") &&
		!strings.HasSuffix(lowered, "/signed/objects")
}

// ExtractObjectPath returns the object-store path encoded in a stored object URL.
func ExtractObjectPath(objectURL string) string {
	raw := strings.TrimSpace(objectURL)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && strings.EqualFold(parsed.Scheme, "local") {
		return ""
	}
	path := ""
	if err == nil {
		path = strings.TrimSpace(parsed.Path)
	}
	const marker = "/objects/"
	if strings.Contains(path, marker) {
		return strings.TrimLeft(strings.SplitN(path, marker, 2)[1], "/")
	}
	if strings.HasPrefix(path, marker) {
		return strings.TrimLeft(strings.TrimPrefix(path, marker), "/")
	}
	if strings.Contains(raw, "://") {
		return ""
	}
	if !strings.Contains(raw, "/") {
		return ""
	}
	return strings.TrimLeft(raw, "/")
}

func extractObjectPath(objectURL string) string {
	return ExtractObjectPath(objectURL)
}

// ExtractLocalObjectPath returns the backend/data-relative path for local:// media objects.
func ExtractLocalObjectPath(objectURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(objectURL))
	if err != nil || !strings.EqualFold(parsed.Scheme, "local") {
		return ""
	}
	parts := []string{strings.Trim(parsed.Host, "/"), strings.Trim(parsed.Path, "/")}
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
}

func joinURLPath(base string, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if base == "" {
		return "/" + path
	}
	if path == "" {
		return base
	}
	return base + "/" + path
}
