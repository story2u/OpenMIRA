package friendadded

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"wework-go/internal/outbox"
	"wework-go/internal/readmodelcache"
	"wework-go/internal/workbench"
)

func TestDecodeRequestJSONPreservesDefaultsAndOptionalNulls(t *testing.T) {
	request, err := DecodeRequestJSON([]byte(`{"device_id":"dev-1","friend_name":"Qiu","timestamp":"2026-03-08T09:12:34+08:00","trace_id":"trace-1","tenant_id":null}`))
	if err != nil {
		t.Fatalf("DecodeRequestJSON returned error: %v", err)
	}
	if request.DeviceID != "dev-1" || request.FriendName != "Qiu" || request.FriendID != "" || request.Source != "" || request.TraceID != "trace-1" {
		t.Fatalf("request = %+v", request)
	}
	if request.TenantID != nil {
		t.Fatalf("TenantID = %#v, want nil", request.TenantID)
	}
	if got := request.Payload()["timestamp"]; got != "2026-03-08T01:12:34Z" {
		t.Fatalf("payload timestamp = %#v", got)
	}
}

func TestDecodeRequestJSONRejectsMissingRequiredField(t *testing.T) {
	_, err := DecodeRequestJSON([]byte(`{"device_id":"dev-1","timestamp":"2026-03-08T09:12:34+08:00","trace_id":"trace-1"}`))
	if !errors.Is(err, ErrInvalidPayload) || !strings.Contains(err.Error(), "friend_name is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceIngestStoresEventAndPublishesFriendAdded(t *testing.T) {
	fixedNow := time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC)
	store := &recordingStore{inserted: true}
	events := &recordingPublisher{}
	invalidator := &recordingInvalidator{}
	service := Service{Store: store, Events: events, ReadModelInvalidator: invalidator, Now: func() time.Time { return fixedNow }}
	tenantID := " tenant-1 "
	accountID := "acct-1"
	weworkUserID := " wxuser "

	response, err := service.Ingest(context.Background(), Request{
		DeviceID:     "dev-1",
		FriendName:   "Qiu",
		FriendID:     "ext-1",
		Source:       "manual",
		Timestamp:    time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TraceID:      "trace-1",
		TenantID:     &tenantID,
		AccountID:    &accountID,
		WeWorkUserID: &weworkUserID,
	})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.Accepted || response.Deduplicated || response.TraceID != "trace-1" || response.AutoGreetQueued {
		t.Fatalf("response = %+v", response)
	}
	if store.event.TraceID != "trace-1" || store.event.CreatedAt != fixedNow || store.event.Timestamp.Format(time.RFC3339) != "2026-03-08T01:12:34Z" {
		t.Fatalf("stored event = %+v", store.event)
	}
	if store.touchCalls != 1 || store.touch.DeviceID != "dev-1" || store.touch.FriendID != "ext-1" || store.touch.FriendName != "Qiu" {
		t.Fatalf("conversation touch calls=%d touch=%+v", store.touchCalls, store.touch)
	}
	if store.touch.TenantID != "tenant-1" || store.touch.AccountID != "acct-1" || store.touch.WeWorkUserID != "wxuser" {
		t.Fatalf("conversation touch scope = %+v", store.touch)
	}
	if events.channel != "conversations" || events.event != "friend.added" || events.topic != "friend.added" {
		t.Fatalf("publish target channel=%q event=%q topic=%q", events.channel, events.event, events.topic)
	}
	if events.payload["account_id"] != "acct-1" || events.payload["tenant_id"] != " tenant-1 " {
		t.Fatalf("payload = %#v", events.payload)
	}
	if !reflect.DeepEqual(invalidator.namespaces, readmodelcache.AllNamespaces()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceIngestReportsDuplicate(t *testing.T) {
	store := &recordingStore{inserted: false}
	invalidator := &recordingInvalidator{}
	service := Service{Store: store, ReadModelInvalidator: invalidator}

	response, err := service.Ingest(context.Background(), Request{TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.Deduplicated || !response.Accepted {
		t.Fatalf("response = %+v", response)
	}
	if store.touchCalls != 0 {
		t.Fatalf("touchCalls = %d, want 0", store.touchCalls)
	}
	if len(invalidator.namespaces) != 0 {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceIngestSwallowsReadModelInvalidationErrors(t *testing.T) {
	invalidator := &recordingInvalidator{err: errors.New("redis down")}
	service := Service{Store: &recordingStore{inserted: true}, ReadModelInvalidator: invalidator}

	response, err := service.Ingest(context.Background(), Request{TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.Accepted || response.Deduplicated {
		t.Fatalf("response = %+v", response)
	}
	if !reflect.DeepEqual(invalidator.namespaces, readmodelcache.AllNamespaces()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceIngestPropagatesPublisherError(t *testing.T) {
	service := Service{Store: &recordingStore{inserted: true}, Events: &recordingPublisher{err: errors.New("redis down")}}

	_, err := service.Ingest(context.Background(), Request{TraceID: "trace-1"})
	if err == nil || !strings.Contains(err.Error(), "redis down") {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceIngestIgnoresConversationTouchError(t *testing.T) {
	store := &recordingStore{inserted: true, touchErr: errors.New("touch down")}
	service := Service{Store: store}

	response, err := service.Ingest(context.Background(), Request{TraceID: "trace-1", Timestamp: time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.Accepted || response.Deduplicated {
		t.Fatalf("response = %+v", response)
	}
	if store.touchCalls != 1 {
		t.Fatalf("touchCalls = %d, want 1", store.touchCalls)
	}
}

func TestServiceIngestQueuesAutoGreetWhenPolicyMatches(t *testing.T) {
	store := &recordingStore{inserted: true}
	outboxSink := &recordingOutbox{}
	service := Service{
		Store:       store,
		Accounts:    accountStore{{AccountID: "acct-1", DeviceID: "dev-1", WeWorkUserID: "wxuser", EnterpriseID: "tenant-account"}},
		SOPFlows:    flowStore{{FlowID: "default", ExecutionMode: "local_days", Enabled: true}},
		SOPPolicies: policyStore{{PolicyID: "policy-1", FlowID: "default", DayStage: "day1", TriggerEvent: "friend_added", Enabled: true, Priority: 10, ReplyMode: "sop_only", UpdatedAt: "2026-03-08T01:00:00Z"}},
		Outbox:      outboxSink,
	}
	tenantID := "tenant-1"

	response, err := service.Ingest(context.Background(), Request{
		DeviceID:   "dev-1",
		FriendName: "Qiu",
		FriendID:   "ext-1",
		Source:     "manual-source",
		Timestamp:  time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TraceID:    "trace-1",
		TenantID:   &tenantID,
	})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.AutoGreetQueued {
		t.Fatalf("response = %+v", response)
	}
	if len(outboxSink.events) != 1 {
		t.Fatalf("outbox events = %#v", outboxSink.events)
	}
	event := outboxSink.events[0]
	if event.EventID != "trace-1:auto-reply" || event.EventType != "conversation.auto_reply.requested" || event.AggregateID != "ww:wxuser:ext-1" {
		t.Fatalf("event = %+v", event)
	}
	if event.TenantID != "tenant-1" || event.PartitionKey != "dev-1:ext-1" || event.AvailableAt != event.OccurredAt.Add(time.Millisecond) {
		t.Fatalf("event timing/scope = %+v", event)
	}
	if event.Payload["trigger_event"] != "friend_added" || event.Payload["content"] != "manual-source" || event.Payload["flow_id"] != "default" || event.Payload["policy_id"] != "policy-1" {
		t.Fatalf("payload = %#v", event.Payload)
	}
	if event.Payload["account_id"] != "acct-1" || event.Payload["wework_user_id"] != "wxuser" || event.Payload["first_message_at"] != "2026-03-08T01:12:34Z" {
		t.Fatalf("payload scope/time = %#v", event.Payload)
	}
}

func TestServiceIngestUsesExplicitAutoGreetContent(t *testing.T) {
	store := &recordingStore{inserted: true}
	outboxSink := &recordingOutbox{}
	service := Service{
		Store:       store,
		Accounts:    accountStore{{AccountID: "acct-1", DeviceID: "dev-1", WeWorkUserID: "wxuser"}},
		SOPFlows:    flowStore{{FlowID: "default", ExecutionMode: "local_days", Enabled: true}},
		SOPPolicies: policyStore{{PolicyID: "policy-1", FlowID: "default", DayStage: "day1", TriggerEvent: "friend_added", Enabled: true}},
		Outbox:      outboxSink,
	}

	response, err := service.Ingest(context.Background(), Request{
		DeviceID:         "dev-1",
		FriendName:       "Qiu",
		FriendID:         "ext-1",
		Source:           "wework_customer_relation_callback",
		AutoGreetContent: "首次加微",
		Timestamp:        time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TraceID:          "trace-1",
	})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.AutoGreetQueued || store.event.Source != "wework_customer_relation_callback" {
		t.Fatalf("response=%+v stored=%+v", response, store.event)
	}
	if len(outboxSink.events) != 1 || outboxSink.events[0].Payload["content"] != "首次加微" {
		t.Fatalf("outbox events = %#v", outboxSink.events)
	}
}

func TestServiceIngestQueuesAutoGreetForPlatformPullAudienceFlow(t *testing.T) {
	outboxSink := &recordingOutbox{}
	service := Service{
		Store:       &recordingStore{inserted: true},
		Accounts:    accountStore{{AccountID: "acct-1", DeviceID: "dev-1", AssigneeID: "cs-1", WeWorkUserID: "wxuser"}},
		SOPFlows:    flowStore{{FlowID: "default", ExecutionMode: "local_days", Enabled: true}, {FlowID: "flow-b", TargetAudience: "cs-1", ExecutionMode: "platform_pull", Enabled: true}},
		SOPPolicies: policyStore{},
		Outbox:      outboxSink,
	}

	response, err := service.Ingest(context.Background(), Request{
		DeviceID:   "dev-1",
		FriendName: "Qiu",
		Timestamp:  time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TraceID:    "trace-1",
	})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !response.AutoGreetQueued || len(outboxSink.events) != 1 {
		t.Fatalf("response=%+v events=%#v", response, outboxSink.events)
	}
	payload := outboxSink.events[0].Payload
	if payload["flow_id"] != "flow-b" || payload["policy_id"] != "" || payload["reply_mode"] != "platform_pull" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["content"] != defaultFriendAddedContent || payload["conversation_id"] != "ww:wxuser:qiu" {
		t.Fatalf("payload content/conversation = %#v", payload)
	}
}

func TestServiceIngestReturnsFalseWhenAutoGreetOutboxFails(t *testing.T) {
	service := Service{
		Store:       &recordingStore{inserted: true},
		Accounts:    accountStore{{DeviceID: "dev-1"}},
		SOPFlows:    flowStore{{FlowID: "default", ExecutionMode: "local_days", Enabled: true}},
		SOPPolicies: policyStore{{PolicyID: "policy-1", FlowID: "default", DayStage: "day1", TriggerEvent: "friend_added", Enabled: true}},
		Outbox:      &recordingOutbox{err: errors.New("outbox down")},
	}

	response, err := service.Ingest(context.Background(), Request{
		DeviceID:   "dev-1",
		FriendName: "Qiu",
		FriendID:   "ext-1",
		Timestamp:  time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TraceID:    "trace-1",
	})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if response.AutoGreetQueued {
		t.Fatalf("response = %+v, want queued false", response)
	}
}

type recordingStore struct {
	inserted   bool
	event      Event
	err        error
	touch      ConversationTouch
	touchCalls int
	touchErr   error
}

func (store *recordingStore) AddFriendEvent(_ context.Context, event Event) (bool, error) {
	store.event = event
	return store.inserted, store.err
}

func (store *recordingStore) TouchConversationFirstMessageAt(_ context.Context, touch ConversationTouch) error {
	store.touch = touch
	store.touchCalls++
	return store.touchErr
}

type recordingPublisher struct {
	channel string
	event   string
	topic   string
	payload map[string]any
	err     error
}

func (publisher *recordingPublisher) Publish(_ context.Context, channel string, event string, topic string, payload map[string]any) error {
	publisher.channel = channel
	publisher.event = event
	publisher.topic = topic
	publisher.payload = payload
	return publisher.err
}

type recordingInvalidator struct {
	namespaces []string
	err        error
}

func (invalidator *recordingInvalidator) InvalidateNamespaces(_ context.Context, namespaces ...string) error {
	invalidator.namespaces = append([]string{}, namespaces...)
	return invalidator.err
}

type accountStore []workbench.AccountRecord

func (store accountStore) ListAccounts(context.Context) ([]workbench.AccountRecord, error) {
	return append([]workbench.AccountRecord(nil), store...), nil
}

type flowStore []workbench.SOPFlowRecord

func (store flowStore) ListSOPFlows(context.Context) ([]workbench.SOPFlowRecord, error) {
	return append([]workbench.SOPFlowRecord(nil), store...), nil
}

type policyStore []workbench.SOPPolicyRecord

func (store policyStore) ListSOPPolicies(context.Context) ([]workbench.SOPPolicyRecord, error) {
	return append([]workbench.SOPPolicyRecord(nil), store...), nil
}

type recordingOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (sink *recordingOutbox) EnqueueMany(_ context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append(sink.events, events...)
	if sink.err != nil {
		return nil, sink.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.Record{EventEnvelope: event, Status: "pending"})
	}
	return records, nil
}
