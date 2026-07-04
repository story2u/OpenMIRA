// Command contact-sync-worker runs scheduled WeWork contact cache sync.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"im-go/internal/app"
	"im-go/internal/archivemedia"
	"im-go/internal/avatarstorage"
	"im-go/internal/config"
	"im-go/internal/contactsmodule"
	"im-go/internal/contactsyncscheduler"
	"im-go/internal/observability"
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "contact_sync_worker"
	logger := observability.NewLogger("im-go-contact-sync-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{OpenDatabase: true})
	if err != nil {
		logger.Errorf("contact sync worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	module, err := contactsmodule.New(contactsmodule.Options{
		DB:            runtime.DB,
		DBDialect:     runtime.Dialect,
		AvatarStorage: buildContactAvatarStorage(cfg),
		BuildSync:     true,
	})
	if err != nil {
		logger.Errorf("contact sync worker assembly failed error=%v", err)
		os.Exit(1)
	}
	tick := contactsyncscheduler.NewTick(module.Scheduler, contactsyncscheduler.OptionsFromConfig(cfg), time.Now)
	logger.Infof(
		"starting runtime_role=%s full_interval_sec=%d refresh_interval_sec=%d refresh_limit=%d full_startup_delay_sec=%d refresh_startup_delay_sec=%d",
		cfg.RuntimeRole,
		cfg.ContactSyncFullIntervalSec,
		cfg.ContactSyncRefreshIntervalSec,
		cfg.ContactSyncRefreshLimit,
		cfg.ContactSyncFullStartupDelaySec,
		cfg.ContactSyncRefreshStartupDelaySec,
	)
	if err := runLoop(ctx, &tick, contextSleep, func(result contactsyncscheduler.TickResult, tickErr error) {
		if tickErr != nil {
			logger.Errorf("contact sync tick failed error=%v", tickErr)
		}
		if result.FullDue {
			logger.Infof("contact sync full tick enterprises_total=%d enterprises_synced=%d enterprises_skipped=%d", result.Full.EnterprisesTotal, result.Full.EnterprisesSynced, result.Full.EnterprisesSkipped)
		}
		if result.RefreshDue {
			logger.Infof("contact sync refresh tick limit=%d external_refreshed=%d external_skipped=%d corp_refreshed=%d", result.Refresh.Limit, result.Refresh.ExternalContactsRefreshed, result.Refresh.ExternalContactsSkipped, result.Refresh.CorpUsersRefreshed)
		}
	}); err != nil {
		logger.Errorf("contact sync worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

func buildContactAvatarStorage(cfg config.Config) avatarstorage.Service {
	var uploader avatarstorage.Uploader
	if cfg.ArchiveMediaUploadURL != "" {
		uploader = archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     time.Duration(cfg.ArchiveMediaUploadTimeoutSec) * time.Second,
		}
	}
	return avatarstorage.Service{
		Uploader:      uploader,
		LocalDataRoot: cfg.DataRoot,
		Access: archivemedia.AccessURLBuilder{
			BaseURL:               cfg.ArchiveMediaBaseURL,
			ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
			PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
			SigningKey:            cfg.ArchiveMediaSigningKey,
			TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
		},
	}
}

type sleepFunc func(context.Context, time.Duration) error

type tickLogger func(contactsyncscheduler.TickResult, error)

func runLoop(ctx context.Context, tick *contactsyncscheduler.Tick, sleep sleepFunc, logTick tickLogger) error {
	if tick == nil {
		return errors.New("contact sync tick is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := tick.RunDue(ctx)
		if logTick != nil && (result.FullDue || result.RefreshDue || err != nil) {
			logTick(result, err)
		}
		if sleepErr := sleep(ctx, nextContactSyncDelay(tick)); sleepErr != nil {
			if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
				return nil
			}
			return sleepErr
		}
	}
}

func nextContactSyncDelay(tick *contactsyncscheduler.Tick) time.Duration {
	if tick == nil {
		return time.Second
	}
	now := time.Now()
	if tick.Now != nil {
		now = tick.Now()
	}
	next := tick.NextFullAt
	if next.IsZero() || (!tick.NextRefreshAt.IsZero() && tick.NextRefreshAt.Before(next)) {
		next = tick.NextRefreshAt
	}
	if next.IsZero() || !next.After(now) {
		return 0
	}
	return next.Sub(now)
}

func contextSleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
