package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/config"
	"wework-go/internal/contacts"
	"wework-go/internal/contactsyncscheduler"
)

func TestNextContactSyncDelayUsesNearestDue(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	tick := &contactsyncscheduler.Tick{
		NextFullAt:    now.Add(2 * time.Hour),
		NextRefreshAt: now.Add(30 * time.Second),
		Now:           func() time.Time { return now },
	}
	if delay := nextContactSyncDelay(tick); delay != 30*time.Second {
		t.Fatalf("delay = %v, want 30s", delay)
	}
	tick.NextRefreshAt = now.Add(-time.Second)
	if delay := nextContactSyncDelay(tick); delay != 0 {
		t.Fatalf("overdue delay = %v, want 0", delay)
	}
}

func TestRunLoopContinuesAfterTickErrorAndSleepsUntilNextDue(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	service := &loopService{refreshErr: errors.New("wework temporary failure")}
	tick := contactsyncscheduler.NewTick(contactsyncscheduler.Scheduler{
		Service:     service,
		Enterprises: loopEnterprises{},
	}, contactsyncscheduler.Options{
		FullInterval:        time.Hour,
		RefreshInterval:     time.Minute,
		RefreshLimit:        5,
		FullStartupDelay:    time.Hour,
		RefreshStartupDelay: 0,
	}, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sleeps []time.Duration
	var loggedErr error
	err := runLoop(ctx, &tick, func(ctx context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		cancel()
		return context.Canceled
	}, func(result contactsyncscheduler.TickResult, err error) {
		loggedErr = err
	})
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if !errors.Is(loggedErr, service.refreshErr) {
		t.Fatalf("logged error = %v", loggedErr)
	}
	if len(sleeps) != 1 || sleeps[0] != time.Minute {
		t.Fatalf("sleeps = %#v", sleeps)
	}
	if service.refreshLimit != 5 {
		t.Fatalf("refresh limit = %d", service.refreshLimit)
	}
}

func TestRunLoopRejectsMissingTick(t *testing.T) {
	if err := runLoop(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("runLoop returned nil error, want missing tick error")
	}
}

func TestBuildContactAvatarStorageUsesArchiveMediaConfig(t *testing.T) {
	storage := buildContactAvatarStorage(config.Config{
		PythonProjectRoot:               "/srv/python",
		ArchiveMediaUploadURL:           "http://object-storage/internal/upload",
		ArchiveMediaUploadToken:         "upload-token",
		ArchiveMediaUploadTimeoutSec:    7,
		ArchiveMediaBaseURL:             "https://cloud.example",
		ArchiveMediaObjectPublicBaseURL: "https://cdn.example/objects",
		ArchiveMediaDirectObjectURL:     false,
		ArchiveMediaSigningKey:          "signing-key",
		ArchiveMediaTokenTTLSeconds:     600,
	})

	if storage.LocalDataRoot != filepath.Join("/srv/python", "backend", "data") {
		t.Fatalf("LocalDataRoot = %q", storage.LocalDataRoot)
	}
	uploader, ok := storage.Uploader.(archivemedia.HTTPUploader)
	if !ok {
		t.Fatalf("Uploader = %T, want archivemedia.HTTPUploader", storage.Uploader)
	}
	if uploader.UploadURL != "http://object-storage/internal/upload" || uploader.UploadToken != "upload-token" || uploader.Timeout != 7*time.Second {
		t.Fatalf("uploader = %+v", uploader)
	}
	access, ok := storage.Access.(archivemedia.AccessURLBuilder)
	if !ok {
		t.Fatalf("Access = %T, want archivemedia.AccessURLBuilder", storage.Access)
	}
	if access.BaseURL != "https://cloud.example" || access.ObjectPublicBaseURL != "https://cdn.example/objects" || access.PreferDirectObjectURL || access.SigningKey != "signing-key" || access.TokenTTL != 600*time.Second {
		t.Fatalf("access builder = %+v", access)
	}
}

func TestBuildContactAvatarStorageKeepsUploaderOptional(t *testing.T) {
	storage := buildContactAvatarStorage(config.Config{PythonProjectRoot: "/srv/python"})

	if storage.Uploader != nil {
		t.Fatalf("Uploader = %T, want nil without upload URL", storage.Uploader)
	}
	if storage.LocalDataRoot != filepath.Join("/srv/python", "backend", "data") {
		t.Fatalf("LocalDataRoot = %q", storage.LocalDataRoot)
	}
	if storage.Access == nil {
		t.Fatal("Access builder is nil")
	}
}

type loopService struct {
	refreshLimit int
	refreshErr   error
}

func (service *loopService) SyncFull(ctx context.Context, request contacts.SyncFullRequest) (contacts.Payload, error) {
	return contacts.Payload{"enterprise_id": request.EnterpriseID}, nil
}

func (service *loopService) RefreshStale(ctx context.Context, request contacts.RefreshStaleRequest) (contacts.Payload, error) {
	service.refreshLimit = request.Limit
	return contacts.Payload{}, service.refreshErr
}

type loopEnterprises struct{}

func (loopEnterprises) ListEnterprises(ctx context.Context) ([]contactsyncscheduler.Enterprise, error) {
	return []contactsyncscheduler.Enterprise{{EnterpriseID: "ent-1", Enabled: true}}, nil
}
