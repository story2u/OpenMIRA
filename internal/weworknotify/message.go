package weworknotify

import (
	"context"
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
	"time"

	"im-go/internal/archivecallback"
	"im-go/internal/incomingqueue"
)

const (
	weworkNotifyConnectorID = "wework.notify"
	weworkNotifyChannel     = "wework"
)

type incomingMessageResult struct {
	TraceID string
}

type weworkNotifyMessageXML struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   string `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Event        string `xml:"Event"`
	Content      string `xml:"Content"`
	MsgID        string `xml:"MsgID"`
	MsgId        string `xml:"MsgId"`
	NewMsgID     string `xml:"NewMsgId"`
	AgentID      string `xml:"AgentID"`
	UserID       string `xml:"UserID"`
	ExternalUser string `xml:"ExternalUserID"`
	ChatID       string `xml:"ChatId"`
	RoomID       string `xml:"RoomId"`
	PicURL       string `xml:"PicUrl"`
	MediaID      string `xml:"MediaId"`
	ThumbMediaID string `xml:"ThumbMediaId"`
	Format       string `xml:"Format"`
	Recognition  string `xml:"Recognition"`
	Title        string `xml:"Title"`
	Description  string `xml:"Description"`
	URL          string `xml:"Url"`
	FileName     string `xml:"FileName"`
	LocationX    string `xml:"Location_X"`
	LocationY    string `xml:"Location_Y"`
	Scale        string `xml:"Scale"`
	Label        string `xml:"Label"`
	AppType      string `xml:"AppType"`
	EventKey     string `xml:"EventKey"`
}

func (service Service) queueIncomingMessage(ctx context.Context, enterprise archivecallback.Enterprise, plain string, callbackEventKey string) (incomingMessageResult, bool, error) {
	message, ok, err := decodeWeWorkNotifyMessage(plain)
	if err != nil {
		return incomingMessageResult{}, false, err
	}
	if !ok {
		return incomingMessageResult{}, false, nil
	}
	if service.Incoming == nil {
		return incomingMessageResult{}, true, HTTPError{StatusCode: http.StatusServiceUnavailable, Detail: "wework incoming queue is not configured"}
	}
	event := service.buildIncomingQueuePayload(enterprise, message, callbackEventKey)
	if _, _, err := service.Incoming.Enqueue(ctx, event, nil); err != nil {
		return incomingMessageResult{}, true, HTTPError{StatusCode: http.StatusServiceUnavailable, Detail: "wework incoming queue enqueue failed: " + err.Error()}
	}
	return incomingMessageResult{TraceID: textValue(event["trace_id"])}, true, nil
}

func decodeWeWorkNotifyMessage(xmlText string) (weworkNotifyMessageXML, bool, error) {
	var message weworkNotifyMessageXML
	if err := xml.Unmarshal([]byte(strings.TrimSpace(xmlText)), &message); err != nil {
		return weworkNotifyMessageXML{}, false, HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback message payload invalid: " + err.Error()}
	}
	msgType := strings.ToLower(strings.TrimSpace(message.MsgType))
	if msgType == "" || msgType == "event" {
		return weworkNotifyMessageXML{}, false, nil
	}
	if _, ok := incomingMessageType(msgType); !ok {
		return weworkNotifyMessageXML{}, false, nil
	}
	if strings.TrimSpace(message.FromUserName) == "" && strings.TrimSpace(message.ExternalUser) == "" {
		return weworkNotifyMessageXML{}, false, HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback message sender is required"}
	}
	return message, true, nil
}

func (service Service) buildIncomingQueuePayload(enterprise archivecallback.Enterprise, message weworkNotifyMessageXML, callbackEventKey string) map[string]any {
	now := service.now()
	occurredAt := parseWeWorkCreateTime(message.CreateTime, now)
	timestamp := occurredAt.UTC().Format(time.RFC3339Nano)
	enterpriseID := strings.TrimSpace(enterprise.EnterpriseID)
	corpID := strings.TrimSpace(enterprise.CorpID)
	traceID := "wework-notify-message:" + strings.TrimSpace(callbackEventKey)
	messageID := firstNonBlank(message.MsgID, message.MsgId, message.NewMsgID)
	roomID := firstNonBlank(message.RoomID, message.ChatID)
	channelUserID := firstNonBlank(message.UserID, message.ToUserName, message.AgentID, corpID)
	externalUserID := firstNonBlank(message.ExternalUser, message.FromUserName)
	senderID := firstNonBlank(message.FromUserName, externalUserID)
	senderName := firstNonBlank(message.FromUserName, externalUserID, "unknown")
	conversationType := "single"
	if strings.TrimSpace(roomID) != "" {
		conversationType = "room"
	}
	conversationKey := weworkConversationKey(channelUserID, externalUserID, roomID)
	msgType, _ := incomingMessageType(message.MsgType)
	content := messageContent(message)
	data := map[string]any{
		"tenant_id":                enterpriseID,
		"connector_id":             weworkNotifyConnectorID,
		"channel":                  weworkNotifyChannel,
		"channel_user_id":          channelUserID,
		"account_user_id":          channelUserID,
		"wework_user_id":           channelUserID,
		"endpoint_id":              firstNonBlank(message.AgentID, channelUserID),
		"device_id":                firstNonBlank(message.AgentID, channelUserID),
		"sender_id":                senderID,
		"sender":                   senderName,
		"sender_name":              senderName,
		"content":                  content,
		"msg_type":                 msgType,
		"conversation_name":        firstNonBlank(message.Label, message.Title, senderName),
		"conversation_key":         conversationKey,
		"account_id":               firstNonBlank(message.AgentID, channelUserID, corpID),
		"external_user_id":         externalUserID,
		"external_userid":          externalUserID,
		"room_id":                  roomID,
		"conversation_type":        conversationType,
		"message_id":               messageID,
		"archive_msgid":            externalArchiveMessageID(messageID),
		"message_origin":           "connector:" + weworkNotifyConnectorID,
		"timestamp":                timestamp,
		"connector_event_id":       callbackEventKey,
		"idempotency_key":          callbackEventKey,
		"external_conversation_id": firstNonBlank(message.ChatID, message.RoomID),
		"raw_msg_type":             strings.ToLower(strings.TrimSpace(message.MsgType)),
		"metadata": map[string]any{
			"corp_id":            corpID,
			"agent_id":           strings.TrimSpace(message.AgentID),
			"callback_event_key": strings.TrimSpace(callbackEventKey),
			"media_id":           strings.TrimSpace(message.MediaID),
			"thumb_media_id":     strings.TrimSpace(message.ThumbMediaID),
			"pic_url":            strings.TrimSpace(message.PicURL),
			"format":             strings.TrimSpace(message.Format),
			"app_type":           strings.TrimSpace(message.AppType),
			"event_key":          strings.TrimSpace(message.EventKey),
		},
	}
	if media := messageMedia(message); len(media) > 0 {
		data["media"] = media
	}
	return map[string]any{
		"event_type":   incomingqueue.EventTypeConnectorInbound,
		"kind":         "connector.inbound_message",
		"event_id":     traceID,
		"trace_id":     traceID,
		"tenant_id":    enterpriseID,
		"connector_id": weworkNotifyConnectorID,
		"channel":      weworkNotifyChannel,
		"device_id":    firstNonBlank(message.AgentID, channelUserID),
		"occurred_at":  timestamp,
		"data":         data,
	}
}

func incomingMessageType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text":
		return "text", true
	case "image":
		return "image", true
	case "voice":
		return "voice", true
	case "video":
		return "video", true
	case "file":
		return "file", true
	case "location", "link":
		return "text", true
	default:
		return "", false
	}
}

func messageContent(message weworkNotifyMessageXML) string {
	switch strings.ToLower(strings.TrimSpace(message.MsgType)) {
	case "text":
		return strings.TrimSpace(message.Content)
	case "image":
		return firstNonBlank(message.PicURL, message.MediaID, "[image]")
	case "voice":
		return firstNonBlank(message.Recognition, message.MediaID, "[voice]")
	case "video":
		return firstNonBlank(message.Title, message.MediaID, "[video]")
	case "file":
		return firstNonBlank(message.FileName, message.Title, message.MediaID, "[file]")
	case "location":
		return firstNonBlank(message.Label, joinedLocation(message.LocationX, message.LocationY), "[location]")
	case "link":
		return firstNonBlank(message.Title, message.Description, message.URL, "[link]")
	default:
		return strings.TrimSpace(message.Content)
	}
}

func messageMedia(message weworkNotifyMessageXML) []map[string]any {
	msgType := strings.ToLower(strings.TrimSpace(message.MsgType))
	mediaID := strings.TrimSpace(message.MediaID)
	picURL := strings.TrimSpace(message.PicURL)
	if mediaID == "" && picURL == "" && strings.TrimSpace(message.ThumbMediaID) == "" {
		return nil
	}
	item := map[string]any{
		"attachment_id": mediaID,
		"type":          msgType,
		"url":           picURL,
		"metadata": map[string]any{
			"thumb_media_id": strings.TrimSpace(message.ThumbMediaID),
			"format":         strings.TrimSpace(message.Format),
			"title":          strings.TrimSpace(message.Title),
			"file_name":      strings.TrimSpace(message.FileName),
		},
	}
	return []map[string]any{item}
}

func parseWeWorkCreateTime(value string, fallback time.Time) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return fallback.UTC()
	}
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err == nil && seconds > 0 {
		return time.Unix(seconds, 0).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed.UTC()
	}
	return fallback.UTC()
}

func weworkConversationKey(channelUserID string, externalUserID string, roomID string) string {
	parts := []string{weworkNotifyChannel}
	if channelUserID = strings.TrimSpace(channelUserID); channelUserID != "" {
		parts = append(parts, channelUserID)
	}
	if roomID = strings.TrimSpace(roomID); roomID != "" {
		parts = append(parts, "room", roomID)
	} else if externalUserID = strings.TrimSpace(externalUserID); externalUserID != "" {
		parts = append(parts, externalUserID)
	}
	return strings.Join(parts, ":")
}

func externalArchiveMessageID(messageID string) string {
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		return weworkNotifyConnectorID + ":" + messageID
	}
	return ""
}

func joinedLocation(x string, y string) string {
	x = strings.TrimSpace(x)
	y = strings.TrimSpace(y)
	if x == "" {
		return y
	}
	if y == "" {
		return x
	}
	return x + "," + y
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
