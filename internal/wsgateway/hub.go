package wsgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"wework-go/internal/realtime"
	"wework-go/internal/streamchannels"
)

// Sender is the small websocket connection shape used by Hub.
type Sender interface {
	WriteText(ctx context.Context, message []byte) error
	Close() error
}

// Client is one local websocket subscriber.
type Client struct {
	Channel string
	Topics  map[string]struct{}
	Sender  Sender
}

// PublishEvent is a Python-compatible websocket event envelope.
type PublishEvent struct {
	Channel string
	Event   string
	Topic   string
	Payload map[string]any
}

// BrokerMessage is the legacy Redis broker message shape.
type BrokerMessage struct {
	Origin   string          `json:"origin,omitempty"`
	Channel  string          `json:"channel,omitempty"`
	Event    string          `json:"event,omitempty"`
	Topic    string          `json:"topic,omitempty"`
	Payload  map[string]any  `json:"payload,omitempty"`
	Envelope map[string]any  `json:"envelope,omitempty"`
	Batch    []BrokerMessage `json:"batch,omitempty"`
}

// EventLog appends strong realtime events after local delivery.
type EventLog interface {
	AppendEvent(ctx context.Context, record realtime.EventRecord) error
}

// Hub tracks local websocket clients and fanouts candidate events.
type Hub struct {
	EventLog EventLog
	mu       sync.RWMutex
	clients  map[string]map[*Client]struct{}
}

var _ streamchannels.StatsProvider = (*Hub)(nil)

// NewHub creates an empty local websocket hub.
func NewHub() *Hub {
	return &Hub{clients: map[string]map[*Client]struct{}{}}
}

// Register adds a client to one channel.
func (hub *Hub) Register(channel string, topics []string, sender Sender) *Client {
	if hub == nil {
		return nil
	}
	channel = strings.TrimSpace(channel)
	client := &Client{
		Channel: channel,
		Topics:  normalizeTopics(topics),
		Sender:  sender,
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.clients == nil {
		hub.clients = map[string]map[*Client]struct{}{}
	}
	if hub.clients[channel] == nil {
		hub.clients[channel] = map[*Client]struct{}{}
	}
	hub.clients[channel][client] = struct{}{}
	return client
}

// Unregister removes a client and closes its sender.
func (hub *Hub) Unregister(client *Client) {
	if hub == nil || client == nil {
		return
	}
	hub.mu.Lock()
	if hub.clients != nil && hub.clients[client.Channel] != nil {
		delete(hub.clients[client.Channel], client)
		if len(hub.clients[client.Channel]) == 0 {
			delete(hub.clients, client.Channel)
		}
	}
	hub.mu.Unlock()
	if client.Sender != nil {
		_ = client.Sender.Close()
	}
}

// Publish sends one event to matching local clients.
func (hub *Hub) Publish(ctx context.Context, event PublishEvent) (int, error) {
	if hub == nil {
		return 0, nil
	}
	envelope, _ := realtime.BuildEnvelope(realtime.EnvelopeInput{
		Channel: event.Channel,
		Event:   event.Event,
		Topic:   event.Topic,
		Payload: event.Payload,
	})
	return hub.DeliverEnvelope(ctx, envelope)
}

// DeliverBrokerPayload consumes one legacy Redis broker JSON message locally.
func (hub *Hub) DeliverBrokerPayload(ctx context.Context, raw []byte, localOrigin string) (int, error) {
	if hub == nil {
		return 0, nil
	}
	var message BrokerMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return 0, err
	}
	if sameOrigin(message.Origin, localOrigin) {
		return 0, nil
	}
	if len(message.Batch) > 0 {
		total := 0
		for _, item := range message.Batch {
			sent, err := hub.DeliverEnvelope(ctx, brokerEnvelope(item))
			if err != nil {
				return total, err
			}
			total += sent
		}
		return total, nil
	}
	return hub.DeliverEnvelope(ctx, brokerEnvelope(message))
}

// DeliverEnvelope sends a Python-compatible websocket envelope to local clients.
func (hub *Hub) DeliverEnvelope(ctx context.Context, envelope map[string]any) (int, error) {
	if hub == nil {
		return 0, nil
	}
	envelope = normalizeEnvelope(envelope)
	data, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	targets := hub.targets(textValue(envelope["channel"]), textValue(envelope["topic"]))
	sent := 0
	for _, client := range targets {
		if client.Sender == nil {
			continue
		}
		if err := client.Sender.WriteText(ctx, data); err != nil {
			hub.Unregister(client)
			continue
		}
		sent++
	}
	if sent > 0 {
		hub.appendDeliveredEventLog(ctx, envelope)
	}
	return sent, nil
}

// ChannelStats returns current local connection counts for /stream/channels.
func (hub *Hub) ChannelStats(ctx context.Context) ([]streamchannels.ConnectionStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hub == nil {
		return nil, nil
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	stats := make([]streamchannels.ConnectionStat, 0, len(hub.clients))
	for channel, clients := range hub.clients {
		stats = append(stats, streamchannels.ConnectionStat{
			Channel:     channel,
			Connections: len(clients),
		})
	}
	return stats, nil
}

// ClientCount returns total local websocket clients across channels.
func (hub *Hub) ClientCount() int {
	if hub == nil {
		return 0
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	count := 0
	for _, clients := range hub.clients {
		count += len(clients)
	}
	return count
}

func (hub *Hub) targets(channel string, topic string) []*Client {
	channel = strings.TrimSpace(channel)
	topic = strings.TrimSpace(topic)
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	clients := hub.clients[channel]
	if len(clients) == 0 {
		return nil
	}
	targets := make([]*Client, 0, len(clients))
	for client := range clients {
		if len(client.Topics) > 0 && topic != "" {
			if _, ok := client.Topics[topic]; !ok {
				continue
			}
		}
		targets = append(targets, client)
	}
	return targets
}

func normalizeTopics(topics []string) map[string]struct{} {
	normalized := map[string]struct{}{}
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic != "" {
			normalized[topic] = struct{}{}
		}
	}
	return normalized
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

func brokerEnvelope(message BrokerMessage) map[string]any {
	if len(message.Envelope) > 0 {
		return cloneAnyMap(message.Envelope)
	}
	envelope, _ := realtime.BuildEnvelope(realtime.EnvelopeInput{
		Channel: message.Channel,
		Event:   message.Event,
		Topic:   message.Topic,
		Payload: message.Payload,
	})
	return envelope
}

func normalizeEnvelope(envelope map[string]any) map[string]any {
	normalized := cloneAnyMap(envelope)
	if _, ok := normalized["payload"].(map[string]any); !ok {
		normalized["payload"] = map[string]any{}
	}
	if strings.TrimSpace(textValue(normalized["consistency"])) == "" {
		normalized["consistency"] = "weak"
	}
	return normalized
}

func (hub *Hub) appendDeliveredEventLog(ctx context.Context, envelope map[string]any) {
	if hub == nil || hub.EventLog == nil {
		return
	}
	if strings.TrimSpace(textValue(envelope["consistency"])) != "strong" {
		return
	}
	scopeKey := textValue(envelope["scope_key"])
	cursor := int64Value(envelope["cursor"])
	if scopeKey == "" || cursor <= 0 {
		return
	}
	payload, _ := envelope["payload"].(map[string]any)
	_ = hub.EventLog.AppendEvent(ctx, realtime.EventRecord{
		ScopeKey:    scopeKey,
		Cursor:      cursor,
		Channel:     textValue(envelope["channel"]),
		Event:       textValue(envelope["event"]),
		Topic:       textValue(envelope["topic"]),
		Consistency: "strong",
		Payload:     clonePayload(payload),
	})
}

func cloneAnyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]any); ok {
			output[key] = cloneAnyMap(nested)
			continue
		}
		output[key] = value
	}
	return output
}

func sameOrigin(origin string, localOrigin string) bool {
	origin = strings.TrimSpace(origin)
	localOrigin = strings.TrimSpace(localOrigin)
	return origin != "" && localOrigin != "" && origin == localOrigin
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(stringFromJSON(typed))
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		return parseInt64Text(typed)
	default:
		return parseInt64Text(textValue(typed))
	}
}

func parseInt64Text(value string) int64 {
	var parsed int64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

func stringFromJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
