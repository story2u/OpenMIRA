package integrationhub

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	mu       sync.RWMutex
	snapshot Snapshot
	now      func() time.Time
}

func NewStore(snapshot Snapshot) *Store {
	return &Store{snapshot: snapshot, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now != nil {
		s.now = now
	}
}

func (s *Store) Overview() (OverviewStats, []Channel, []RecentIncident, []TrafficPoint) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.overviewLocked(), cloneSlice(s.snapshot.Channels), cloneSlice(s.snapshot.Incidents), cloneSlice(s.snapshot.TrafficSeries)
}

func (s *Store) Channels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSlice(s.snapshot.Channels)
}

func (s *Store) TestChannel(id string) (Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.snapshot.Channels {
		if s.snapshot.Channels[i].ID == id {
			s.snapshot.Channels[i].LastSyncAt = s.now()
			s.appendAuditLocked("System", ActorSystem, "Tested channel connection", id, &s.snapshot.Channels[i].Kind, AuditSuccess, nil)
			return s.snapshot.Channels[i], nil
		}
	}
	return Channel{}, ErrNotFound
}

func (s *Store) SetChannelStatus(id string, status ChannelStatus) (Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.snapshot.Channels {
		if s.snapshot.Channels[i].ID == id {
			s.snapshot.Channels[i].Status = status
			s.snapshot.Channels[i].LastSyncAt = s.now()
			action := fmt.Sprintf("Set channel status to %s", status)
			s.appendAuditLocked("System", ActorSystem, action, id, &s.snapshot.Channels[i].Kind, AuditSuccess, nil)
			return s.snapshot.Channels[i], nil
		}
	}
	return Channel{}, ErrNotFound
}

type MessageEventFilter struct {
	Channel   string
	Status    string
	EventType string
	TraceID   string
}

func (s *Store) MessageFlow(filter MessageEventFilter) ([]PipelineStageStats, []MessageEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]MessageEvent, 0, len(s.snapshot.MessageEvents))
	for _, event := range s.snapshot.MessageEvents {
		if filter.Channel != "" && filter.Channel != "all" && string(event.Channel) != filter.Channel {
			continue
		}
		if filter.Status != "" && filter.Status != "all" && string(event.Status) != filter.Status {
			continue
		}
		if filter.EventType != "" && filter.EventType != "all" && event.EventType != filter.EventType {
			continue
		}
		if filter.TraceID != "" && !strings.Contains(strings.ToLower(event.TraceID), strings.ToLower(filter.TraceID)) {
			continue
		}
		events = append(events, event)
	}
	return cloneSlice(s.snapshot.PipelineStats), events
}

func (s *Store) Conversations(channel, q string) ([]Conversation, []ConversationMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversations := make([]Conversation, 0, len(s.snapshot.Conversations))
	for _, conversation := range s.snapshot.Conversations {
		if channel != "" && channel != "all" && string(conversation.Channel) != channel {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(conversation.ContactName+" "+conversation.ContactHandle), strings.ToLower(q)) {
			continue
		}
		conversations = append(conversations, conversation)
	}
	return conversations, cloneSlice(s.snapshot.Messages)
}

func (s *Store) ConversationMessages(conversationID string) ([]ConversationMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasConversationLocked(conversationID) {
		return nil, ErrNotFound
	}
	messages := make([]ConversationMessage, 0)
	for _, message := range s.snapshot.Messages {
		if message.ConversationID == conversationID {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

type SendMessageInput struct {
	Content string `json:"content"`
	Sender  string `json:"sender"`
}

func (s *Store) SendMessage(conversationID string, input SendMessageInput) (OutboxItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var conversation *Conversation
	for i := range s.snapshot.Conversations {
		if s.snapshot.Conversations[i].ID == conversationID {
			conversation = &s.snapshot.Conversations[i]
			break
		}
	}
	if conversation == nil {
		return OutboxItem{}, ErrNotFound
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return OutboxItem{}, errors.New("content is required")
	}
	sender := strings.TrimSpace(input.Sender)
	if sender == "" {
		sender = "Operator"
	}
	now := s.now()
	messageID := fmt.Sprintf("msg_%d", len(s.snapshot.Messages)+1)
	s.snapshot.Messages = append(s.snapshot.Messages, ConversationMessage{
		ID: messageID, ConversationID: conversationID, Channel: conversation.Channel,
		Direction: DirectionOutbound, Author: sender, Content: content, Time: now,
	})
	outbox := OutboxItem{
		ID: fmt.Sprintf("out_%d", len(s.snapshot.OutboxItems)+1), CreatedAt: now,
		Channel: conversation.Channel, ConversationID: conversationID, ConversationLabel: conversation.ContactName,
		MessageType: "Text", Sender: sender, DeliveryMethod: SendAPI, Status: OutboxPending,
	}
	s.snapshot.OutboxItems = append([]OutboxItem{outbox}, s.snapshot.OutboxItems...)
	s.snapshot.MessageEvents = append([]MessageEvent{{
		ID: fmt.Sprintf("evt_%d", 1000+len(s.snapshot.MessageEvents)), Time: now,
		Channel: conversation.Channel, Direction: DirectionOutbound, ConversationID: conversationID,
		ConversationLabel: conversation.ContactName, EventType: "message.queued", Status: EventPending,
		LatencyMs: 0, TraceID: fmt.Sprintf("trace-%06x", len(s.snapshot.MessageEvents)*4099),
	}}, s.snapshot.MessageEvents...)
	s.appendAuditLocked(sender, ActorUser, "Queued outbound message", outbox.ID, &conversation.Channel, AuditSuccess, nil)
	return outbox, nil
}

func (s *Store) AIPolicies() []AIPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSlice(s.snapshot.AIPolicies)
}

func (s *Store) SetAIPolicyEnabled(id string, enabled bool) (AIPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.snapshot.AIPolicies {
		if s.snapshot.AIPolicies[i].ID == id {
			s.snapshot.AIPolicies[i].Enabled = enabled
			s.appendAuditLocked("System", ActorSystem, "Updated AI policy", id, nil, AuditSuccess, nil)
			return s.snapshot.AIPolicies[i], nil
		}
	}
	return AIPolicy{}, ErrNotFound
}

func (s *Store) SOPWorkflows() []SOPWorkflow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSlice(s.snapshot.SOPWorkflows)
}

func (s *Store) Outbox(status string) []OutboxItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]OutboxItem, 0, len(s.snapshot.OutboxItems))
	for _, item := range s.snapshot.OutboxItems {
		if status != "" && status != "all" && string(item.Status) != status {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (s *Store) SetOutboxStatus(id string, status OutboxStatus) (OutboxItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.snapshot.OutboxItems {
		if s.snapshot.OutboxItems[i].ID == id {
			item := &s.snapshot.OutboxItems[i]
			if status == OutboxCanceled {
				removed := *item
				s.snapshot.OutboxItems = append(s.snapshot.OutboxItems[:i], s.snapshot.OutboxItems[i+1:]...)
				s.appendAuditLocked("System", ActorSystem, "Canceled outbox message", id, &removed.Channel, AuditSuccess, nil)
				return removed, nil
			}
			if status == OutboxSending {
				item.RetryCount++
				item.LastError = nil
			}
			item.Status = status
			s.appendAuditLocked("System", ActorSystem, fmt.Sprintf("Set outbox status to %s", status), id, &item.Channel, AuditSuccess, nil)
			return *item, nil
		}
	}
	return OutboxItem{}, ErrNotFound
}

func (s *Store) Observability() ([]Channel, []MessageEvent, []TrafficPoint, OverviewStats) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSlice(s.snapshot.Channels), cloneSlice(s.snapshot.MessageEvents), cloneSlice(s.snapshot.TrafficSeries), s.overviewLocked()
}

func (s *Store) AuditLog() []AuditLogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSlice(s.snapshot.AuditLog)
}

func (s *Store) Settings() PlatformSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot.Settings
}

func (s *Store) UpdateSettings(settings PlatformSettings) PlatformSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if settings.Environment != "" {
		s.snapshot.Settings.Environment = settings.Environment
	}
	if settings.Region != "" {
		s.snapshot.Settings.Region = settings.Region
	}
	if settings.RetentionDays > 0 {
		s.snapshot.Settings.RetentionDays = settings.RetentionDays
	}
	if settings.WebhookURL != "" {
		s.snapshot.Settings.WebhookURL = settings.WebhookURL
	}
	if settings.EnabledProviders != nil {
		s.snapshot.Settings.EnabledProviders = settings.EnabledProviders
	}
	s.appendAuditLocked("System", ActorSystem, "Updated platform settings", "settings", nil, AuditSuccess, nil)
	return s.snapshot.Settings
}

func (s *Store) overviewLocked() OverviewStats {
	activeChannels := 0
	messages := 0
	pending := 0
	for _, channel := range s.snapshot.Channels {
		if channel.Status != ChannelDisabled {
			activeChannels++
		}
		messages += channel.MessagesToday
	}
	for _, item := range s.snapshot.OutboxItems {
		if item.Status == OutboxPending || item.Status == OutboxRequiresApproval {
			pending++
		}
	}
	return OverviewStats{
		ActiveChannels: activeChannels, TotalChannels: len(s.snapshot.Channels),
		MessagesIngestedToday: messages, AIActionsToday: 6420, OutboxPending: pending,
		ErrorRate: 0.021, P95LatencyMs: 1180,
	}
}

func (s *Store) appendAuditLocked(actor string, actorType AuditActorType, action, target string, channel *ChannelKind, result AuditResult, ip *string) {
	entry := AuditLogEntry{
		ID: fmt.Sprintf("aud_%d", len(s.snapshot.AuditLog)+1), Time: s.now(),
		Actor: actor, ActorType: actorType, Action: action, Target: target, Channel: channel, Result: result, IP: ip,
	}
	s.snapshot.AuditLog = append([]AuditLogEntry{entry}, s.snapshot.AuditLog...)
}

func (s *Store) hasConversationLocked(id string) bool {
	return slices.ContainsFunc(s.snapshot.Conversations, func(conversation Conversation) bool {
		return conversation.ID == id
	})
}

func cloneSlice[T any](in []T) []T {
	return append([]T(nil), in...)
}
