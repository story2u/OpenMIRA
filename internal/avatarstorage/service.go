// Package avatarstorage normalizes contact avatar references using the legacy
// archive media object boundary.
package avatarstorage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"wework-go/internal/archivemedia"
)

const maxInlineAvatarBytes = 256 * 1024

var base64ImagePrefixes = []string{"iVBOR", "/9j/", "R0lGOD", "UklGR", "Qk0", "PD94bWw"}

// Uploader stores inline avatar image bytes as archive media objects.
type Uploader interface {
	UploadArchiveMedia(ctx context.Context, input archivemedia.UploadInput) (string, error)
}

// AccessURLBuilder turns stored object references into browser-facing URLs.
type AccessURLBuilder interface {
	BuildAccessURL(taskID string, objectURL string) string
}

// Service mirrors the Python avatar storage behavior used by contact sync.
type Service struct {
	Uploader      Uploader
	Access        AccessURLBuilder
	LocalDataRoot string
}

// PersistAvatarReference returns a persistence-safe avatar reference.
func (service Service) PersistAvatarReference(ctx context.Context, enterpriseID string, sourceKey string, avatarValue string) string {
	avatar := strings.TrimSpace(avatarValue)
	if avatar == "" {
		return ""
	}
	if archivemedia.ExtractObjectPath(avatar) != "" {
		return avatar
	}
	if strings.HasPrefix(strings.ToLower(avatar), "http://") || strings.HasPrefix(strings.ToLower(avatar), "https://") {
		return inlineAvatarDisplayValue(avatar)
	}
	content, contentType, extension, ok := readAvatarPayload(avatar)
	if ok && isSafeInlineAvatarPayload(content, contentType) {
		if uploaded := service.uploadAvatarBytes(ctx, enterpriseID, sourceKey, content, contentType, extension); uploaded != "" {
			return uploaded
		}
	}
	return inlineAvatarDisplayValue(avatar)
}

// ResolveAvatarURL returns the display URL for a stored avatar reference.
func (service Service) ResolveAvatarURL(avatarValue string) string {
	avatar := strings.TrimSpace(avatarValue)
	if avatar == "" {
		return ""
	}
	if local := localAvatarFromDiceBearURL(avatar); local != "" {
		return local
	}
	if parsed, err := url.Parse(avatar); err == nil && strings.EqualFold(parsed.Scheme, "local") {
		return service.localAvatarDisplayValue(avatar)
	}
	if strings.HasPrefix(strings.ToLower(avatar), "http://") || strings.HasPrefix(strings.ToLower(avatar), "https://") {
		if looksLikeInlineAvatarPayload(avatar) {
			return inlineAvatarDisplayValue(avatar)
		}
		if archivemedia.ExtractObjectPath(avatar) != "" {
			return service.buildAccessURL("avatar", avatar)
		}
		return avatar
	}
	if archivemedia.ExtractObjectPath(avatar) != "" {
		return service.buildAccessURL("avatar", avatar)
	}
	if looksLikeInlineAvatarPayload(avatar) {
		return inlineAvatarDisplayValue(avatar)
	}
	return avatar
}

func (service Service) uploadAvatarBytes(ctx context.Context, enterpriseID string, sourceKey string, content []byte, contentType string, extension string) string {
	if service.Uploader == nil || len(content) == 0 {
		return ""
	}
	ent := strings.TrimSpace(enterpriseID)
	if ent == "" {
		ent = "default"
	}
	key := strings.TrimSpace(sourceKey)
	if key == "" {
		key = "avatar"
	}
	contentDigest := sha256.Sum256(content)
	sdkDigest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%x", ent, key, contentDigest)))
	objectURL, err := service.Uploader.UploadArchiveMedia(ctx, archivemedia.UploadInput{
		EnterpriseID: ent,
		SDKFileID:    "avatar-" + fmt.Sprintf("%x", sdkDigest),
		Filename:     "avatar" + firstNonBlank(extension, ".bin"),
		ContentType:  firstNonBlank(contentType, "application/octet-stream"),
		Content:      content,
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(objectURL)
}

func (service Service) buildAccessURL(taskID string, objectURL string) string {
	if service.Access == nil {
		return strings.TrimSpace(objectURL)
	}
	return strings.TrimSpace(service.Access.BuildAccessURL(taskID, objectURL))
}

func (service Service) localAvatarDisplayValue(avatar string) string {
	localFile := service.resolveLocalObjectFile(avatar)
	if localFile == "" {
		return ""
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(localFile)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return ""
	}
	content, err := os.ReadFile(localFile)
	if err != nil {
		return ""
	}
	if !isSafeInlineAvatarPayload(content, contentType) {
		return ""
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(content)
}

func (service Service) resolveLocalObjectFile(objectURL string) string {
	relativePath := archivemedia.ExtractLocalObjectPath(objectURL)
	if relativePath == "" {
		return ""
	}
	root := strings.TrimSpace(service.LocalDataRoot)
	if root == "" {
		root = "data"
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	candidate := filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(relativePath)))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return candidate
}

func readAvatarPayload(avatar string) ([]byte, string, string, bool) {
	if content, contentType, extension, ok := parseDataURL(avatar); ok {
		return content, contentType, extension, true
	}
	if content, ok := decodeRawBase64(avatar); ok {
		return content, "image/png", ".png", true
	}
	return nil, "", "", false
}

func isSafeInlineAvatarPayload(content []byte, contentType string) bool {
	mimeType := normalizeContentType(contentType)
	return len(content) > 0 && len(content) <= maxInlineAvatarBytes && strings.HasPrefix(mimeType, "image/")
}

func isSupportedInlineAvatarDisplayPayload(content []byte, contentType string) bool {
	if !isSafeInlineAvatarPayload(content, contentType) {
		return false
	}
	switch normalizeContentType(contentType) {
	case "image/jpeg", "image/jpg":
		return len(content) >= 2 && content[0] == 0xff && content[1] == 0xd8
	case "image/png":
		return strings.HasPrefix(string(content), "\x89PNG\r\n\x1a\n")
	case "image/gif":
		return strings.HasPrefix(string(content), "GIF87a") || strings.HasPrefix(string(content), "GIF89a")
	case "image/webp":
		return len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP"
	default:
		return false
	}
}

func looksLikeInlineAvatarPayload(value string) bool {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return false
	}
	lowered := strings.ToLower(candidate)
	if strings.HasPrefix(lowered, "data:image/") || strings.HasPrefix(lowered, "data:") {
		return true
	}
	if len(candidate) >= 128 {
		for _, prefix := range base64ImagePrefixes {
			if strings.HasPrefix(candidate, prefix) {
				return true
			}
		}
	}
	if len(candidate) >= 512 && !strings.HasPrefix(lowered, "http://") && !strings.HasPrefix(lowered, "https://") && !strings.HasPrefix(lowered, "/") {
		_, ok := decodeRawBase64(candidate)
		return ok
	}
	return false
}

func inlineAvatarDisplayValue(value string) string {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return ""
	}
	if local := localAvatarFromDiceBearURL(candidate); local != "" {
		return local
	}
	lowered := strings.ToLower(candidate)
	if strings.HasPrefix(lowered, "data:image/") {
		content, contentType, _, ok := parseDataURL(candidate)
		if ok && isSupportedInlineAvatarDisplayPayload(content, contentType) {
			return candidate
		}
		return ""
	}
	if strings.HasPrefix(lowered, "data:") {
		return ""
	}
	if _, ok := decodeRawBase64(candidate); ok {
		return ""
	}
	return candidate
}

func localAvatarFromDiceBearURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(parsed.Host, "api.dicebear.com") || !strings.Contains(strings.ToLower(parsed.Path), "/initials/svg") {
		return ""
	}
	seed := strings.TrimSpace(parsed.Query().Get("seed"))
	return localInitialsAvatar(seed)
}

func localInitialsAvatar(accountName string) string {
	name := strings.TrimSpace(accountName)
	if name == "" {
		return ""
	}
	initials := []rune(name)
	if len(initials) > 2 {
		initials = initials[:2]
	}
	svg := "<svg xmlns='http://www.w3.org/2000/svg' width='96' height='96' viewBox='0 0 96 96'>" +
		"<rect width='96' height='96' rx='48' fill='#2563eb'/>" +
		"<text x='48' y='55' text-anchor='middle' font-size='30' font-family='Arial,sans-serif' font-weight='700' fill='white'>" +
		html.EscapeString(string(initials)) +
		"</text></svg>"
	return "data:image/svg+xml;charset=utf-8," + url.PathEscape(svg)
}

func parseDataURL(value string) ([]byte, string, string, bool) {
	candidate := strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(candidate), "data:") {
		return nil, "", "", false
	}
	head, payload, ok := strings.Cut(candidate, ",")
	if !ok {
		return nil, "", "", false
	}
	mediaType := strings.TrimSpace(head[len("data:"):])
	if beforeParams, _, hasParams := strings.Cut(mediaType, ";"); hasParams {
		mediaType = beforeParams
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	content, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil {
		return nil, "", "", false
	}
	return content, mediaType, guessExtension(mediaType, ""), true
}

func decodeRawBase64(value string) ([]byte, bool) {
	candidate := strings.TrimSpace(value)
	if candidate == "" || len(candidate) < 32 || len(candidate)%4 != 0 {
		return nil, false
	}
	content, err := base64.StdEncoding.Strict().DecodeString(candidate)
	if err != nil {
		return nil, false
	}
	return content, true
}

func guessExtension(contentType string, source string) string {
	normalized := normalizeContentType(contentType)
	switch normalized {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	if extensions, err := mime.ExtensionsByType(normalized); err == nil && len(extensions) > 0 {
		if extensions[0] == ".jpe" {
			return ".jpg"
		}
		return extensions[0]
	}
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err == nil {
		if extension := filepath.Ext(parsed.Path); extension != "" {
			return extension
		}
	}
	return ".bin"
}

func normalizeContentType(contentType string) string {
	mimeType := strings.TrimSpace(contentType)
	if beforeParams, _, ok := strings.Cut(mimeType, ";"); ok {
		mimeType = beforeParams
	}
	if mimeType == "" {
		return "application/octet-stream"
	}
	return strings.ToLower(mimeType)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
