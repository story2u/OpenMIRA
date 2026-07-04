package archiveadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultWeWorkTokenURL = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"

var (
	// ErrOfficialEnterpriseIDRequired mirrors the legacy FastAPI validation detail.
	ErrOfficialEnterpriseIDRequired = errors.New("enterprise_id is required")
	// ErrOfficialEnterpriseNotFound means the requested enterprise binding is missing.
	ErrOfficialEnterpriseNotFound = errors.New("enterprise not found")
)

// OfficialCheckRequest carries POST /api/v1/archive/official/check input.
type OfficialCheckRequest struct {
	EnterpriseID string
	BaseURL      string
}

// SDKStatus is the official finance SDK availability payload.
type SDKStatus struct {
	Available      bool
	LibPath        string
	Error          string
	MediaAvailable bool
	MediaError     string
}

// SDKStatusProvider returns side-effect-free finance SDK readiness.
type SDKStatusProvider interface {
	OfficialSDKStatus(ctx context.Context) SDKStatus
}

// TokenCheckResult is the qyapi gettoken compatibility result.
type TokenCheckResult struct {
	OK    bool
	Error string
}

// TokenChecker verifies that corp_id/corp_secret can issue an access token.
type TokenChecker interface {
	CheckToken(ctx context.Context, corpID string, corpSecret string) (TokenCheckResult, error)
}

// FileSDKStatusProvider reports SDK readiness from the configured C library path.
type FileSDKStatusProvider struct {
	LibPath string
	Stat    func(string) (os.FileInfo, error)
}

// OfficialSDKStatus reports the same stable keys as Python's official_sdk_status.
func (provider FileSDKStatusProvider) OfficialSDKStatus(ctx context.Context) SDKStatus {
	_ = ctx
	libPath := strings.TrimSpace(provider.LibPath)
	if libPath == "" {
		return SDKStatus{LibPath: libPath, Error: "WEWORK_FINANCE_SDK_LIB_PATH is empty"}
	}
	stat := provider.Stat
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(libPath)
	if err != nil || (info != nil && info.IsDir()) {
		return SDKStatus{LibPath: libPath, Error: "sdk library not found: " + libPath}
	}
	return SDKStatus{
		Available:      true,
		LibPath:        libPath,
		MediaAvailable: true,
	}
}

// HTTPTokenChecker calls the WeCom gettoken endpoint.
type HTTPTokenChecker struct {
	Client   *http.Client
	Endpoint string
}

// CheckToken maps gettoken errcode/errmsg into the legacy token_ok/token_error fields.
func (checker HTTPTokenChecker) CheckToken(ctx context.Context, corpID string, corpSecret string) (TokenCheckResult, error) {
	endpoint := strings.TrimSpace(checker.Endpoint)
	if endpoint == "" {
		endpoint = defaultWeWorkTokenURL
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return TokenCheckResult{}, err
	}
	query := u.Query()
	query.Set("corpid", strings.TrimSpace(corpID))
	query.Set("corpsecret", strings.TrimSpace(corpSecret))
	u.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return TokenCheckResult{}, err
	}
	client := checker.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return TokenCheckResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TokenCheckResult{}, fmt.Errorf("gettoken failed: status=%d", response.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return TokenCheckResult{}, err
	}
	errcode := intFromAny(payload["errcode"])
	accessToken := strings.TrimSpace(stringFromAny(payload["access_token"]))
	if errcode == 0 && accessToken != "" {
		return TokenCheckResult{OK: true}, nil
	}
	message := strings.TrimSpace(stringFromAny(payload["errmsg"]))
	if message == "" {
		message = fmt.Sprintf("errcode=%d", errcode)
	}
	return TokenCheckResult{Error: message}, nil
}

// OfficialCheck builds the legacy official archive configuration guide response.
func (service Service) OfficialCheck(ctx context.Context, request OfficialCheckRequest) (Payload, error) {
	if service.Enterprises == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	if enterpriseID == "" {
		return nil, ErrOfficialEnterpriseIDRequired
	}
	enterprise, err := service.Enterprises.GetEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	if enterprise == nil {
		return nil, fmt.Errorf("%w: %s", ErrOfficialEnterpriseNotFound, enterpriseID)
	}

	corpID := strings.TrimSpace(enterprise.CorpID)
	archiveSecret := strings.TrimSpace(enterprise.CorpSecret)
	contactSecret := strings.TrimSpace(enterprise.ContactSecret)
	pullURL := strings.TrimSpace(enterprise.ArchivePullURL)
	mediaURL := strings.TrimSpace(enterprise.MediaPullURL)
	callbackToken := strings.TrimSpace(enterprise.ArchiveEventCallbackToken)
	callbackAES := strings.TrimSpace(enterprise.ArchiveEventCallbackAESKey)
	sdkStatus := SDKStatus{}
	if service.SDKStatus != nil {
		sdkStatus = service.SDKStatus.OfficialSDKStatus(ctx)
	}

	tokenOK := false
	tokenError := ""
	if corpID != "" && archiveSecret != "" && service.TokenChecker != nil {
		result, err := service.TokenChecker.CheckToken(ctx, corpID, archiveSecret)
		if err != nil {
			tokenError = err.Error()
		} else {
			tokenOK = result.OK
			tokenError = strings.TrimSpace(result.Error)
		}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	callbackURL := ""
	if baseURL != "" && corpID != "" {
		callbackURL = baseURL + "/api/v1/archive/callback/" + corpID
	}
	return Payload{
		"accepted":      true,
		"enterprise_id": enterpriseID,
		"checks": Payload{
			"has_corp_id":          corpID != "",
			"has_corp_secret":      archiveSecret != "",
			"has_contact_secret":   contactSecret != "",
			"has_archive_pull_url": pullURL != "",
			"has_media_pull_url":   mediaURL != "",
			"has_callback_token":   callbackToken != "",
			"has_callback_aes_key": callbackAES != "",
			"sdk_available":        sdkStatus.Available,
			"sdk_lib_path":         strings.TrimSpace(sdkStatus.LibPath),
			"sdk_error":            strings.TrimSpace(sdkStatus.Error),
			"sdk_media_available":  sdkStatus.MediaAvailable,
			"sdk_media_error":      strings.TrimSpace(sdkStatus.MediaError),
			"token_ok":             tokenOK,
			"token_error":          tokenError,
		},
		"missing_required": buildOfficialMissingRequired(corpID, archiveSecret, pullURL, sdkStatus.Available),
		"suggested_bridge_urls": Payload{
			"archive_pull_url":   bridgeURL(baseURL, "/api/v1/archive/sdk/pull"),
			"media_pull_url":     bridgeURL(baseURL, "/api/v1/archive/sdk/media/pull"),
			"event_callback_url": callbackURL,
		},
		"callback_wizard": buildOfficialCallbackWizard(callbackToken, callbackAES, callbackURL),
		"next_steps": []string{
			"1) 在企业绑定中填写 corp_id + corp_secret（会话存档 Secret）",
			"1.1) 如需通过用户名补齐企微内部 userid / 获取成员资料，补充 contact_secret（通讯录管理 Secret）",
			"2) 二选一：配置 WEWORK_FINANCE_SDK_LIB_PATH（内置 SDK）或填写 archive_pull_url（SDK 桥接服务）",
			"3) 如需事件回调触发实时补拉，按 callback_wizard 指引配置 archive_event_callback_token、archive_event_callback_aes_key 与 callback URL",
			"4) 如需拉取媒体，配置 media_pull_url 并调用 /archive/media/sync/run",
		},
	}, nil
}

func buildOfficialMissingRequired(corpID string, archiveSecret string, pullURL string, sdkAvailable bool) []string {
	missing := []string{}
	if strings.TrimSpace(corpID) == "" {
		missing = append(missing, "corp_id")
	}
	if strings.TrimSpace(archiveSecret) == "" {
		missing = append(missing, "corp_secret(会话存档 Secret)")
	}
	if strings.TrimSpace(pullURL) == "" && !sdkAvailable {
		missing = append(missing, "archive_pull_url(消息补拉 URL) 或 WEWORK_FINANCE_SDK_LIB_PATH(内置 SDK 路径)")
	}
	return missing
}

func buildOfficialCallbackWizard(callbackToken string, callbackAES string, callbackURL string) Payload {
	hasCallbackCredentials := strings.TrimSpace(callbackToken) != "" && strings.TrimSpace(callbackAES) != ""
	canConfigureCallback := hasCallbackCredentials && strings.TrimSpace(callbackURL) != ""
	return Payload{
		"ready":   canConfigureCallback,
		"summary": "回调 URL 应保持稳定不变，业务逻辑统一放到异步消费侧。",
		"steps": []Payload{
			{
				"id":          "fill_callback_credentials",
				"title":       "填写并保存回调 Token / AESKey",
				"status":      ternaryString(hasCallbackCredentials, "completed", "current"),
				"description": "在企业绑定中填写 archive_event_callback_token 和 archive_event_callback_aes_key，并先保存企业配置。",
				"field_keys":  []string{"archive_event_callback_token", "archive_event_callback_aes_key"},
			},
			{
				"id":          "copy_callback_url",
				"title":       "把回调 URL 配到企微后台",
				"status":      ternaryString(canConfigureCallback, "current", "blocked"),
				"description": "在企业微信管理后台配置回调 URL，后续不要频繁变更该地址。",
				"field_keys":  []string{},
				"value_label": "回调 URL",
				"value":       strings.TrimSpace(callbackURL),
			},
			{
				"id":          "verify_callback_handshake",
				"title":       "完成企微 URL 验证",
				"status":      ternaryString(canConfigureCallback, "pending", "blocked"),
				"description": "在企微后台保存配置时会触发一次 URL 验证，请确保 Token、AESKey 和公网地址完全一致。",
				"field_keys":  []string{},
			},
			{
				"id":          "observe_callback_receipts",
				"title":       "查看回调回执与失败原因",
				"status":      ternaryString(canConfigureCallback, "pending", "blocked"),
				"description": "配置完成后，可在管理后台回调监控里查看 received/processed/failed、重复次数和失败原因。",
				"field_keys":  []string{},
			},
		},
	}
}

func bridgeURL(baseURL string, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + path
}

func ternaryString(condition bool, trueValue string, falseValue string) string {
	if condition {
		return trueValue
	}
	return falseValue
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		integer, _ := typed.Int64()
		return int(integer)
	case string:
		var integer int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &integer)
		return integer
	default:
		return 0
	}
}
