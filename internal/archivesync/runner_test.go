package archivesync

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"wework-go/internal/archivepull"
	"wework-go/internal/infra/archiveingesttask"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/outboxarchivesync"
)

func TestRunOncePullsAndAdvancesCursor(t *testing.T) {
	cursorStore := &fakeCursorStore{cursor: "10"}
	puller := &fakePuller{result: archivepull.Result{
		Source: "self_decrypt",
		Cursor: stringPtr("20"),
	}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:     "ent-1",
			Enabled:          true,
			CorpID:           "corp-1",
			ArchiveMode:      "self_decrypt",
			ArchiveSource:    "self_decrypt",
			ArchivePullURL:   "https://archive.example/pull",
			ArchivePullToken: "token",
		}},
		Cursors: cursorStore,
		Puller:  puller,
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1", Limit: 5000})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Cursor == nil || *result.Cursor != "20" || result.PulledTotal != 0 || result.Skipped {
		t.Fatalf("result = %#v", result)
	}
	if puller.input.Source != "self_decrypt" || puller.input.Cursor == nil || *puller.input.Cursor != "10" || puller.input.Limit != 2000 || puller.input.EnterpriseID != "ent-1" || puller.input.PullURL != "https://archive.example/pull" || puller.input.PullToken != "token" {
		t.Fatalf("pull input = %#v", puller.input)
	}
	if cursorStore.upsertSource != "self_decrypt" || cursorStore.upsertCursor != "20" || cursorStore.upsertEnterpriseID != "ent-1" {
		t.Fatalf("cursor store = %#v", cursorStore)
	}
}

func TestArchiveSyncLockKeyUsesEnterpriseAndSourceOnly(t *testing.T) {
	if got := ArchiveSyncScopeKey(" ent-1 ", " self_decrypt "); got != "ent-1|self_decrypt" {
		t.Fatalf("scope key = %q", got)
	}
	if got := ArchiveSyncLockKey("ent-1", "self_decrypt"); got != "archive-sync:lock:ent-1|self_decrypt" {
		t.Fatalf("lock key = %q", got)
	}
}

func TestRunOnceUsesDistributedLockAroundPull(t *testing.T) {
	order := []string{}
	locks := &fakeLockStore{acquired: true, order: &order}
	puller := &fakePuller{
		order: &order,
		result: archivepull.Result{
			Source: "self_decrypt",
			Cursor: stringPtr("20"),
		},
	}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors:      &fakeCursorStore{},
		Puller:       puller,
		Locks:        locks,
		LockTTL:      45 * time.Second,
		NewLockToken: func() string { return "token-1" },
	}

	_, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1", Source: "self_decrypt"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if locks.acquireKey != "archive-sync:lock:ent-1|self_decrypt" || locks.acquireToken != "token-1" || locks.acquireTTL != 45*time.Second {
		t.Fatalf("lock acquire = %#v", locks)
	}
	if locks.releaseKey != locks.acquireKey || locks.releaseToken != "token-1" {
		t.Fatalf("lock release = %#v", locks)
	}
	if strings.Join(order, ",") != "acquire,pull,release" {
		t.Fatalf("order = %#v", order)
	}
}

func TestRunOnceSkipsWhenDistributedLockHeld(t *testing.T) {
	locks := &fakeLockStore{acquired: false}
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("20")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors:      &fakeCursorStore{cursor: "10"},
		Puller:       puller,
		Locks:        locks,
		NewLockToken: func() string { return "token-1" },
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1", Source: "self_decrypt"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "distributed_lock_held" || result.Cursor == nil || *result.Cursor != "10" {
		t.Fatalf("result = %#v", result)
	}
	if len(puller.inputs) != 0 || locks.releaseKey != "" {
		t.Fatalf("puller=%#v locks=%#v", puller, locks)
	}
}

func TestRunOnceContinuesWhenDistributedLockAcquireErrors(t *testing.T) {
	locks := &fakeLockStore{acquireErr: errors.New("redis down")}
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("20")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors:      &fakeCursorStore{},
		Puller:       puller,
		Locks:        locks,
		NewLockToken: func() string { return "token-1" },
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1", Source: "self_decrypt"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Skipped || len(puller.inputs) != 1 || locks.releaseKey != "" {
		t.Fatalf("result=%#v inputs=%#v locks=%#v", result, puller.inputs, locks)
	}
}

func TestRunOnceUsesEnterpriseSourceWhenRequestSourceBlank(t *testing.T) {
	puller := &fakePuller{result: archivepull.Result{Source: "provider", Cursor: stringPtr("2")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveMode:    "self_decrypt",
			ArchiveSource:  "provider",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: &fakeCursorStore{},
		Puller:  puller,
	}

	_, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if puller.input.Source != "provider" {
		t.Fatalf("source = %q", puller.input.Source)
	}
}

func TestRunOnceUsesDefaultPullerWhenDefaultEnterpriseMissing(t *testing.T) {
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("2")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{},
		Cursors:     &fakeCursorStore{},
		Puller:      puller,
	}

	result, err := runner.RunOnce(context.Background(), Request{})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Skipped || len(puller.inputs) != 1 || puller.input.EnterpriseID != "default" {
		t.Fatalf("result=%#v pull inputs=%#v", result, puller.inputs)
	}
}

func TestRunOnceAllowsEnterprisePullURLWithoutCorpID(t *testing.T) {
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("2")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: &fakeCursorStore{},
		Puller:  puller,
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Skipped || len(puller.inputs) != 1 {
		t.Fatalf("result=%#v inputs=%#v", result, puller.inputs)
	}
}

func TestRunOnceSkipsIncompleteConfig(t *testing.T) {
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{EnterpriseID: "ent-1", Enabled: true}},
		Cursors:     &fakeCursorStore{cursor: "10"},
		Puller:      &fakePuller{},
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "self_decrypt_pull_url_missing" || result.Cursor == nil || *result.Cursor != "10" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunOnceRequiresIngestTargetForPulledMessages(t *testing.T) {
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: &fakeCursorStore{},
		Puller: &fakePuller{result: archivepull.Result{
			Source:   "self_decrypt",
			Cursor:   stringPtr("20"),
			Messages: []map[string]any{{"archive_msgid": "msg-1"}},
		}},
	}

	_, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err == nil || !strings.Contains(err.Error(), "archive ingest is not configured") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunOnceStagesMessagesBeforeAdvancingCursor(t *testing.T) {
	order := []string{}
	cursorStore := &fakeCursorStore{}
	taskStore := &fakeTaskStore{taskID: "ait-1", order: &order}
	cursorStore.order = &order
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: cursorStore,
		Puller: &fakePuller{result: archivepull.Result{
			Source:   "self_decrypt",
			Cursor:   stringPtr("20"),
			Messages: []map[string]any{{"seq": 20, "archive_msgid": "msg-1"}},
		}},
		Tasks: taskStore,
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.StagedTaskID != "ait-1" || result.PulledTotal != 1 {
		t.Fatalf("result = %#v", result)
	}
	if taskStore.input.EnterpriseID != "ent-1" || taskStore.input.Cursor != "20" || len(taskStore.input.MessagesPayload) != 1 {
		t.Fatalf("task input = %#v", taskStore.input)
	}
	if cursorStore.upsertCursor != "20" || strings.Join(order, ",") != "stage,cursor" {
		t.Fatalf("order=%#v cursor=%#v", order, cursorStore)
	}
}

func TestRunOnceDoesNotAdvanceCursorWhenStageFails(t *testing.T) {
	cursorStore := &fakeCursorStore{}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: cursorStore,
		Puller: &fakePuller{result: archivepull.Result{
			Source:   "self_decrypt",
			Cursor:   stringPtr("20"),
			Messages: []map[string]any{{"seq": 20, "archive_msgid": "msg-1"}},
		}},
		Tasks: &fakeTaskStore{err: errors.New("stage down")},
	}

	_, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err == nil || !strings.Contains(err.Error(), "stage down") {
		t.Fatalf("error = %v", err)
	}
	if cursorStore.upsertCursor != "" {
		t.Fatalf("cursor store = %#v", cursorStore)
	}
}

func TestRunOnceFallsBackToDirectIngestor(t *testing.T) {
	cursorStore := &fakeCursorStore{}
	ingestor := &fakeIngestor{}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors:  cursorStore,
		Puller:   &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("20"), Messages: []map[string]any{{"archive_msgid": "msg-1"}}}},
		Ingestor: ingestor,
	}

	result, err := runner.RunOnce(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.StagedTaskID != "" || len(ingestor.messages) != 1 || cursorStore.upsertCursor != "20" {
		t.Fatalf("result=%#v ingestor=%#v cursor=%#v", result, ingestor, cursorStore)
	}
}

func TestTriggerArchiveSyncRunsOnce(t *testing.T) {
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("20")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: &fakeCursorStore{},
		Puller:  puller,
	}

	err := runner.TriggerArchiveSync(context.Background(), outboxarchivesync.Request{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Cursor:       stringPtr("15"),
		Limit:        77,
		Reason:       "archive_primary_device_hint",
	})
	if err != nil {
		t.Fatalf("TriggerArchiveSync returned error: %v", err)
	}
	if puller.input.EnterpriseID != "ent-1" || puller.input.Cursor == nil || *puller.input.Cursor != "15" || puller.input.Limit != 77 {
		t.Fatalf("pull input = %#v", puller.input)
	}
}

func TestRunScopeOnceListsEnabledEnterprisesAndContinuesFullBatches(t *testing.T) {
	store := fakeEnterpriseStore{
		enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", Enabled: true, CorpID: "corp-a", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-a"},
			{EnterpriseID: "ent-b", Enabled: true, CorpID: "corp-b", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-b"},
		},
	}
	puller := &fakePuller{results: []archivepull.Result{
		{Source: "self_decrypt", Cursor: stringPtr("2"), Messages: []map[string]any{{"seq": 1}, {"seq": 2}}},
		{Source: "self_decrypt", Cursor: stringPtr("3"), Messages: []map[string]any{{"seq": 3}}},
		{Source: "self_decrypt", Cursor: stringPtr("10")},
	}}
	runner := Runner{
		Enterprises: store,
		Cursors:     &fakeCursorStore{},
		Puller:      puller,
		Tasks:       &fakeTaskStore{taskID: "ait-scope"},
	}

	result, err := runner.RunScopeOnce(context.Background(), ScopeRequest{Limit: 2, MaxRounds: 4, TriggerReason: "scope_catch_up"})
	if err != nil {
		t.Fatalf("RunScopeOnce returned error: %v", err)
	}
	if result.ProcessedCount() != 3 || len(result.Results) != 3 || len(result.Failures) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(puller.inputs) != 3 || puller.inputs[0].EnterpriseID != "ent-a" || puller.inputs[1].EnterpriseID != "ent-a" || puller.inputs[2].EnterpriseID != "ent-b" {
		t.Fatalf("pull inputs = %#v", puller.inputs)
	}
	if puller.inputs[0].Limit != 2 || puller.inputs[2].PullURL != "https://archive.example/pull-b" {
		t.Fatalf("pull input details = %#v", puller.inputs)
	}
}

func TestRunScopeOnceExplicitEnterpriseDoesNotRequireLister(t *testing.T) {
	puller := &fakePuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("2")}}
	runner := Runner{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			ArchivePullURL: "https://archive.example/pull",
		}},
		Cursors: &fakeCursorStore{},
		Puller:  puller,
	}

	result, err := runner.RunScopeOnce(context.Background(), ScopeRequest{EnterpriseID: " ent-1 ", Limit: 1})
	if err != nil {
		t.Fatalf("RunScopeOnce returned error: %v", err)
	}
	if result.ProcessedCount() != 1 || len(puller.inputs) != 1 || puller.inputs[0].EnterpriseID != "ent-1" {
		t.Fatalf("result=%#v inputs=%#v", result, puller.inputs)
	}
}

func TestRunScopeOnceRecordsFailureAndContinues(t *testing.T) {
	store := fakeEnterpriseStore{
		enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", Enabled: true, CorpID: "corp-a", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-a"},
			{EnterpriseID: "ent-b", Enabled: true, CorpID: "corp-b", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-b"},
		},
	}
	puller := &fakePuller{
		results: []archivepull.Result{{Source: "self_decrypt", Cursor: stringPtr("2")}},
		errs:    []error{errors.New("bridge down"), nil},
	}
	runner := Runner{
		Enterprises: store,
		Cursors:     &fakeCursorStore{},
		Puller:      puller,
	}

	result, err := runner.RunScopeOnce(context.Background(), ScopeRequest{Limit: 1, MaxRounds: 1})
	if err != nil {
		t.Fatalf("RunScopeOnce returned error: %v", err)
	}
	if len(result.Failures) != 1 || result.Failures[0].EnterpriseID != "ent-a" || len(result.Results) != 1 || result.Results[0].EnterpriseID != "ent-b" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunScopeOnceRunsEnterpriseTargetsConcurrently(t *testing.T) {
	store := fakeEnterpriseStore{
		enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", Enabled: true, CorpID: "corp-a", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-a"},
			{EnterpriseID: "ent-b", Enabled: true, CorpID: "corp-b", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-b"},
			{EnterpriseID: "ent-c", Enabled: true, CorpID: "corp-c", ArchiveSource: "self_decrypt", ArchivePullURL: "https://archive.example/pull-c"},
		},
	}
	puller := newBlockingPuller(2)
	runner := Runner{
		Enterprises: store,
		Cursors:     &fakeCursorStore{},
		Puller:      puller,
	}

	done := make(chan scopeRunOutput, 1)
	go func() {
		result, err := runner.RunScopeOnce(context.Background(), ScopeRequest{Limit: 1, MaxRounds: 1, Concurrency: 2})
		done <- scopeRunOutput{result: result, err: err}
	}()

	released := false
	release := func() {
		if !released {
			close(puller.release)
			released = true
		}
	}
	defer release()
	select {
	case <-puller.ready:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("scope targets did not run concurrently")
	}
	release()
	select {
	case output := <-done:
		if output.err != nil {
			t.Fatalf("RunScopeOnce returned error: %v", output.err)
		}
		if output.result.ProcessedCount() != 3 || len(output.result.Results) != 3 || puller.maxActive() < 2 {
			t.Fatalf("result=%#v max_active=%d", output.result, puller.maxActive())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunScopeOnce did not finish")
	}
}

type fakeEnterpriseStore struct {
	enterprise  *enterprisestore.ArchivePullEnterprise
	enterprises []enterprisestore.ArchivePullEnterprise
	err         error
	listErr     error
}

func (store fakeEnterpriseStore) GetArchivePullEnterprise(_ context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error) {
	if store.err != nil {
		return nil, store.err
	}
	if store.enterprise != nil {
		return store.enterprise, nil
	}
	for _, enterprise := range store.enterprises {
		if enterprise.EnterpriseID == strings.TrimSpace(enterpriseID) {
			record := enterprise
			return &record, nil
		}
	}
	return nil, nil
}

func (store fakeEnterpriseStore) ListEnabledArchivePullEnterprises(context.Context) ([]enterprisestore.ArchivePullEnterprise, error) {
	if store.listErr != nil {
		return nil, store.listErr
	}
	return append([]enterprisestore.ArchivePullEnterprise(nil), store.enterprises...), nil
}

type fakeCursorStore struct {
	cursor             string
	upsertSource       string
	upsertCursor       string
	upsertEnterpriseID string
	order              *[]string
}

func (store *fakeCursorStore) GetCursor(context.Context, string, string) (*archivesynccursor.Record, error) {
	if strings.TrimSpace(store.cursor) == "" {
		return nil, nil
	}
	return &archivesynccursor.Record{Source: "self_decrypt", Cursor: store.cursor}, nil
}

func (store *fakeCursorStore) UpsertCursor(ctx context.Context, source string, cursor string, enterpriseID string) (archivesynccursor.Record, error) {
	if store.order != nil {
		*store.order = append(*store.order, "cursor")
	}
	store.upsertSource = source
	store.upsertCursor = cursor
	store.upsertEnterpriseID = enterpriseID
	return archivesynccursor.Record{Source: source, Cursor: cursor}, nil
}

type fakePuller struct {
	input   archivepull.PullInput
	inputs  []archivepull.PullInput
	result  archivepull.Result
	results []archivepull.Result
	errs    []error
	order   *[]string
}

type scopeRunOutput struct {
	result ScopeResult
	err    error
}

type blockingPuller struct {
	mu        sync.Mutex
	active    int
	max       int
	want      int
	ready     chan struct{}
	readyOnce sync.Once
	release   chan struct{}
}

func newBlockingPuller(want int) *blockingPuller {
	return &blockingPuller{
		want:    want,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (puller *blockingPuller) PullSelfDecrypt(ctx context.Context, input archivepull.PullInput) (archivepull.Result, error) {
	puller.mu.Lock()
	puller.active++
	if puller.active > puller.max {
		puller.max = puller.active
	}
	if puller.active >= puller.want {
		puller.readyOnce.Do(func() { close(puller.ready) })
	}
	puller.mu.Unlock()
	select {
	case <-ctx.Done():
		return archivepull.Result{}, ctx.Err()
	case <-puller.release:
	}
	puller.mu.Lock()
	puller.active--
	puller.mu.Unlock()
	return archivepull.Result{Source: input.Source}, nil
}

func (puller *blockingPuller) maxActive() int {
	puller.mu.Lock()
	defer puller.mu.Unlock()
	return puller.max
}

func (puller *fakePuller) PullSelfDecrypt(ctx context.Context, input archivepull.PullInput) (archivepull.Result, error) {
	if puller.order != nil {
		*puller.order = append(*puller.order, "pull")
	}
	puller.input = input
	puller.inputs = append(puller.inputs, input)
	if len(puller.errs) > 0 {
		err := puller.errs[0]
		puller.errs = puller.errs[1:]
		if err != nil {
			return archivepull.Result{}, err
		}
	}
	if len(puller.results) > 0 {
		result := puller.results[0]
		puller.results = puller.results[1:]
		return result, nil
	}
	return puller.result, nil
}

type fakeTaskStore struct {
	input  archiveingesttask.EnqueueBatchInput
	inputs []archiveingesttask.EnqueueBatchInput
	taskID string
	err    error
	order  *[]string
}

func (store *fakeTaskStore) EnqueueBatch(ctx context.Context, input archiveingesttask.EnqueueBatchInput) (archiveingesttask.Record, error) {
	if store.order != nil {
		*store.order = append(*store.order, "stage")
	}
	store.input = input
	store.inputs = append(store.inputs, input)
	if store.err != nil {
		return archiveingesttask.Record{}, store.err
	}
	return archiveingesttask.Record{TaskID: store.taskID}, nil
}

type fakeIngestor struct {
	messages []map[string]any
}

func (ingestor *fakeIngestor) IngestArchivePull(ctx context.Context, enterprise enterprisestore.ArchivePullEnterprise, result archivepull.Result) error {
	ingestor.messages = append([]map[string]any(nil), result.Messages...)
	return nil
}

type fakeLockStore struct {
	acquired     bool
	acquireErr   error
	acquireKey   string
	acquireToken string
	acquireTTL   time.Duration
	refreshKey   string
	refreshToken string
	refreshTTL   time.Duration
	releaseKey   string
	releaseToken string
	order        *[]string
}

func (store *fakeLockStore) AcquireArchiveSyncLock(_ context.Context, key string, token string, ttl time.Duration) (bool, error) {
	if store.order != nil {
		*store.order = append(*store.order, "acquire")
	}
	store.acquireKey = key
	store.acquireToken = token
	store.acquireTTL = ttl
	if store.acquireErr != nil {
		return false, store.acquireErr
	}
	return store.acquired, nil
}

func (store *fakeLockStore) RefreshArchiveSyncLock(_ context.Context, key string, token string, ttl time.Duration) error {
	store.refreshKey = key
	store.refreshToken = token
	store.refreshTTL = ttl
	return nil
}

func (store *fakeLockStore) ReleaseArchiveSyncLock(_ context.Context, key string, token string) error {
	if store.order != nil {
		*store.order = append(*store.order, "release")
	}
	store.releaseKey = key
	store.releaseToken = token
	return nil
}

func stringPtr(value string) *string {
	return &value
}
