package p1screen

import "testing"

func TestServiceBuildsLegacyScreenURL(t *testing.T) {
	service := Service{Config: Config{InternalIP: "10.0.0.30"}}

	payload, err := service.ScreenURL(3, "0")
	if err != nil {
		t.Fatalf("ScreenURL returned error: %v", err)
	}
	if payload.WebRTCTCPPort != 30207 || payload.WebRTCUDPPort != 30208 {
		t.Fatalf("ports = %d/%d", payload.WebRTCTCPPort, payload.WebRTCUDPPort)
	}
	wantURL := "/webplayer/play.html?shost=10.0.0.30&sport=30207&q=0&v=h264&rtc_i=10.0.0.30&rtc_p=30208"
	if payload.URL != wantURL {
		t.Fatalf("url = %q, want %q", payload.URL, wantURL)
	}
}

func TestServiceUsesPortOverridesOnlyWhenBothConfigured(t *testing.T) {
	service := Service{Config: Config{WebRTCTCPOverride: 39007, WebRTCUDPOverride: 39008}}

	tcpPort, udpPort, err := service.Ports(5)
	if err != nil {
		t.Fatalf("Ports returned error: %v", err)
	}
	if tcpPort != 39007 || udpPort != 39008 {
		t.Fatalf("override ports = %d/%d", tcpPort, udpPort)
	}

	service.Config.WebRTCUDPOverride = 0
	tcpPort, udpPort, err = service.Ports(5)
	if err != nil {
		t.Fatalf("Ports returned error: %v", err)
	}
	if tcpPort != 30407 || udpPort != 30408 {
		t.Fatalf("fallback ports = %d/%d", tcpPort, udpPort)
	}
}

func TestServiceBuildsAPIURLAndSlotPorts(t *testing.T) {
	service := Service{}

	apiURL, err := service.APIURL(1, "")
	if err != nil {
		t.Fatalf("APIURL returned error: %v", err)
	}
	if apiURL["html_url"] != "/api/p1/screen/1?quality=1" || apiURL["tcp_port"] != 30007 {
		t.Fatalf("api payload = %#v", apiURL)
	}

	slots := service.SlotsPorts()["slots"].(map[string]any)
	slot24 := slots["P1-24"].(map[string]any)
	if slot24["rpa_port"] != 32302 || slot24["webrtc_tcp_port"] != 32307 || slot24["webrtc_udp_port"] != 32308 {
		t.Fatalf("slot24 = %#v", slot24)
	}
}

func TestValidationRejectsInvalidInputs(t *testing.T) {
	if err := ValidateQuality("2"); err == nil {
		t.Fatal("ValidateQuality accepted invalid quality")
	}
	if _, err := ParseSlot("25"); err == nil {
		t.Fatal("ParseSlot accepted invalid slot")
	}
	if _, err := ParseSlot("abc"); err == nil {
		t.Fatal("ParseSlot accepted non-integer slot")
	}
}
