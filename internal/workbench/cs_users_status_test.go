package workbench

import (
	"context"
	"testing"
	"time"

	"im-go/internal/auth"
)

func TestServiceCSUsersStatusBuildsOnlinePayload(t *testing.T) {
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-001", AssigneeName: "消息端A", Role: "admin", Enabled: true, AIEnabled: true, LastSeenAt: time.Date(2026, 6, 29, 9, 58, 0, 0, time.UTC)},
			{AssigneeID: "cs-002", AssigneeName: "消息端B", Role: "cs", Enabled: false, AIEnabled: false, LastSeenAt: "2026-06-29T09:40:00Z"},
			{AssigneeID: "", AssigneeName: "跳过", Role: "cs", Enabled: true},
		}},
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.CSUsersStatus(context.Background(), CSUsersStatusRequest{Session: auth.Session{Role: "admin"}})
	if err != nil {
		t.Fatalf("CSUsersStatus returned error: %v", err)
	}
	status := payload["status"].([]ProjectionRow)
	if len(status) != 2 {
		t.Fatalf("status = %+v, want two rows", status)
	}
	if rowText(status[0], "assignee_id") != "cs-001" || status[0]["is_online"] != true || status[0]["last_seen_at"] == nil {
		t.Fatalf("unexpected first status row: %+v", status[0])
	}
	if rowText(status[1], "assignee_id") != "cs-002" || status[1]["is_online"] != false || status[1]["enabled"] != false {
		t.Fatalf("unexpected second status row: %+v", status[1])
	}
	if _, ok := status[0]["has_password"]; ok {
		t.Fatalf("status payload leaked list-only fields: %+v", status[0])
	}
}

func TestServiceCSUsersStatusFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).CSUsersStatus(context.Background(), CSUsersStatusRequest{})
	if err != ErrCSUserStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrCSUserStoreUnavailable)
	}
}
