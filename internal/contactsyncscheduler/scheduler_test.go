package contactsyncscheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/config"
	"wework-go/internal/contacts"
)

func TestOptionsFromConfigMirrorsPythonSchedulerBounds(t *testing.T) {
	options := OptionsFromConfig(config.Config{
		ContactSyncFullIntervalSec:        120,
		ContactSyncRefreshIntervalSec:     5,
		ContactSyncRefreshLimit:           0,
		ContactSyncFullStartupDelaySec:    -10,
		ContactSyncRefreshStartupDelaySec: -5,
	})

	if options.FullInterval != time.Hour ||
		options.RefreshInterval != time.Minute ||
		options.RefreshLimit != DefaultRefreshLimit ||
		options.FullStartupDelay != 0 ||
		options.RefreshStartupDelay != 0 {
		t.Fatalf("options = %+v", options)
	}
}

func TestSchedulerRunFullOnceSyncsEnabledEnterprisesUntilFirstError(t *testing.T) {
	service := &fakeService{fullErr: map[string]error{"ent-3": errors.New("wework down")}}
	scheduler := Scheduler{
		Service: service,
		Enterprises: &fakeEnterpriseLister{enterprises: []Enterprise{
			{EnterpriseID: " ent-1 ", Enabled: true},
			{EnterpriseID: "ent-disabled", Enabled: false},
			{EnterpriseID: "", Enabled: true},
			{EnterpriseID: "ent-3", Enabled: true},
			{EnterpriseID: "ent-4", Enabled: true},
		}},
	}

	result, err := scheduler.RunFullOnce(context.Background())
	if err == nil {
		t.Fatal("RunFullOnce returned nil error, want first sync failure")
	}
	if result.EnterprisesTotal != 5 || result.EnterprisesSynced != 1 || result.EnterprisesSkipped != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(service.fullRequests) != 2 || service.fullRequests[0] != "ent-1" || service.fullRequests[1] != "ent-3" {
		t.Fatalf("full requests = %#v", service.fullRequests)
	}
}

func TestSchedulerRunRefreshOnceUsesPythonDefaultLimitAndMapsPayload(t *testing.T) {
	service := &fakeService{refreshPayload: contacts.Payload{
		"external_contacts_refreshed": 2,
		"external_contacts_skipped":   float64(1),
		"corp_users_refreshed":        int64(3),
	}}
	scheduler := Scheduler{Service: service}

	result, err := scheduler.RunRefreshOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunRefreshOnce returned error: %v", err)
	}
	if service.refreshLimit != DefaultRefreshLimit || result.Limit != DefaultRefreshLimit {
		t.Fatalf("limits service=%d result=%d", service.refreshLimit, result.Limit)
	}
	if result.ExternalContactsRefreshed != 2 || result.ExternalContactsSkipped != 1 || result.CorpUsersRefreshed != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestTickRunDueHonorsStartupDelaysAndAdvancesAfterErrors(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	service := &fakeService{refreshErr: errors.New("temporary refresh failure")}
	tick := NewTick(Scheduler{
		Service: service,
		Enterprises: &fakeEnterpriseLister{enterprises: []Enterprise{
			{EnterpriseID: "ent-1", Enabled: true},
		}},
	}, Options{
		FullInterval:        time.Hour,
		RefreshInterval:     time.Minute,
		RefreshLimit:        7,
		FullStartupDelay:    2 * time.Minute,
		RefreshStartupDelay: 30 * time.Second,
	}, func() time.Time { return now })

	result, err := tick.RunDue(context.Background())
	if err != nil || result.FullDue || result.RefreshDue {
		t.Fatalf("early RunDue result=%+v err=%v", result, err)
	}

	now = now.Add(30 * time.Second)
	result, err = tick.RunDue(context.Background())
	if !errors.Is(err, service.refreshErr) || result.FullDue || !result.RefreshDue {
		t.Fatalf("refresh RunDue result=%+v err=%v", result, err)
	}
	if service.refreshLimit != 7 || !tick.NextRefreshAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("refresh state limit=%d next=%s", service.refreshLimit, tick.NextRefreshAt)
	}

	service.refreshErr = nil
	now = time.Date(2026, 7, 2, 9, 2, 0, 0, time.UTC)
	result, err = tick.RunDue(context.Background())
	if err != nil || !result.FullDue || !result.RefreshDue {
		t.Fatalf("full RunDue result=%+v err=%v", result, err)
	}
	if len(service.fullRequests) != 1 || service.fullRequests[0] != "ent-1" || !tick.NextFullAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("full state requests=%#v next=%s", service.fullRequests, tick.NextFullAt)
	}
	if !tick.NextRefreshAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("refresh next after second due = %s", tick.NextRefreshAt)
	}
}

func TestSchedulerFailsClosedWithoutDependencies(t *testing.T) {
	if _, err := (Scheduler{}).RunFullOnce(context.Background()); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("RunFullOnce missing service error = %v", err)
	}
	if _, err := (Scheduler{Service: &fakeService{}}).RunFullOnce(context.Background()); !errors.Is(err, ErrEnterprisesUnavailable) {
		t.Fatalf("RunFullOnce missing enterprises error = %v", err)
	}
	if _, err := (Scheduler{}).RunRefreshOnce(context.Background(), 1); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("RunRefreshOnce missing service error = %v", err)
	}
}

type fakeEnterpriseLister struct {
	enterprises []Enterprise
	err         error
}

func (lister *fakeEnterpriseLister) ListEnterprises(ctx context.Context) ([]Enterprise, error) {
	if lister.err != nil {
		return nil, lister.err
	}
	return lister.enterprises, nil
}

type fakeService struct {
	fullRequests   []string
	fullErr        map[string]error
	refreshLimit   int
	refreshPayload contacts.Payload
	refreshErr     error
}

func (service *fakeService) SyncFull(ctx context.Context, request contacts.SyncFullRequest) (contacts.Payload, error) {
	service.fullRequests = append(service.fullRequests, request.EnterpriseID)
	if service.fullErr != nil {
		if err := service.fullErr[request.EnterpriseID]; err != nil {
			return nil, err
		}
	}
	return contacts.Payload{"enterprise_id": request.EnterpriseID}, nil
}

func (service *fakeService) RefreshStale(ctx context.Context, request contacts.RefreshStaleRequest) (contacts.Payload, error) {
	service.refreshLimit = request.Limit
	return service.refreshPayload, service.refreshErr
}
