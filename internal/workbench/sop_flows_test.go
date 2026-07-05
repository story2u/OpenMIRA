package workbench

import (
	"context"
	"strings"
	"testing"

	"im-go/internal/auth"
)

// TestServiceSOPFlowsBuildsPayload keeps the legacy flows[] shape stable.
func TestServiceSOPFlowsBuildsPayload(t *testing.T) {
	store := fakeSOPFlowStore{flows: []SOPFlowRecord{
		{FlowID: " default ", FlowName: " 默认流程 ", ExecutionMode: "local_days", DayCount: 3, PlatformPullDriver: "conversation", PlatformTaskLimit: 20, PlatformDispatchQueue: "slow", Enabled: true, CreatedAt: "2026-06-28T09:00:00Z", UpdatedAt: "2026-06-29T09:00:00Z"},
		{FlowID: "", FlowName: "blank", Enabled: true},
	}}
	service := Service{SOPFlowStore: store}

	payload, err := service.SOPFlows(context.Background(), SOPFlowsRequest{})
	if err != nil {
		t.Fatalf("SOPFlows returned error: %v", err)
	}
	flows := payload["flows"].([]ProjectionRow)
	if len(flows) != 1 {
		t.Fatalf("len(flows) = %d; flows=%+v", len(flows), flows)
	}
	if rowText(flows[0], "flow_id") != "default" || rowText(flows[0], "flow_name") != "默认流程" || flows[0]["day_count"] != 3 {
		t.Fatalf("flow payload = %+v", flows[0])
	}
}

// TestServiceSOPFlowsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceSOPFlowsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).SOPFlows(context.Background(), SOPFlowsRequest{})
	if err != ErrSOPFlowStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrSOPFlowStoreUnavailable)
	}
}

// TestServiceUpsertSOPFlowNormalizesAndPublishes keeps Python flow write semantics.
func TestServiceUpsertSOPFlowNormalizesAndPublishes(t *testing.T) {
	platformTaskLimit := 0
	flowName := " 流程B "
	store := &fakeSOPFlowWriteStore{flow: SOPFlowRecord{FlowID: "flow-b", FlowName: "流程B", TargetAudience: "cs-2,cs-3", ExecutionMode: "platform_pull", DayCount: 2, PlatformPullDriver: "conversation", PlatformTaskLimit: 1, PlatformDispatchQueue: "fast", Enabled: true}}
	publisher := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	service := Service{
		SOPFlowStore:      fakeSOPFlowStore{flows: []SOPFlowRecord{{FlowID: "default", TargetAudience: "cs-1", ExecutionMode: "local_days"}}},
		SOPFlowWriteStore: store,
		SOPEvents:         publisher,
		AuditLogWriter:    audit,
	}

	payload, err := service.UpsertSOPFlow(context.Background(), NewSOPFlowUpsertRequest(SOPFlowUpsertBody{
		FlowID:               " flow-b ",
		FlowName:             &flowName,
		TargetAudience:       " cs-2，cs-3\ncs-2 ",
		ExecutionMode:        "platform_pull",
		DayCount:             2,
		PlatformPullDriver:   "bad-driver",
		PlatformTaskLimit:    &platformTaskLimit,
		ExecutionTimeWindows: []map[string]any{{"start": "09:00", "end": "18:00"}, {"start": "bad", "end": "19:00"}},
		Enabled:              true,
	}, auth.Session{AssigneeID: "admin-1", Role: "admin"}))
	if err != nil {
		t.Fatalf("UpsertSOPFlow returned error: %v", err)
	}
	if store.command.FlowID != "flow-b" || store.command.FlowName != "流程B" || store.command.TargetAudience != "cs-2,cs-3" || store.command.PlatformPullDriver != "conversation" || store.command.PlatformTaskLimit != 1 {
		t.Fatalf("command = %+v", store.command)
	}
	if store.command.ExecutionTimeWindows != `[{"end":"18:00","start":"09:00"}]` && store.command.ExecutionTimeWindows != `[{"start":"09:00","end":"18:00"}]` {
		t.Fatalf("execution windows = %q", store.command.ExecutionTimeWindows)
	}
	flow := payload["flow"].(ProjectionRow)
	if payload["success"] != true || rowText(flow, "flow_id") != "flow-b" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "sop.flow.updated" || publisher.events[0].topic != "sop.changed" || publisher.events[0].payload["flow_id"] != "flow-b" {
		t.Fatalf("events = %+v", publisher.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "config" || !strings.Contains(audit.entries[0].Detail, "flow-b") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

// TestServiceUpsertSOPFlowRejectsInvalidAudience preserves FastAPI 422 causes.
func TestServiceUpsertSOPFlowRejectsInvalidAudience(t *testing.T) {
	service := Service{SOPFlowStore: fakeSOPFlowStore{}, SOPFlowWriteStore: &fakeSOPFlowWriteStore{}}
	_, err := service.UpsertSOPFlow(context.Background(), NewSOPFlowUpsertRequest(SOPFlowUpsertBody{FlowID: "flow-b", Enabled: true}, auth.Session{}))
	var validation SOPConfigValidationError
	if err == nil || !strings.Contains(err.Error(), "启用规则集前请先选择消息端") || !asSOPValidation(err, &validation) {
		t.Fatalf("error = %v, want SOPConfigValidationError", err)
	}
}

// TestServiceUpsertSOPFlowRejectsOverlappingAudience keeps same-mode ownership exclusive.
func TestServiceUpsertSOPFlowRejectsOverlappingAudience(t *testing.T) {
	service := Service{
		SOPFlowStore: fakeSOPFlowStore{flows: []SOPFlowRecord{
			{FlowID: "flow-a", TargetAudience: "cs-1,cs-2", ExecutionMode: "local_days"},
		}},
		SOPFlowWriteStore: &fakeSOPFlowWriteStore{},
	}
	_, err := service.UpsertSOPFlow(context.Background(), NewSOPFlowUpsertRequest(SOPFlowUpsertBody{FlowID: "flow-b", TargetAudience: "cs-2", ExecutionMode: "local_days", Enabled: true}, auth.Session{}))
	if err == nil || !strings.Contains(err.Error(), "不能共用消息端") {
		t.Fatalf("error = %v, want overlap validation", err)
	}
}

// TestServiceDeleteSOPFlowRejectsDefaultAndPublishesDeleted keeps delete semantics.
func TestServiceDeleteSOPFlowRejectsDefaultAndPublishesDeleted(t *testing.T) {
	_, err := (Service{SOPFlowWriteStore: &fakeSOPFlowWriteStore{}}).DeleteSOPFlow(context.Background(), NewSOPFlowDeleteRequest("default", auth.Session{}))
	if err == nil || !strings.Contains(err.Error(), "default flow cannot be deleted") {
		t.Fatalf("default delete error = %v", err)
	}

	store := &fakeSOPFlowWriteStore{deleted: true}
	publisher := &fakeScriptEventPublisher{}
	service := Service{SOPFlowWriteStore: store, SOPEvents: publisher, AuditLogWriter: &fakeAuditWriter{}}
	payload, err := service.DeleteSOPFlow(context.Background(), NewSOPFlowDeleteRequest(" flow-b ", auth.Session{AssigneeID: "admin-1"}))
	if err != nil {
		t.Fatalf("DeleteSOPFlow returned error: %v", err)
	}
	if payload["success"] != true || store.flowID != "flow-b" {
		t.Fatalf("payload=%+v flowID=%q", payload, store.flowID)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "sop.flow.deleted" || publisher.events[0].payload["flow_id"] != "flow-b" {
		t.Fatalf("events = %+v", publisher.events)
	}
}

type fakeSOPFlowStore struct {
	flows []SOPFlowRecord
}

func (store fakeSOPFlowStore) ListSOPFlows(ctx context.Context) ([]SOPFlowRecord, error) {
	return store.flows, nil
}

type fakeSOPFlowWriteStore struct {
	command SOPFlowCommand
	flow    SOPFlowRecord
	flowID  string
	deleted bool
}

func (store *fakeSOPFlowWriteStore) UpsertSOPFlow(ctx context.Context, command SOPFlowCommand) (SOPFlowRecord, error) {
	store.command = command
	return store.flow, nil
}

func (store *fakeSOPFlowWriteStore) DeleteSOPFlow(ctx context.Context, flowID string) (bool, error) {
	store.flowID = flowID
	return store.deleted, nil
}

func asSOPValidation(err error, target *SOPConfigValidationError) bool {
	if typed, ok := err.(SOPConfigValidationError); ok {
		*target = typed
		return true
	}
	return false
}
