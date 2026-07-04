package taskstore

import (
	"strings"
	"testing"
)

// TestBuildSDKDispatchClaimSelectMatchesLegacyFilters freezes claim predicates.
func TestBuildSDKDispatchClaimSelectMatchesLegacyFilters(t *testing.T) {
	sql, args, err := BuildSDKDispatchClaimSelect(SDKDispatchClaimQuery{
		DeviceIDs:           []string{" zimo ", "", "ada"},
		TaskTypes:           []string{"send_text", "send_image"},
		ForUpdateSkipLocked: true,
	})
	if err != nil {
		t.Fatalf("BuildSDKDispatchClaimSelect returned error: %v", err)
	}
	for _, fragment := range []string{
		"FROM tasks",
		"status = ?",
		"target_agent_id LIKE ?",
		"task_type IN (?, ?)",
		"target_device_id IN (?, ?)",
		"payload_json LIKE ? OR payload_json LIKE ?",
		"created_at ASC",
		"task_id ASC",
		"LIMIT 1 FOR UPDATE SKIP LOCKED",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sql missing %q: %s", fragment, sql)
		}
	}
	wantArgs := []any{
		"accepted",
		"sdk:%",
		"send_text",
		"send_image",
		"zimo",
		"ada",
		`%"queue": "fast"%`,
		`%"queue":"fast"%`,
		`%"queue": "slow"%`,
		`%"queue":"slow"%`,
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("arg[%d] = %#v, want %#v; args=%#v", index, args[index], wantArgs[index], args)
		}
	}
}

// TestBuildSDKDispatchClaimSelectRejectsAmbiguousInput keeps claim scope explicit.
func TestBuildSDKDispatchClaimSelectRejectsAmbiguousInput(t *testing.T) {
	if _, _, err := BuildSDKDispatchClaimSelect(SDKDispatchClaimQuery{}); err == nil || !strings.Contains(err.Error(), "task_types is required") {
		t.Fatalf("missing task types error = %v", err)
	}
	if _, _, err := BuildSDKDispatchClaimSelect(SDKDispatchClaimQuery{TaskTypes: []string{"send_text"}, DeviceIDs: []string{" "}}); err == nil || !strings.Contains(err.Error(), "device_ids cannot be empty") {
		t.Fatalf("empty devices error = %v", err)
	}
}
