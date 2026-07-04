// Package rtccontrolclient adapts device RTC input to an HTTP control bridge.
package rtccontrolclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wework-go/internal/devicesdk"
)

// Client forwards validated low-latency RTC input to a bridge that owns the
// actual MytRpc/P1 connection.
type Client struct {
	BaseURL string
	Token   string
	Timeout time.Duration
	Client  *http.Client
}

// SendControlInput posts one Python-compatible control/input request.
func (client Client) SendControlInput(ctx context.Context, command devicesdk.ControlInputCommand) (devicesdk.ControlInputResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(client.BaseURL), "/")
	if baseURL == "" {
		return devicesdk.ControlInputResult{}, devicesdk.ControlInputError{Cause: devicesdk.ErrSDKControlInputUnavailable}
	}
	body, err := json.Marshal(map[string]any{
		"participant_identity": command.ParticipantIdentity,
		"kind":                 command.Kind,
		"action":               command.Action,
		"x":                    command.RatioX,
		"y":                    command.RatioY,
		"delta_x":              command.DeltaX,
		"delta_y":              command.DeltaY,
		"key":                  command.Key,
		"text":                 command.Text,
		"ts":                   command.TimestampMillis,
		"screen_width":         command.ScreenWidth,
		"screen_height":        command.ScreenHeight,
		"pixel_x":              command.X,
		"pixel_y":              command.Y,
		"normalized_key":       command.NormalizedKey,
		"key_code":             command.KeyCode,
	})
	if err != nil {
		return devicesdk.ControlInputResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint(baseURL, command.DeviceID), bytes.NewReader(body))
	if err != nil {
		return devicesdk.ControlInputResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(client.Token); token != "" {
		request.Header.Set("X-Agent-Token", token)
	}

	response, err := client.httpClient().Do(request)
	if err != nil {
		return devicesdk.ControlInputResult{}, devicesdk.ControlInputError{Cause: devicesdk.ErrSDKControlInputUnavailable, Detail: err.Error()}
	}
	defer response.Body.Close()
	payload, readErr := readPayload(response.Body)
	if readErr != nil {
		return devicesdk.ControlInputResult{}, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return devicesdk.ControlInputResult{}, statusError(response.StatusCode, detail(payload))
	}
	return devicesdk.ControlInputResult{
		Sent:          boolValue(payload["sent"], true),
		Detail:        stringValue(payload["detail"]),
		Route:         firstNonEmpty(stringValue(payload["route"]), "mytrpc"),
		AcquireMillis: intValue(payload["acquire_ms"]),
		SendMillis:    intValue(payload["send_ms"]),
	}, nil
}

func (client Client) endpoint(baseURL string, deviceID string) string {
	escapedDeviceID := url.PathEscape(strings.TrimSpace(deviceID))
	if strings.HasSuffix(baseURL, "/api/v1") {
		return baseURL + "/devices/" + escapedDeviceID + "/control/input"
	}
	return baseURL + "/api/v1/devices/" + escapedDeviceID + "/control/input"
}

func (client Client) httpClient() *http.Client {
	if client.Client != nil {
		return client.Client
	}
	timeout := client.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func readPayload(reader io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func statusError(status int, detail string) error {
	switch status {
	case http.StatusForbidden:
		return devicesdk.ControlInputError{Cause: devicesdk.ErrSDKControlInputForbidden, Detail: detail}
	case http.StatusNotFound:
		return devicesdk.ControlInputError{Cause: devicesdk.ErrSDKDeviceNotConfigured, Detail: detail}
	case http.StatusUnprocessableEntity:
		return devicesdk.ControlInputError{Cause: devicesdk.ErrSDKParticipantIdentityRequired, Detail: detail}
	case http.StatusServiceUnavailable:
		return devicesdk.ControlInputError{Cause: devicesdk.ErrSDKControlInputUnavailable, Detail: detail}
	default:
		if detail == "" {
			detail = fmt.Sprintf("control bridge returned HTTP %d", status)
		}
		return devicesdk.ControlInputError{Cause: devicesdk.ErrSDKControlInputFailed, Detail: detail}
	}
}

func detail(payload map[string]any) string {
	value := payload["detail"]
	if nested, ok := value.(map[string]any); ok {
		if detail := stringValue(nested["detail"]); detail != "" {
			return detail
		}
	}
	return stringValue(value)
}

func boolValue(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		default:
			return fallback
		}
	case nil:
		return fallback
	default:
		return fallback
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		var parsed int
		_, _ = fmt.Sscan(strings.TrimSpace(fmt.Sprint(value)), &parsed)
		return parsed
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
