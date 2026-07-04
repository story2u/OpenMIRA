package contactidentity

import (
	"strings"
	"testing"
	"time"
)

func TestBuildProfileUpsertStoresScopedProfileWithoutGlobalRemark(t *testing.T) {
	record, ok := BuildProfileUpsert(ProfileUpsert{
		EnterpriseID:          " ent-1 ",
		SenderID:              " WMExternal123 ",
		SenderName:            "Deep Memory",
		SenderRemark:          "Scoped Remark",
		SenderAvatar:          "https://example.com/avatar.png",
		ScopeWeWorkUserID:     " WJ0011 ",
		ProfileVerifiedSource: "edit_external_contact_callback",
		ProfileVerifiedAt:     "2026-07-02T18:30:00+08:00",
		Now:                   time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
	}, nil)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if record.EnterpriseID != "ent-1" || record.SenderID != "WMExternal123" || record.IdentityStatus != "ready" {
		t.Fatalf("record identity = %+v", record)
	}
	if record.DisplayName != "Deep Memory" || record.RemarkName != "" || record.Nickname != "Deep Memory" {
		t.Fatalf("global display fields = %+v", record)
	}
	if record.SourcePriority != "wework_contact_nickname" || record.SourceVersion != 1 || record.NeedsRefresh {
		t.Fatalf("state fields = %+v", record)
	}
	profiles := record.ExtraJSON[ScopedProfilesKey].(map[string]any)
	profile := profiles["wj0011"].(map[string]any)
	if profile["remark_name"] != "Scoped Remark" || profile["display_name"] != "Scoped Remark" || profile["nickname"] != "Deep Memory" {
		t.Fatalf("scoped profile = %+v", profile)
	}
	if profile["profile_verified_source"] != "edit_external_contact_callback" || record.ExtraJSON["profile_verified_at"] != "2026-07-02T18:30:00+08:00" {
		t.Fatalf("verified fields = %+v", record.ExtraJSON)
	}
}

func TestBuildProfileUpsertKeepsHigherPriorityReadyIdentity(t *testing.T) {
	existing := &Record{
		EnterpriseID:   "ent-1",
		SenderID:       "ext-1",
		IdentityStatus: "ready",
		DisplayName:    "Priority Remark",
		RemarkName:     "Priority Remark",
		Nickname:       "Nick",
		SourcePriority: "wework_contact_remark",
		SourceVersion:  3,
		ExtraJSON:      map[string]any{"keep": true},
	}
	record, ok := BuildProfileUpsert(ProfileUpsert{
		EnterpriseID: "ent-1",
		SenderID:     "ext-1",
		SenderName:   "Lower Nick",
		Source:       "message_sender_name",
		Now:          time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
	}, existing)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if record.DisplayName != "Priority Remark" || record.Nickname != "Nick" || record.SourceVersion != 3 {
		t.Fatalf("record should keep higher priority existing identity: %+v", record)
	}
	if record.ExtraJSON["keep"] != true {
		t.Fatalf("extra_json = %+v", record.ExtraJSON)
	}
}

func TestScopedDisplayRowsIndexesScopedRemarkAndLowercase(t *testing.T) {
	record := Record{
		EnterpriseID: "ent-1",
		SenderID:     "WMExternal123",
		DisplayName:  "Nick Name",
		Nickname:     "Nick Name",
		ExtraJSON: map[string]any{
			ScopedProfilesKey: map[string]any{
				"wj0011": map[string]any{"remark_name": "Scoped Remark", "display_name": "Scoped Remark"},
			},
		},
	}
	rows := ScopedDisplayRows(record)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].EnterpriseID != "ent-1" || rows[0].WeWorkUserID != "wj0011" || rows[0].SenderID != "WMExternal123" {
		t.Fatalf("row identity = %+v", rows[0])
	}
	if rows[0].DisplayName == rows[1].DisplayName || rows[0].DisplayNameKey == "" || rows[0].SenderIDKey == "" {
		t.Fatalf("index rows = %+v", rows)
	}
}

func TestScopedProfileMatchesSeparatedWeWorkUserID(t *testing.T) {
	record := Record{ExtraJSON: map[string]any{
		ScopedProfilesKey: map[string]any{
			"DY-1": map[string]any{"remark_name": "Scoped"},
		},
	}}
	profile := ScopedProfile(record, "dy1")
	if profile["remark_name"] != "Scoped" {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestMarkAndClearScopedRPASafeSearchName(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	record, err := MarkScopedRPASafeSearchName(Record{
		EnterpriseID:   "ent-1",
		SenderID:       "ext-1",
		IdentityStatus: "partial",
		SourceVersion:  4,
		ExtraJSON:      map[string]any{},
	}, RPASafeMark{
		EnterpriseID:   "ent-1",
		SenderID:       "ext-1",
		WeWorkUserID:   "DY-1",
		BusinessRemark: "Alice",
		SafeSearchName: "Alice#QWE",
		SafeCode:       "QWE",
		SenderName:     "Alice Nick",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("MarkScopedRPASafeSearchName returned error: %v", err)
	}
	if record.IdentityStatus != "ready" || record.NeedsRefresh || record.SourceVersion != 5 || record.Nickname != "Alice Nick" {
		t.Fatalf("record = %+v", record)
	}
	profile := ScopedProfile(record, "dy1")
	if profile["remark_name"] != "Alice#QWE" || profile["rpa_safe_business_remark"] != "Alice" || profile["rpa_safe_name_status"] != "synced" {
		t.Fatalf("marked profile = %+v", profile)
	}

	cleared, err := ClearScopedRPASafeSearchName(record, RPASafeClear{
		EnterpriseID:   "ent-1",
		SenderID:       "ext-1",
		WeWorkUserID:   "dy1",
		BusinessRemark: "Alice",
		SenderName:     "Alice Nick",
		Now:            now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ClearScopedRPASafeSearchName returned error: %v", err)
	}
	clearedProfile := ScopedProfile(cleared, "dy1")
	if clearedProfile["remark_name"] != "Alice" || clearedProfile["display_name"] != "Alice" {
		t.Fatalf("cleared profile display = %+v", clearedProfile)
	}
	for key := range clearedProfile {
		if strings.HasPrefix(key, "rpa_safe_") {
			t.Fatalf("rpa safe field was not cleared: %+v", clearedProfile)
		}
	}
}

func TestBuildRPASafeSearchNameMatchesPythonSeed(t *testing.T) {
	candidate, code := BuildRPASafeSearchName("ent-1", "dy1", "ext-1", "Alice", func(candidate string) bool {
		return candidate == "Alice#ONP"
	})
	if code == "ONP" || candidate == "Alice#ONP" {
		t.Fatalf("ambiguous first code should be skipped: candidate=%q code=%q", candidate, code)
	}
	firstCandidate, firstCode := BuildRPASafeSearchName("ent-1", "dy1", "ext-1", "Alice", nil)
	if firstCode != "ONP" || firstCandidate != "Alice#ONP" {
		t.Fatalf("first candidate = %q/%q, want Alice#ONP/ONP", firstCandidate, firstCode)
	}
}

func TestBuildRPASafeSearchNameCheckedAbortsOnAmbiguityError(t *testing.T) {
	candidate, code, unknown := BuildRPASafeSearchNameChecked("ent-1", "dy1", "ext-1", "Alice", func(candidate string) (bool, error) {
		return false, ErrInvalidRPASafeInput
	})
	if candidate != "" || code != "" || !unknown {
		t.Fatalf("checked candidate = %q/%q unknown=%v, want empty unknown", candidate, code, unknown)
	}
}
