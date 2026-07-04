// Package archivepull contains archive pull request and normalization rules.
package archivepull

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultPullLimitMaximum = 2000
	DefaultSDKLimitMaximum  = 1000
)

// Result mirrors Python ArchivePullResult.
type Result struct {
	Source   string
	Cursor   *string
	Messages []map[string]any
}

// BuildMessageInput contains decrypted archive message projection inputs.
type BuildMessageInput struct {
	RawItem          map[string]any
	Decrypted        map[string]any
	ArchiveMsgID     string
	ItemSeq          int
	NowISO           string
	EnterpriseID     string
	PublicKeyVersion any
	EncryptRandomKey string
	EncryptChatMsg   string
	DecryptError     string
}

// ClampPullLimit mirrors Python clamp_pull_limit.
func ClampPullLimit(limit int, maximum int) int {
	if maximum <= 0 {
		maximum = DefaultPullLimitMaximum
	}
	if limit < 1 {
		return 1
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

// ClampSDKLimit mirrors Python clamp_sdk_limit.
func ClampSDKLimit(limit int, maximum int) int {
	if maximum <= 0 {
		maximum = DefaultSDKLimitMaximum
	}
	if limit < 1 {
		return 1
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

// BuildAuthHeaders builds the optional bearer token header.
func BuildAuthHeaders(token string) map[string]string {
	token = strings.TrimSpace(token)
	if token == "" {
		return map[string]string{}
	}
	return map[string]string{"Authorization": "Bearer " + token}
}

// BuildPullPayload mirrors Python build_pull_payload.
func BuildPullPayload(source string, cursor *string, limit int, enterpriseID string, mode string) map[string]any {
	payload := map[string]any{
		"source": strings.TrimSpace(source),
		"cursor": nil,
		"limit":  ClampPullLimit(limit, DefaultPullLimitMaximum),
	}
	if cursor != nil {
		payload["cursor"] = strings.TrimSpace(*cursor)
	}
	if enterpriseID = strings.TrimSpace(enterpriseID); enterpriseID != "" {
		payload["enterprise_id"] = enterpriseID
	}
	if mode = strings.TrimSpace(mode); mode != "" {
		payload["archive_mode"] = mode
	}
	return payload
}

// NormalizePullResponse mirrors Python normalize_pull_response.
func NormalizePullResponse(data map[string]any, source string) Result {
	if data == nil {
		data = map[string]any{}
	}
	var cursor *string
	if value, ok := data["cursor"]; ok && value != nil {
		text := fmt.Sprint(value)
		cursor = &text
	}
	return Result{
		Source:   defaultText(textValue(data["source"]), source),
		Cursor:   cursor,
		Messages: messageMaps(data["messages"]),
	}
}

// BuildArchiveMessage mirrors Python ArchivePullClient._build_archive_message.
func BuildArchiveMessage(input BuildMessageInput) map[string]any {
	rawItem := cloneMap(input.RawItem)
	payload := cloneMap(input.Decrypted)
	archiveMsgID := strings.TrimSpace(input.ArchiveMsgID)
	senderID := firstText(payload["from"], payload["user"], "encrypted:"+archiveMsgID)
	roomID := textValue(payload["roomid"])
	externalUserID := ExtractExternalUserID(senderID, payload["tolist"])
	conversationName := firstText(roomID, externalUserID, senderID, "official-msgaudit")
	msgTypeRaw := textValue(payload["msgtype"])
	action := textValue(payload["action"])
	if msgTypeRaw == "" && action != "" {
		msgTypeRaw = "event"
	}
	if msgTypeRaw == "" {
		msgTypeRaw = "encrypted"
	}
	msgType := "unknown"
	content := "[官方加密消息，待解密]"
	sdkFileID := ""

	switch msgTypeRaw {
	case "text":
		msgType = "text"
		content = defaultText(textValue(mapValue(payload["text"])["content"]), "[文本消息]")
	case "image":
		msgType = "image"
		sdkFileID = textValue(mapValue(payload["image"])["sdkfileid"])
		content = "[图片消息]"
	case "voice", "audio":
		msgType = "voice"
		sdkFileID = textValue(mapValue(payload["voice"])["sdkfileid"])
		content = "[语音消息]"
	case "video":
		msgType = "video"
		sdkFileID = textValue(mapValue(payload["video"])["sdkfileid"])
		content = "[视频消息]"
	case "file":
		msgType = "file"
		filePayload := mapValue(payload["file"])
		sdkFileID = textValue(filePayload["sdkfileid"])
		content = defaultText(textValue(filePayload["filename"]), "[文件消息]")
	case "link":
		msgType = "text"
		content = buildLinkDisplayText(mapValue(payload["link"]))
	case "weapp":
		msgType = "text"
		content = buildWeappDisplayText(mapValue(payload["weapp"]))
	case "location":
		msgType = "text"
		content = buildLocationDisplayText(mapValue(payload["location"]))
	case "revoke":
		content = "[消息已撤回]"
	case "voiptext", "meeting_voice_call", "voice_call":
		content = "[音视频通话消息]"
	case "disagree":
		content = "[会话存档授权] 客户拒绝"
	case "agree":
		content = "[会话存档授权] 客户同意"
	case "event":
		content = "[会话事件] " + defaultText(action, "unknown")
	default:
		if len(payload) > 0 {
			content = "[" + msgTypeRaw + "消息]"
		}
	}

	timestamp := strings.TrimSpace(input.NowISO)
	if value, ok := int64Value(payload["msgtime"]); ok {
		timestamp = time.UnixMilli(value).UTC().Format(time.RFC3339Nano)
	}
	toList := stringList(payload["tolist"])
	internalUserID := ExtractInternalUserID(senderID, toList)
	direction := "outgoing"
	if LooksExternalUserID(senderID) {
		direction = "incoming"
	}
	deviceID := ""
	if internalUserID != "" {
		deviceID = "archive_user:" + internalUserID
	} else {
		deviceID = "enterprise:" + defaultText(input.EnterpriseID, "default")
	}
	rawJSON := cloneMap(rawItem)
	rawJSON["decrypted"] = payload

	return map[string]any{
		"archive_msgid":      archiveMsgID,
		"device_id":          deviceID,
		"sender_id":          senderID,
		"sender_name":        senderID,
		"conversation_name":  conversationName,
		"content":            content,
		"msg_type":           msgType,
		"direction":          direction,
		"timestamp":          timestamp,
		"seq":                input.ItemSeq,
		"action":             action,
		"from_id":            senderID,
		"to_list":            toList,
		"room_id":            roomID,
		"msg_type_raw":       msgTypeRaw,
		"sdk_file_id":        sdkFileID,
		"internal_user_id":   internalUserID,
		"external_userid":    externalUserID,
		"publickey_ver":      optionalInt(input.PublicKeyVersion),
		"encrypt_random_key": strings.TrimSpace(input.EncryptRandomKey),
		"encrypt_chat_msg":   strings.TrimSpace(input.EncryptChatMsg),
		"is_system_event":    msgTypeRaw == "event",
		"decrypt_error":      strings.TrimSpace(input.DecryptError),
		"raw_json":           rawJSON,
	}
}

// LooksExternalUserID mirrors Python looks_external_user_id.
func LooksExternalUserID(value string) bool {
	userID := strings.ToLower(strings.TrimSpace(value))
	if userID == "" {
		return false
	}
	return strings.HasPrefix(userID, "wo") || strings.HasPrefix(userID, "wm") || strings.HasPrefix(userID, "external_")
}

// ExtractInternalUserID returns the sender or first recipient that is not external.
func ExtractInternalUserID(senderID string, toList []string) string {
	sender := strings.TrimSpace(senderID)
	if sender != "" && !LooksExternalUserID(sender) {
		return sender
	}
	for _, item := range toList {
		item = strings.TrimSpace(item)
		if item != "" && !LooksExternalUserID(item) {
			return item
		}
	}
	return ""
}

// ExtractExternalUserID returns sender or recipient external user id.
func ExtractExternalUserID(senderID string, toList any) string {
	sender := strings.TrimSpace(senderID)
	if sender != "" && LooksExternalUserID(sender) {
		return sender
	}
	for _, item := range stringList(toList) {
		if LooksExternalUserID(item) {
			return item
		}
	}
	return ""
}

func buildWeappDisplayText(payload map[string]any) string {
	title := textValue(payload["title"])
	description := textValue(payload["description"])
	normalizedPaymentTitle := normalizePaymentTitle(title)
	if normalizedPaymentTitle != "" && normalizedPaymentTitle != title {
		return "付款给：" + normalizedPaymentTitle
	}
	if strings.Contains(title, "预约") || strings.Contains(description, "预约") {
		if title != "" && description != "" {
			return "预约成功：" + title + "（" + description + "）"
		}
		if title != "" {
			return "预约成功：" + title
		}
		if description != "" {
			return "预约成功：" + description
		}
	}
	return firstText(title, description, "[小程序消息]")
}

func buildLocationDisplayText(payload map[string]any) string {
	title := textValue(payload["title"])
	address := textValue(payload["address"])
	if title != "" {
		return "门店位置：" + title
	}
	if address != "" && !looksLikeURL(address) {
		return "门店位置：" + address
	}
	return "[位置消息]"
}

func buildLinkDisplayText(payload map[string]any) string {
	title := textValue(payload["title"])
	description := textValue(payload["description"])
	url := textValue(payload["link_url"])
	if title != "" && isMapShortlinkURL(url) {
		return "门店位置：" + title
	}
	pieces := []string{}
	for _, item := range []string{title, description, url} {
		if item != "" {
			pieces = append(pieces, item)
		}
	}
	if len(pieces) == 0 {
		return "[链接消息]"
	}
	return strings.Join(pieces, " / ")
}

func normalizePaymentTitle(title string) string {
	title = strings.TrimSpace(title)
	return strings.TrimSpace(strings.TrimPrefix(title, "付款给 "))
}

func looksLikeURL(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")
}

func isMapShortlinkURL(value string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(value)), "mmapgwh.map.qq.com/shortlink")
}

func messageMaps(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if maps, ok := value.([]map[string]any); ok {
			return append([]map[string]any(nil), maps...)
		}
		return []map[string]any{}
	}
	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			messages = append(messages, mapped)
		}
	}
	return messages
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	case []any:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := textValue(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case map[string]any:
		output := []string{}
		for _, item := range typed {
			if text := textValue(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	}
	return []string{}
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := textValue(value); text != "" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func optionalInt(value any) any {
	if parsed, ok := int64Value(value); ok {
		return int(parsed)
	}
	return nil
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case string:
		var parsed int64
		_, err := fmt.Sscan(strings.TrimSpace(typed), &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}
