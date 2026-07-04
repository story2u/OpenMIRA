// Package contactsyncscheduler runs contact cache refresh work without owning
// goroutines or sleeps. Commands decide when to call the tick.
package contactsyncscheduler

import (
	"context"
	"errors"
	"strings"
	"time"

	"wework-go/internal/config"
	"wework-go/internal/contacts"
)

const (
	DefaultFullInterval        = 24 * time.Hour
	DefaultRefreshInterval     = 5 * time.Minute
	DefaultRefreshLimit        = 50
	DefaultFullStartupDelay    = 3 * time.Minute
	DefaultRefreshStartupDelay = 30 * time.Second
	MinimumFullInterval        = time.Hour
	MinimumRefreshInterval     = time.Minute
)

var (
	ErrServiceUnavailable     = errors.New("contact sync scheduler service unavailable")
	ErrEnterprisesUnavailable = errors.New("contact sync scheduler enterprise lister unavailable")
)

// Options mirrors Python ContactSyncScheduler env defaults.
type Options struct {
	FullInterval        time.Duration
	RefreshInterval     time.Duration
	RefreshLimit        int
	FullStartupDelay    time.Duration
	RefreshStartupDelay time.Duration
}

// OptionsFromConfig builds scheduler options from process config.
func OptionsFromConfig(cfg config.Config) Options {
	return Options{
		FullInterval:        time.Duration(cfg.ContactSyncFullIntervalSec) * time.Second,
		RefreshInterval:     time.Duration(cfg.ContactSyncRefreshIntervalSec) * time.Second,
		RefreshLimit:        cfg.ContactSyncRefreshLimit,
		FullStartupDelay:    time.Duration(cfg.ContactSyncFullStartupDelaySec) * time.Second,
		RefreshStartupDelay: time.Duration(cfg.ContactSyncRefreshStartupDelaySec) * time.Second,
	}.Normalize()
}

// Normalize applies Python-compatible lower bounds and defaults.
func (options Options) Normalize() Options {
	if options.FullInterval <= 0 {
		options.FullInterval = DefaultFullInterval
	}
	if options.FullInterval < MinimumFullInterval {
		options.FullInterval = MinimumFullInterval
	}
	if options.RefreshInterval <= 0 {
		options.RefreshInterval = DefaultRefreshInterval
	}
	if options.RefreshInterval < MinimumRefreshInterval {
		options.RefreshInterval = MinimumRefreshInterval
	}
	if options.RefreshLimit <= 0 {
		options.RefreshLimit = DefaultRefreshLimit
	}
	if options.FullStartupDelay < 0 {
		options.FullStartupDelay = 0
	}
	if options.RefreshStartupDelay < 0 {
		options.RefreshStartupDelay = 0
	}
	return options
}

// Enterprise is the minimal shape the scheduler needs from the enterprise store.
type Enterprise struct {
	EnterpriseID string
	Enabled      bool
}

// EnterpriseLister lists all enterprises considered by the full sync loop.
type EnterpriseLister interface {
	ListEnterprises(ctx context.Context) ([]Enterprise, error)
}

// FullSyncer performs one enterprise full contact sync.
type FullSyncer interface {
	SyncFull(ctx context.Context, request contacts.SyncFullRequest) (contacts.Payload, error)
}

// StaleRefresher performs one stale contact refresh pass.
type StaleRefresher interface {
	RefreshStale(ctx context.Context, request contacts.RefreshStaleRequest) (contacts.Payload, error)
}

// Service is the contact sync boundary used by scheduler ticks.
type Service interface {
	FullSyncer
	StaleRefresher
}

// Scheduler owns contact sync orchestration for one process tick.
type Scheduler struct {
	Service     Service
	Enterprises EnterpriseLister
}

// FullRunResult summarizes one full sync pass across enabled enterprises.
type FullRunResult struct {
	EnterprisesTotal   int
	EnterprisesSynced  int
	EnterprisesSkipped int
	EnterpriseIDs      []string
}

// RefreshRunResult summarizes one stale refresh pass.
type RefreshRunResult struct {
	Limit                     int
	ExternalContactsRefreshed int
	ExternalContactsSkipped   int
	CorpUsersRefreshed        int
}

// TickResult summarizes due work executed by Tick.RunDue.
type TickResult struct {
	FullDue    bool
	RefreshDue bool
	Full       FullRunResult
	Refresh    RefreshRunResult
}

// Tick is a caller-owned scheduler state machine.
type Tick struct {
	Scheduler     Scheduler
	Options       Options
	NextFullAt    time.Time
	NextRefreshAt time.Time
	Now           func() time.Time
}

// NewTick initializes due times using startup delays without starting goroutines.
func NewTick(scheduler Scheduler, options Options, now func() time.Time) Tick {
	tick := Tick{Scheduler: scheduler, Options: options.Normalize(), Now: now}
	base := tick.now()
	tick.NextFullAt = base.Add(tick.Options.FullStartupDelay)
	tick.NextRefreshAt = base.Add(tick.Options.RefreshStartupDelay)
	return tick
}

// RunFullOnce mirrors Python's full sync loop body for one round.
func (scheduler Scheduler) RunFullOnce(ctx context.Context) (FullRunResult, error) {
	if scheduler.Service == nil {
		return FullRunResult{}, ErrServiceUnavailable
	}
	if scheduler.Enterprises == nil {
		return FullRunResult{}, ErrEnterprisesUnavailable
	}
	enterprises, err := scheduler.Enterprises.ListEnterprises(ctx)
	if err != nil {
		return FullRunResult{}, err
	}
	result := FullRunResult{EnterprisesTotal: len(enterprises)}
	for _, enterprise := range enterprises {
		enterpriseID := strings.TrimSpace(enterprise.EnterpriseID)
		if enterpriseID == "" || !enterprise.Enabled {
			result.EnterprisesSkipped++
			continue
		}
		if _, err := scheduler.Service.SyncFull(ctx, contacts.SyncFullRequest{EnterpriseID: enterpriseID}); err != nil {
			return result, err
		}
		result.EnterprisesSynced++
		result.EnterpriseIDs = append(result.EnterpriseIDs, enterpriseID)
	}
	return result, nil
}

// RunRefreshOnce mirrors Python's stale refresh loop body for one round.
func (scheduler Scheduler) RunRefreshOnce(ctx context.Context, limit int) (RefreshRunResult, error) {
	if scheduler.Service == nil {
		return RefreshRunResult{}, ErrServiceUnavailable
	}
	if limit <= 0 {
		limit = DefaultRefreshLimit
	}
	payload, err := scheduler.Service.RefreshStale(ctx, contacts.RefreshStaleRequest{Limit: limit})
	result := RefreshRunResult{
		Limit:                     limit,
		ExternalContactsRefreshed: payloadInt(payload, "external_contacts_refreshed"),
		ExternalContactsSkipped:   payloadInt(payload, "external_contacts_skipped"),
		CorpUsersRefreshed:        payloadInt(payload, "corp_users_refreshed"),
	}
	return result, err
}

// RunDue executes due full/refresh work and advances attempted due times.
func (tick *Tick) RunDue(ctx context.Context) (TickResult, error) {
	if tick == nil {
		return TickResult{}, ErrServiceUnavailable
	}
	tick.ensureInitialized()
	now := tick.now()
	options := tick.options()
	result := TickResult{}
	var firstErr error
	if !now.Before(tick.NextFullAt) {
		result.FullDue = true
		full, err := tick.Scheduler.RunFullOnce(ctx)
		result.Full = full
		tick.NextFullAt = now.Add(options.FullInterval)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if !now.Before(tick.NextRefreshAt) {
		result.RefreshDue = true
		refresh, err := tick.Scheduler.RunRefreshOnce(ctx, options.RefreshLimit)
		result.Refresh = refresh
		tick.NextRefreshAt = now.Add(options.RefreshInterval)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return result, firstErr
}

func (tick *Tick) ensureInitialized() {
	if tick.Options == (Options{}) {
		tick.Options = Options{}.Normalize()
	} else {
		tick.Options = tick.Options.Normalize()
	}
	now := tick.now()
	if tick.NextFullAt.IsZero() {
		tick.NextFullAt = now.Add(tick.Options.FullStartupDelay)
	}
	if tick.NextRefreshAt.IsZero() {
		tick.NextRefreshAt = now.Add(tick.Options.RefreshStartupDelay)
	}
}

func (tick *Tick) options() Options {
	if tick == nil {
		return Options{}.Normalize()
	}
	return tick.Options.Normalize()
}

func (tick *Tick) now() time.Time {
	if tick != nil && tick.Now != nil {
		return tick.Now()
	}
	return time.Now()
}

func payloadInt(payload contacts.Payload, key string) int {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
