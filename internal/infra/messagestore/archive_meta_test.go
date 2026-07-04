package messagestore

import "testing"

func TestArchiveFallbackContentRichTypes(t *testing.T) {
	tests := []struct {
		name      string
		msgType   string
		decrypted map[string]any
		wantType  string
		wantText  string
	}{
		{
			name:    "map shortlink",
			msgType: "link",
			decrypted: map[string]any{"link": map[string]any{
				"title":    "深圳门店",
				"link_url": "https://mmapgwh.map.qq.com/shortlink/x",
			}},
			wantType: "text",
			wantText: "门店位置：深圳门店",
		},
		{
			name:      "weapp payment",
			msgType:   "weapp",
			decrypted: map[string]any{"weapp": map[string]any{"title": "付款给 张三"}},
			wantType:  "text",
			wantText:  "付款给：张三",
		},
		{
			name:      "weapp booking",
			msgType:   "weapp",
			decrypted: map[string]any{"weapp": map[string]any{"title": "预约服务", "description": "明天上午"}},
			wantType:  "text",
			wantText:  "预约成功：预约服务（明天上午）",
		},
		{
			name:      "location title",
			msgType:   "location",
			decrypted: map[string]any{"location": map[string]any{"title": "南山店", "address": "https://example.invalid/map"}},
			wantType:  "text",
			wantText:  "门店位置：南山店",
		},
		{
			name:      "voice call",
			msgType:   "voice_call",
			decrypted: map[string]any{},
			wantType:  "unknown",
			wantText:  "[音视频通话消息]",
		},
		{
			name:      "agree",
			msgType:   "agree",
			decrypted: map[string]any{},
			wantType:  "unknown",
			wantText:  "[会话存档授权] 客户同意",
		},
		{
			name:      "action fallback",
			msgType:   "unknown_event",
			decrypted: map[string]any{"action": "room_create"},
			wantType:  "unknown",
			wantText:  "[会话事件] room_create",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotText := archiveFallbackContent(tt.msgType, tt.decrypted)
			if gotType != tt.wantType || gotText != tt.wantText {
				t.Fatalf("fallback = (%q, %q), want (%q, %q)", gotType, gotText, tt.wantType, tt.wantText)
			}
		})
	}
}
