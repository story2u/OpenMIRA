package httpserver

import (
	"net/http"
	"testing"

	"wework-go/internal/config"
	"wework-go/internal/p1screen"
	"wework-go/internal/p1screenhttp"
)

func TestNewWithModulesCanMountP1ScreenCandidate(t *testing.T) {
	p1Handler := p1screenhttp.New(p1screen.Service{Config: p1screen.Config{InternalIP: "10.0.0.30"}})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{P1Screen: &p1Handler, P1ScreenCandidate: true})

	assertStatus(t, handler, "/api/p1/screen/3/url?quality=0", http.StatusOK, `"slot_name":"P1-3"`)
	assertStatus(t, handler, "/api/p1/screen/3/api-url", http.StatusOK, `"tcp_port":30207`)
	assertStatus(t, handler, "/api/p1/screen/3", http.StatusOK, "P1")
	assertStatus(t, handler, "/api/p1/slots/ports", http.StatusOK, `"P1-24"`)

	routes := RoutesWithModules(Modules{P1Screen: &p1Handler, P1ScreenCandidate: true})
	if len(routes) != 8 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 8", len(routes))
	}
	first := routes[len(routes)-4]
	last := routes[len(routes)-1]
	if first.Path != "/api/p1/screen/{slot_index}" || first.Method != http.MethodGet || first.Phase != "phase4-p1-screen-candidate" {
		t.Fatalf("unexpected p1 screen route metadata: %+v", first)
	}
	if last.Path != "/api/p1/slots/ports" || last.Method != http.MethodGet || last.Phase != "phase4-p1-screen-candidate" {
		t.Fatalf("unexpected p1 slots route metadata: %+v", last)
	}
}
