// Package p1screen builds legacy P1 screen projection URLs.
package p1screen

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultInternalIP = "192.168.1.30"
	minSlotIndex      = 1
	maxSlotIndex      = 24
)

// Config carries the environment-backed P1 screen settings.
type Config struct {
	InternalIP        string
	WebRTCTCPOverride int
	WebRTCUDPOverride int
}

// Service builds P1 screen response payloads.
type Service struct {
	Config Config
}

// ScreenURL carries the legacy /api/p1/screen/{slot}/url JSON shape.
type ScreenURL struct {
	SlotIndex     int    `json:"slot_index"`
	SlotName      string `json:"slot_name"`
	IP            string `json:"ip"`
	WebRTCTCPPort int    `json:"webrtc_tcp_port"`
	WebRTCUDPPort int    `json:"webrtc_udp_port"`
	URL           string `json:"url"`
	Notes         string `json:"notes"`
}

// APIPayload carries /api/p1/screen/{slot}/api-url fields.
type APIPayload map[string]any

// SlotsPayload carries /api/p1/slots/ports fields.
type SlotsPayload map[string]any

// Ports returns the WebRTC TCP/UDP ports for a slot.
func (service Service) Ports(slotIndex int) (int, int, error) {
	if err := ValidateSlot(slotIndex); err != nil {
		return 0, 0, err
	}
	if service.Config.WebRTCTCPOverride > 0 && service.Config.WebRTCUDPOverride > 0 {
		return service.Config.WebRTCTCPOverride, service.Config.WebRTCUDPOverride, nil
	}
	return defaultTCPPort(slotIndex), defaultUDPPort(slotIndex), nil
}

// ScreenURL returns the legacy URL payload for a slot.
func (service Service) ScreenURL(slotIndex int, quality string) (ScreenURL, error) {
	quality = NormalizeQuality(quality)
	tcpPort, udpPort, err := service.Ports(slotIndex)
	if err != nil {
		return ScreenURL{}, err
	}
	ip := service.internalIP()
	return ScreenURL{
		SlotIndex:     slotIndex,
		SlotName:      fmt.Sprintf("P1-%d", slotIndex),
		IP:            ip,
		WebRTCTCPPort: tcpPort,
		WebRTCUDPPort: udpPort,
		URL:           BuildWebRTCURL(ip, tcpPort, udpPort, quality, "h264"),
		Notes:         "将此 URL 拷贝到支持 WebRTC 的浏览器打开。需确保 webplayer/ 已解压到服务器根目录。",
	}, nil
}

// APIURL returns the legacy iframe helper payload for a slot.
func (service Service) APIURL(slotIndex int, quality string) (APIPayload, error) {
	screenURL, err := service.ScreenURL(slotIndex, quality)
	if err != nil {
		return nil, err
	}
	return APIPayload{
		"html_url":       fmt.Sprintf("/api/p1/screen/%d?quality=%s", slotIndex, NormalizeQuality(quality)),
		"raw_webrtc_url": screenURL.URL,
		"slot_index":     slotIndex,
		"ip":             screenURL.IP,
		"tcp_port":       screenURL.WebRTCTCPPort,
		"udp_port":       screenURL.WebRTCUDPPort,
	}, nil
}

// SlotsPorts returns all 24 legacy slot port mappings.
func (service Service) SlotsPorts() SlotsPayload {
	slots := map[string]any{}
	for index := minSlotIndex; index <= maxSlotIndex; index++ {
		slots[fmt.Sprintf("P1-%d", index)] = map[string]any{
			"index":           index,
			"rpa_port":        defaultRPAPort(index),
			"webrtc_tcp_port": defaultTCPPort(index),
			"webrtc_udp_port": defaultUDPPort(index),
		}
	}
	return SlotsPayload{"slots": slots}
}

// ScreenHTML returns a compact legacy-compatible player wrapper.
func (service Service) ScreenHTML(slotIndex int, quality string) (string, error) {
	screenURL, err := service.ScreenURL(slotIndex, quality)
	if err != nil {
		return "", err
	}
	qualityText := "高"
	if NormalizeQuality(quality) == "0" {
		qualityText = "低"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>P1 投屏 - 坑位 %d</title>
</head>
<body>
    <h1>P1 云手机投屏 - 坑位 %d</h1>
    <p>WebRTC 实时流媒体 | TCP %d | UDP %d | 质量 %s</p>
    <iframe src="%s" style="width:100%%;height:90vh;border:none"></iframe>
</body>
</html>`, slotIndex, slotIndex, screenURL.WebRTCTCPPort, screenURL.WebRTCUDPPort, qualityText, screenURL.URL), nil
}

// BuildWebRTCURL builds the legacy local webplayer path.
func BuildWebRTCURL(ip string, tcpPort int, udpPort int, quality string, codec string) string {
	return fmt.Sprintf("/webplayer/play.html?shost=%s&sport=%d&q=%s&v=%s&rtc_i=%s&rtc_p=%d", ip, tcpPort, NormalizeQuality(quality), defaultText(codec, "h264"), ip, udpPort)
}

// NormalizeQuality applies Python's default quality query value.
func NormalizeQuality(value string) string {
	quality := strings.TrimSpace(value)
	if quality == "" {
		return "1"
	}
	return quality
}

// ValidateQuality preserves the legacy 0/1 query boundary.
func ValidateQuality(value string) error {
	quality := NormalizeQuality(value)
	if quality != "0" && quality != "1" {
		return fmt.Errorf("quality must be 0 or 1")
	}
	return nil
}

// ValidateSlot preserves the legacy 1..24 slot boundary.
func ValidateSlot(slotIndex int) error {
	if slotIndex < minSlotIndex || slotIndex > maxSlotIndex {
		return fmt.Errorf("slot_index 必须在 1-24 之间，收到 %d", slotIndex)
	}
	return nil
}

// ParseSlot parses the path slot value.
func ParseSlot(value string) (int, error) {
	slotIndex, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("slot_index must be an integer")
	}
	return slotIndex, ValidateSlot(slotIndex)
}

func (service Service) internalIP() string {
	return defaultText(service.Config.InternalIP, DefaultInternalIP)
}

func defaultTCPPort(slotIndex int) int {
	return 30000 + (slotIndex-1)*100 + 7
}

func defaultUDPPort(slotIndex int) int {
	return 30000 + (slotIndex-1)*100 + 8
}

func defaultRPAPort(slotIndex int) int {
	return 30000 + (slotIndex-1)*100 + 2
}

func defaultText(value string, fallback string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return fallback
	}
	return text
}
