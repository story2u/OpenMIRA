// Admin stats read models expose management dashboard aggregates. Each handler
// stays opt-in so read contracts can move behind harness fixtures one endpoint
// at a time while Python continues to own the default route surface.
package workbench

import (
	"context"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrStatsOverviewStoreUnavailable means dashboard counters cannot be loaded.
	ErrStatsOverviewStoreUnavailable = errors.New("workbench stats overview store is unavailable")
	// ErrStatsTrendStoreUnavailable means dashboard trend counters cannot be loaded.
	ErrStatsTrendStoreUnavailable = errors.New("workbench stats trend store is unavailable")
	// ErrStatsAgentsStoreUnavailable means agent ranking counters cannot be loaded.
	ErrStatsAgentsStoreUnavailable = errors.New("workbench stats agents store is unavailable")
	// ErrInvalidStatsDays preserves the legacy days query range.
	ErrInvalidStatsDays = errors.New("invalid days, expected 1..90")
	// ErrStatsAIReplyOverviewStoreUnavailable means AI reply counters cannot be loaded.
	ErrStatsAIReplyOverviewStoreUnavailable = errors.New("workbench stats ai reply overview store is unavailable")
	// ErrInvalidStatsDate preserves the legacy YYYY-MM-DD date query.
	ErrInvalidStatsDate = errors.New("invalid date, expected YYYY-MM-DD")
	// ErrStatsAIReplyBreakdownStoreUnavailable means AI reply failure buckets cannot be loaded.
	ErrStatsAIReplyBreakdownStoreUnavailable = errors.New("workbench stats ai reply breakdown store is unavailable")
	// ErrStatsAIReplyTrendStoreUnavailable means AI reply daily counters cannot be loaded.
	ErrStatsAIReplyTrendStoreUnavailable = errors.New("workbench stats ai reply trend store is unavailable")
)

var statsBeijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// StatsOverviewRecord carries raw aggregate counters for one Beijing day.
type StatsOverviewRecord struct {
	ConversationsToday       int
	MessagesToday            int
	AIAutoReplyConversations int
	OnlineDevices            int
}

// StatsTrendRecord carries daily aggregate maps keyed by YYYY-MM-DD.
type StatsTrendRecord struct {
	ConversationsByDay map[string]int
	MessagesByDay      map[string]int
}

// StatsAgentRecord carries one assignee workload row.
type StatsAgentRecord struct {
	AssigneeID    string
	AssigneeName  string
	Conversations int
	Messages      int
}

// StatsAIReplyOverviewRecord carries one-day AI reply attempt aggregates.
type StatsAIReplyOverviewRecord struct {
	Attempts            int
	SuccessCount        int
	SentCount           int
	UnreplyableCount    int
	FailedCount         int
	SendFailedCount     int
	AvgAICallDurationMS *float64
	AvgTotalDurationMS  *float64
}

// StatsAIReplyTrendRecord carries daily AI reply aggregates keyed by YYYY-MM-DD.
type StatsAIReplyTrendRecord struct {
	ByDay map[string]StatsAIReplyOverviewRecord
}

// StatsAIReplyBreakdownItem carries one failure_type bucket.
type StatsAIReplyBreakdownItem struct {
	FailureType string
	Count       int
}

// StatsOverviewRequest carries the authenticated management session.
type StatsOverviewRequest struct {
	Session auth.Session
}

// StatsTrendRequest carries normalized trend query input.
type StatsTrendRequest struct {
	Session auth.Session
	Days    int
}

// StatsAgentsRequest carries the authenticated management session.
type StatsAgentsRequest struct {
	Session auth.Session
}

// StatsAIReplyOverviewRequest carries optional AI reply stat date input.
type StatsAIReplyOverviewRequest struct {
	Session   auth.Session
	TargetDay time.Time
	HasDate   bool
}

// StatsAIReplyTrendRequest carries normalized AI reply trend input.
type StatsAIReplyTrendRequest struct {
	Session auth.Session
	Days    int
}

// StatsAIReplyBreakdownRequest carries optional AI reply breakdown date input.
type StatsAIReplyBreakdownRequest struct {
	Session   auth.Session
	TargetDay time.Time
	HasDate   bool
}

// NewStatsOverviewRequest normalizes the stats overview request boundary.
func NewStatsOverviewRequest(session auth.Session) StatsOverviewRequest {
	return StatsOverviewRequest{Session: session}
}

// NewStatsTrendRequest validates the legacy days query range.
func NewStatsTrendRequest(values url.Values, session auth.Session) (StatsTrendRequest, error) {
	days, err := parseStatsDays(values)
	if err != nil {
		return StatsTrendRequest{}, ErrInvalidStatsDays
	}
	return StatsTrendRequest{Session: session, Days: days}, nil
}

// NewStatsAgentsRequest normalizes the stats agents request boundary.
func NewStatsAgentsRequest(session auth.Session) StatsAgentsRequest {
	return StatsAgentsRequest{Session: session}
}

// NewStatsAIReplyTrendRequest validates the legacy days query range.
func NewStatsAIReplyTrendRequest(values url.Values, session auth.Session) (StatsAIReplyTrendRequest, error) {
	days, err := parseStatsDays(values)
	if err != nil {
		return StatsAIReplyTrendRequest{}, ErrInvalidStatsDays
	}
	return StatsAIReplyTrendRequest{Session: session, Days: days}, nil
}

// NewStatsAIReplyOverviewRequest validates the optional YYYY-MM-DD date query.
func NewStatsAIReplyOverviewRequest(values url.Values, session auth.Session) (StatsAIReplyOverviewRequest, error) {
	dateText := strings.TrimSpace(values.Get("date"))
	if dateText == "" {
		return StatsAIReplyOverviewRequest{Session: session}, nil
	}
	target, err := parseStatsDate(dateText)
	if err != nil {
		return StatsAIReplyOverviewRequest{}, ErrInvalidStatsDate
	}
	return StatsAIReplyOverviewRequest{Session: session, TargetDay: target, HasDate: true}, nil
}

// NewStatsAIReplyBreakdownRequest validates the optional YYYY-MM-DD date query.
func NewStatsAIReplyBreakdownRequest(values url.Values, session auth.Session) (StatsAIReplyBreakdownRequest, error) {
	dateText := strings.TrimSpace(values.Get("date"))
	if dateText == "" {
		return StatsAIReplyBreakdownRequest{Session: session}, nil
	}
	target, err := parseStatsDate(dateText)
	if err != nil {
		return StatsAIReplyBreakdownRequest{}, ErrInvalidStatsDate
	}
	return StatsAIReplyBreakdownRequest{Session: session, TargetDay: target, HasDate: true}, nil
}

// StatsOverview builds the read-only /api/v1/admin/stats/overview payload.
func (service Service) StatsOverview(ctx context.Context, request StatsOverviewRequest) (Payload, error) {
	if service.StatsOverviewStore == nil {
		return nil, ErrStatsOverviewStoreUnavailable
	}
	start, end := statsOverviewDayBounds(service.now())
	record, err := service.StatsOverviewStore.GetStatsOverview(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return Payload{
		"conversations_today": record.ConversationsToday,
		"messages_today":      record.MessagesToday,
		"ai_reply_rate":       statsAIReplyRate(record.AIAutoReplyConversations, record.ConversationsToday),
		"online_devices":      record.OnlineDevices,
	}, nil
}

// StatsTrend builds the read-only /api/v1/admin/stats/trend payload.
func (service Service) StatsTrend(ctx context.Context, request StatsTrendRequest) (Payload, error) {
	if service.StatsTrendStore == nil {
		return nil, ErrStatsTrendStoreUnavailable
	}
	days := boundedStatsDays(request.Days)
	start, end := statsTrendWindow(service.now(), days)
	record, err := service.StatsTrendStore.GetStatsTrend(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return Payload{"data": statsTrendPayload(start, days, record)}, nil
}

// StatsAgents builds the read-only /api/v1/admin/stats/agents payload.
func (service Service) StatsAgents(ctx context.Context, request StatsAgentsRequest) (Payload, error) {
	if service.StatsAgentsStore == nil {
		return nil, ErrStatsAgentsStoreUnavailable
	}
	records, err := service.StatsAgentsStore.GetStatsAgents(ctx, 20)
	if err != nil {
		return nil, err
	}
	return Payload{"agents": statsAgentsPayload(records)}, nil
}

// StatsAIReplyOverview builds /api/v1/admin/stats/ai-replies/overview.
func (service Service) StatsAIReplyOverview(ctx context.Context, request StatsAIReplyOverviewRequest) (Payload, error) {
	if service.StatsAIReplyStore == nil {
		return nil, ErrStatsAIReplyOverviewStoreUnavailable
	}
	start, end := statsAIReplyOverviewBounds(service.now(), request)
	record, err := service.StatsAIReplyStore.GetStatsAIReplyOverview(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return statsAIReplyOverviewPayload(start, record), nil
}

// StatsAIReplyTrend builds /api/v1/admin/stats/ai-replies/trend.
func (service Service) StatsAIReplyTrend(ctx context.Context, request StatsAIReplyTrendRequest) (Payload, error) {
	if service.StatsAITrendStore == nil {
		return nil, ErrStatsAIReplyTrendStoreUnavailable
	}
	days := boundedStatsDays(request.Days)
	start, end := statsTrendWindow(service.now(), days)
	record, err := service.StatsAITrendStore.GetStatsAIReplyTrend(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return Payload{"data": statsAIReplyTrendPayload(start, days, record)}, nil
}

// StatsAIReplyBreakdown builds /api/v1/admin/stats/ai-replies/breakdown.
func (service Service) StatsAIReplyBreakdown(ctx context.Context, request StatsAIReplyBreakdownRequest) (Payload, error) {
	if service.StatsBreakdownStore == nil {
		return nil, ErrStatsAIReplyBreakdownStoreUnavailable
	}
	start, end := statsAIReplyBreakdownBounds(service.now(), request)
	items, err := service.StatsBreakdownStore.GetStatsAIReplyBreakdown(ctx, start, end)
	if err != nil {
		return nil, err
	}
	var responseDate any
	if request.HasDate {
		responseDate = start.In(statsBeijingLocation).Format("2006-01-02")
	}
	return Payload{"date": responseDate, "items": statsAIReplyBreakdownPayload(items)}, nil
}

func statsOverviewDayBounds(now time.Time) (time.Time, time.Time) {
	beijingNow := now.In(statsBeijingLocation)
	start := time.Date(beijingNow.Year(), beijingNow.Month(), beijingNow.Day(), 0, 0, 0, 0, statsBeijingLocation)
	return start, start.Add(24 * time.Hour)
}

func statsAIReplyOverviewBounds(now time.Time, request StatsAIReplyOverviewRequest) (time.Time, time.Time) {
	if request.HasDate {
		start := request.TargetDay.In(statsBeijingLocation)
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, statsBeijingLocation)
		return start, start.Add(24 * time.Hour)
	}
	return statsOverviewDayBounds(now)
}

func statsAIReplyBreakdownBounds(now time.Time, request StatsAIReplyBreakdownRequest) (time.Time, time.Time) {
	if request.HasDate {
		start := request.TargetDay.In(statsBeijingLocation)
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, statsBeijingLocation)
		return start, start.Add(24 * time.Hour)
	}
	return statsOverviewDayBounds(now)
}

func statsAIReplyOverviewPayload(start time.Time, record StatsAIReplyOverviewRecord) Payload {
	return Payload{
		"date":                    start.In(statsBeijingLocation).Format("2006-01-02"),
		"attempts":                record.Attempts,
		"success_count":           record.SuccessCount,
		"sent_count":              record.SentCount,
		"unreplyable_count":       record.UnreplyableCount,
		"failed_count":            record.FailedCount,
		"send_failed_count":       record.SendFailedCount,
		"avg_ai_call_duration_ms": nullableFloat(record.AvgAICallDurationMS),
		"avg_total_duration_ms":   nullableFloat(record.AvgTotalDurationMS),
	}
}

func statsAIReplyBreakdownPayload(items []StatsAIReplyBreakdownItem) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(items))
	for _, item := range items {
		payload = append(payload, ProjectionRow{
			"failure_type": strings.TrimSpace(item.FailureType),
			"count":        item.Count,
		})
	}
	return payload
}

func statsAIReplyTrendPayload(start time.Time, days int, record StatsAIReplyTrendRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, days)
	byDay := record.ByDay
	if byDay == nil {
		byDay = map[string]StatsAIReplyOverviewRecord{}
	}
	for offset := 0; offset < days; offset++ {
		current := start.AddDate(0, 0, offset).In(statsBeijingLocation)
		key := current.Format("2006-01-02")
		summary := byDay[key]
		payload = append(payload, ProjectionRow{
			"day":                     key,
			"date":                    current.Format("01-02"),
			"attempts":                summary.Attempts,
			"success_count":           summary.SuccessCount,
			"sent_count":              summary.SentCount,
			"unreplyable_count":       summary.UnreplyableCount,
			"failed_count":            summary.FailedCount,
			"send_failed_count":       summary.SendFailedCount,
			"avg_ai_call_duration_ms": nullableFloat(summary.AvgAICallDurationMS),
			"avg_total_duration_ms":   nullableFloat(summary.AvgTotalDurationMS),
		})
	}
	return payload
}

func statsTrendWindow(now time.Time, days int) (time.Time, time.Time) {
	todayStart, todayEnd := statsOverviewDayBounds(now)
	return todayStart.AddDate(0, 0, -(boundedStatsDays(days) - 1)), todayEnd
}

func statsTrendPayload(start time.Time, days int, record StatsTrendRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, days)
	conversationsByDay := record.ConversationsByDay
	if conversationsByDay == nil {
		conversationsByDay = map[string]int{}
	}
	messagesByDay := record.MessagesByDay
	if messagesByDay == nil {
		messagesByDay = map[string]int{}
	}
	for offset := 0; offset < days; offset++ {
		current := start.AddDate(0, 0, offset).In(statsBeijingLocation)
		key := current.Format("2006-01-02")
		conversations := conversationsByDay[key]
		payload = append(payload, ProjectionRow{
			"day":           key,
			"date":          current.Format("01-02"),
			"conversations": conversations,
			"messages":      messagesByDay[key],
			"value":         conversations,
		})
	}
	return payload
}

func statsAgentsPayload(records []StatsAgentRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.AssigneeName)
		assigneeID := strings.TrimSpace(record.AssigneeID)
		if name == "" {
			name = assigneeID
		}
		payload = append(payload, ProjectionRow{
			"assignee_id":       assigneeID,
			"assignee_name":     name,
			"conversations":     record.Conversations,
			"messages":          record.Messages,
			"avg_response_time": "-",
		})
	}
	return payload
}

func boundedStatsDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > 90 {
		return 90
	}
	return days
}

func parseStatsDays(values url.Values) (int, error) {
	daysText := strings.TrimSpace(values.Get("days"))
	if daysText == "" {
		return 7, nil
	}
	days, err := strconv.Atoi(daysText)
	if err != nil || days < 1 || days > 90 {
		return 0, ErrInvalidStatsDays
	}
	return days, nil
}

func parseStatsDate(value string) (time.Time, error) {
	text := strings.TrimSpace(value)
	if len(text) != len("2006-01-02") || text[4] != '-' || text[7] != '-' {
		return time.Time{}, ErrInvalidStatsDate
	}
	parsed, err := time.ParseInLocation("2006-01-02", text, statsBeijingLocation)
	if err != nil {
		return time.Time{}, ErrInvalidStatsDate
	}
	return parsed, nil
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func statsAIReplyRate(aiEnabled int, conversations int) float64 {
	denominator := conversations
	if denominator < 1 {
		denominator = 1
	}
	return math.Round((float64(aiEnabled)*100/float64(denominator))*10) / 10
}
