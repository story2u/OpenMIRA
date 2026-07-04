package main

import (
	"net/http"
	"testing"

	"wework-go/internal/harness/routediff"
	"wework-go/internal/httpserver"
)

func TestSelectGoRoutesDefault(t *testing.T) {
	routes, err := selectGoRoutes("default")
	if err != nil {
		t.Fatalf("select default routes: %v", err)
	}
	if len(routes) != len(httpserver.Routes()) {
		t.Fatalf("default routes count = %d, want %d", len(routes), len(httpserver.Routes()))
	}
	if hasRoute(routes, http.MethodGet, "/api/v1/session/me") {
		t.Fatalf("default route set unexpectedly includes session candidate")
	}
}

func TestSelectGoRoutesCandidate(t *testing.T) {
	routes, err := selectGoRoutes("candidate")
	if err != nil {
		t.Fatalf("select candidate routes: %v", err)
	}
	if len(routes) <= len(httpserver.Routes()) {
		t.Fatalf("candidate routes count = %d, want more than default %d", len(routes), len(httpserver.Routes()))
	}
	if !hasRoute(routes, http.MethodGet, "/api/v1/session/me") {
		t.Fatalf("candidate route set missing session candidate")
	}
}

func TestSelectGoRoutesRejectsUnknownSet(t *testing.T) {
	if _, err := selectGoRoutes("enabled"); err == nil {
		t.Fatalf("select unknown route set returned nil error")
	}
}

func TestEnforceRouteDiffGatesRouteMode(t *testing.T) {
	report := routediff.Report{
		PythonOnly: make([]routediff.RouteRef, 3),
		GoOnly:     make([]routediff.RouteRef, 5),
	}
	if err := enforceRouteDiffGates("route", report, nil, 4, 6, -1); err != nil {
		t.Fatalf("route gate unexpectedly failed: %v", err)
	}
	if err := enforceRouteDiffGates("route", report, nil, 2, 6, -1); err == nil {
		t.Fatalf("route gate expected reference-only failure")
	}
	if err := enforceRouteDiffGates("route", report, nil, 4, 4, -1); err == nil {
		t.Fatalf("route gate expected go-only failure")
	}
}

func TestEnforceRouteDiffGatesSchemaMode(t *testing.T) {
	report := routediff.Report{
		PythonOnly: make([]routediff.RouteRef, 2),
		GoOnly:     make([]routediff.RouteRef, 1),
	}
	schemaReport := routediff.SchemaDriftReport{
		SchemaMismatchCount: 2,
	}
	if err := enforceRouteDiffGates("schema-drift", report, &schemaReport, -1, -1, 2); err != nil {
		t.Fatalf("schema-drift gate unexpectedly failed: %v", err)
	}
	if err := enforceRouteDiffGates("schema-drift", report, &schemaReport, -1, -1, 1); err == nil {
		t.Fatalf("schema-drift gate expected mismatch failure")
	}
}

func TestEnforceOpenAPIDriftGate(t *testing.T) {
	report := routediff.OpenAPIDriftReport{MismatchCount: 2}
	if err := enforceOpenAPIDriftGate(report, 2); err != nil {
		t.Fatalf("openapi-drift gate unexpectedly failed: %v", err)
	}
	if err := enforceOpenAPIDriftGate(report, 1); err == nil {
		t.Fatalf("openapi-drift gate expected mismatch failure")
	}
}

func hasRoute(routes []httpserver.Route, method string, path string) bool {
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
