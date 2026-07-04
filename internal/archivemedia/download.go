package archivemedia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wework-go/internal/infra/archivemediatask"
)

const (
	DefaultObjectInternalBaseURL = "http://object-storage:9102"
	DefaultProxyDownloadTimeout  = 30 * time.Second
)

var (
	ErrMediaAccessTokenRequired  = errors.New("token is required")
	ErrMediaAccessTokenInvalid   = errors.New("token invalid or expired")
	ErrMediaTaskStoreUnavailable = errors.New("archive media task store is unavailable")
	ErrMediaObjectNotFound       = errors.New("media object not found")
	ErrMediaLocalFileNotFound    = errors.New("media local file not found")
	ErrObjectNotFound            = errors.New("object not found")
)

// TaskReader loads completed archive media tasks for browser download.
type TaskReader interface {
	GetTask(ctx context.Context, taskID string) (*archivemediatask.Record, error)
}

// DownloadService serves legacy archive media download routes.
type DownloadService struct {
	Tasks                 TaskReader
	Access                AccessURLBuilder
	ObjectInternalBaseURL string
	LocalDataRoot         string
	HTTPClient            *http.Client
	HTTPTimeout           time.Duration
}

// DownloadResponse carries an opened media stream and headers for HTTP serialization.
type DownloadResponse struct {
	Body          io.ReadCloser
	ContentType   string
	Filename      string
	ContentLength int64
}

// DownloadTask returns a task-scoped media file stream.
func (service DownloadService) DownloadTask(ctx context.Context, taskID string, token string) (DownloadResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if strings.TrimSpace(token) == "" {
		return DownloadResponse{}, ErrMediaAccessTokenRequired
	}
	if !service.Access.VerifyAccessToken(taskID, token) {
		return DownloadResponse{}, ErrMediaAccessTokenInvalid
	}
	if service.Tasks == nil {
		return DownloadResponse{}, ErrMediaTaskStoreUnavailable
	}
	task, err := service.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return DownloadResponse{}, err
	}
	if task == nil {
		return DownloadResponse{}, ErrMediaTaskNotFound
	}
	contentType, extension := GuessDownloadMeta(task.ObjectURL)
	filename := strings.TrimSpace(task.SDKFileID) + extension
	if strings.TrimSpace(task.SDKFileID) == "" {
		filename = "media" + extension
	}
	objectURL := strings.TrimSpace(task.ObjectURL)
	if objectURL == "" {
		return DownloadResponse{}, ErrMediaObjectNotFound
	}
	if localPath := service.resolveLocalObjectFile(objectURL); localPath != "" {
		return openLocalDownload(localPath, contentType, filename)
	}
	response, err := service.openUpstream(ctx, objectURL)
	if err != nil {
		return DownloadResponse{}, fmt.Errorf("media proxy failed: %w", err)
	}
	response.ContentType = contentType
	response.Filename = filename
	return response, nil
}

// DownloadObject returns an object-path-scoped media object stream.
func (service DownloadService) DownloadObject(ctx context.Context, objectPath string, token string) (DownloadResponse, error) {
	normalizedPath := strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if normalizedPath == "" {
		return DownloadResponse{}, ErrObjectNotFound
	}
	if strings.TrimSpace(token) == "" {
		return DownloadResponse{}, ErrMediaAccessTokenRequired
	}
	if !service.Access.VerifyObjectAccessToken(normalizedPath, token) {
		return DownloadResponse{}, ErrMediaAccessTokenInvalid
	}
	upstreamURL := service.BuildObjectUpstreamURL(normalizedPath)
	if upstreamURL == "" {
		return DownloadResponse{}, ErrObjectNotFound
	}
	response, err := service.openUpstream(ctx, upstreamURL)
	if err != nil {
		return DownloadResponse{}, fmt.Errorf("object proxy failed: %w", err)
	}
	response.Filename = filepath.Base(filepath.FromSlash(normalizedPath))
	if strings.TrimSpace(response.Filename) == "." || strings.TrimSpace(response.Filename) == "" {
		response.Filename = "media.bin"
	}
	return response, nil
}

// DownloadLocalObject returns a local:// object stream for admin preview routes.
func (service DownloadService) DownloadLocalObject(ctx context.Context, objectURL string) (DownloadResponse, error) {
	_ = ctx
	localPath := service.resolveLocalObjectFile(strings.TrimSpace(objectURL))
	if localPath == "" {
		return DownloadResponse{}, ErrMediaLocalFileNotFound
	}
	return openLocalDownload(localPath, guessLocalPreviewContentType(localPath), filepath.Base(localPath))
}

// BuildObjectUpstreamURL mirrors Python ArchiveMediaStorageService.build_object_upstream_url.
func (service DownloadService) BuildObjectUpstreamURL(objectPath string) string {
	normalizedPath := strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if normalizedPath == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(service.ObjectInternalBaseURL), "/")
	if base == "" {
		base = DefaultObjectInternalBaseURL
	}
	return joinURLPath(base, "objects/"+normalizedPath)
}

// GuessDownloadMeta mirrors Python guess_media_download_meta for response headers.
func GuessDownloadMeta(objectURL string) (contentType string, extension string) {
	raw := strings.TrimSpace(objectURL)
	pathValue := raw
	if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
		pathValue = parsed.Path
	}
	extension = strings.ToLower(filepath.Ext(pathValue))
	if extension != "" {
		if detected := mime.TypeByExtension(extension); detected != "" {
			return detected, extension
		}
	}
	return "application/octet-stream", ".bin"
}

func guessLocalPreviewContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

func (service DownloadService) resolveLocalObjectFile(objectURL string) string {
	relativePath := ExtractLocalObjectPath(objectURL)
	if relativePath == "" {
		return ""
	}
	root := strings.TrimSpace(service.LocalDataRoot)
	if root == "" {
		root = filepath.Join("data")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	candidate := filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(relativePath)))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return ""
	}
	return candidate
}

func openLocalDownload(path string, contentType string, filename string) (DownloadResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DownloadResponse{}, ErrMediaLocalFileNotFound
		}
		return DownloadResponse{}, err
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return DownloadResponse{}, err
	}
	if stat.IsDir() {
		_ = file.Close()
		return DownloadResponse{}, ErrMediaLocalFileNotFound
	}
	return DownloadResponse{
		Body:          file,
		ContentType:   defaultText(contentType, "application/octet-stream"),
		Filename:      defaultText(filename, "media.bin"),
		ContentLength: stat.Size(),
	}, nil
}

func (service DownloadService) openUpstream(ctx context.Context, targetURL string) (DownloadResponse, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return DownloadResponse{}, ErrObjectNotFound
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return DownloadResponse{}, err
	}
	response, err := service.httpClient().Do(request)
	if err != nil {
		return DownloadResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return DownloadResponse{}, fmt.Errorf("upstream status %d", response.StatusCode)
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return DownloadResponse{
		Body:          response.Body,
		ContentType:   contentType,
		Filename:      "media.bin",
		ContentLength: response.ContentLength,
	}, nil
}

func (service DownloadService) httpClient() *http.Client {
	if service.HTTPClient != nil {
		return service.HTTPClient
	}
	timeout := service.HTTPTimeout
	if timeout <= 0 {
		timeout = DefaultProxyDownloadTimeout
	}
	return &http.Client{Timeout: timeout}
}
