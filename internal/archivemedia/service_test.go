package archivemedia

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/infra/archivemediatask"
)

func TestServiceRunOnceUploadsFinishedMediaAndNotifies(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{
		Response:  map[string]any{"is_finish": true},
		Content:   []byte("media"),
		NextIndex: "idx-next",
		IsFinish:  true,
	}}}
	storage := &fakeStorage{objectURL: "https://objects/ent-1/am-1.bin"}
	notifier := &fakeNotifier{}

	result, err := (Service{
		Store:    store,
		Puller:   puller,
		Storage:  storage,
		Notifier: notifier,
		Now:      func() time.Time { return now },
	}).RunOnce(context.Background(), " ent-1 ", " self_decrypt ")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Total != 1 || result.Success != 1 || result.Pending != 0 || result.Failed != 0 || result.Requeued != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(storage.uploads) != 1 || string(storage.uploads[0].Content) != "media" {
		t.Fatalf("uploads = %#v", storage.uploads)
	}
	if len(store.updates) != 1 || store.updates[0].Status != archivemediatask.StatusSuccess || !store.updates[0].IsFinish || store.updates[0].ObjectURL != "https://objects/ent-1/am-1.bin" {
		t.Fatalf("updates = %#v", store.updates)
	}
	if len(notifier.events) != 1 || notifier.events[0].MediaTaskID != "amt-1" {
		t.Fatalf("notifications = %#v", notifier.events)
	}
}

func TestArchiveMediaLockKeyUsesEnterpriseAndSourceOnly(t *testing.T) {
	if got := ArchiveMediaScopeKey(" ent-1 ", " self_decrypt "); got != "ent-1|self_decrypt" {
		t.Fatalf("scope key = %q", got)
	}
	if got := ArchiveMediaLockKey("ent-1", "self_decrypt"); got != "archive-media:lock:ent-1|self_decrypt" {
		t.Fatalf("lock key = %q", got)
	}
}

func TestServiceRunOnceUsesDistributedLockAroundScope(t *testing.T) {
	order := []string{}
	locks := &fakeLockStore{acquired: true, order: &order}
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}, order: &order}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.bin", IsFinish: true}}, order: &order}

	_, err := (Service{
		Store:        store,
		Puller:       puller,
		Locks:        locks,
		LockTTL:      45 * time.Second,
		NewLockToken: func() string { return "token-1" },
	}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if locks.acquireKey != "archive-media:lock:ent-1|self_decrypt" || locks.acquireToken != "token-1" || locks.acquireTTL != 45*time.Second {
		t.Fatalf("lock acquire = %#v", locks)
	}
	if locks.releaseKey != locks.acquireKey || locks.releaseToken != "token-1" {
		t.Fatalf("lock release = %#v", locks)
	}
	if got := joinOrder(order); got != "acquire,requeue,claim,pull,release" {
		t.Fatalf("order = %#v", order)
	}
}

func TestServiceRunOnceSkipsWhenDistributedLockHeld(t *testing.T) {
	locks := &fakeLockStore{acquired: false}
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.bin", IsFinish: true}}}

	result, err := (Service{
		Store:        store,
		Puller:       puller,
		Locks:        locks,
		NewLockToken: func() string { return "token-1" },
	}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "distributed_lock_held" || result.Total != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(store.requeueCalls) != 0 || len(store.claimCalls) != 0 || len(puller.results) != 1 || locks.releaseKey != "" {
		t.Fatalf("store=%#v puller=%#v locks=%#v", store, puller, locks)
	}
}

func TestServiceRunOnceContinuesWhenDistributedLockAcquireErrors(t *testing.T) {
	locks := &fakeLockStore{acquireErr: errors.New("redis down")}
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.bin", IsFinish: true}}}

	result, err := (Service{
		Store:        store,
		Puller:       puller,
		Locks:        locks,
		NewLockToken: func() string { return "token-1" },
	}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Skipped || result.Total != 1 || len(store.claimCalls) != 1 || locks.releaseKey != "" {
		t.Fatalf("result=%#v store=%#v locks=%#v", result, store, locks)
	}
}

func TestServiceRunOnceHydratesMediaReadyMessageContext(t *testing.T) {
	messageTime := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.bin", IsFinish: true}}}
	notifier := &fakeNotifier{}
	messages := &fakeMessageLookup{context: MessageContext{
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		DeviceID:       "dev-1",
		SenderID:       "sender-1",
		SenderName:     "Alice",
		MsgType:        "image",
		Direction:      "incoming",
		Timestamp:      messageTime,
		CreatedAt:      messageTime,
	}, ok: true}

	result, err := (Service{Store: store, Puller: puller, Notifier: notifier, Messages: messages}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Success != 1 || len(notifier.events) != 1 {
		t.Fatalf("result=%#v notifications=%#v", result, notifier.events)
	}
	event := notifier.events[0]
	if event.ConversationID != "conv-1" || event.TraceID != "trace-1" || event.MsgType != "image" || !event.Timestamp.Equal(messageTime) {
		t.Fatalf("event = %#v", event)
	}
	if messages.tenantID != "ent-1" || messages.archiveMsgID != "am-1" {
		t.Fatalf("lookup scope = %q/%q", messages.tenantID, messages.archiveMsgID)
	}
}

func TestServiceRunOnceEnqueuesVoiceTranscriptionBeforeMediaReady(t *testing.T) {
	order := []string{}
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.amr", IsFinish: true}}}
	notifier := &fakeNotifier{order: &order}
	voice := &fakeVoiceTranscription{order: &order, err: errors.New("voice enqueue down")}
	messages := &fakeMessageLookup{context: MessageContext{
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		MsgType:        "voice",
		Direction:      "incoming",
	}, ok: true}

	result, err := (Service{Store: store, Puller: puller, Notifier: notifier, Messages: messages, VoiceTranscription: voice}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Success != 1 || len(notifier.events) != 1 || len(voice.inputs) != 1 {
		t.Fatalf("result=%#v notifications=%#v voice=%#v", result, notifier.events, voice.inputs)
	}
	if voice.inputs[0].ConversationID != "conv-1" || voice.inputs[0].ArchiveMsgID != "am-1" || voice.inputs[0].ObjectURL != "https://objects/ent-1/am-1.amr" {
		t.Fatalf("voice input = %#v", voice.inputs[0])
	}
	if len(order) != 2 || order[0] != "voice" || order[1] != "notify" {
		t.Fatalf("order = %#v", order)
	}
}

func TestServiceRunOnceMarksRetryableWhenMessageLookupFails(t *testing.T) {
	expected := errors.New("message db down")
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/ent-1/am-1.bin", IsFinish: true}}}
	notifier := &fakeNotifier{}
	messages := &fakeMessageLookup{err: expected}

	result, err := (Service{Store: store, Puller: puller, Notifier: notifier, Messages: messages}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Failed != 1 || len(notifier.events) != 0 {
		t.Fatalf("result=%#v notifications=%#v", result, notifier.events)
	}
	if len(store.updates) != 1 || store.updates[0].Status != archivemediatask.StatusFailedRetryable || store.updates[0].LastError != expected.Error() {
		t.Fatalf("updates = %#v", store.updates)
	}
}

func TestServiceRunOnceStoresRunningProgressWithoutUpload(t *testing.T) {
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}}
	puller := &fakePuller{results: []PullResult{{
		Response:  map[string]any{"is_finish": false, "next": "idx-2"},
		NextIndex: "idx-2",
		IsFinish:  false,
	}}}

	result, err := (Service{Store: store, Puller: puller}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Pending != 1 || result.Success != 0 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(store.updates) != 1 || store.updates[0].Status != archivemediatask.StatusRunning || store.updates[0].IndexBuf != "idx-2" || store.updates[0].IsFinish {
		t.Fatalf("updates = %#v", store.updates)
	}
}

func TestServiceRunOnceMarksRetryableFailureWithBackoff(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 1)}}
	puller := &fakePuller{err: errors.New("bridge unavailable")}

	result, err := (Service{
		Store:               store,
		Puller:              puller,
		RetryBackoffBaseSec: 10,
		RetryBackoffMaxSec:  60,
		RetryMaxAttempts:    8,
		Now:                 func() time.Time { return now },
	}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	update := store.updates[0]
	if update.Status != archivemediatask.StatusFailedRetryable || update.RetryCount != 2 || update.NextRetryAt == nil {
		t.Fatalf("update = %#v", update)
	}
	if !update.NextRetryAt.Equal(now.Add(20 * time.Second)) {
		t.Fatalf("next retry = %s", update.NextRetryAt)
	}
}

func TestServiceRunOnceMarksTerminalWhenRetryLimitReached(t *testing.T) {
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 7)}}
	puller := &fakePuller{err: errors.New("still failing")}

	result, err := (Service{Store: store, Puller: puller, RetryMaxAttempts: 8}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	update := store.updates[0]
	if update.Status != archivemediatask.StatusFailedTerminal || update.RetryCount != 7 || update.NextRetryAt != nil {
		t.Fatalf("update = %#v", update)
	}
}

func TestServiceRunOnceReturnsStoreErrors(t *testing.T) {
	expected := errors.New("store down")
	store := &fakeTaskStore{tasks: []archivemediatask.Record{mediaTask("amt-1", 0)}, updateErr: expected}
	puller := &fakePuller{results: []PullResult{{ObjectURL: "https://objects/existing.bin", IsFinish: true}}}

	_, err := (Service{Store: store, Puller: puller}).RunOnce(context.Background(), "ent-1", "self_decrypt")
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceRunTaskProcessesClaimedTask(t *testing.T) {
	store := &fakeTaskStore{tasks: []archivemediatask.Record{{
		TaskID:       "amt-1",
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		ArchiveMsgID: "am-1",
		SDKFileID:    "sdk-1",
		Status:       archivemediatask.StatusPending,
	}}}
	puller := &fakePuller{results: []PullResult{{Content: []byte("voice-bytes"), IsFinish: true, Response: map[string]any{"is_finish": true}}}}
	storage := &fakeStorage{objectURL: "https://objects/ent-1/am-1.amr"}

	result, err := (Service{Store: store, Puller: puller, Storage: storage}).RunTask(context.Background(), "amt-1")
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Total != 1 || result.Success != 1 || store.claimTaskID != "amt-1" {
		t.Fatalf("result=%#v claim=%q", result, store.claimTaskID)
	}
	if len(store.updates) != 1 || store.updates[0].Status != archivemediatask.StatusSuccess || store.updates[0].ObjectURL != "https://objects/ent-1/am-1.amr" {
		t.Fatalf("updates = %#v", store.updates)
	}
}

func TestServiceRunTaskReturnsFinishedTaskWithoutPull(t *testing.T) {
	store := &fakeTaskStore{tasks: []archivemediatask.Record{{
		TaskID:       "amt-finished",
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		IsFinish:     true,
		Status:       archivemediatask.StatusSuccess,
		ObjectURL:    "https://objects/ent-1/existing.amr",
	}}}
	puller := &fakePuller{}

	result, err := (Service{Store: store, Puller: puller}).RunTask(context.Background(), "amt-finished")
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Success != 1 || len(store.updates) != 0 || len(puller.results) != 0 {
		t.Fatalf("result=%#v updates=%#v", result, store.updates)
	}
}

func TestServicePruneFinishedBeforeDeletesObjectsBeforeTasks(t *testing.T) {
	cutoff := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	order := []string{}
	store := &fakeTaskStore{
		finished: []archivemediatask.Record{
			{TaskID: "amt-1", ObjectURL: "http://object-storage:9102/objects/ent-1/file.png"},
			{TaskID: "amt-2"},
		},
		order: &order,
	}
	storage := &fakeStorage{order: &order}

	result, err := (Service{Store: store, Storage: storage}).PruneFinishedBefore(context.Background(), cutoff, 7)
	if err != nil {
		t.Fatalf("PruneFinishedBefore returned error: %v", err)
	}
	if result.Candidates != 2 || result.DeletedObjects != 1 || result.DeletedTasks != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(storage.deletedObjectURLs) != 2 || storage.deletedObjectURLs[0] != "http://object-storage:9102/objects/ent-1/file.png" || storage.deletedObjectURLs[1] != "" {
		t.Fatalf("deleted object urls = %#v", storage.deletedObjectURLs)
	}
	if len(store.deletedTaskIDs) != 2 || store.deletedTaskIDs[0] != "amt-1" || store.deletedTaskIDs[1] != "amt-2" {
		t.Fatalf("deleted task ids = %#v", store.deletedTaskIDs)
	}
	if !store.pruneCutoff.Equal(cutoff) || store.pruneBatchSize != 7 {
		t.Fatalf("prune cutoff=%s batch=%d", store.pruneCutoff, store.pruneBatchSize)
	}
	if got := joinOrder(order); got != "list_finished,delete_object,delete_object,delete_tasks" {
		t.Fatalf("order = %#v", order)
	}
}

func TestServicePruneFinishedBeforeDeletesTasksWithoutStorageDeleter(t *testing.T) {
	store := &fakeTaskStore{finished: []archivemediatask.Record{{TaskID: "amt-1", ObjectURL: "http://object-storage:9102/objects/ent-1/file.png"}}}

	result, err := (Service{Store: store}).PruneFinishedBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 0)
	if err != nil {
		t.Fatalf("PruneFinishedBefore returned error: %v", err)
	}
	if result.Candidates != 1 || result.DeletedObjects != 0 || result.DeletedTasks != 1 {
		t.Fatalf("result = %#v", result)
	}
}

type fakeTaskStore struct {
	tasks          []archivemediatask.Record
	finished       []archivemediatask.Record
	updates        []archivemediatask.UpdateInput
	requeueCalls   []archivemediatask.RequeueOptions
	claimCalls     []archivemediatask.ClaimOptions
	deletedTaskIDs []string
	claimTaskID    string
	pruneCutoff    time.Time
	pruneBatchSize int
	updateErr      error
	order          *[]string
}

func (store *fakeTaskStore) RequeueRetryable(ctx context.Context, options archivemediatask.RequeueOptions) (int, error) {
	if store.order != nil {
		*store.order = append(*store.order, "requeue")
	}
	store.requeueCalls = append(store.requeueCalls, options)
	return 1, nil
}

func (store *fakeTaskStore) ClaimPending(ctx context.Context, options archivemediatask.ClaimOptions) ([]archivemediatask.Record, error) {
	if store.order != nil {
		*store.order = append(*store.order, "claim")
	}
	store.claimCalls = append(store.claimCalls, options)
	return append([]archivemediatask.Record(nil), store.tasks...), nil
}

func (store *fakeTaskStore) ClaimTask(ctx context.Context, taskID string, processingLeaseSeconds int) (*archivemediatask.Record, error) {
	store.claimTaskID = taskID
	for index := range store.tasks {
		if store.tasks[index].TaskID == taskID {
			if !store.tasks[index].IsFinish {
				store.tasks[index].Status = archivemediatask.StatusRunning
			}
			return &store.tasks[index], nil
		}
	}
	return nil, nil
}

func (store *fakeTaskStore) UpdateProgress(ctx context.Context, input archivemediatask.UpdateInput) (*archivemediatask.Record, error) {
	store.updates = append(store.updates, input)
	if store.updateErr != nil {
		return nil, store.updateErr
	}
	return &archivemediatask.Record{TaskID: input.TaskID, Status: input.Status, ObjectURL: input.ObjectURL}, nil
}

func (store *fakeTaskStore) ReleaseClaimed(ctx context.Context, taskIDs []string) (int64, error) {
	return int64(len(taskIDs)), nil
}

func (store *fakeTaskStore) ListFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) ([]archivemediatask.Record, error) {
	if store.order != nil {
		*store.order = append(*store.order, "list_finished")
	}
	store.pruneCutoff = cutoff
	store.pruneBatchSize = batchSize
	return append([]archivemediatask.Record(nil), store.finished...), nil
}

func (store *fakeTaskStore) DeleteTasks(ctx context.Context, taskIDs []string) (int, error) {
	if store.order != nil {
		*store.order = append(*store.order, "delete_tasks")
	}
	store.deletedTaskIDs = append(store.deletedTaskIDs, taskIDs...)
	return len(taskIDs), nil
}

type fakePuller struct {
	results []PullResult
	err     error
	order   *[]string
}

func (puller *fakePuller) PullArchiveMedia(ctx context.Context, input PullInput) (PullResult, error) {
	if puller.order != nil {
		*puller.order = append(*puller.order, "pull")
	}
	if puller.err != nil {
		return PullResult{}, puller.err
	}
	if len(puller.results) == 0 {
		return PullResult{}, nil
	}
	result := puller.results[0]
	puller.results = puller.results[1:]
	return result, nil
}

type fakeStorage struct {
	objectURL         string
	uploads           []UploadInput
	deletedObjectURLs []string
	order             *[]string
}

func (storage *fakeStorage) UploadArchiveMedia(ctx context.Context, input UploadInput) (string, error) {
	storage.uploads = append(storage.uploads, input)
	return storage.objectURL, nil
}

func (storage *fakeStorage) DeleteArchiveMedia(ctx context.Context, objectURL string) (bool, error) {
	if storage.order != nil {
		*storage.order = append(*storage.order, "delete_object")
	}
	storage.deletedObjectURLs = append(storage.deletedObjectURLs, objectURL)
	return objectURL != "", nil
}

type fakeNotifier struct {
	events []MediaReadyEvent
	order  *[]string
}

func (notifier *fakeNotifier) NotifyArchiveMediaReady(ctx context.Context, event MediaReadyEvent) error {
	if notifier.order != nil {
		*notifier.order = append(*notifier.order, "notify")
	}
	notifier.events = append(notifier.events, event)
	return nil
}

type fakeMessageLookup struct {
	context      MessageContext
	ok           bool
	err          error
	tenantID     string
	archiveMsgID string
}

func (lookup *fakeMessageLookup) FindArchiveMessage(ctx context.Context, tenantID string, archiveMsgID string) (MessageContext, bool, error) {
	lookup.tenantID = tenantID
	lookup.archiveMsgID = archiveMsgID
	if lookup.err != nil {
		return MessageContext{}, false, lookup.err
	}
	return lookup.context, lookup.ok, nil
}

type fakeVoiceTranscription struct {
	inputs []VoiceTranscriptionInput
	err    error
	order  *[]string
}

func (voice *fakeVoiceTranscription) EnqueueVoiceTranscription(ctx context.Context, input VoiceTranscriptionInput) (bool, error) {
	if voice.order != nil {
		*voice.order = append(*voice.order, "voice")
	}
	voice.inputs = append(voice.inputs, input)
	if voice.err != nil {
		return false, voice.err
	}
	return true, nil
}

func mediaTask(taskID string, retryCount int) archivemediatask.Record {
	return archivemediatask.Record{
		TaskID:         taskID,
		EnterpriseID:   "ent-1",
		Source:         "self_decrypt",
		ArchiveMsgID:   "am-1",
		SDKFileID:      "sdk-1",
		Status:         archivemediatask.StatusRunning,
		PayloadJSON:    `{"payload":true}`,
		StorageBackend: "local",
		RetryCount:     retryCount,
	}
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

func (store *fakeLockStore) AcquireArchiveMediaLock(_ context.Context, key string, token string, ttl time.Duration) (bool, error) {
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

func (store *fakeLockStore) RefreshArchiveMediaLock(_ context.Context, key string, token string, ttl time.Duration) error {
	store.refreshKey = key
	store.refreshToken = token
	store.refreshTTL = ttl
	return nil
}

func (store *fakeLockStore) ReleaseArchiveMediaLock(_ context.Context, key string, token string) error {
	if store.order != nil {
		*store.order = append(*store.order, "release")
	}
	store.releaseKey = key
	store.releaseToken = token
	return nil
}

func joinOrder(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += ","
		}
		result += value
	}
	return result
}
