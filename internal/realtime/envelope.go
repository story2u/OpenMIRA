package realtime

import "strings"

// StrongConsistencyEvents mirrors Python WebSocketHub.STRONG_CONSISTENCY_EVENTS.
var StrongConsistencyEvents = map[string]struct{}{
	"message_created":                        {},
	"conversation.message":                   {},
	"conversation.media_ready":               {},
	"conversation.voice_transcription_ready": {},
	"conversation_updated":                   {},
	"conversation_assigned":                  {},
	"conversation.assignment":                {},
	"conversation_unread_changed":            {},
	"identity_updated":                       {},
	"identity.updated":                       {},
	"friend.added":                           {},
	"customer.relation.changed":              {},
	"customer.relation":                      {},
}

// EnvelopeInput contains one realtime publish event before cursor metadata.
type EnvelopeInput struct {
	Channel string
	Event   string
	Topic   string
	Payload map[string]any
	Cursor  int64
}

// EnvelopeMetadata is the strong-consistency metadata used by event-log appenders.
type EnvelopeMetadata struct {
	ScopeKey    string
	ScopeTopic  string
	Cursor      int64
	Consistency string
}

// FullScopeKey preserves Python's channel-prefixed scope-key rule.
func FullScopeKey(channel string, scopeKey string) string {
	scopeKey = strings.TrimSpace(scopeKey)
	channel = strings.TrimSpace(channel)
	if scopeKey == "" {
		return ""
	}
	if strings.Contains(scopeKey, ":") || channel == "" {
		return scopeKey
	}
	return channel + ":" + scopeKey
}

// ResolveScopeMetadata resolves raw scope, full scope key, and strong-event status.
func ResolveScopeMetadata(channel string, event string, topic string) (string, string, bool) {
	event = strings.TrimSpace(event)
	topic = strings.TrimSpace(topic)
	rawScope := topic
	if rawScope == "" {
		rawScope = event
	}
	scopeKey := FullScopeKey(channel, rawScope)
	return rawScope, scopeKey, IsStrongConsistencyEvent(event, topic)
}

// IsStrongConsistencyEvent reports whether event/topic requires replay support.
func IsStrongConsistencyEvent(event string, topic string) bool {
	_, eventStrong := StrongConsistencyEvents[strings.TrimSpace(event)]
	_, topicStrong := StrongConsistencyEvents[strings.TrimSpace(topic)]
	return eventStrong || topicStrong
}

// BuildEnvelope builds the Python-compatible WebSocket event envelope.
func BuildEnvelope(input EnvelopeInput) (map[string]any, EnvelopeMetadata) {
	channel := strings.TrimSpace(input.Channel)
	event := strings.TrimSpace(input.Event)
	topic := strings.TrimSpace(input.Topic)
	rawScope, scopeKey, isStrong := ResolveScopeMetadata(channel, event, topic)
	cursor := int64(0)
	if isStrong && scopeKey != "" && input.Cursor > 0 {
		cursor = input.Cursor
	}
	consistency := "weak"
	if cursor > 0 {
		consistency = "strong"
	}
	envelope := map[string]any{
		"channel":     channel,
		"event":       event,
		"topic":       topic,
		"payload":     clonePayload(input.Payload),
		"consistency": consistency,
	}
	if cursor > 0 {
		envelope["cursor"] = cursor
		envelope["scope_key"] = scopeKey
		envelope["scope_topic"] = rawScope
	}
	return envelope, EnvelopeMetadata{
		ScopeKey:    scopeKey,
		ScopeTopic:  rawScope,
		Cursor:      cursor,
		Consistency: consistency,
	}
}

func clonePayload(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
