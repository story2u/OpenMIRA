package archivecoldstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultObjectFinalizeTimeout = 30 * time.Second

// HTTPObjectFinalizer uploads cold archive artifacts through the object-storage HTTP API.
type HTTPObjectFinalizer struct {
	UploadURL   string
	UploadToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// FinalizeObject implements ObjectFinalizer.
func (finalizer HTTPObjectFinalizer) FinalizeObject(ctx context.Context, input ObjectFinalizeInput) (ObjectFinalizeResult, error) {
	path := strings.TrimSpace(input.LocalFilePath)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectFinalizeResult{Backend: "http_uploader"}, nil
		}
		return ObjectFinalizeResult{}, err
	}
	if info.IsDir() {
		return ObjectFinalizeResult{Backend: "http_uploader"}, nil
	}
	uploadURL := strings.TrimSpace(finalizer.UploadURL)
	if uploadURL == "" {
		return ObjectFinalizeResult{}, fmt.Errorf("ARCHIVE_MEDIA_OBJECT_UPLOAD_URL is not configured")
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		if err := writeObjectFinalizeMultipart(multipartWriter, input, path); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		_ = writer.Close()
	}()
	defer func() {
		_ = reader.Close()
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, reader)
	if err != nil {
		return ObjectFinalizeResult{}, err
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	for key, value := range objectFinalizeAuthHeaders(finalizer.UploadToken) {
		request.Header.Set(key, value)
	}
	response, err := finalizer.httpClient().Do(request)
	if err != nil {
		return ObjectFinalizeResult{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return ObjectFinalizeResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ObjectFinalizeResult{}, objectFinalizeStatusError(response.StatusCode, responseBody)
	}
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ObjectFinalizeResult{}, err
	}
	objectURL := firstObjectFinalizeText(textFromAny(payload["url"]), textFromAny(payload["object_url"]))
	if objectURL == "" {
		return ObjectFinalizeResult{}, fmt.Errorf("object upload response missing url")
	}
	return ObjectFinalizeResult{Backend: "http_uploader", ObjectURL: objectURL}, nil
}

func writeObjectFinalizeMultipart(writer *multipart.Writer, input ObjectFinalizeInput, path string) error {
	if err := writer.WriteField("enterprise_id", defaultText(input.EnterpriseID, "default")); err != nil {
		return err
	}
	if err := writer.WriteField("sdk_file_id", strings.TrimSpace(input.SDKFileID)); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeObjectFinalizeFilename(filepath.Base(path))))
	partHeader.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	return writer.Close()
}

func (finalizer HTTPObjectFinalizer) httpClient() *http.Client {
	if finalizer.HTTPClient != nil {
		return finalizer.HTTPClient
	}
	timeout := finalizer.Timeout
	if timeout <= 0 {
		timeout = defaultObjectFinalizeTimeout
	}
	return &http.Client{Timeout: timeout}
}

func objectFinalizeAuthHeaders(token string) map[string]string {
	token = strings.TrimSpace(token)
	if token == "" {
		return map[string]string{}
	}
	return map[string]string{"Authorization": "Bearer " + token}
}

func objectFinalizeStatusError(statusCode int, body []byte) error {
	detail := ""
	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		detail = firstObjectFinalizeText(textFromAny(data["detail"]), textFromAny(data["message"]))
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail == "" {
		detail = http.StatusText(statusCode)
	}
	return fmt.Errorf("object upload failed status=%d detail=%s", statusCode, detail)
}

func firstObjectFinalizeText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func textFromAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func escapeObjectFinalizeFilename(filename string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, `\"`).Replace(filename)
}
