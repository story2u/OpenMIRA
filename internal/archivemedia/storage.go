package archivemedia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const DefaultUploadTimeout = 30 * time.Second

// HTTPUploader uploads completed archive media bytes to the object-storage service.
type HTTPUploader struct {
	UploadURL   string
	DeleteURL   string
	UploadToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// UploadArchiveMedia uploads bytes using the Python-compatible multipart contract.
func (uploader HTTPUploader) UploadArchiveMedia(ctx context.Context, input UploadInput) (string, error) {
	if len(input.Content) == 0 {
		return "", fmt.Errorf("upload content is empty")
	}
	uploadURL := strings.TrimSpace(uploader.UploadURL)
	if uploadURL == "" {
		return "", fmt.Errorf("ARCHIVE_MEDIA_OBJECT_UPLOAD_URL is not configured")
	}
	filename, contentType := ResolveUploadMeta(input.SDKFileID, input.PayloadJSON, input.Content)
	if configured := strings.TrimSpace(input.Filename); configured != "" {
		filename = configured
	}
	if configured := strings.TrimSpace(input.ContentType); configured != "" {
		contentType = configured
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("enterprise_id", defaultText(input.EnterpriseID, "default")); err != nil {
		return "", err
	}
	if err := writer.WriteField("sdk_file_id", strings.TrimSpace(input.SDKFileID)); err != nil {
		return "", err
	}
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFilename(filename)))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(input.Content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range buildAuthHeaders(uploader.UploadToken) {
		request.Header.Set(key, value)
	}
	response, err := uploader.httpClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", bridgeStatusError(response.StatusCode, responseBody)
	}
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return "", err
	}
	objectURL := firstNonBlank(textAny(payload["url"]), textAny(payload["object_url"]))
	if objectURL == "" {
		return "", fmt.Errorf("object upload response missing url")
	}
	return objectURL, nil
}

// DeleteArchiveMedia removes an uploaded object via the object-storage internal delete API.
func (uploader HTTPUploader) DeleteArchiveMedia(ctx context.Context, objectURL string) (bool, error) {
	objectPath := extractObjectPath(objectURL)
	if objectPath == "" {
		return false, nil
	}
	deleteURL := uploader.deleteURL()
	if deleteURL == "" {
		return false, nil
	}
	values := url.Values{}
	values.Set("object_path", objectPath)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, deleteURL, strings.NewReader(values.Encode()))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for key, value := range buildAuthHeaders(uploader.UploadToken) {
		request.Header.Set(key, value)
	}
	response, err := uploader.httpClient().Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, nil
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, nil
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return false, nil
	}
	return truthy(payload["deleted"]), nil
}

func (uploader HTTPUploader) httpClient() *http.Client {
	if uploader.HTTPClient != nil {
		return uploader.HTTPClient
	}
	timeout := uploader.Timeout
	if timeout <= 0 {
		timeout = DefaultUploadTimeout
	}
	return &http.Client{Timeout: timeout}
}

func (uploader HTTPUploader) deleteURL() string {
	if configured := strings.TrimSpace(uploader.DeleteURL); configured != "" {
		return configured
	}
	uploadURL := strings.TrimSpace(uploader.UploadURL)
	if uploadURL == "" {
		return ""
	}
	parsed, err := url.Parse(uploadURL)
	if err != nil {
		return ""
	}
	if !strings.Contains(parsed.Path, "/internal/upload") {
		return ""
	}
	parsed.Path = strings.Replace(parsed.Path, "/internal/upload", "/internal/delete", 1)
	parsed.RawQuery = ""
	return parsed.String()
}

// ResolveUploadMeta mirrors Python resolve_archive_media_upload_meta.
func ResolveUploadMeta(sdkFileID string, payloadJSON string, content []byte) (string, string) {
	payload := parsePayloadJSON(payloadJSON)
	decrypted := mapAny(payload["decrypted"])
	if len(decrypted) == 0 {
		decrypted = payload
	}
	msgType := strings.ToLower(strings.TrimSpace(textAny(decrypted["msgtype"])))
	switch msgType {
	case "voice", "audio":
		extension, contentType := sniffVoiceMeta(content)
		return strings.TrimSpace(sdkFileID) + extension, contentType
	case "image":
		extension, contentType := sniffImageMeta(content)
		return strings.TrimSpace(sdkFileID) + extension, contentType
	case "video":
		extension, contentType := sniffVideoMeta(content)
		return strings.TrimSpace(sdkFileID) + extension, contentType
	case "file":
		filePayload := mapAny(decrypted["file"])
		originalName := strings.TrimSpace(textAny(filePayload["filename"]))
		if originalName != "" {
			return originalName, guessContentTypeFromName(originalName)
		}
	}
	return strings.TrimSpace(sdkFileID) + ".bin", "application/octet-stream"
}

func parsePayloadJSON(payloadJSON string) map[string]any {
	payloadJSON = strings.TrimSpace(payloadJSON)
	if payloadJSON == "" {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func mapAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func sniffVoiceMeta(content []byte) (string, string) {
	head := content
	if len(head) > 32 {
		head = head[:32]
	}
	switch {
	case bytes.HasPrefix(head, []byte("#!AMR\n")), bytes.HasPrefix(head, []byte("#!AMR-WB\n")):
		return ".amr", "audio/amr"
	case bytes.HasPrefix(head, []byte("#!SILK_V3")):
		return ".silk", "audio/x-silk"
	case bytes.HasPrefix(head, []byte("RIFF")) && bytes.Contains(head, []byte("WAVE")):
		return ".wav", "audio/wav"
	case bytes.HasPrefix(head, []byte("ID3")), len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0:
		return ".mp3", "audio/mpeg"
	case bytes.HasPrefix(head, []byte("OggS")):
		return ".ogg", "audio/ogg"
	default:
		return ".bin", "application/octet-stream"
	}
}

func sniffImageMeta(content []byte) (string, string) {
	switch {
	case bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return ".png", "image/png"
	case bytes.HasPrefix(content, []byte{0xFF, 0xD8, 0xFF}):
		return ".jpg", "image/jpeg"
	case bytes.HasPrefix(content, []byte("GIF87a")), bytes.HasPrefix(content, []byte("GIF89a")):
		return ".gif", "image/gif"
	case bytes.HasPrefix(content, []byte("RIFF")) && len(content) >= 12 && string(content[8:12]) == "WEBP":
		return ".webp", "image/webp"
	default:
		return ".bin", "application/octet-stream"
	}
}

func sniffVideoMeta(content []byte) (string, string) {
	if len(content) >= 12 && string(content[4:8]) == "ftyp" {
		return ".mp4", "video/mp4"
	}
	return ".bin", "application/octet-stream"
}

func guessContentTypeFromName(name string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".zip":
		return "application/zip"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func escapeMultipartFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "blob.bin"
	}
	filename = strings.ReplaceAll(filename, `\`, `\\`)
	return strings.ReplaceAll(filename, `"`, `\"`)
}
