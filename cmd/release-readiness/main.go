// Command release-readiness emits profile-based release readiness reports.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	readiness "wework-go/internal/cutover"
	"wework-go/internal/httpserver"
)

type aggregateReport struct {
	Profiles    []readiness.Report `json:"profiles"`
	ProfileName []string           `json:"profile_names"`
	ReadyCount  int                `json:"ready_count"`
	TotalCount  int                `json:"total_count"`
	Ready       bool               `json:"ready"`
}

func main() {
	profileName := flag.String("profile", "admin-observability", "release profile name")
	profiles := flag.String("profiles", "", "comma-separated profile names")
	allProfiles := flag.Bool("all", false, "evaluate all built-in profiles")
	envPath := flag.String("env", "deploy/cloud/.env.example", "env file to inspect")
	composePath := flag.String("compose", "deploy/cloud/docker-compose.yml", "compose file to inspect")
	goldenRoot := flag.String("golden-root", "testdata/golden", "golden fixture directory")
	format := flag.String("format", "json", "output format: json, markdown, or runbook")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	strict := flag.Bool("strict", false, "exit non-zero when any requested profile is not ready")
	listProfiles := flag.Bool("list-profiles", false, "list known profiles")
	flag.Parse()

	if *listProfiles {
		for _, profile := range readiness.DefaultProfiles() {
			fmt.Printf("%s\t%s\n", profile.Name, profile.Description)
		}
		return
	}

	selectedProfiles := resolveSelectedProfiles(*profileName, *profiles, *allProfiles)
	if len(selectedProfiles) == 0 {
		fmt.Fprintf(os.Stderr, "no profile requested\n")
		os.Exit(1)
	}

	if *format == "runbook" {
		fmt.Print(markdownRunbook(selectedProfiles))
		return
	}

	env, err := loadEnv(*envPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load env: %v\n", err)
		os.Exit(1)
	}
	services, err := loadServices(*composePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load compose: %v\n", err)
		os.Exit(1)
	}
	suites, err := loadSuites(*goldenRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load golden suites: %v\n", err)
		os.Exit(1)
	}

	report := evaluateProfiles(selectedProfiles, readiness.Inputs{
		Routes:       httpserver.CandidateRoutes(),
		Env:          env,
		Services:     services,
		GoldenSuites: suites,
	})

	switch *format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		if *pretty {
			encoder.SetIndent("", "  ")
		}
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode report: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		fmt.Print(markdownAggregateReport(report))
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(1)
	}

	if *strict && !report.Ready {
		os.Exit(2)
	}
}

func markdownRunbook(profileNames []string) string {
	var b strings.Builder
	b.WriteString("# Release Readiness Profile Guides\n\n")
	b.WriteString("These guides are generated from the release readiness profile catalog and define the minimum route, flag, env, service, and golden-fixture evidence expected before a profile can be released.\n\n")
	b.WriteString("Common sequence:\n\n")
	b.WriteString("1. Run `go run ./cmd/release-readiness -profile <profile> -format markdown` and inspect failures.\n")
	b.WriteString("2. Fix route, golden, and compose-service failures first; these are repository issues.\n")
	b.WriteString("3. Configure required env/secrets in the release environment.\n")
	b.WriteString("4. Run the related golden/live/shadow gate, then enable the listed `GO_ENABLE_*` flags.\n")
	b.WriteString("5. Re-run with `-strict` before canary or traffic movement.\n\n")
	b.WriteString("## Profile Index\n\n")
	for _, name := range profileNames {
		profile := resolveProfileOrExit(name)
		b.WriteString(fmt.Sprintf("- [%s](#%s): %s\n", profile.Name, anchorForProfile(profile.Name), escapeRunbookText(profile.Description)))
	}
	b.WriteString("\n")

	for _, name := range profileNames {
		profile := resolveProfileOrExit(name)
		writeProfileRunbook(&b, profile)
	}
	return b.String()
}

func writeProfileRunbook(b *strings.Builder, profile readiness.Profile) {
	b.WriteString("## ")
	b.WriteString(profile.Name)
	b.WriteString("\n\n")
	b.WriteString(profile.Description)
	b.WriteString("\n\n")
	b.WriteString("Readiness command:\n\n")
	b.WriteString("```bash\n")
	b.WriteString(fmt.Sprintf("go run ./cmd/release-readiness -profile %s -format markdown\n", profile.Name))
	b.WriteString(fmt.Sprintf("go run ./cmd/release-readiness -profile %s -strict\n", profile.Name))
	b.WriteString("```\n\n")
	b.WriteString("Suggested order:\n\n")
	b.WriteString("1. Ensure every route below is present in Go candidate metadata.\n")
	b.WriteString("2. Confirm every golden fixture below exists and passes the relevant golden/live gate.\n")
	b.WriteString("3. Ensure every compose service below is present in `go/deploy/cloud/docker-compose.yml`.\n")
	b.WriteString("4. Populate required env/secrets.\n")
	b.WriteString("5. Enable the listed `GO_ENABLE_*` flags only after the previous checks pass.\n\n")
	writeRouteTable(b, profile.Routes)
	writeListSection(b, "Flags", profile.Flags)
	writeListSection(b, "Required Env", profile.RequiredEnv)
	writeEnvChoiceSection(b, profile.RequiredEnvAny)
	writeListSection(b, "Services", profile.Services)
	writeListSection(b, "Golden Suites", profile.GoldenSuites)
}

func writeRouteTable(b *strings.Builder, routes []readiness.RouteRequirement) {
	b.WriteString("### Routes\n\n")
	if len(routes) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	b.WriteString("| Method | Path |\n")
	b.WriteString("| --- | --- |\n")
	for _, route := range routes {
		b.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", escapeRunbookText(route.Method), escapeRunbookText(route.Path)))
	}
	b.WriteString("\n")
}

func writeListSection(b *strings.Builder, title string, values []string) {
	b.WriteString("### ")
	b.WriteString(title)
	b.WriteString("\n\n")
	if len(values) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, value := range values {
		b.WriteString("- `")
		b.WriteString(escapeRunbookText(value))
		b.WriteString("`\n")
	}
	b.WriteString("\n")
}

func writeEnvChoiceSection(b *strings.Builder, choices []readiness.EnvChoiceRequirement) {
	b.WriteString("### Required Env Alternatives\n\n")
	if len(choices) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, choice := range choices {
		b.WriteString("- ")
		b.WriteString(escapeRunbookText(choice.Name))
		b.WriteString(": one of ")
		parts := make([]string, 0, len(choice.Alternatives))
		for _, alternative := range choice.Alternatives {
			keys := make([]string, 0, len(alternative.Keys))
			for _, key := range alternative.Keys {
				keys = append(keys, "`"+escapeRunbookText(key)+"`")
			}
			parts = append(parts, escapeRunbookText(alternative.Name)+" ("+strings.Join(keys, ", ")+")")
		}
		b.WriteString(strings.Join(parts, "; "))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func anchorForProfile(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func escapeRunbookText(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func resolveSelectedProfiles(profileName string, profileList string, all bool) []string {
	if all {
		profiles := releaseProfileNames()
		sort.Strings(profiles)
		return profiles
	}
	if strings.TrimSpace(profileList) != "" {
		seen := map[string]bool{}
		result := make([]string, 0)
		for _, candidate := range strings.Split(profileList, ",") {
			name := strings.TrimSpace(candidate)
			if name == "" {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, name)
		}
		return result
	}
	if strings.TrimSpace(profileName) == "" {
		return nil
	}
	return []string{strings.TrimSpace(profileName)}
}

func resolveProfileOrExit(name string) readiness.Profile {
	profile, ok := readiness.ProfileByName(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown profile %q\nknown profiles: %s\n", name, strings.Join(profileNames(), ", "))
		os.Exit(1)
	}
	return profile
}

func releaseProfileNames() []string {
	profiles := readiness.DefaultProfiles()
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
}

func evaluateProfiles(profileNames []string, inputs readiness.Inputs) aggregateReport {
	reports := make([]readiness.Report, 0, len(profileNames))
	readyCount := 0
	for _, name := range profileNames {
		profile := resolveProfileOrExit(name)
		report := readiness.Evaluate(profile, inputs)
		if report.Ready {
			readyCount++
		}
		reports = append(reports, report)
	}

	return aggregateReport{
		Profiles:    reports,
		ProfileName: profileNames,
		ReadyCount:  readyCount,
		TotalCount:  len(profileNames),
		Ready:       readyCount == len(profileNames),
	}
}

func markdownAggregateReport(report aggregateReport) string {
	var b strings.Builder
	b.WriteString("# Release Readiness Aggregate Report\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Total profiles | %d |\n", report.TotalCount))
	b.WriteString(fmt.Sprintf("| Ready profiles | %d |\n", report.ReadyCount))
	b.WriteString(fmt.Sprintf("| Ready | `%t` |\n\n", report.Ready))

	for _, profileReport := range report.Profiles {
		b.WriteString("## Profile `")
		b.WriteString(profileReport.Profile)
		b.WriteString("`\n\n")
		b.WriteString(fmt.Sprintf("Description: %s\n\n", profileReport.Description))
		b.WriteString(readiness.MarkdownReport(profileReport))
	}
	return b.String()
}

func loadEnv(path string) (map[string]string, error) {
	return readiness.LoadDotEnv(path)
}

func loadServices(path string) ([]string, error) {
	return readiness.LoadComposeServices(path)
}

func loadSuites(path string) ([]string, error) {
	return readiness.ListGoldenSuites(path)
}

func profileNames() []string {
	profiles := readiness.DefaultProfiles()
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
}
