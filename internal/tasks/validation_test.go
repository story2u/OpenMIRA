package tasks

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestValidateCreateJSONBuildsAcceptedRecord freezes the initial task state.
func TestValidateCreateJSONBuildsAcceptedRecord(t *testing.T) {
	request, err := ValidateCreateJSON([]byte(validTaskCreateBody()))
	if err != nil {
		t.Fatalf("ValidateCreateJSON returned error: %v", err)
	}
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	record := NewAcceptedRecord(request, now)

	if record.Status != StatusAccepted || record.TaskID != "task-golden-0001" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.Target.AgentID != "sdk:zimo" || record.Target.DeviceID != "zimo" {
		t.Fatalf("unexpected target: %#v", record.Target)
	}
	if record.Payload["text"] != "hello" || record.RetryCount != 0 {
		t.Fatalf("unexpected payload/retry: %#v", record)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if !strings.Contains(string(encoded), `"status":"accepted"`) {
		t.Fatalf("encoded record missing status: %s", string(encoded))
	}
}

func TestNewAcceptedRecordPrefersChannelUserID(t *testing.T) {
	channelUserID := "channel-user-001"
	weworkUserID := "wework-user-001"
	request, err := ValidateCreateJSON([]byte(validTaskCreateBody()))
	if err != nil {
		t.Fatalf("ValidateCreateJSON returned error: %v", err)
	}
	request.ChannelUserID = &channelUserID
	request.WeWorkUserID = &weworkUserID

	record := NewAcceptedRecord(request, time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC))

	if record.ChannelUserID == nil || *record.ChannelUserID != channelUserID {
		t.Fatalf("ChannelUserID = %#v, want %q", record.ChannelUserID, channelUserID)
	}
	if record.WeWorkUserID == nil || *record.WeWorkUserID != channelUserID {
		t.Fatalf("WeWorkUserID = %#v, want compatibility alias %q", record.WeWorkUserID, channelUserID)
	}
}

// TestValidateCreateJSONRejectsContractDrift covers schema-critical failures.
func TestValidateCreateJSONRejectsContractDrift(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown top-level field",
			body: replaceInValid(`"trace_id":"trace-golden-0001"`, `"trace_id":"trace-golden-0001","extra":true`),
			want: "extra: unknown field",
		},
		{
			name: "invalid source",
			body: replaceInValid(`"source":"cloud-web"`, `"source":"browser"`),
			want: "source: is not allowed",
		},
		{
			name: "missing target device",
			body: replaceInValid(`"device_id":"zimo"`, `"device_id":""`),
			want: "target.device_id",
		},
		{
			name: "unknown payload field",
			body: replaceInValid(`"text":"hello"`, `"text":"hello","unexpected":1`),
			want: "payload.unexpected: unknown field",
		},
		{
			name: "send image missing media url",
			body: strings.ReplaceAll(replaceInValid(`"task_type":"send_text"`, `"task_type":"send_image"`), `,"text":"hello"`, ""),
			want: "payload.media_url: is required for send_image",
		},
		{
			name: "invalid messages item",
			body: `{"task_id":"task-golden-0002","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"send_mixed_messages","payload":{"username":"Qiu","receiver":"Qiu","entity":"123","msg_id":"m1","messages":[{"type":"text"}]},"created_at":"2026-06-29T09:00:00Z"}`,
			want: "payload.messages[0].content: is required",
		},
		{
			name: "invalid timestamp",
			body: replaceInValid(`"created_at":"2026-06-29T09:00:00Z"`, `"created_at":"2026/06/29"`),
			want: "created_at: must be an RFC3339 timestamp",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ValidateCreateJSON([]byte(testCase.body))
			if !errors.Is(err, ErrInvalidCreate) {
				t.Fatalf("error = %v, want ErrInvalidCreate", err)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), testCase.want)
			}
		})
	}
}

// TestValidateCreateJSONSupportsShareBundleSend keeps the experimental type valid.
func TestValidateCreateJSONSupportsShareBundleSend(t *testing.T) {
	body := `{"task_id":"task-share-0001","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"share_bundle_send","payload":{"username":"Qiu","receiver":"Qiu","entity":"123","msg_id":"m1","share_mode":"multi","messages":[{"type":"text","content":"hello"}]},"created_at":"2026-06-29T09:00:00Z"}`
	request, err := ValidateCreateJSON([]byte(body))
	if err != nil {
		t.Fatalf("ValidateCreateJSON returned error: %v", err)
	}
	if request.TaskType != "share_bundle_send" {
		t.Fatalf("TaskType = %q", request.TaskType)
	}
}

func TestValidateCreateJSONSupportsRPACallTaskTypes(t *testing.T) {
	cases := []struct {
		name    string
		task    string
		payload string
	}{
		{name: "voice", task: "rpa_voice_call", payload: `"username":"Qiu","receiver":"Alice","call_type":"voice"`},
		{name: "video", task: "rpa_video_call", payload: `"username":"Qiu","receiver":"Alice","call_type":"video"`},
		{name: "hangup", task: "rpa_hangup_call", payload: `"username":"Qiu","receiver":"Alice"`},
		{name: "prepare-audio", task: "rpa_prepare_call_audio_output", payload: `"username":"__device__"`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := `{"task_id":"task-rpa-` + testCase.name + `","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"` + testCase.task + `","payload":{` + testCase.payload + `},"created_at":"2026-06-29T09:00:00Z"}`
			request, err := ValidateCreateJSON([]byte(body))
			if err != nil {
				t.Fatalf("ValidateCreateJSON returned error: %v", err)
			}
			if request.TaskType != testCase.task {
				t.Fatalf("TaskType = %q, want %q", request.TaskType, testCase.task)
			}
		})
	}
}

func TestValidateCreateJSONSupportsDeviceAppControlPayload(t *testing.T) {
	body := `{"task_id":"task-device-app","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"device_open_app","payload":{"username":"__device__","app_id":"crm","package_name":"com.example.crm"},"created_at":"2026-06-29T09:00:00Z"}`

	request, err := ValidateCreateJSON([]byte(body))
	if err != nil {
		t.Fatalf("ValidateCreateJSON returned error: %v", err)
	}
	if request.TaskType != "device_open_app" || request.Payload["app_id"] != "crm" || request.Payload["package_name"] != "com.example.crm" {
		t.Fatalf("request = %+v", request)
	}
}

func TestValidateCreateJSONSupportsConnectorLoginTaskTypes(t *testing.T) {
	cases := []struct {
		name string
		task string
	}{
		{name: "neutral", task: "connector_login_verify"},
		{name: "compat", task: "wework_login_verify"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := `{"task_id":"task-login-` + testCase.name + `","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"` + testCase.task + `","payload":{"username":"__login__","verify_code":"123456","verify_type":"sms"},"created_at":"2026-06-29T09:00:00Z"}`
			request, err := ValidateCreateJSON([]byte(body))
			if err != nil {
				t.Fatalf("ValidateCreateJSON returned error: %v", err)
			}
			if request.TaskType != testCase.task {
				t.Fatalf("TaskType = %q, want %q", request.TaskType, testCase.task)
			}
		})
	}
}

func validTaskCreateBody() string {
	return `{"task_id":"task-golden-0001","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"send_text","payload":{"username":"Qiu","receiver":"Qiu","text":"hello","queue":"fast","client_batch_id":"batch-1","client_batch_index":0},"created_at":"2026-06-29T09:00:00Z","trace_id":"trace-golden-0001"}`
}

func replaceInValid(oldValue string, newValue string) string {
	return strings.Replace(validTaskCreateBody(), oldValue, newValue, 1)
}
