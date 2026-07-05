package workbench

import (
	"context"
	"net/url"
	"testing"
	"time"

	"im-go/internal/auth"
)

// TestServiceStatsOverviewBuildsPayload keeps Python's overview math and keys.
func TestServiceStatsOverviewBuildsPayload(t *testing.T) {
	store := &fakeStatsStore{overview: StatsOverviewRecord{
		ConversationsToday:       8,
		MessagesToday:            21,
		AIAutoReplyConversations: 3,
		OnlineDevices:            5,
	}}
	service := Service{
		StatsOverviewStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsOverview(context.Background(), StatsOverviewRequest{})
	if err != nil {
		t.Fatalf("StatsOverview returned error: %v", err)
	}
	if payload["conversations_today"] != 8 || payload["messages_today"] != 21 || payload["online_devices"] != 5 {
		t.Fatalf("payload counters = %+v", payload)
	}
	if payload["ai_reply_rate"] != 37.5 {
		t.Fatalf("ai_reply_rate = %v, want 37.5", payload["ai_reply_rate"])
	}
	if store.start.Format(time.RFC3339) != "2026-06-29T00:00:00+08:00" || store.end.Format(time.RFC3339) != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("unexpected day bounds start=%s end=%s", store.start.Format(time.RFC3339), store.end.Format(time.RFC3339))
	}
}

// TestServiceStatsOverviewFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsOverviewFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsOverview(context.Background(), StatsOverviewRequest{})
	if err != ErrStatsOverviewStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsOverviewStoreUnavailable)
	}
}

// TestServiceStatsTrendBuildsContinuousPayload keeps legacy day row fields.
func TestServiceStatsTrendBuildsContinuousPayload(t *testing.T) {
	store := &fakeStatsStore{trend: StatsTrendRecord{
		ConversationsByDay: map[string]int{"2026-06-27": 2, "2026-06-29": 8},
		MessagesByDay:      map[string]int{"2026-06-27": 5, "2026-06-29": 21},
	}}
	service := Service{
		StatsTrendStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsTrend(context.Background(), StatsTrendRequest{Days: 3})
	if err != nil {
		t.Fatalf("StatsTrend returned error: %v", err)
	}
	rows := payload["data"].([]ProjectionRow)
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d; rows=%+v", len(rows), rows)
	}
	if rowText(rows[0], "day") != "2026-06-27" || rowInt(rows[0], "value") != 2 || rowInt(rows[1], "conversations") != 0 || rowInt(rows[2], "messages") != 21 {
		t.Fatalf("trend rows = %+v", rows)
	}
	if store.trendStart.Format(time.RFC3339) != "2026-06-27T00:00:00+08:00" || store.trendEnd.Format(time.RFC3339) != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("unexpected trend bounds start=%s end=%s", store.trendStart.Format(time.RFC3339), store.trendEnd.Format(time.RFC3339))
	}
}

// TestNewStatsTrendRequestValidatesDays keeps FastAPI's query range boundary.
func TestNewStatsTrendRequestValidatesDays(t *testing.T) {
	request, err := NewStatsTrendRequest(nil, auth.Session{Role: "admin"})
	if err != nil || request.Days != 7 {
		t.Fatalf("default request=%+v err=%v", request, err)
	}
	for _, raw := range []string{"0", "91", "abc"} {
		values := url.Values{"days": []string{raw}}
		if _, err := NewStatsTrendRequest(values, auth.Session{Role: "admin"}); err != ErrInvalidStatsDays {
			t.Fatalf("days=%q error=%v, want %v", raw, err, ErrInvalidStatsDays)
		}
	}
}

// TestServiceStatsTrendFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsTrendFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsTrend(context.Background(), StatsTrendRequest{Days: 7})
	if err != ErrStatsTrendStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsTrendStoreUnavailable)
	}
}

// TestServiceStatsAgentsBuildsPayload keeps agent ranking response keys stable.
func TestServiceStatsAgentsBuildsPayload(t *testing.T) {
	store := &fakeStatsStore{agents: []StatsAgentRecord{
		{AssigneeID: "cs-001", AssigneeName: "消息端一", Conversations: 3, Messages: 8},
		{AssigneeID: "cs-002", Conversations: 1, Messages: 2},
	}}
	service := Service{StatsAgentsStore: store}

	payload, err := service.StatsAgents(context.Background(), StatsAgentsRequest{})
	if err != nil {
		t.Fatalf("StatsAgents returned error: %v", err)
	}
	rows := payload["agents"].([]ProjectionRow)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d; rows=%+v", len(rows), rows)
	}
	if rowText(rows[0], "assignee_id") != "cs-001" || rowInt(rows[0], "messages") != 8 || rowText(rows[0], "avg_response_time") != "-" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rowText(rows[1], "assignee_name") != "cs-002" {
		t.Fatalf("fallback assignee name row = %+v", rows[1])
	}
	if store.agentsLimit != 20 {
		t.Fatalf("agents limit = %d, want 20", store.agentsLimit)
	}
}

// TestServiceStatsAgentsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsAgentsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsAgents(context.Background(), StatsAgentsRequest{})
	if err != ErrStatsAgentsStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsAgentsStoreUnavailable)
	}
}

// TestServiceStatsAIReplyOverviewBuildsPayload keeps AI reply summary fields.
func TestServiceStatsAIReplyOverviewBuildsPayload(t *testing.T) {
	avgAI := 123.4
	avgTotal := 456.7
	store := &fakeStatsStore{aiReplyOverview: StatsAIReplyOverviewRecord{
		Attempts:            10,
		SuccessCount:        6,
		SentCount:           4,
		UnreplyableCount:    1,
		FailedCount:         2,
		SendFailedCount:     1,
		AvgAICallDurationMS: &avgAI,
		AvgTotalDurationMS:  &avgTotal,
	}}
	service := Service{
		StatsAIReplyStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsAIReplyOverview(context.Background(), StatsAIReplyOverviewRequest{})
	if err != nil {
		t.Fatalf("StatsAIReplyOverview returned error: %v", err)
	}
	if payload["date"] != "2026-06-29" || payload["attempts"] != 10 || payload["sent_count"] != 4 {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["avg_ai_call_duration_ms"] != 123.4 || payload["avg_total_duration_ms"] != 456.7 {
		t.Fatalf("avg payload = %+v", payload)
	}
	if store.aiReplyStart.Format(time.RFC3339) != "2026-06-29T00:00:00+08:00" || store.aiReplyEnd.Format(time.RFC3339) != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("unexpected ai reply bounds start=%s end=%s", store.aiReplyStart.Format(time.RFC3339), store.aiReplyEnd.Format(time.RFC3339))
	}
}

// TestStatsAIReplyOverviewUsesExplicitDate keeps the date query bound to Beijing days.
func TestStatsAIReplyOverviewUsesExplicitDate(t *testing.T) {
	store := &fakeStatsStore{}
	target, err := parseStatsDate("2026-06-28")
	if err != nil {
		t.Fatalf("parseStatsDate returned error: %v", err)
	}
	service := Service{
		StatsAIReplyStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsAIReplyOverview(context.Background(), StatsAIReplyOverviewRequest{TargetDay: target, HasDate: true})
	if err != nil {
		t.Fatalf("StatsAIReplyOverview returned error: %v", err)
	}
	if payload["date"] != "2026-06-28" || payload["avg_ai_call_duration_ms"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	if store.aiReplyStart.Format(time.RFC3339) != "2026-06-28T00:00:00+08:00" {
		t.Fatalf("unexpected ai reply explicit start=%s", store.aiReplyStart.Format(time.RFC3339))
	}
}

// TestNewStatsAIReplyOverviewRequestValidatesDate keeps FastAPI date behavior.
func TestNewStatsAIReplyOverviewRequestValidatesDate(t *testing.T) {
	request, err := NewStatsAIReplyOverviewRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil || request.HasDate {
		t.Fatalf("default request=%+v err=%v", request, err)
	}
	valid, err := NewStatsAIReplyOverviewRequest(url.Values{"date": []string{"2026-06-29"}}, auth.Session{Role: "admin"})
	if err != nil || !valid.HasDate || valid.TargetDay.Format("2006-01-02") != "2026-06-29" {
		t.Fatalf("valid request=%+v err=%v", valid, err)
	}
	for _, raw := range []string{"2026-6-29", "2026-13-01", "not-date"} {
		if _, err := NewStatsAIReplyOverviewRequest(url.Values{"date": []string{raw}}, auth.Session{Role: "admin"}); err != ErrInvalidStatsDate {
			t.Fatalf("date=%q error=%v, want %v", raw, err, ErrInvalidStatsDate)
		}
	}
}

// TestServiceStatsAIReplyOverviewFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsAIReplyOverviewFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsAIReplyOverview(context.Background(), StatsAIReplyOverviewRequest{})
	if err != ErrStatsAIReplyOverviewStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsAIReplyOverviewStoreUnavailable)
	}
}

// TestServiceStatsAIReplyTrendBuildsPayload keeps daily AI summary fields.
func TestServiceStatsAIReplyTrendBuildsPayload(t *testing.T) {
	avgAI := 111.1
	store := &fakeStatsStore{aiReplyTrend: StatsAIReplyTrendRecord{
		ByDay: map[string]StatsAIReplyOverviewRecord{
			"2026-06-27": {Attempts: 2, SentCount: 1, AvgAICallDurationMS: &avgAI},
			"2026-06-29": {Attempts: 10, SuccessCount: 6, SentCount: 4, FailedCount: 2},
		},
	}}
	service := Service{
		StatsAITrendStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsAIReplyTrend(context.Background(), StatsAIReplyTrendRequest{Days: 3})
	if err != nil {
		t.Fatalf("StatsAIReplyTrend returned error: %v", err)
	}
	rows := payload["data"].([]ProjectionRow)
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d; rows=%+v", len(rows), rows)
	}
	if rowText(rows[0], "day") != "2026-06-27" || rowInt(rows[0], "attempts") != 2 || rows[0]["avg_ai_call_duration_ms"] != 111.1 {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rowInt(rows[1], "attempts") != 0 || rows[1]["avg_ai_call_duration_ms"] != nil {
		t.Fatalf("missing day row = %+v", rows[1])
	}
	if rowInt(rows[2], "sent_count") != 4 || rowInt(rows[2], "failed_count") != 2 {
		t.Fatalf("last row = %+v", rows[2])
	}
	if store.aiTrendStart.Format(time.RFC3339) != "2026-06-27T00:00:00+08:00" || store.aiTrendEnd.Format(time.RFC3339) != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("unexpected ai trend bounds start=%s end=%s", store.aiTrendStart.Format(time.RFC3339), store.aiTrendEnd.Format(time.RFC3339))
	}
}

// TestNewStatsAIReplyTrendRequestValidatesDays keeps query rules shared.
func TestNewStatsAIReplyTrendRequestValidatesDays(t *testing.T) {
	request, err := NewStatsAIReplyTrendRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil || request.Days != 7 {
		t.Fatalf("default request=%+v err=%v", request, err)
	}
	if _, err := NewStatsAIReplyTrendRequest(url.Values{"days": []string{"91"}}, auth.Session{Role: "admin"}); err != ErrInvalidStatsDays {
		t.Fatalf("error=%v, want %v", err, ErrInvalidStatsDays)
	}
}

// TestServiceStatsAIReplyTrendFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsAIReplyTrendFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsAIReplyTrend(context.Background(), StatsAIReplyTrendRequest{Days: 7})
	if err != ErrStatsAIReplyTrendStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsAIReplyTrendStoreUnavailable)
	}
}

// TestServiceStatsAIReplyBreakdownBuildsPayload keeps Python's date nil behavior.
func TestServiceStatsAIReplyBreakdownBuildsPayload(t *testing.T) {
	store := &fakeStatsStore{aiReplyBreakdown: []StatsAIReplyBreakdownItem{
		{FailureType: "device_offline", Count: 3},
		{FailureType: " llm_timeout ", Count: 2},
	}}
	service := Service{
		StatsBreakdownStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsAIReplyBreakdown(context.Background(), StatsAIReplyBreakdownRequest{})
	if err != nil {
		t.Fatalf("StatsAIReplyBreakdown returned error: %v", err)
	}
	if payload["date"] != nil {
		t.Fatalf("date = %v, want nil", payload["date"])
	}
	items := payload["items"].([]ProjectionRow)
	if len(items) != 2 || rowText(items[0], "failure_type") != "device_offline" || rowInt(items[1], "count") != 2 {
		t.Fatalf("items = %+v", items)
	}
	if store.breakdownStart.Format(time.RFC3339) != "2026-06-29T00:00:00+08:00" || store.breakdownEnd.Format(time.RFC3339) != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("unexpected breakdown bounds start=%s end=%s", store.breakdownStart.Format(time.RFC3339), store.breakdownEnd.Format(time.RFC3339))
	}
}

// TestStatsAIReplyBreakdownUsesExplicitDate returns the queried date string.
func TestStatsAIReplyBreakdownUsesExplicitDate(t *testing.T) {
	store := &fakeStatsStore{}
	target, err := parseStatsDate("2026-06-28")
	if err != nil {
		t.Fatalf("parseStatsDate returned error: %v", err)
	}
	service := Service{
		StatsBreakdownStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 45, 0, 0, time.UTC)
		},
	}

	payload, err := service.StatsAIReplyBreakdown(context.Background(), StatsAIReplyBreakdownRequest{TargetDay: target, HasDate: true})
	if err != nil {
		t.Fatalf("StatsAIReplyBreakdown returned error: %v", err)
	}
	if payload["date"] != "2026-06-28" || store.breakdownStart.Format(time.RFC3339) != "2026-06-28T00:00:00+08:00" {
		t.Fatalf("payload=%+v start=%s", payload, store.breakdownStart.Format(time.RFC3339))
	}
}

// TestNewStatsAIReplyBreakdownRequestValidatesDate keeps date errors shared.
func TestNewStatsAIReplyBreakdownRequestValidatesDate(t *testing.T) {
	request, err := NewStatsAIReplyBreakdownRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil || request.HasDate {
		t.Fatalf("default request=%+v err=%v", request, err)
	}
	if _, err := NewStatsAIReplyBreakdownRequest(url.Values{"date": []string{"2026-02-30"}}, auth.Session{Role: "admin"}); err != ErrInvalidStatsDate {
		t.Fatalf("invalid date error=%v, want %v", err, ErrInvalidStatsDate)
	}
}

// TestServiceStatsAIReplyBreakdownFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceStatsAIReplyBreakdownFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).StatsAIReplyBreakdown(context.Background(), StatsAIReplyBreakdownRequest{})
	if err != ErrStatsAIReplyBreakdownStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrStatsAIReplyBreakdownStoreUnavailable)
	}
}

// TestStatsAIReplyRateUsesLegacyZeroConversationDenominator mirrors Python max(..., 1).
func TestStatsAIReplyRateUsesLegacyZeroConversationDenominator(t *testing.T) {
	if rate := statsAIReplyRate(0, 0); rate != 0 {
		t.Fatalf("rate = %v, want 0", rate)
	}
	if rate := statsAIReplyRate(1, 3); rate != 33.3 {
		t.Fatalf("rate = %v, want 33.3", rate)
	}
}

// fakeStatsStore captures the day windows passed by the service.
type fakeStatsStore struct {
	overview         StatsOverviewRecord
	trend            StatsTrendRecord
	agents           []StatsAgentRecord
	aiReplyOverview  StatsAIReplyOverviewRecord
	aiReplyTrend     StatsAIReplyTrendRecord
	aiReplyBreakdown []StatsAIReplyBreakdownItem
	start            time.Time
	end              time.Time
	trendStart       time.Time
	trendEnd         time.Time
	agentsLimit      int
	aiReplyStart     time.Time
	aiReplyEnd       time.Time
	aiTrendStart     time.Time
	aiTrendEnd       time.Time
	breakdownStart   time.Time
	breakdownEnd     time.Time
}

// GetStatsOverview returns static counters for service tests.
func (store *fakeStatsStore) GetStatsOverview(ctx context.Context, start time.Time, end time.Time) (StatsOverviewRecord, error) {
	store.start = start
	store.end = end
	return store.overview, nil
}

// GetStatsTrend returns static daily counters for service tests.
func (store *fakeStatsStore) GetStatsTrend(ctx context.Context, start time.Time, end time.Time) (StatsTrendRecord, error) {
	store.trendStart = start
	store.trendEnd = end
	return store.trend, nil
}

// GetStatsAgents returns static agent rows for service tests.
func (store *fakeStatsStore) GetStatsAgents(ctx context.Context, limit int) ([]StatsAgentRecord, error) {
	store.agentsLimit = limit
	return store.agents, nil
}

// GetStatsAIReplyOverview returns static AI reply counters for service tests.
func (store *fakeStatsStore) GetStatsAIReplyOverview(ctx context.Context, start time.Time, end time.Time) (StatsAIReplyOverviewRecord, error) {
	store.aiReplyStart = start
	store.aiReplyEnd = end
	return store.aiReplyOverview, nil
}

// GetStatsAIReplyTrend returns static AI reply daily counters for service tests.
func (store *fakeStatsStore) GetStatsAIReplyTrend(ctx context.Context, start time.Time, end time.Time) (StatsAIReplyTrendRecord, error) {
	store.aiTrendStart = start
	store.aiTrendEnd = end
	return store.aiReplyTrend, nil
}

// GetStatsAIReplyBreakdown returns static AI reply failure buckets for service tests.
func (store *fakeStatsStore) GetStatsAIReplyBreakdown(ctx context.Context, start time.Time, end time.Time) ([]StatsAIReplyBreakdownItem, error) {
	store.breakdownStart = start
	store.breakdownEnd = end
	return store.aiReplyBreakdown, nil
}
