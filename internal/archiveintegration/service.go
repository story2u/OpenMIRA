// Package archiveintegration implements the archive integration diagnostic flow.
package archiveintegration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"wework-go/internal/archiveadmin"
	"wework-go/internal/archivecontacts"
	"wework-go/internal/archiveingest"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesdk"
	"wework-go/internal/archivesync"
	"wework-go/internal/infra/enterprisestore"
)

const (
	DefaultSource       = archiveadmin.DefaultSource
	DefaultPullLimit    = 20
	DefaultSyncLimit    = 100
	DefaultContactLimit = 100
	DefaultMediaLimit   = 20
)

var (
	// ErrEnterpriseIDRequired mirrors the legacy validation detail.
	ErrEnterpriseIDRequired = errors.New("enterprise_id is required")
	// ErrEnterpriseNotFound mirrors the legacy 404 detail.
	ErrEnterpriseNotFound = errors.New("enterprise not found")
	// ErrEnterpriseStoreUnavailable means enterprise config cannot be checked.
	ErrEnterpriseStoreUnavailable = errors.New("archive enterprise store is unavailable")
)

// Payload is a JSON-compatible response shape.
type Payload map[string]any

// Request carries POST /api/v1/archive/integration/test input.
type Request struct {
	EnterpriseID string
	Source       string
	PullLimit    int
	SyncLimit    int
	ContactLimit int
	MediaLimit   int
}

// EnterpriseStore reads enterprise archive configuration.
type EnterpriseStore interface {
	GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error)
}

// SDKPuller runs the built-in SDK pull bridge.
type SDKPuller interface {
	Pull(ctx context.Context, request archivesdk.PullRequest) (archivesdk.Payload, error)
}

// SyncRunner runs one archive sync pass.
type SyncRunner interface {
	RunArchiveSyncOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error)
}

// TaskProcessor processes a staged archive ingest task.
type TaskProcessor interface {
	ProcessTask(ctx context.Context, taskID string) (*archiveingest.Result, error)
}

// ContactsSyncer runs archive contact refresh.
type ContactsSyncer interface {
	SyncArchiveContacts(ctx context.Context, request archivecontacts.Request) (archivecontacts.Payload, error)
}

// MediaRunner runs one archive media pass.
type MediaRunner interface {
	RunOnce(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error)
}

// MediaRunnerWithLimit runs one archive media pass with a caller scoped limit.
type MediaRunnerWithLimit interface {
	RunOnceWithLimit(ctx context.Context, enterpriseID string, source string, limit int) (archivemedia.RunResult, error)
}

// Service owns the integration diagnostic flow.
type Service struct {
	Enterprises EnterpriseStore
	SDKStatus   archiveadmin.SDKStatusProvider
	SDKPull     SDKPuller
	SyncRun     SyncRunner
	SyncIngest  TaskProcessor
	Contacts    ContactsSyncer
	Media       MediaRunner
}

// Test runs the legacy archive integration diagnostic flow.
func (service Service) Test(ctx context.Context, request Request) (Payload, error) {
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	if enterpriseID == "" {
		return nil, ErrEnterpriseIDRequired
	}
	if service.Enterprises == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	enterprise, err := service.Enterprises.GetEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	if enterprise == nil {
		return nil, fmt.Errorf("%w: %s", ErrEnterpriseNotFound, enterpriseID)
	}

	source := defaultText(request.Source, DefaultSource)
	limits := normalizedLimits(request)
	steps := []Payload{}
	missing := service.missingConfig(ctx, enterprise)
	if len(missing) > 0 {
		steps = append(steps, Payload{
			"name":   "配置检查",
			"status": "failed",
			"detail": "存在必填缺失项",
			"error":  "缺失：" + strings.Join(missing, ", "),
		})
		return Payload{"passed": false, "enterprise_id": enterpriseID, "steps": steps}, nil
	}
	steps = append(steps, Payload{
		"name":   "配置检查",
		"status": "passed",
		"detail": "配置项完整，可开始联调",
		"error":  "",
	})
	steps = append(steps, service.sdkPullStep(ctx, enterpriseID, source, limits.PullLimit))
	steps = append(steps, service.syncStep(ctx, enterpriseID, source, limits.SyncLimit))
	steps = append(steps, service.contactsStep(ctx, enterpriseID, limits.ContactLimit))
	steps = append(steps, service.mediaStep(ctx, enterpriseID, source, limits.MediaLimit))
	return Payload{"passed": allPassed(steps), "enterprise_id": enterpriseID, "steps": steps}, nil
}

func (service Service) missingConfig(ctx context.Context, enterprise *enterprisestore.EnterpriseRecord) []string {
	missing := []string{}
	if strings.TrimSpace(enterprise.CorpID) == "" {
		missing = append(missing, "corp_id")
	}
	if strings.TrimSpace(enterprise.CorpSecret) == "" {
		missing = append(missing, "corp_secret(会话存档 Secret)")
	}
	if strings.TrimSpace(enterprise.ContactSecret) == "" {
		missing = append(missing, "contact_secret(通讯录管理 Secret，影响内部成员 userid/资料补齐)")
	}
	sdkAvailable := false
	if service.SDKStatus != nil {
		sdkAvailable = service.SDKStatus.OfficialSDKStatus(ctx).Available
	}
	if !sdkAvailable && strings.TrimSpace(enterprise.ArchivePullURL) == "" {
		missing = append(missing, "WEWORK_FINANCE_SDK_LIB_PATH / archive_pull_url")
	}
	return missing
}

func (service Service) sdkPullStep(ctx context.Context, enterpriseID string, source string, limit int) Payload {
	if service.SDKPull == nil {
		return failedStep("SDK拉取与解密检查", "拉取失败", "archive sdk bridge service is not configured")
	}
	payload, err := service.SDKPull.Pull(ctx, archivesdk.PullRequest{EnterpriseID: enterpriseID, Source: source, Limit: limit})
	if err != nil {
		return failedStep("SDK拉取与解密检查", "拉取失败", err.Error())
	}
	messages := messageList(payload["messages"])
	decrypted := 0
	encrypted := 0
	versions := map[string]bool{}
	decryptErrors := []string{}
	for _, message := range messages {
		msgTypeRaw := strings.TrimSpace(textValue(message["msg_type_raw"]))
		if msgTypeRaw == "encrypted" {
			encrypted++
		} else {
			decrypted++
		}
		if version := strings.TrimSpace(textValue(message["publickey_ver"])); version != "" {
			versions[version] = true
		}
		if decryptError := strings.TrimSpace(textValue(message["decrypt_error"])); decryptError != "" {
			decryptErrors = append(decryptErrors, decryptError)
		}
	}
	switch {
	case len(messages) == 0:
		return Payload{"name": "SDK拉取与解密检查", "status": "warning", "detail": "拉取成功，但当前窗口无新消息", "error": ""}
	case decrypted == 0:
		return Payload{
			"name":   "SDK拉取与解密检查",
			"status": "failed",
			"detail": fmt.Sprintf("total=%d, encrypted=%d, observed_publickey_ver=%s", len(messages), encrypted, versionList(versions)),
			"error":  firstText(decryptErrors, "消息均为加密态。请核对企微私钥版本与企业配置是否一致。"),
		}
	default:
		return Payload{"name": "SDK拉取与解密检查", "status": "passed", "detail": fmt.Sprintf("共拉取 %d 条，成功解密 %d 条", len(messages), decrypted), "error": ""}
	}
}

func (service Service) syncStep(ctx context.Context, enterpriseID string, source string, limit int) Payload {
	if service.SyncRun == nil {
		return failedStep("会话入库检查", "入库失败", "archive sync service is not configured")
	}
	result, err := service.SyncRun.RunArchiveSyncOnce(ctx, archivesync.Request{
		EnterpriseID:  enterpriseID,
		Source:        source,
		Limit:         limit,
		TriggerReason: "integration_test",
	})
	if err != nil {
		return failedStep("会话入库检查", "入库失败", err.Error())
	}
	if result.Skipped {
		return failedStep("会话入库检查", "入库跳过", result.SkipReason)
	}
	if strings.TrimSpace(result.StagedTaskID) != "" && service.SyncIngest != nil {
		processed, err := service.SyncIngest.ProcessTask(ctx, result.StagedTaskID)
		if err != nil {
			return failedStep("会话入库检查", "入库失败", err.Error())
		}
		if processed != nil {
			return Payload{
				"name":   "会话入库检查",
				"status": "passed",
				"detail": fmt.Sprintf("total=%d, inserted=%d, deduplicated=%d, cursor=%s", processed.Total, processed.Inserted, processed.Deduplicated, textFromPtr(processed.Cursor)),
				"error":  "",
			}
		}
	}
	return Payload{
		"name":   "会话入库检查",
		"status": "passed",
		"detail": fmt.Sprintf("pulled_total=%d, staged_task_id=%s, cursor=%s", result.PulledTotal, strings.TrimSpace(result.StagedTaskID), textFromPtr(result.Cursor)),
		"error":  "",
	}
}

func (service Service) contactsStep(ctx context.Context, enterpriseID string, limit int) Payload {
	if service.Contacts == nil {
		return failedStep("联系人资料补齐检查", "补齐失败", "archive contacts sync service is not configured")
	}
	payload, err := service.Contacts.SyncArchiveContacts(ctx, archivecontacts.Request{EnterpriseID: enterpriseID, ForceRefresh: true, Limit: limit})
	if err != nil {
		return failedStep("联系人资料补齐检查", "补齐失败", err.Error())
	}
	profiles := messageList(payload["profiles"])
	withName := 0
	withAvatar := 0
	for _, profile := range profiles {
		if strings.TrimSpace(textValue(profile["sender_name"])) != "" {
			withName++
		}
		if strings.TrimSpace(textValue(profile["sender_avatar"])) != "" {
			withAvatar++
		}
	}
	status := "passed"
	errText := ""
	if len(profiles) == 0 {
		status = "warning"
		errText = "当前无可补齐 sender_id，请先确保会话消息已入库。"
	}
	return Payload{"name": "联系人资料补齐检查", "status": status, "detail": fmt.Sprintf("total=%d, with_name=%d, with_avatar=%d", len(profiles), withName, withAvatar), "error": errText}
}

func (service Service) mediaStep(ctx context.Context, enterpriseID string, source string, limit int) Payload {
	if service.Media == nil {
		return failedStep("媒体补拉检查", "媒体补拉失败", "archive media service is not configured")
	}
	var (
		result archivemedia.RunResult
		err    error
	)
	if limited, ok := service.Media.(MediaRunnerWithLimit); ok {
		result, err = limited.RunOnceWithLimit(ctx, enterpriseID, source, limit)
	} else {
		result, err = service.Media.RunOnce(ctx, enterpriseID, source)
	}
	if err != nil {
		return failedStep("媒体补拉检查", "媒体补拉失败", err.Error())
	}
	status := "passed"
	errText := ""
	if result.Total == 0 {
		status = "warning"
		errText = "当前没有媒体任务（需先有带 sdkfileid 的消息）。"
	} else if result.Failed > 0 {
		status = "failed"
	}
	return Payload{"name": "媒体补拉检查", "status": status, "detail": fmt.Sprintf("total=%d, success=%d, failed=%d, pending=%d", result.Total, result.Success, result.Failed, result.Pending), "error": errText}
}

type limits struct {
	PullLimit    int
	SyncLimit    int
	ContactLimit int
	MediaLimit   int
}

func normalizedLimits(request Request) limits {
	return limits{
		PullLimit:    clamp(request.PullLimit, DefaultPullLimit, 1, 200),
		SyncLimit:    clamp(request.SyncLimit, DefaultSyncLimit, 1, 500),
		ContactLimit: clamp(request.ContactLimit, DefaultContactLimit, 1, 500),
		MediaLimit:   clamp(request.MediaLimit, DefaultMediaLimit, 1, 100),
	}
}

func clamp(value int, fallback int, minValue int, maxValue int) int {
	if value == 0 {
		value = fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func failedStep(name string, detail string, err string) Payload {
	return Payload{"name": name, "status": "failed", "detail": detail, "error": strings.TrimSpace(err)}
}

func allPassed(steps []Payload) bool {
	for _, step := range steps {
		if strings.TrimSpace(textValue(step["status"])) != "passed" {
			return false
		}
	}
	return true
}

func messageList(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []Payload:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, map[string]any(item))
		}
		return items
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
		return items
	default:
		return []map[string]any{}
	}
}

func versionList(values map[string]bool) string {
	if len(values) == 0 {
		return "-"
	}
	versions := make([]string, 0, len(values))
	for version := range values {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return strings.Join(versions, ",")
}

func firstText(values []string, fallback string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fallback
}

func textFromPtr(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "-"
	}
	return strings.TrimSpace(*value)
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}
