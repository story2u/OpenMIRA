package archivepull

import (
	"testing"
	"time"
)

func TestLimitClampAndHeaders(t *testing.T) {
	if ClampPullLimit(99999, 0) != 2000 {
		t.Fatalf("ClampPullLimit did not clamp to 2000")
	}
	if ClampSDKLimit(0, 0) != 1 {
		t.Fatalf("ClampSDKLimit did not clamp to 1")
	}
	if BuildAuthHeaders(" token-1 ")["Authorization"] != "Bearer token-1" {
		t.Fatalf("auth header mismatch")
	}
	if len(BuildAuthHeaders("")) != 0 {
		t.Fatalf("blank auth header should be empty")
	}
}

func TestBuildPullPayloadKeepsOptionalFields(t *testing.T) {
	cursor := "12"
	payload := BuildPullPayload("official", &cursor, 3000, "ent-1", "official")

	if payload["limit"] != 2000 || payload["enterprise_id"] != "ent-1" || payload["archive_mode"] != "official" || payload["cursor"] != "12" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNormalizePullResponse(t *testing.T) {
	result := NormalizePullResponse(map[string]any{
		"source":   "official",
		"cursor":   20,
		"messages": []any{map[string]any{"id": float64(1)}},
	}, "self_decrypt")

	if result.Source != "official" || result.Cursor == nil || *result.Cursor != "20" || len(result.Messages) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildArchiveMessageExtractsInternalAndExternalIdentity(t *testing.T) {
	nowISO := time.Now().UTC().Format(time.RFC3339Nano)
	message := BuildArchiveMessage(BuildMessageInput{
		RawItem: map[string]any{"seq": 10},
		Decrypted: map[string]any{
			"from":    "wm_external_001",
			"tolist":  []any{"zhangsan"},
			"msgtype": "text",
			"text":    map[string]any{"content": "你好"},
			"msgtime": int64(1710000000),
		},
		ArchiveMsgID:     "msg-1",
		ItemSeq:          10,
		NowISO:           nowISO,
		EnterpriseID:     "ent-1",
		PublicKeyVersion: 3,
		EncryptRandomKey: "rk",
		EncryptChatMsg:   "ecm",
	})

	if message["device_id"] != "archive_user:zhangsan" || message["direction"] != "incoming" || message["external_userid"] != "wm_external_001" || message["content"] != "你好" {
		t.Fatalf("message = %#v", message)
	}
}

func TestBuildArchiveMessageHandlesEventPayload(t *testing.T) {
	nowISO := time.Now().UTC().Format(time.RFC3339Nano)
	message := BuildArchiveMessage(BuildMessageInput{
		RawItem:          map[string]any{"seq": 11},
		Decrypted:        map[string]any{"from": "lisi", "action": "switch", "tolist": []any{}},
		ArchiveMsgID:     "msg-2",
		ItemSeq:          11,
		NowISO:           nowISO,
		EncryptRandomKey: "",
		EncryptChatMsg:   "",
	})

	if message["msg_type_raw"] != "event" || message["content"] != "[会话事件] switch" || message["device_id"] != "archive_user:lisi" || message["is_system_event"] != true {
		t.Fatalf("message = %#v", message)
	}
}

func TestBuildArchiveMessageDisplayTextVariants(t *testing.T) {
	nowISO := time.Now().UTC().Format(time.RFC3339Nano)
	cases := []struct {
		name      string
		decrypted map[string]any
		want      string
	}{
		{
			name:      "weapp payment",
			decrypted: map[string]any{"from": "zhangsan", "msgtype": "weapp", "weapp": map[string]any{"title": "付款给 张三"}},
			want:      "付款给：张三",
		},
		{
			name:      "location title",
			decrypted: map[string]any{"from": "zhangsan", "msgtype": "location", "location": map[string]any{"title": "萤火虫大厦"}},
			want:      "门店位置：萤火虫大厦",
		},
		{
			name:      "map shortlink",
			decrypted: map[string]any{"from": "zhangsan", "msgtype": "link", "link": map[string]any{"title": "门店", "link_url": "https://mmapgwh.map.qq.com/shortlink/x"}},
			want:      "门店位置：门店",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			message := BuildArchiveMessage(BuildMessageInput{Decrypted: tt.decrypted, ArchiveMsgID: "msg-1", NowISO: nowISO})
			if message["content"] != tt.want {
				t.Fatalf("content = %#v, want %q", message["content"], tt.want)
			}
		})
	}
}
