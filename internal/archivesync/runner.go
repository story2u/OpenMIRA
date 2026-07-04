// Package archivesync coordinates archive pull runner steps.
package archivesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"wework-go/internal/archivepull"
	"wework-go/internal/infra/archiveingesttask"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/outboxarchivesync"
)

const (
	DefaultEnterpriseID = "default"
	DefaultSource       = "self_decrypt"
	DefaultLimit        = 200
	DefaultLockTTL      = 30 * time.Second
	MinimumLockTTL      = 10 * time.Second
	LockReleaseScript   = "if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end"
	LockRefreshScript   = "if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('pexpire', KEYS[1], ARGV[2]) else return 0 end"
)

// EnterpriseStore reads archive pull enterprise configuration.
type EnterpriseStore interface {
	GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error)
}

// EnterpriseLister lists archive-enabled enterprises for scoped catch-up passes.
type EnterpriseLister interface {
	ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error)
}

// CursorStore reads and advances archive sync cursors.
type CursorStore interface {
	GetCursor(ctx context.Context, source string, enterpriseID string) (*archivesynccursor.Record, error)
	UpsertCursor(ctx context.Context, source string, cursor string, enterpriseID string) (archivesynccursor.Record, error)
}

// Puller pulls archive messages from a source.
type Puller interface {
	PullSelfDecrypt(ctx context.Context, input archivepull.PullInput) (archivepull.Result, error)
}

// TaskStore stages pulled archive messages for lock-external ingest.
type TaskStore interface {
	EnqueueBatch(ctx context.Context, input archiveingesttask.EnqueueBatchInput) (archiveingesttask.Record, error)
}

// LockStore owns the optional Redis SETNX lease around one enterprise/source pull scope.
type LockStore interface {
	AcquireArchiveSyncLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error)
	RefreshArchiveSyncLock(ctx context.Context, key string, token string, ttl time.Duration) error
	ReleaseArchiveSyncLock(ctx context.Context, key string, token string) error
}

// Ingestor persists pulled archive messages.
type Ingestor interface {
	IngestArchivePull(ctx context.Context, enterprise enterprisestore.ArchivePullEnterprise, result archivepull.Result) error
}

// Request describes one runner trigger.
type Request struct {
	EnterpriseID  string
	Source        string
	Cursor        *string
	Limit         int
	WeWorkUserID  string
	TriggerReason string
}

// ScopeRequest describes one multi-enterprise catch-up tick.
type ScopeRequest struct {
	EnterpriseID  string
	Source        string
	Limit         int
	MaxRounds     int
	Concurrency   int
	TriggerReason string
}

// Result summarizes one runner pass.
type Result struct {
	EnterpriseID string
	Source       string
	Cursor       *string
	PulledTotal  int
	StagedTaskID string
	Skipped      bool
	SkipReason   string
}

// ScopeFailure records one enterprise/source catch-up error without aborting the whole tick.
type ScopeFailure struct {
	EnterpriseID string
	Source       string
	Err          error
}

// ScopeResult summarizes one multi-enterprise catch-up tick.
type ScopeResult struct {
	Results  []Result
	Failures []ScopeFailure
}

// ProcessedCount reports whether the tick performed observable work.
func (result ScopeResult) ProcessedCount() int {
	return len(result.Results) + len(result.Failures)
}

// Runner coordinates one archive pull pass.
type Runner struct {
	Enterprises  EnterpriseStore
	Cursors      CursorStore
	Puller       Puller
	Tasks        TaskStore
	Ingestor     Ingestor
	Locks        LockStore
	DefaultLimit int
	LockTTL      time.Duration
	LockRenew    time.Duration
	NewLockToken func() string
}

// TriggerArchiveSync implements outboxarchivesync.Trigger.
func (runner Runner) TriggerArchiveSync(ctx context.Context, request outboxarchivesync.Request) error {
	_, err := runner.RunOnce(ctx, Request{
		EnterpriseID:  request.EnterpriseID,
		Source:        request.Source,
		Cursor:        request.Cursor,
		Limit:         request.Limit,
		WeWorkUserID:  request.WeWorkUserID,
		TriggerReason: request.Reason,
	})
	return err
}

// RunScopeOnce performs a bounded catch-up pass for one enterprise or all enabled enterprises.
func (runner Runner) RunScopeOnce(ctx context.Context, request ScopeRequest) (ScopeResult, error) {
	targets, err := runner.scopeTargets(ctx, request.EnterpriseID)
	if err != nil {
		return ScopeResult{}, err
	}
	normalizedTargets := normalizeScopeTargets(targets, request.Source)
	requestedLimit := runner.limit(request.Limit)
	concurrency := normalizeScopeConcurrency(request.Concurrency, len(normalizedTargets))
	if concurrency <= 1 {
		result := ScopeResult{}
		for _, target := range normalizedTargets {
			appendScopeResult(&result, runner.runScopeTarget(ctx, request, target, requestedLimit))
		}
		return result, nil
	}
	results := make([]ScopeResult, len(normalizedTargets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = runner.runScopeTarget(ctx, request, normalizedTargets[index], requestedLimit)
			}
		}()
	}
	for index := range normalizedTargets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	result := ScopeResult{}
	for _, item := range results {
		appendScopeResult(&result, item)
	}
	return result, nil
}

type scopeTarget struct {
	EnterpriseID string
	Source       string
}

func normalizeScopeTargets(targets []enterprisestore.ArchivePullEnterprise, source string) []scopeTarget {
	normalized := make([]scopeTarget, 0, len(targets))
	for _, target := range targets {
		enterpriseID := strings.TrimSpace(target.EnterpriseID)
		if enterpriseID == "" {
			continue
		}
		normalized = append(normalized, scopeTarget{
			EnterpriseID: enterpriseID,
			Source:       defaultText(source, target.ArchiveSource),
		})
	}
	return normalized
}

func normalizeScopeConcurrency(value int, targetCount int) int {
	if value <= 1 || targetCount <= 1 {
		return 1
	}
	if value > targetCount {
		return targetCount
	}
	return value
}

func appendScopeResult(result *ScopeResult, item ScopeResult) {
	if result == nil {
		return
	}
	result.Results = append(result.Results, item.Results...)
	result.Failures = append(result.Failures, item.Failures...)
}

func (runner Runner) runScopeTarget(ctx context.Context, request ScopeRequest, target scopeTarget, requestedLimit int) ScopeResult {
	result := ScopeResult{}
	rounds := request.MaxRounds
	if rounds <= 0 {
		rounds = 1
	}
	for round := 0; round < rounds; round++ {
		pass, err := runner.RunOnce(ctx, Request{
			EnterpriseID:  target.EnterpriseID,
			Source:        target.Source,
			Limit:         request.Limit,
			TriggerReason: request.TriggerReason,
		})
		if err != nil {
			result.Failures = append(result.Failures, ScopeFailure{EnterpriseID: target.EnterpriseID, Source: target.Source, Err: err})
			break
		}
		result.Results = append(result.Results, pass)
		if pass.Skipped || pass.PulledTotal < requestedLimit {
			break
		}
	}
	return result
}

// RunOnce performs one self-decrypt archive pull pass.
func (runner Runner) RunOnce(ctx context.Context, request Request) (Result, error) {
	if runner.Enterprises == nil {
		return Result{}, fmt.Errorf("archive sync enterprise store is not configured")
	}
	if runner.Cursors == nil {
		return Result{}, fmt.Errorf("archive sync cursor store is not configured")
	}
	enterpriseID := defaultText(request.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(request.Source, DefaultSource)
	enterprise, err := runner.Enterprises.GetArchivePullEnterprise(ctx, enterpriseID)
	if err != nil {
		return Result{}, err
	}
	if enterprise != nil && enterprise.ArchiveSource != "" && strings.TrimSpace(request.Source) == "" {
		source = strings.TrimSpace(enterprise.ArchiveSource)
	}
	lease, locked, err := runner.acquireScopeLock(ctx, enterpriseID, source)
	if err != nil {
		return Result{}, err
	}
	if !locked {
		cursor, err := runner.resultCursor(ctx, source, enterpriseID, request.Cursor)
		if err != nil {
			return Result{}, err
		}
		return Result{EnterpriseID: enterpriseID, Source: source, Cursor: cursor, Skipped: true, SkipReason: "distributed_lock_held"}, nil
	}
	stopWatchdog := runner.startScopeLockWatchdog(ctx, lease)
	defer stopWatchdog()
	defer runner.releaseScopeLock(ctx, lease)
	cursor := request.Cursor
	if cursor == nil {
		record, err := runner.Cursors.GetCursor(ctx, source, enterpriseID)
		if err != nil {
			return Result{}, err
		}
		if record != nil && strings.TrimSpace(record.Cursor) != "" {
			value := strings.TrimSpace(record.Cursor)
			cursor = &value
		}
	}
	pullEnterprise := enterprisestore.ArchivePullEnterprise{
		EnterpriseID:  enterpriseID,
		Enabled:       true,
		ArchiveSource: source,
	}
	if enterprise == nil {
		if enterpriseID != DefaultEnterpriseID {
			return Result{EnterpriseID: enterpriseID, Source: source, Cursor: cursor, Skipped: true, SkipReason: "enterprise_missing"}, nil
		}
	} else {
		pullEnterprise = *enterprise
	}
	if enterprise != nil && !enterprise.Enabled {
		return Result{EnterpriseID: enterpriseID, Source: source, Cursor: cursor, Skipped: true, SkipReason: "enterprise_disabled"}, nil
	}
	if enterprise != nil && strings.TrimSpace(enterprise.ArchivePullURL) == "" {
		return Result{EnterpriseID: enterpriseID, Source: source, Cursor: cursor, Skipped: true, SkipReason: "self_decrypt_pull_url_missing"}, nil
	}
	if runner.Puller == nil {
		return Result{}, fmt.Errorf("archive sync puller is not configured")
	}
	pulled, err := runner.Puller.PullSelfDecrypt(ctx, archivepull.PullInput{
		Source:       source,
		Cursor:       cursor,
		Limit:        runner.limit(request.Limit),
		EnterpriseID: enterpriseID,
		Mode:         pullEnterprise.ArchiveMode,
		PullURL:      pullEnterprise.ArchivePullURL,
		PullToken:    pullEnterprise.ArchivePullToken,
	})
	if err != nil {
		return Result{}, err
	}
	stagedTaskID := ""
	if len(pulled.Messages) > 0 {
		if runner.Tasks != nil {
			task, err := runner.Tasks.EnqueueBatch(ctx, archiveingesttask.EnqueueBatchInput{
				EnterpriseID:    enterpriseID,
				Source:          pulled.Source,
				Cursor:          pulledCursorText(pulled.Cursor),
				MessagesPayload: pulled.Messages,
			})
			if err != nil {
				return Result{}, err
			}
			stagedTaskID = strings.TrimSpace(task.TaskID)
		} else if runner.Ingestor != nil {
			if err := runner.Ingestor.IngestArchivePull(ctx, pullEnterprise, pulled); err != nil {
				return Result{}, err
			}
		} else {
			return Result{}, fmt.Errorf("archive ingest is not configured")
		}
	}
	if pulled.Cursor != nil && strings.TrimSpace(*pulled.Cursor) != "" {
		advanced, err := runner.Cursors.UpsertCursor(ctx, pulled.Source, strings.TrimSpace(*pulled.Cursor), enterpriseID)
		if err != nil {
			return Result{}, err
		}
		value := strings.TrimSpace(advanced.Cursor)
		cursor = &value
	}
	return Result{EnterpriseID: enterpriseID, Source: pulled.Source, Cursor: cursor, PulledTotal: len(pulled.Messages), StagedTaskID: stagedTaskID}, nil
}

// ArchiveSyncScopeKey returns the legacy pull-lock scope. The user hint is intentionally excluded.
func ArchiveSyncScopeKey(enterpriseID string, source string) string {
	return defaultText(enterpriseID, DefaultEnterpriseID) + "|" + defaultText(source, DefaultSource)
}

// ArchiveSyncLockKey returns the Redis key used by Python archive sync distributed locks.
func ArchiveSyncLockKey(enterpriseID string, source string) string {
	return "archive-sync:lock:" + ArchiveSyncScopeKey(enterpriseID, source)
}

// NewLockToken returns a random owner token for a pull lease.
func NewLockToken() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

type scopeLockLease struct {
	Key      string
	Token    string
	TTL      time.Duration
	Acquired bool
}

func (runner Runner) acquireScopeLock(ctx context.Context, enterpriseID string, source string) (scopeLockLease, bool, error) {
	if runner.Locks == nil {
		return scopeLockLease{}, true, nil
	}
	token := ""
	if runner.NewLockToken != nil {
		token = strings.TrimSpace(runner.NewLockToken())
	}
	if token == "" {
		token = NewLockToken()
	}
	lease := scopeLockLease{
		Key:   ArchiveSyncLockKey(enterpriseID, source),
		Token: token,
		TTL:   runner.lockTTL(),
	}
	acquired, err := runner.Locks.AcquireArchiveSyncLock(ctx, lease.Key, lease.Token, lease.TTL)
	if err != nil {
		return scopeLockLease{}, true, nil
	}
	if !acquired {
		return lease, false, nil
	}
	lease.Acquired = true
	return lease, true, nil
}

func (runner Runner) releaseScopeLock(ctx context.Context, lease scopeLockLease) {
	if runner.Locks == nil || !lease.Acquired || strings.TrimSpace(lease.Key) == "" || strings.TrimSpace(lease.Token) == "" {
		return
	}
	_ = runner.Locks.ReleaseArchiveSyncLock(ctx, lease.Key, lease.Token)
}

func (runner Runner) startScopeLockWatchdog(ctx context.Context, lease scopeLockLease) func() {
	renewEvery := runner.lockRenewInterval(lease.TTL)
	if runner.Locks == nil || !lease.Acquired || renewEvery <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(renewEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = runner.Locks.RefreshArchiveSyncLock(ctx, lease.Key, lease.Token, lease.TTL)
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(stop)
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func (runner Runner) resultCursor(ctx context.Context, source string, enterpriseID string, explicit *string) (*string, error) {
	if explicit != nil {
		value := strings.TrimSpace(*explicit)
		if value == "" {
			return nil, nil
		}
		return &value, nil
	}
	if runner.Cursors == nil {
		return nil, nil
	}
	record, err := runner.Cursors.GetCursor(ctx, source, enterpriseID)
	if err != nil {
		return nil, err
	}
	if record == nil || strings.TrimSpace(record.Cursor) == "" {
		return nil, nil
	}
	value := strings.TrimSpace(record.Cursor)
	return &value, nil
}

func (runner Runner) lockTTL() time.Duration {
	if runner.LockTTL < MinimumLockTTL {
		return DefaultLockTTL
	}
	return runner.LockTTL
}

func (runner Runner) lockRenewInterval(ttl time.Duration) time.Duration {
	if runner.LockRenew > 0 {
		if runner.LockRenew >= ttl {
			return ttl - time.Second
		}
		return runner.LockRenew
	}
	renew := ttl / 3
	if renew < time.Second {
		return time.Second
	}
	if renew >= ttl {
		return ttl - time.Second
	}
	return renew
}

func (runner Runner) scopeTargets(ctx context.Context, enterpriseID string) ([]enterprisestore.ArchivePullEnterprise, error) {
	enterpriseID = strings.TrimSpace(enterpriseID)
	if enterpriseID != "" {
		return []enterprisestore.ArchivePullEnterprise{{EnterpriseID: enterpriseID}}, nil
	}
	lister, ok := runner.Enterprises.(EnterpriseLister)
	if !ok {
		return nil, fmt.Errorf("archive sync enterprise lister is not configured")
	}
	records, err := lister.ListEnabledArchivePullEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (runner Runner) limit(value int) int {
	if value <= 0 {
		value = runner.DefaultLimit
	}
	if value <= 0 {
		value = DefaultLimit
	}
	return archivepull.ClampPullLimit(value, archivepull.DefaultPullLimitMaximum)
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func pulledCursorText(cursor *string) string {
	if cursor == nil {
		return ""
	}
	return strings.TrimSpace(*cursor)
}
