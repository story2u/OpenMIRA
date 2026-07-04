// Workbench account/device payload tests pin the DB-backed hydration subset.
// They cover identity-safe binding cleanup without relying on SDK/P1 live state.
package workbench

import "testing"

func TestValidateAccountDeviceBindingsClearsConfirmedMismatch(t *testing.T) {
	accounts := ValidateAccountDeviceBindings(
		[]ProjectionRow{{
			"account_id":             "acc-1",
			"account_name":           "子墨",
			"device_id":              "device-old",
			"wework_user_id":         "wx-zimo",
			"account_wework_user_id": "wx-zimo",
		}},
		[]ProjectionRow{{
			"device_id":             "device-old",
			"online":                true,
			"wework_logged_in":      true,
			"wework_status":         "normal",
			"login_wework_user_id":  "wx-other",
			"login_account_name":    "其他账号",
			"login_organization_id": "ent-a",
		}},
	)

	if rowText(accounts[0], "device_id") != "" {
		t.Fatalf("device_id = %q, want cleared", rowText(accounts[0], "device_id"))
	}
}

func TestValidateAccountDeviceBindingsPreservesUnconfirmedDevice(t *testing.T) {
	accounts := ValidateAccountDeviceBindings(
		[]ProjectionRow{{
			"account_id":             "acc-1",
			"account_name":           "子墨",
			"device_id":              "device-old",
			"wework_user_id":         "wx-zimo",
			"account_wework_user_id": "wx-zimo",
		}},
		[]ProjectionRow{{
			"device_id":            "device-old",
			"online":               false,
			"wework_logged_in":     nil,
			"wework_status":        nil,
			"login_wework_user_id": "wx-other",
		}},
	)

	if rowText(accounts[0], "device_id") != "device-old" {
		t.Fatalf("device_id = %q, want preserved", rowText(accounts[0], "device_id"))
	}
}

func TestBuildScopedDevicesPayloadOverlaysLoginSession(t *testing.T) {
	logged := true
	devices := BuildScopedDevicesPayload(
		[]DeviceRecord{{
			AgentID:        "agent-a",
			DeviceID:       "device-1",
			Online:         true,
			WeWorkLoggedIn: &logged,
			WeWorkStatus:   "",
			Model:          "P1",
		}},
		[]LoginSessionRecord{{
			DeviceID:         "device-1",
			Status:           "success",
			AccountName:      "子墨",
			WeWorkUserID:     "wx-zimo",
			OrganizationName: "企业A",
			AccountAvatar:    "avatar.png",
		}},
	)

	if len(devices) != 1 {
		t.Fatalf("devices = %+v", devices)
	}
	row := devices[0]
	if row["wework_logged_in"] != true || rowText(row, "login_wework_user_id") != "wx-zimo" || rowText(row, "login_account_name") != "子墨" {
		t.Fatalf("unexpected device payload: %+v", row)
	}
}

func TestBuildAccountSummaryPayloadUsesAccountFacts(t *testing.T) {
	accounts := BuildAccountSummaryPayload([]AccountRecord{{
		AccountID:    "acc-1",
		AccountName:  "子墨",
		DeviceID:     "device-1",
		WeWorkUserID: "wx-zimo",
		AssigneeID:   "cs-1",
		AssigneeName: "客服1",
		EnterpriseID: "ent-a",
		AIEnabled:    true,
	}})

	row := accounts[0]
	if rowText(row, "account_name") != "子墨" || rowText(row, "assignee_name") != "客服1" || row["enterprise_bound"] != true || row["ai_enabled"] != true {
		t.Fatalf("unexpected account summary: %+v", row)
	}
}
