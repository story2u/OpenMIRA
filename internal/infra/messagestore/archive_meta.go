// Archive metadata parsing mirrors the read-only parts of Python
// chat_archive_adapter.py for message detail payloads. It only enriches the
// current message page and never changes archive storage state.
package messagestore

import (
	"encoding/json"
	"fmt"
	"strings"

	"wework-go/internal/messages"
)

func applyArchiveRawMetadata(record *messages.Record, raw map[string]any) {
	payload, decrypted := parseArchiveRawPayload(stringFromDB(raw["raw_json"]))
	msgTypeRaw := strings.ToLower(firstNonEmptyString(stringFromDB(raw["msg_type_raw"]), archiveString(decrypted["msgtype"])))
	if msgTypeRaw != "" {
		record.ArchiveTypeRaw = msgTypeRaw
	}
	if msgtime := extractArchiveMsgtimeMS(payload, decrypted); msgtime > 0 {
		record.ArchiveMsgtime = &msgtime
	}
	if textPayload := archiveMap(decrypted["text"]); textPayload != nil && strings.TrimSpace(record.Content) == "" {
		record.Content = archiveString(textPayload["content"])
	}
	applyArchiveMediaMetadata(record, msgTypeRaw, decrypted)
	if strings.TrimSpace(record.Content) == "[官方加密消息，待解密]" {
		fallbackType, fallbackContent := archiveFallbackContent(msgTypeRaw, decrypted)
		if fallbackType != "" {
			record.MsgType = fallbackType
		}
		if fallbackContent != "" {
			record.Content = fallbackContent
		}
	}
}

func parseArchiveRawPayload(rawJSON string) (map[string]any, map[string]any) {
	payload := map[string]any{}
	if strings.TrimSpace(rawJSON) != "" {
		_ = json.Unmarshal([]byte(rawJSON), &payload)
	}
	decrypted := archiveMap(payload["decrypted"])
	if decrypted == nil {
		decrypted = map[string]any{}
	}
	return payload, decrypted
}

func extractArchiveMsgtimeMS(payload map[string]any, decrypted map[string]any) int64 {
	for _, source := range []map[string]any{decrypted, payload} {
		value, ok := archiveInt64(source["msgtime"])
		if !ok || value <= 0 {
			continue
		}
		if value < 10_000_000_000 {
			return value * 1000
		}
		return value
	}
	return 0
}

func applyArchiveMediaMetadata(record *messages.Record, msgTypeRaw string, decrypted map[string]any) {
	imagePayload := archiveMap(decrypted["image"])
	videoPayload := archiveMap(decrypted["video"])
	voicePayload := archiveMap(decrypted["voice"])
	filePayload := archiveMap(decrypted["file"])
	record.FileName = firstNonEmptyString(record.FileName, archiveString(filePayload["filename"]))
	mediaPayload := map[string]any(nil)
	switch msgTypeRaw {
	case "image":
		record.MsgType = "image"
		mediaPayload = imagePayload
	case "video":
		record.MsgType = "video"
		mediaPayload = videoPayload
	case "voice", "audio":
		record.MsgType = "voice"
		mediaPayload = voicePayload
		if duration, ok := archiveInt64(firstNonNilArchive(voicePayload["play_length"], voicePayload["duration"])); ok && duration > 0 {
			record.VoiceDurationSec = int(duration)
		}
		record.VoiceText = firstNonEmptyString(
			record.VoiceText,
			archiveString(voicePayload["recognition"]),
			archiveString(voicePayload["text"]),
			archiveString(voicePayload["transcript"]),
			archiveString(decrypted["speech_to_text"]),
		)
	case "file":
		record.MsgType = "file"
		mediaPayload = filePayload
	}
	if mediaPayload == nil {
		return
	}
	record.MediaFingerprint = strings.ToLower(firstNonEmptyString(
		record.MediaFingerprint,
		archiveString(mediaPayload["md5sum"]),
		archiveString(mediaPayload["md5"]),
		archiveString(mediaPayload["filemd5"]),
	))
	if size, ok := archiveInt64(firstNonNilArchive(mediaPayload["filesize"], mediaPayload["voice_size"], mediaPayload["size"])); ok && size > 0 {
		record.MediaSizeBytes = size
	}
}

func archiveFallbackContent(msgTypeRaw string, decrypted map[string]any) (string, string) {
	switch msgTypeRaw {
	case "text":
		textPayload := archiveMap(decrypted["text"])
		return "text", firstNonEmptyString(archiveString(textPayload["content"]), "[文本消息]")
	case "image":
		return "image", "[图片消息]"
	case "video":
		return "video", "[视频消息]"
	case "file":
		filePayload := archiveMap(decrypted["file"])
		return "file", firstNonEmptyString(archiveString(filePayload["filename"]), "[文件消息]")
	case "voice", "audio":
		return "voice", "[语音消息]"
	case "link":
		return "text", buildArchiveLinkDisplayText(archiveMap(decrypted["link"]))
	case "weapp":
		return "text", buildArchiveWeappDisplayText(archiveMap(decrypted["weapp"]))
	case "location":
		return "text", buildArchiveLocationDisplayText(archiveMap(decrypted["location"]))
	case "revoke":
		return "unknown", "[消息已撤回]"
	case "voiptext", "meeting_voice_call", "voice_call":
		return "unknown", "[音视频通话消息]"
	case "agree":
		return "unknown", "[会话存档授权] 客户同意"
	case "disagree":
		return "unknown", "[会话存档授权] 客户拒绝"
	default:
		if action := archiveString(decrypted["action"]); action != "" {
			return "unknown", "[会话事件] " + action
		}
		if msgTypeRaw != "" {
			return "unknown", "[" + msgTypeRaw + "消息]"
		}
		return "unknown", "[会话存档消息]"
	}
}

func buildArchiveWeappDisplayText(appPayload map[string]any) string {
	title := archiveString(appPayload["title"])
	description := archiveString(appPayload["description"])
	normalizedPaymentTitle := normalizeArchivePaymentTitle(title)
	if normalizedPaymentTitle != "" && normalizedPaymentTitle != title {
		return "付款给：" + normalizedPaymentTitle
	}
	if strings.Contains(title, "预约") || strings.Contains(description, "预约") {
		switch {
		case title != "" && description != "":
			return "预约成功：" + title + "（" + description + "）"
		case title != "":
			return "预约成功：" + title
		case description != "":
			return "预约成功：" + description
		}
	}
	return firstNonEmptyString(title, description, "[小程序消息]")
}

func buildArchiveLocationDisplayText(locationPayload map[string]any) string {
	title := archiveString(locationPayload["title"])
	address := archiveString(locationPayload["address"])
	if title != "" {
		return "门店位置：" + title
	}
	if address != "" && !looksLikeArchiveURL(address) {
		return "门店位置：" + address
	}
	return "[位置消息]"
}

func buildArchiveLinkDisplayText(linkPayload map[string]any) string {
	title := archiveString(linkPayload["title"])
	description := archiveString(linkPayload["description"])
	linkURL := archiveString(linkPayload["link_url"])
	if title != "" && strings.Contains(strings.ToLower(linkURL), "mmapgwh.map.qq.com/shortlink") {
		return "门店位置：" + title
	}
	pieces := make([]string, 0, 3)
	for _, value := range []string{title, description, linkURL} {
		if value != "" {
			pieces = append(pieces, value)
		}
	}
	if len(pieces) == 0 {
		return "[链接消息]"
	}
	return strings.Join(pieces, " / ")
}

func normalizeArchivePaymentTitle(title string) string {
	title = strings.TrimSpace(title)
	if strings.HasPrefix(title, "付款给 ") {
		return strings.TrimSpace(strings.TrimPrefix(title, "付款给 "))
	}
	return title
}

func looksLikeArchiveURL(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://")
}

func archiveMap(value any) map[string]any {
	switch current := value.(type) {
	case map[string]any:
		return current
	case map[string]string:
		result := make(map[string]any, len(current))
		for key, val := range current {
			result[key] = val
		}
		return result
	default:
		return nil
	}
}

func archiveString(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(current)
	case []byte:
		return strings.TrimSpace(string(current))
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func archiveInt64(value any) (int64, bool) {
	switch current := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(current), true
	case int64:
		return current, true
	case int32:
		return int64(current), true
	case float64:
		return int64(current), true
	case float32:
		return int64(current), true
	case json.Number:
		parsed, err := current.Int64()
		return parsed, err == nil
	case string:
		return parseInt64(current)
	case []byte:
		return parseInt64(string(current))
	default:
		return parseInt64(fmt.Sprint(current))
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNilArchive(values ...any) any {
	for _, value := range values {
		if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return value
		}
	}
	return nil
}
