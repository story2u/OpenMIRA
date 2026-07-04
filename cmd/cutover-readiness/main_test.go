package main

import (
	"strings"
	"testing"

	"wework-go/internal/cutover"
	"wework-go/internal/httpserver"
)

func TestResolveSelectedProfiles(t *testing.T) {
	t.Run("single profile", func(t *testing.T) {
		names := resolveSelectedProfiles("admin-observability", "", false)
		if len(names) != 1 || names[0] != "admin-observability" {
			t.Fatalf("unexpected selected profiles: %#v", names)
		}
	})

	t.Run("profiles list overrides default", func(t *testing.T) {
		names := resolveSelectedProfiles("admin-observability", "admin-assignments,admin-accounts,admin-assignments", false)
		if len(names) != 2 || names[0] != "admin-assignments" || names[1] != "admin-accounts" {
			t.Fatalf("unexpected selected profiles: %#v", names)
		}
	})

	t.Run("all includes deterministic sorted names", func(t *testing.T) {
		names := resolveSelectedProfiles("admin-observability", "", true)
		if len(names) == 0 {
			t.Fatal("expected all profiles")
		}
		for i := 1; i < len(names); i++ {
			if names[i-1] >= names[i] {
				t.Fatalf("profile names not sorted: %v", names)
			}
		}
	})
}

func TestMarkdownAggregateReport(t *testing.T) {
	report := aggregateReport{
		Profiles: []cutover.Report{
			{
				Profile:     "admin-observability",
				Description: "Admin observability",
				Ready:       true,
			},
			{
				Profile:     "workbench-read",
				Description: "Workbench read",
				Ready:       false,
			},
		},
		ProfileName: []string{"admin-observability", "workbench-read"},
		ReadyCount:  1,
		TotalCount:  2,
		Ready:       false,
	}
	reportMarkdown := markdownAggregateReport(report)

	if !contains(reportMarkdown, "## Profile `admin-observability`") {
		t.Fatalf("missing admin-observability profile section")
	}
	if !contains(reportMarkdown, "## Profile `workbench-read`") {
		t.Fatalf("missing workbench-read profile section")
	}
	if !contains(reportMarkdown, "| Total profiles | 2 |") {
		t.Fatalf("missing total profiles summary")
	}
}

func TestMarkdownRunbookIncludesProfileRequirements(t *testing.T) {
	runbook := markdownRunbook([]string{"session-access", "archive-voice-transcription"})

	for _, want := range []string{
		"# Release Profile Runbooks",
		"[session-access](#session-access)",
		"## session-access",
		"go run ./cmd/cutover-readiness -profile session-access -strict",
		"`GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE`",
		"`CLOUD_DB_DSN`",
		"`go-api`",
		"`phase2-session-admin-login.json`",
		"## archive-voice-transcription",
		"VOICE_TRANSCRIPTION_COZE_API_KEY",
		"COZE_JWT_OAUTH inline key",
	} {
		if !contains(runbook, want) {
			t.Fatalf("runbook missing %q\n%s", want, runbook)
		}
	}
}

func TestEvaluateProfilesAggregateCounts(t *testing.T) {
	profileNames := []string{"session-access", "admin-observability"}
	env, services, suites := syntheticCutoverInputs(t, profileNames)

	report := evaluateProfiles(profileNames, cutover.Inputs{
		Routes:       httpserver.CandidateRoutes(),
		Env:          env,
		Services:     services,
		GoldenSuites: suites,
	})

	if report.TotalCount != 2 {
		t.Fatalf("expected total count 2, got %d", report.TotalCount)
	}
	if report.ReadyCount != 2 {
		t.Fatalf("expected ready count 2 with synthetic inputs, got %d (ready=%t)", report.ReadyCount, report.Ready)
	}
	if !report.Ready {
		t.Fatalf("expected aggregate report ready=true")
	}
}

func TestEvaluateProfilesCountsWithFailure(t *testing.T) {
	profileNames := []string{"session-access", "admin-observability"}
	env, services, suites := syntheticCutoverInputs(t, profileNames)
	delete(env, "GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE")

	report := evaluateProfiles(profileNames, cutover.Inputs{
		Routes:       httpserver.CandidateRoutes(),
		Env:          env,
		Services:     services,
		GoldenSuites: suites,
	})

	if report.Ready {
		t.Fatalf("expected aggregate report not ready when required flag missing")
	}
	if report.ReadyCount != 1 {
		t.Fatalf("expected one ready profile, got %d", report.ReadyCount)
	}
}

func syntheticCutoverInputs(t *testing.T, profileNames []string) (map[string]string, []string, []string) {
	env := map[string]string{}
	serviceSet := map[string]struct{}{}
	suiteSet := map[string]struct{}{}
	for _, name := range profileNames {
		profile, ok := cutover.ProfileByName(name)
		if !ok {
			t.Fatalf("profile %q missing", name)
		}
		for _, flag := range profile.Flags {
			env[flag] = "1"
		}
		for _, k := range profile.RequiredEnv {
			env[k] = "postgres://db"
		}
		for _, choice := range profile.RequiredEnvAny {
			if len(choice.Alternatives) == 0 {
				continue
			}
			if len(choice.Alternatives[0].Keys) == 0 {
				continue
			}
			env[choice.Alternatives[0].Keys[0]] = "dummy"
		}
		for _, service := range profile.Services {
			serviceSet[service] = struct{}{}
		}
		for _, suite := range profile.GoldenSuites {
			suiteSet[suite] = struct{}{}
		}
	}

	services := make([]string, 0, len(serviceSet))
	for service := range serviceSet {
		services = append(services, service)
	}
	suites := make([]string, 0, len(suiteSet))
	for suite := range suiteSet {
		suites = append(suites, suite)
	}
	return env, services, suites
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
