package devicesdk

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
)

var ErrSDKDiscoveryProbeTargetRequired = errors.New("device_ip is required when no manager device IP candidate is configured")

// DiscoveryProbeRequest describes a provider discovery probe body.
type DiscoveryProbeRequest struct {
	DeviceIP       string
	ManagerHost    string
	ManagerPort    int
	SDKHost        string
	WebRTCHost     string
	TimeoutSec     float64
	ApplyOnSuccess bool
}

// ProbeDiscovery returns a cache-backed /api/v1/devices/discovery/probe payload.
func (service Service) ProbeDiscovery(ctx context.Context, request DiscoveryProbeRequest) (map[string]any, error) {
	_ = ctx
	resolved := service.resolveProbeHosts(request)
	requestedDeviceIP := clean(resolved["device_ip"])
	candidateDeviceIPs := stringValuesFromAny(resolved["candidate_device_ips"])
	if len(candidateDeviceIPs) == 0 {
		return nil, ErrSDKDiscoveryProbeTargetRequired
	}
	managerHost := clean(resolved["manager_host"])
	managerPort := positiveInt(resolved["manager_port"])
	sdkHost := clean(resolved["sdk_host"])
	webrtcHost := clean(resolved["webrtc_host"])
	requestedManagerHost := clean(resolved["requested_manager_host"])
	requestedSDKHost := clean(resolved["requested_sdk_host"])
	requestedWebRTCHost := clean(resolved["requested_webrtc_host"])
	candidateManagerHosts, _ := resolved["candidate_manager_hosts"].(map[string]string)
	managerCandidates := make([]map[string]any, 0, len(candidateDeviceIPs))
	managerDevices := []map[string]any{}
	managerErrors := []string{}
	seenDevices := map[string]bool{}
	for _, candidateDeviceIP := range firstNStrings(candidateDeviceIPs, 16) {
		candidateManagerHost := resolveProbeManagerHostForDevice(candidateDeviceIP, requestedManagerHost, managerHost, candidateManagerHosts)
		candidateSDKHost := resolveProbeSDKHostForDevice(candidateDeviceIP, requestedSDKHost, sdkHost, candidateManagerHost, candidateDeviceIPs)
		candidateWebRTCHost := resolveProbeWebRTCHostForDevice(candidateDeviceIP, requestedWebRTCHost, webrtcHost, candidateManagerHost, candidateDeviceIPs)
		candidateDevices := service.cacheDevicesForProbeCandidate(candidateDeviceIP, candidateManagerHost)
		for _, item := range candidateDevices {
			row := cloneMap(item)
			row["probe_candidate_device_ip"] = candidateDeviceIP
			row["probe_manager_host"] = candidateManagerHost
			row["probe_manager_port"] = managerPort
			row["probe_sdk_host"] = candidateSDKHost
			row["probe_webrtc_host"] = candidateWebRTCHost
			key := probeDeviceDedupeKey(row, candidateDeviceIP)
			if key == "" || seenDevices[key] {
				continue
			}
			seenDevices[key] = true
			managerDevices = append(managerDevices, row)
		}
		candidateErrors := []string{}
		if len(candidateDevices) == 0 {
			endpoint := "-"
			if candidateManagerHost != "" {
				endpoint = candidateManagerHost + ":" + strconv.Itoa(managerPort)
			}
			candidateErrors = append(candidateErrors, "cache: no cached manager devices for "+candidateDeviceIP+" via "+endpoint)
			managerErrors = append(managerErrors, candidateDeviceIP+" via "+endpoint+": cache: no cached manager devices")
		}
		managerCandidates = append(managerCandidates, map[string]any{
			"device_ip":     candidateDeviceIP,
			"manager_host":  candidateManagerHost,
			"manager_port":  managerPort,
			"sdk_host":      candidateSDKHost,
			"webrtc_host":   candidateWebRTCHost,
			"docker_base":   "",
			"method":        "manager_cache",
			"success":       len(candidateDevices) > 0,
			"device_count":  len(candidateDevices),
			"running_count": countOnline(candidateDevices),
			"errors":        candidateErrors,
		})
	}
	detectedDeviceIPs := detectedProbeDeviceIPs(managerDevices)
	if requestedDeviceIP != "" && !containsString(detectedDeviceIPs, requestedDeviceIP) {
		detectedDeviceIPs = append([]string{requestedDeviceIP}, detectedDeviceIPs...)
	}
	selectedDeviceIP := requestedDeviceIP
	if selectedDeviceIP == "" {
		if len(detectedDeviceIPs) > 0 {
			selectedDeviceIP = detectedDeviceIPs[0]
		} else {
			selectedDeviceIP = candidateDeviceIPs[0]
		}
	}
	selectedCandidate := selectProbeCandidate(managerCandidates, selectedDeviceIP)
	selectedManagerHost := defaultText(clean(selectedCandidate["manager_host"]), managerHost)
	selectedManagerPort := positiveInt(firstValue(selectedCandidate["manager_port"], managerPort))
	selectedSDKHost := defaultText(clean(selectedCandidate["sdk_host"]), sdkHost)
	selectedWebRTCHost := defaultText(clean(selectedCandidate["webrtc_host"]), webrtcHost)
	manager := map[string]any{
		"success":       len(managerDevices) > 0,
		"method":        "manager_cache",
		"raw_count":     sumCandidateDeviceCount(managerCandidates),
		"device_count":  len(managerDevices),
		"running_count": countOnline(managerDevices),
		"devices":       firstNMaps(managerDevices, 80),
		"errors":        firstNStrings(managerErrors, 40),
		"candidates":    managerCandidates,
		"auto_detected": requestedDeviceIP == "",
	}
	managerTCPSuccess := selectedManagerHost != "" && selectedManagerPort > 0 && len(managerDevices) > 0
	rpa := probeRPATargetsFromCache(managerDevices, selectedSDKHost, selectedManagerHost)
	webrtc := probeWebRTCTargetsFromCache(managerDevices, selectedWebRTCHost)
	payload := map[string]any{
		"success": managerTCPSuccess && (len(managerDevices) > 0 || boolFromAny(rpa["success"])),
		"target": map[string]any{
			"device_ip":                  selectedDeviceIP,
			"requested_device_ip":        requestedDeviceIP,
			"candidate_device_ips":       candidateDeviceIPs,
			"detected_device_ips":        detectedDeviceIPs,
			"auto_detected":              requestedDeviceIP == "",
			"manager_host":               selectedManagerHost,
			"manager_port":               selectedManagerPort,
			"sdk_host":                   selectedSDKHost,
			"webrtc_host":                selectedWebRTCHost,
			"docker_base":                "",
			"tailscale_route_candidates": []map[string]any{},
		},
		"detected_device_ips": detectedDeviceIPs,
		"probe_candidates":    managerCandidates,
		"route": map[string]any{
			"checked":    false,
			"route":      "",
			"latency_ms": nil,
			"error":      "tailscale route probe is not available in the Go candidate",
			"source":     "go_candidate",
		},
		"manager_tcp": map[string]any{
			"success":    managerTCPSuccess,
			"latency_ms": nil,
			"error":      cacheBackedProbeError(managerTCPSuccess),
			"source":     "manager_cache",
		},
		"manager":       manager,
		"rpa":           rpa,
		"webrtc":        webrtc,
		"suggested_env": buildProbeEnvSuggestions(selectedDeviceIP, selectedManagerHost, selectedManagerPort, selectedSDKHost, selectedWebRTCHost, ""),
	}
	if request.ApplyOnSuccess && boolFromAny(payload["success"]) {
		payload["applied"] = false
		payload["apply_errors"] = []string{"runtime_target: apply_on_success is not available in the Go candidate"}
	}
	return payload, nil
}

func (service Service) resolveProbeHosts(request DiscoveryProbeRequest) map[string]any {
	deviceIP := clean(request.DeviceIP)
	requestedManagerHost := clean(request.ManagerHost)
	managerCacheDevices := service.loadManagerDevices()
	candidateManagerHosts := managerHostsByDeviceIP(managerCacheDevices)
	candidateDeviceIPs := probeCandidateDeviceIPs(deviceIP, requestedManagerHost, managerCacheDevices)
	firstCandidate := ""
	if len(candidateDeviceIPs) > 0 {
		firstCandidate = candidateDeviceIPs[0]
	}
	managerHost := firstNonEmpty(
		requestedManagerHost,
		mappedProbeHost(os.Getenv("P1_MANAGER_CONNECT_HOSTS"), firstNonEmpty(deviceIP, firstCandidate)),
		strings.TrimSpace(os.Getenv("P1_MANAGER_CONNECT_HOST")),
		strings.TrimSpace(os.Getenv("P1_INTERNAL_IP")),
		strings.TrimSpace(os.Getenv("P1_DEFAULT_HOST")),
		firstCandidate,
	)
	if len(candidateDeviceIPs) == 0 {
		candidateDeviceIPs = probeCandidateDeviceIPs(deviceIP, managerHost, managerCacheDevices)
		if len(candidateDeviceIPs) > 0 {
			firstCandidate = candidateDeviceIPs[0]
		}
	}
	managerPort := request.ManagerPort
	if managerPort <= 0 {
		managerPort = probeEnvInt("P1_MANAGER_API_PORT", 83)
	}
	sdkHost := firstNonEmpty(
		clean(request.SDKHost),
		mappedProbeHost(firstNonEmpty(os.Getenv("P1_SDK_CONNECT_HOSTS"), os.Getenv("P1_RPA_CONNECT_HOSTS")), firstNonEmpty(deviceIP, firstCandidate)),
		strings.TrimSpace(firstNonEmpty(os.Getenv("P1_SDK_CONNECT_HOST"), os.Getenv("P1_RPA_CONNECT_HOST"))),
		managerHost,
	)
	webrtcHost := firstNonEmpty(
		clean(request.WebRTCHost),
		mappedProbeHost(os.Getenv("P1_BRIDGE_WEBRTC_HOSTS"), firstNonEmpty(deviceIP, firstCandidate)),
		strings.TrimSpace(os.Getenv("P1_BRIDGE_WEBRTC_HOST")),
		managerHost,
	)
	timeoutSec := request.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 8.0
	}
	return map[string]any{
		"device_ip":                  deviceIP,
		"candidate_device_ips":       candidateDeviceIPs,
		"candidate_manager_hosts":    candidateManagerHosts,
		"tailscale_route_candidates": []map[string]any{},
		"auto_detect":                deviceIP == "",
		"manager_host":               managerHost,
		"manager_port":               managerPort,
		"sdk_host":                   sdkHost,
		"webrtc_host":                webrtcHost,
		"requested_manager_host":     requestedManagerHost,
		"requested_sdk_host":         clean(request.SDKHost),
		"requested_webrtc_host":      clean(request.WebRTCHost),
		"timeout_sec":                timeoutSec,
	}
}

func (service Service) cacheDevicesForProbeCandidate(deviceIP string, managerHost string) []map[string]any {
	result := []map[string]any{}
	for _, device := range service.loadManagerDevices() {
		rowDeviceIP := firstNonEmpty(clean(device["device_ip"]), clean(device["manager_device_ip"]))
		rowManagerHost := firstNonEmpty(clean(device["manager_host"]), clean(device["host"]), clean(device["p1_manager_host"]))
		if deviceIP != "" && rowDeviceIP == deviceIP {
			result = append(result, device)
			continue
		}
		if deviceIP != "" && clean(device["host"]) == deviceIP {
			result = append(result, device)
			continue
		}
		if managerHost != "" && rowManagerHost == managerHost {
			result = append(result, device)
		}
	}
	return result
}

func probeCandidateDeviceIPs(deviceIP string, managerHost string, managerDevices []map[string]any) []string {
	candidates := []string{}
	add := func(value string) {
		normalized := strings.TrimSpace(value)
		if normalized != "" && !containsString(candidates, normalized) {
			candidates = append(candidates, normalized)
		}
	}
	add(deviceIP)
	configured := configuredProbeDeviceIPs()
	for _, value := range mappedProbeDeviceIPs(os.Getenv("P1_MANAGER_CONNECT_HOSTS"), managerHost, configured) {
		add(value)
	}
	if matchesSingleProbeManagerHost(managerHost) {
		for _, value := range configured {
			add(value)
		}
	}
	for _, item := range managerDevices {
		rowDeviceIP := firstNonEmpty(clean(item["device_ip"]), clean(item["manager_device_ip"]))
		rowManagerHost := firstNonEmpty(clean(item["manager_host"]), clean(item["host"]), clean(item["p1_manager_host"]))
		if managerHost != "" && rowManagerHost == managerHost {
			add(rowDeviceIP)
		}
	}
	if len(candidates) == 0 && isPrivateIPv4(managerHost) {
		add(managerHost)
	}
	return candidates
}

func configuredProbeDeviceIPs() []string {
	candidates := []string{}
	for _, name := range []string{"P1_MANAGER_DEVICE_IPS", "P1_MANAGER_DEVICE_IP", "P1_INTERNAL_IP", "P1_DEFAULT_HOST"} {
		for _, value := range splitProbeEnvValues(os.Getenv(name)) {
			if !containsString(candidates, value) {
				candidates = append(candidates, value)
			}
		}
	}
	return candidates
}

func splitProbeEnvValues(raw string) []string {
	replacer := strings.NewReplacer(";", ",", "\n", ",", "\t", ",", " ", ",")
	tokens := strings.Split(replacer.Replace(raw), ",")
	values := []string{}
	for _, token := range tokens {
		value := strings.TrimSpace(token)
		if value != "" && !containsString(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func appendProbeEnvValue(raw string, value string) string {
	values := splitProbeEnvValues(raw)
	normalized := strings.TrimSpace(value)
	if normalized != "" && !containsString(values, normalized) {
		values = append(values, normalized)
	}
	return strings.Join(values, ",")
}

func upsertProbeEnvMap(raw string, key string, value string) string {
	normalizedKey := strings.TrimSpace(key)
	normalizedValue := strings.TrimSpace(value)
	values := splitProbeEnvValues(raw)
	if normalizedKey == "" || normalizedValue == "" {
		return strings.Join(values, ",")
	}
	pairs := make([][2]string, 0, len(values)+1)
	seenKey := false
	for _, token := range values {
		if !strings.Contains(token, "=") {
			pairs = append(pairs, [2]string{token, ""})
			continue
		}
		left, right, _ := strings.Cut(token, "=")
		currentKey := strings.TrimSpace(left)
		currentValue := strings.TrimSpace(right)
		if currentKey == "" {
			continue
		}
		if currentKey == normalizedKey {
			pairs = append(pairs, [2]string{normalizedKey, normalizedValue})
			seenKey = true
		} else {
			pairs = append(pairs, [2]string{currentKey, currentValue})
		}
	}
	if !seenKey {
		pairs = append(pairs, [2]string{normalizedKey, normalizedValue})
	}
	result := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair[1] == "" {
			result = append(result, pair[0])
		} else {
			result = append(result, pair[0]+"="+pair[1])
		}
	}
	return strings.Join(result, ",")
}

func mappedProbeHost(raw string, deviceIP string) string {
	normalizedDeviceIP := strings.TrimSpace(deviceIP)
	for _, token := range splitProbeEnvValues(raw) {
		left, right, ok := strings.Cut(token, "=")
		if ok && strings.TrimSpace(left) == normalizedDeviceIP && strings.TrimSpace(right) != "" {
			return strings.TrimSpace(right)
		}
	}
	return ""
}

func mappedProbeDeviceIPs(raw string, managerHost string, configuredDeviceIPs []string) []string {
	normalizedHost := strings.TrimSpace(managerHost)
	if normalizedHost == "" {
		return nil
	}
	matches := []string{}
	orderedHosts := []string{}
	for _, token := range splitProbeEnvValues(raw) {
		left, right, ok := strings.Cut(token, "=")
		if ok {
			deviceIP := strings.TrimSpace(left)
			host := strings.TrimSpace(right)
			if deviceIP != "" && host == normalizedHost && !containsString(matches, deviceIP) {
				matches = append(matches, deviceIP)
			}
			continue
		}
		orderedHosts = append(orderedHosts, token)
	}
	for index, host := range orderedHosts {
		if index >= len(configuredDeviceIPs) {
			break
		}
		deviceIP := configuredDeviceIPs[index]
		if strings.TrimSpace(host) == normalizedHost && !containsString(matches, deviceIP) {
			matches = append(matches, deviceIP)
		}
	}
	return matches
}

func matchesSingleProbeManagerHost(managerHost string) bool {
	requested := strings.ToLower(strings.Trim(strings.TrimSpace(managerHost), "."))
	if requested == "" {
		return true
	}
	for _, value := range []string{os.Getenv("P1_MANAGER_CONNECT_HOST"), os.Getenv("P1_INTERNAL_IP"), os.Getenv("P1_DEFAULT_HOST")} {
		normalized := strings.ToLower(strings.Trim(strings.TrimSpace(value), "."))
		if normalized != "" && normalized == requested {
			return true
		}
	}
	return false
}

func resolveProbeManagerHostForDevice(deviceIP string, requestedManagerHost string, defaultManagerHost string, candidateManagerHosts map[string]string) string {
	return firstNonEmpty(requestedManagerHost, mappedProbeHost(os.Getenv("P1_MANAGER_CONNECT_HOSTS"), deviceIP), candidateManagerHosts[deviceIP], defaultManagerHost, deviceIP)
}

func resolveProbeSDKHostForDevice(deviceIP string, requestedSDKHost string, defaultSDKHost string, managerHost string, candidates []string) string {
	if requestedSDKHost != "" {
		return requestedSDKHost
	}
	if mapped := mappedProbeHost(firstNonEmpty(os.Getenv("P1_SDK_CONNECT_HOSTS"), os.Getenv("P1_RPA_CONNECT_HOSTS")), deviceIP); mapped != "" {
		return mapped
	}
	singleHost := firstNonEmpty(os.Getenv("P1_SDK_CONNECT_HOST"), os.Getenv("P1_RPA_CONNECT_HOST"))
	if singleHost != "" {
		if isPrivateIPv4(singleHost) && isPrivateIPv4(deviceIP) && singleHost != deviceIP {
			return deviceIP
		}
		if len(candidates) > 1 && containsString(candidates, singleHost) && singleHost != deviceIP {
			return deviceIP
		}
		return singleHost
	}
	return firstNonEmpty(defaultSDKHost, managerHost)
}

func resolveProbeWebRTCHostForDevice(deviceIP string, requestedWebRTCHost string, defaultWebRTCHost string, managerHost string, candidates []string) string {
	if requestedWebRTCHost != "" {
		return requestedWebRTCHost
	}
	if mapped := mappedProbeHost(os.Getenv("P1_BRIDGE_WEBRTC_HOSTS"), deviceIP); mapped != "" {
		return mapped
	}
	singleHost := os.Getenv("P1_BRIDGE_WEBRTC_HOST")
	if singleHost != "" {
		if len(candidates) > 1 && containsString(candidates, singleHost) && singleHost != deviceIP {
			return deviceIP
		}
		return singleHost
	}
	return firstNonEmpty(defaultWebRTCHost, managerHost)
}

func buildProbeEnvSuggestions(deviceIP string, managerHost string, managerPort int, sdkHost string, webrtcHost string, dockerBase string) []map[string]any {
	suggestions := []map[string]any{}
	add := func(name string, value string) {
		current := strings.TrimSpace(os.Getenv(name))
		suggestions = append(suggestions, map[string]any{"name": name, "value": value, "changed": value != current})
	}
	add("P1_MANAGER_DEVICE_IPS", appendProbeEnvValue(os.Getenv("P1_MANAGER_DEVICE_IPS"), deviceIP))
	if managerHost != "" && managerHost != deviceIP {
		add("P1_MANAGER_CONNECT_HOSTS", upsertProbeEnvMap(os.Getenv("P1_MANAGER_CONNECT_HOSTS"), deviceIP, managerHost))
	}
	if sdkHost != "" && sdkHost != deviceIP {
		add("P1_SDK_CONNECT_HOSTS", upsertProbeEnvMap(os.Getenv("P1_SDK_CONNECT_HOSTS"), deviceIP, sdkHost))
	}
	if webrtcHost != "" && webrtcHost != deviceIP {
		add("P1_BRIDGE_WEBRTC_HOSTS", upsertProbeEnvMap(os.Getenv("P1_BRIDGE_WEBRTC_HOSTS"), deviceIP, webrtcHost))
	}
	if dockerBase != "" {
		add("RPA_DOCKER_BASES", upsertProbeEnvMap(firstNonEmpty(os.Getenv("RPA_DOCKER_BASES"), os.Getenv("MYTOS_DOCKER_BASES"), os.Getenv("MYTOS_DOCKER_API_BASES")), deviceIP, dockerBase))
	}
	if managerHost != "" {
		add("P1_KEEPALIVE_TARGETS", appendProbeEnvValue(os.Getenv("P1_KEEPALIVE_TARGETS"), managerHost+":"+strconv.Itoa(managerPort)))
	}
	return suggestions
}

func probeRPATargetsFromCache(managerDevices []map[string]any, sdkHost string, managerHost string) map[string]any {
	targets := []map[string]any{}
	seen := map[string]bool{}
	for _, item := range managerDevices {
		port := positiveInt(item["port"])
		host := firstNonEmpty(clean(item["host"]), sdkHost, managerHost)
		if host == "" || port <= 0 {
			continue
		}
		key := host + ":" + strconv.Itoa(port)
		if seen[key] {
			continue
		}
		seen[key] = true
		success := boolFromAny(item["p1_manager_online"])
		targets = append(targets, map[string]any{
			"host":       host,
			"port":       port,
			"device_id":  item["device_id"],
			"slot":       item["slot"],
			"success":    success,
			"latency_ms": nil,
			"error":      cacheBackedProbeError(success),
			"source":     "manager_cache",
		})
	}
	return map[string]any{
		"success":          anyTargetSuccess(targets),
		"targets":          firstNMaps(targets, 80),
		"detected_devices": []map[string]any{},
		"errors":           []string{},
	}
}

func probeWebRTCTargetsFromCache(managerDevices []map[string]any, webrtcHost string) map[string]any {
	targets := []map[string]any{}
	seen := map[string]bool{}
	for _, item := range managerDevices {
		for _, field := range []string{"p1_webrtc2_port", "p1_webrtc_port"} {
			port := positiveInt(item[field])
			host := firstNonEmpty(clean(item["probe_webrtc_host"]), webrtcHost, clean(item["host"]))
			if host == "" || port <= 0 {
				continue
			}
			key := host + ":" + strconv.Itoa(port)
			if seen[key] {
				continue
			}
			seen[key] = true
			success := boolFromAny(item["p1_manager_online"])
			kind := "webrtc"
			if field == "p1_webrtc2_port" {
				kind = "webrtc2"
			}
			targets = append(targets, map[string]any{
				"host":       host,
				"port":       port,
				"kind":       kind,
				"device_id":  item["device_id"],
				"slot":       item["slot"],
				"success":    success,
				"latency_ms": nil,
				"error":      cacheBackedProbeError(success),
				"source":     "manager_cache",
			})
		}
	}
	return map[string]any{"success": anyTargetSuccess(targets), "targets": firstNMaps(targets, 80)}
}

func cacheBackedProbeError(success bool) string {
	if success {
		return ""
	}
	return "live tcp probe is not available in the Go candidate"
}

func managerHostsByDeviceIP(devices []map[string]any) map[string]string {
	result := map[string]string{}
	for _, device := range devices {
		deviceIP := firstNonEmpty(clean(device["device_ip"]), clean(device["manager_device_ip"]))
		host := firstNonEmpty(clean(device["manager_host"]), clean(device["host"]), clean(device["p1_manager_host"]))
		if deviceIP != "" && host != "" {
			result[deviceIP] = host
		}
	}
	return result
}

func detectedProbeDeviceIPs(devices []map[string]any) []string {
	result := []string{}
	for _, item := range devices {
		detected := firstNonEmpty(clean(item["device_ip"]), clean(item["manager_device_ip"]))
		if detected != "" && !containsString(result, detected) {
			result = append(result, detected)
		}
	}
	return result
}

func selectProbeCandidate(candidates []map[string]any, deviceIP string) map[string]any {
	for _, item := range candidates {
		if clean(item["device_ip"]) == deviceIP {
			return item
		}
	}
	for _, item := range candidates {
		if boolFromAny(item["success"]) {
			return item
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return map[string]any{}
}

func probeDeviceDedupeKey(device map[string]any, fallbackDeviceIP string) string {
	deviceIP := firstNonEmpty(clean(device["device_ip"]), clean(device["manager_device_ip"]), fallbackDeviceIP)
	deviceID := clean(device["device_id"])
	slot := positiveInt(device["slot"])
	if deviceIP == "" && deviceID == "" && slot <= 0 {
		return ""
	}
	return deviceIP + "|" + deviceID + "|" + strconv.Itoa(slot)
}

func countOnline(devices []map[string]any) int {
	count := 0
	for _, device := range devices {
		if boolFromAny(device["p1_manager_online"]) {
			count++
		}
	}
	return count
}

func sumCandidateDeviceCount(candidates []map[string]any) int {
	total := 0
	for _, item := range candidates {
		total += positiveInt(item["device_count"])
	}
	return total
}

func anyTargetSuccess(targets []map[string]any) bool {
	for _, target := range targets {
		if boolFromAny(target["success"]) {
			return true
		}
	}
	return false
}

func firstNMaps(values []map[string]any, limit int) []map[string]any {
	if limit >= 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func firstNStrings(values []string, limit int) []string {
	if limit >= 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func stringValuesFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := clean(item); normalized != "" {
				values = append(values, normalized)
			}
		}
		return values
	default:
		return nil
	}
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func probeEnvInt(name string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return parsed
}

func isPrivateIPv4(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 || ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 || ip4[0] == 192 && ip4[1] == 168
}
