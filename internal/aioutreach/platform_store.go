package aioutreach

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPDoer is the minimal HTTP client boundary used by platform adapters.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// PlatformStoreEnricher resolves store_id-only address actions through the
// legacy platform_agent store detail endpoint.
type PlatformStoreEnricher struct {
	BaseURL       string
	APIToken      string
	DefaultUserID int
	DefaultCorpID string
	DefaultWechat string
	Timeout       time.Duration
	Client        HTTPDoer
}

// EnrichStoreActions implements StoreActionEnricher.
func (enricher PlatformStoreEnricher) EnrichStoreActions(ctx context.Context, actions []ReplyAction) ([]ReplyAction, error) {
	if len(actions) == 0 {
		return actions, nil
	}
	enriched := append([]ReplyAction(nil), actions...)
	for index, action := range actions {
		if cleanLower(action.Type) != "store_address" {
			continue
		}
		if clean(action.StoreID) == "" || clean(action.Address) != "" {
			continue
		}
		detail, err := enricher.fetchStoreInfo(ctx, clean(action.StoreID))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		next := action
		if storeName := firstClean(detail.Name, action.StoreName); storeName != "" {
			next.StoreName = storeName
		}
		if address := firstClean(detail.TencentAddress, detail.Address); address != "" {
			next.Address = address
			next.Content = address
		} else if next.StoreName != "" {
			next.Content = next.StoreName
		}
		if tencentMapStore := clean(detail.TencentMapStore); tencentMapStore != "" {
			next.TencentMapStore = tencentMapStore
		}
		enriched[index] = next
	}
	return enriched, nil
}

type platformStoreInfo struct {
	Name            string `json:"name"`
	Address         string `json:"address"`
	TencentAddress  string `json:"tencent_address"`
	TencentMapStore string `json:"tencent_map_store"`
}

type platformStoreInfoResponse struct {
	Code    any               `json:"code"`
	Message string            `json:"msg"`
	Data    platformStoreInfo `json:"data"`
}

func (enricher PlatformStoreEnricher) fetchStoreInfo(ctx context.Context, storeID string) (platformStoreInfo, error) {
	endpoint, err := enricher.storeInfoURL(storeID)
	if err != nil {
		return platformStoreInfo{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return platformStoreInfo{}, err
	}
	token := clean(enricher.APIToken)
	if token != "" {
		request.Header.Set("token", token)
		request.Header.Set("Authorization", token)
	}
	request.Header.Set("Request-From", "platform_agent")
	request.Header.Set("Content-Type", "application/json")

	response, err := enricher.httpClient().Do(request)
	if err != nil {
		return platformStoreInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return platformStoreInfo{}, fmt.Errorf("platform store info status %d", response.StatusCode)
	}

	var payload platformStoreInfoResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return platformStoreInfo{}, err
	}
	if !platformResponseCodeOK(payload.Code) {
		return platformStoreInfo{}, fmt.Errorf("platform store info code %s", clean(payload.Message))
	}
	return payload.Data, nil
}

func (enricher PlatformStoreEnricher) storeInfoURL(storeID string) (string, error) {
	baseURL := strings.TrimRight(clean(enricher.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("platform base url is empty")
	}
	parsed, err := url.Parse(baseURL + "/platform_agent/store/info")
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("platform base url is invalid")
	}
	query := parsed.Query()
	query.Set("id", clean(storeID))
	if enricher.DefaultUserID > 0 {
		query.Set("user_id", strconv.Itoa(enricher.DefaultUserID))
	}
	if corpID := clean(enricher.DefaultCorpID); corpID != "" {
		query.Set("corp_id", corpID)
	}
	if wechat := clean(enricher.DefaultWechat); wechat != "" {
		query.Set("wechat", wechat)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (enricher PlatformStoreEnricher) httpClient() HTTPDoer {
	if enricher.Client != nil {
		return enricher.Client
	}
	timeout := enricher.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func platformResponseCodeOK(value any) bool {
	switch typed := value.(type) {
	case float64:
		return typed == http.StatusOK
	case string:
		return clean(typed) == strconv.Itoa(http.StatusOK)
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed == http.StatusOK
	case int:
		return typed == http.StatusOK
	case int64:
		return typed == http.StatusOK
	default:
		return false
	}
}
