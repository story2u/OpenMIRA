// Package sopmedia handles SOP media upload candidates.
package sopmedia

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"wework-go/internal/archivemedia"
)

const MaxUploadBytes = 50 * 1024 * 1024

var (
	ErrInvalidMediaType = errors.New("media_type must be image or video")
	ErrContentEmpty     = errors.New("upload content is empty")
	ErrUploadTooLarge   = errors.New("upload file is too large")
	ErrUploaderMissing  = errors.New("sop media upload service is not configured")
	ErrBlockedExtension = errors.New("blocked file extension")
	ErrUnsupportedMIME  = errors.New("unsupported file type")
	ErrObjectURLMissing = errors.New("object upload response missing url")
	ErrUploadFailed     = errors.New("sop media upload failed")
)

// Uploader stores SOP media bytes and returns an object URL.
type Uploader interface {
	UploadArchiveMedia(ctx context.Context, input archivemedia.UploadInput) (string, error)
}

// Service uploads SOP configuration media and builds preview URLs.
type Service struct {
	Uploader     Uploader
	Access       archivemedia.AccessURLBuilder
	RandomSuffix func() string
}

// Request mirrors the legacy multipart upload fields.
type Request struct {
	MediaType   string
	Filename    string
	ContentType string
	Content     []byte
}

// Result mirrors the legacy JSON response.
type Result struct {
	Success     bool   `json:"success"`
	MediaType   string `json:"media_type"`
	ObjectURL   string `json:"object_url"`
	AccessURL   string `json:"access_url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// ValidationError carries Python-compatible upload validation details.
type ValidationError struct {
	Err    error
	Detail string
}

func (err ValidationError) Error() string {
	return err.Detail
}

func (err ValidationError) Unwrap() error {
	return err.Err
}

// Upload validates and stores one SOP media file.
func (service Service) Upload(ctx context.Context, request Request) (Result, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(request.MediaType))
	if normalizedType != "image" && normalizedType != "video" {
		return Result{}, ErrInvalidMediaType
	}
	if len(request.Content) > MaxUploadBytes {
		return Result{}, ValidationError{Err: ErrUploadTooLarge, Detail: "文件大小超过限制（最大 50MB）"}
	}
	extension := strings.ToLower(filepath.Ext(request.Filename))
	if isBlockedExtension(extension) {
		return Result{}, ValidationError{Err: ErrBlockedExtension, Detail: "不允许上传该类型文件：" + extension}
	}
	if !allowedContentType(normalizedType, request.ContentType) {
		return Result{}, ValidationError{Err: ErrUnsupportedMIME, Detail: "不支持的文件类型：" + request.ContentType}
	}
	if len(request.Content) == 0 {
		return Result{}, ErrContentEmpty
	}
	if service.Uploader == nil {
		return Result{}, ErrUploaderMissing
	}
	sdkFileID := service.sdkFileID(normalizedType, request.Content)
	filename := strings.TrimSpace(request.Filename)
	if filename == "" {
		filename = normalizedType + ".bin"
	}
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = normalizedType + "/*"
	}
	objectURL, err := service.Uploader.UploadArchiveMedia(ctx, archivemedia.UploadInput{
		EnterpriseID: "sop-config",
		SDKFileID:    sdkFileID,
		Filename:     filename,
		ContentType:  contentType,
		Content:      request.Content,
	})
	if err != nil {
		return Result{}, ErrUploadFailed
	}
	objectURL = strings.TrimSpace(objectURL)
	if objectURL == "" {
		return Result{}, ErrObjectURLMissing
	}
	return Result{
		Success:     true,
		MediaType:   normalizedType,
		ObjectURL:   objectURL,
		AccessURL:   service.previewURL(objectURL, "sop-preview-"+service.randomSuffix()),
		Filename:    strings.TrimSpace(request.Filename),
		ContentType: strings.TrimSpace(request.ContentType),
	}, nil
}

func (service Service) sdkFileID(mediaType string, content []byte) string {
	sum := sha256.Sum256(content)
	return "sop-" + mediaType + "-" + hex.EncodeToString(sum[:])[:24] + "-" + service.randomSuffix()
}

func (service Service) randomSuffix() string {
	if service.RandomSuffix != nil {
		if suffix := strings.TrimSpace(service.RandomSuffix()); suffix != "" {
			return suffix
		}
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(buf)
}

func (service Service) previewURL(rawURL string, taskID string) string {
	normalized := strings.TrimSpace(rawURL)
	if normalized == "" {
		return ""
	}
	lowered := strings.ToLower(normalized)
	if strings.HasPrefix(lowered, "http://") || strings.HasPrefix(lowered, "https://") || strings.HasPrefix(lowered, "data:") {
		return normalized
	}
	if strings.HasPrefix(lowered, "local://") {
		escaped := strings.ReplaceAll(url.QueryEscape(normalized), "+", "%20")
		return "/api/v1/admin/sop/media/local?object_url=" + escaped
	}
	if accessURL := strings.TrimSpace(service.Access.BuildAccessURL(taskID, normalized)); accessURL != "" {
		return accessURL
	}
	return normalized
}

func allowedContentType(mediaType string, contentType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if normalized == "" {
		return true
	}
	if mediaType == "image" {
		return map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/gif":  true,
			"image/webp": true,
			"image/bmp":  true,
		}[normalized]
	}
	return map[string]bool{
		"video/mp4":       true,
		"video/mpeg":      true,
		"video/quicktime": true,
		"video/x-msvideo": true,
		"video/webm":      true,
	}[normalized]
}

func isBlockedExtension(extension string) bool {
	switch extension {
	case ".exe", ".bat", ".cmd", ".ps1", ".sh", ".bash", ".dll", ".so", ".dylib", ".js", ".vbs", ".wsf", ".hta", ".php", ".asp", ".aspx", ".jsp", ".py", ".pyc", ".pyo", ".jar", ".class", ".scr", ".pif", ".com":
		return true
	default:
		return false
	}
}
