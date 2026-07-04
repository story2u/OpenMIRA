package weworkuserinfo

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/workbench"
)

func TestServiceReturnsCandidatesFromLoginIdentity(t *testing.T) {
	candidateStore := &fakeCandidateStore{responses: [][]InternalUserCandidate{{
		{EnterpriseID: "ent-1", UserID: "zhangsan", Name: " 张三 ", DepartmentJSON: []any{float64(1)}, Position: "dev", Avatar: "avatar-a", SyncedAt: "2026-07-01 08:00:00", UpdatedAt: "2026-07-01 09:00:00"},
		{EnterpriseID: "ent-1", UserID: "zhangsan2", Name: "张三", DepartmentJSON: nil, Position: "ops", Avatar: "avatar-b"},
	}}}
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:         "device-1",
			AccountName:      " 张三 ",
			OrganizationName: "企微组织",
		}}},
		Enterprises: fakeEnterpriseStore{records: []EnterpriseRecord{{
			EnterpriseID: "ent-1",
			Name:         "企微组织有限公司",
			CorpID:       "ww-ent",
		}}},
		UserCandidates: candidateStore,
	}

	result, err := service.Candidates(context.Background(), " device-1 ", 20)
	if err != nil {
		t.Fatalf("Candidates returned error: %v", err)
	}
	if !result.Success || !result.RequiresSelection || result.DeviceID != "device-1" || result.AccountName != "张三" || result.OrganizationName != "企微组织" || result.EnterpriseID != "ent-1" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].UserID != "zhangsan" || result.Candidates[1].DepartmentJSON == nil {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if len(candidateStore.calls) != 1 || candidateStore.calls[0].enterpriseID != "ent-1" || candidateStore.calls[0].names[0] != "张三" {
		t.Fatalf("candidate calls = %#v", candidateStore.calls)
	}
}

func TestServiceFallsBackToGeneratedNameCandidates(t *testing.T) {
	candidateStore := &fakeCandidateStore{responses: [][]InternalUserCandidate{
		{},
		{{EnterpriseID: "ent-1", UserID: "lisi", Name: "李四"}},
	}}
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:         "device-1",
			AccountName:      "李四-ab1234",
			OrganizationName: "ww-ent",
		}}},
		Enterprises:    fakeEnterpriseStore{records: []EnterpriseRecord{{EnterpriseID: "ent-1", CorpID: "ww-ent", Name: "企业"}}},
		UserCandidates: candidateStore,
	}

	result, err := service.Candidates(context.Background(), "device-1", 100)
	if err != nil {
		t.Fatalf("Candidates returned error: %v", err)
	}
	if result.RequiresSelection || len(result.Candidates) != 1 || result.Candidates[0].UserID != "lisi" {
		t.Fatalf("result = %+v", result)
	}
	if len(candidateStore.calls) != 2 {
		t.Fatalf("candidate calls = %#v", candidateStore.calls)
	}
	if candidateStore.calls[1].limit != 50 {
		t.Fatalf("fallback limit = %d, want 50", candidateStore.calls[1].limit)
	}
	foundTrimmed := false
	for _, name := range candidateStore.calls[1].names {
		if name == "李四" {
			foundTrimmed = true
		}
	}
	if !foundTrimmed {
		t.Fatalf("fallback names = %#v", candidateStore.calls[1].names)
	}
}

func TestServiceReturnsEmptyWhenSnapshotIncompleteOrEnterpriseMissing(t *testing.T) {
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:    "device-1",
			AccountName: "张三",
		}}},
		Enterprises:    fakeEnterpriseStore{records: []EnterpriseRecord{{EnterpriseID: "ent-1", Name: "企业"}}},
		UserCandidates: &fakeCandidateStore{},
	}
	result, err := service.Candidates(context.Background(), "device-1", 20)
	if err != nil {
		t.Fatalf("Candidates returned error: %v", err)
	}
	if result.EnterpriseID != "" || len(result.Candidates) != 0 || result.RequiresSelection {
		t.Fatalf("incomplete result = %+v", result)
	}

	service.LoginSessions = fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
		DeviceID:         "device-1",
		AccountName:      "张三",
		OrganizationName: "未知组织",
	}}}
	result, err = service.Candidates(context.Background(), "device-1", 20)
	if err != nil {
		t.Fatalf("Candidates returned error: %v", err)
	}
	if result.EnterpriseID != "" || len(result.Candidates) != 0 {
		t.Fatalf("missing enterprise result = %+v", result)
	}
}

func TestServiceReportsRequiredStoreErrors(t *testing.T) {
	_, err := (Service{}).Candidates(context.Background(), "device-1", 20)
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrStoreUnavailable)
	}
}

type fakeLoginSessionStore struct {
	sessions []workbench.LoginSessionRecord
	err      error
}

func (store fakeLoginSessionStore) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error) {
	_ = ctx
	_ = deviceIDs
	return store.sessions, store.err
}

type fakeEnterpriseStore struct {
	records []EnterpriseRecord
	err     error
}

func (store fakeEnterpriseStore) ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error) {
	_ = ctx
	return store.records, store.err
}

type fakeCandidateStore struct {
	responses [][]InternalUserCandidate
	calls     []candidateCall
	err       error
}

type candidateCall struct {
	enterpriseID string
	names        []string
	limit        int
}

func (store *fakeCandidateStore) ListInternalUserCandidatesByNames(ctx context.Context, enterpriseID string, names []string, limit int) ([]InternalUserCandidate, error) {
	_ = ctx
	store.calls = append(store.calls, candidateCall{enterpriseID: enterpriseID, names: append([]string{}, names...), limit: limit})
	if store.err != nil {
		return nil, store.err
	}
	index := len(store.calls) - 1
	if index < len(store.responses) {
		return store.responses[index], nil
	}
	return []InternalUserCandidate{}, nil
}
