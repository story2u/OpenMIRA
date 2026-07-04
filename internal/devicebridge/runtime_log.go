package devicebridge

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const logStatusTailBytes int64 = 128 * 1024

var lineFieldPattern = regexp.MustCompile(`([A-Za-z_]+)=([^ ]+)`)

func (service Service) mergeRuntimeLogStatus(raw map[string]any) map[string]any {
	merged := copyMap(raw)
	if !boolValue(merged["running"]) {
		return merged
	}
	logPath := service.safeRuntimeLogPath(clean(merged["log_file"]))
	if logPath == "" {
		return merged
	}
	stat, err := os.Stat(logPath)
	if err != nil || stat.IsDir() {
		return merged
	}
	parsed := parseRuntimeLogStatus(readLogTail(logPath, logStatusTailBytes))
	if len(parsed) == 0 {
		return merged
	}
	if previous, err := parseTimestamp(clean(merged["updated_at"])); err == nil && stat.ModTime().Before(previous) {
		return merged
	}
	observedAt := stat.ModTime().UTC().Format(time.RFC3339Nano)
	merged["status"] = defaultText(parsed["status"], clean(merged["status"]))
	merged["detail"] = defaultText(parsed["detail"], clean(merged["detail"]))
	merged["runtime_observed_at"] = observedAt
	merged["updated_at"] = observedAt
	return merged
}

func (service Service) safeRuntimeLogPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	roots := service.runtimeLogRoots()
	for _, candidate := range service.runtimeLogPathCandidates(value, roots) {
		resolvedCandidate, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		stat, err := os.Stat(resolvedCandidate)
		if err != nil || stat.IsDir() {
			continue
		}
		for _, root := range roots {
			if pathIsInside(resolvedCandidate, root) {
				return resolvedCandidate
			}
		}
	}
	return ""
}

func (service Service) runtimeLogRoots() []string {
	roots := make([]string, 0, 3)
	statusRoot := strings.TrimSpace(service.statusFile())
	if statusRoot != "" {
		roots = appendRoot(roots, filepath.Dir(filepath.Dir(statusRoot)))
	}
	if strings.TrimSpace(service.HostDataRoot) != "" {
		roots = appendRoot(roots, service.HostDataRoot)
	}
	roots = appendRoot(roots, "/app/host-data")
	return roots
}

func (service Service) runtimeLogPathCandidates(value string, roots []string) []string {
	candidates := make([]string, 0, 1+len(roots))
	candidates = appendPathCandidate(candidates, value)
	normalized := strings.ReplaceAll(value, "\\", "/")
	const marker = "/backend/data/"
	if strings.Contains(normalized, marker) {
		suffix := strings.Trim(strings.SplitN(normalized, marker, 2)[1], "/")
		for _, root := range roots {
			candidates = appendPathCandidate(candidates, filepath.Join(root, filepath.FromSlash(suffix)))
		}
	}
	return candidates
}

func parseRuntimeLogStatus(text string) map[string]string {
	lines := nonEmptyLines(text)
	vendorStatus, vendorIndex := parseVendorMonopipeStatus(lines)
	fridaStatus, fridaIndex := parseFridaConnectionStatus(lines)
	if len(fridaStatus) > 0 && fridaIndex > vendorIndex {
		return fridaStatus
	}
	if len(vendorStatus) > 0 {
		return vendorStatus
	}
	if len(fridaStatus) > 0 {
		return fridaStatus
	}
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if strings.Contains(line, "bridge_tick") || strings.Contains(line, "bridge_summary") {
			fields := lineFields(line)
			byteCount := positiveInt(fields["bytes"])
			frameCount := positiveInt(fields["frames"])
			count := positiveInt(fields["count"])
			if byteCount > 0 {
				return map[string]string{
					"status": "bridging",
					"detail": fmt.Sprintf("桥接进程运行中，已观察到通话音频数据：bytes=%d count=%d frames=%d peak=%s", byteCount, count, frameCount, fields["peak"]),
				}
			}
			if count > 0 || frameCount > 0 {
				return map[string]string{
					"status": "probing",
					"detail": "桥接进程已观察到候选音频回调，等待 remote_submix 写入字节增长。",
				}
			}
		}
		if strings.Contains(line, "remote_submix_missing_skip_track_probe") || strings.Contains(line, "未发现 AUDIO_DEVICE_OUT_REMOTE_SUBMIX") {
			return map[string]string{
				"status": "waiting_remote_submix",
				"detail": "桥接进程运行中，等待投屏/LiveKit 音频消费者创建 remote_submix。",
			}
		}
		if strings.Contains(line, "candidate_probe_no_active_track") {
			return map[string]string{
				"status": "waiting_call_track",
				"detail": "桥接进程运行中，暂未发现真实供帧的 IM 通话下行 Track。",
			}
		}
		if strings.Contains(line, "bridge_idle_rediscover") {
			return map[string]string{
				"status": "rediscovering",
				"detail": "通话 Track 空闲，桥接进程正在重新发现新的下行音频 Track。",
			}
		}
		if strings.Contains(line, "Docker shell command failed") || strings.Contains(line, "pidof 读取失败") {
			return map[string]string{
				"status": "transport_wait",
				"detail": "桥接进程运行中，Docker shell 探测超时，等待下一轮重试。",
			}
		}
		if strings.Contains(line, "discover_wait reason=") {
			reason := strings.TrimSpace(strings.SplitN(line, "discover_wait reason=", 2)[1])
			if strings.Contains(reason, "AUDIO_DEVICE_OUT_REMOTE_SUBMIX") || strings.Contains(reason, "remote_submix") {
				return map[string]string{
					"status": "waiting_remote_submix",
					"detail": "桥接进程运行中，等待投屏/LiveKit 音频消费者创建 remote_submix。",
				}
			}
			return map[string]string{
				"status": "discover_wait",
				"detail": "桥接进程运行中，等待可桥接音频：" + reason,
			}
		}
	}
	return nil
}

func parseVendorMonopipeStatus(lines []string) (map[string]string, int) {
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if !strings.Contains(line, "vendor_monopipe_summary") {
			continue
		}
		fields := lineFields(line)
		writeCount := positiveInt(fields["writes"])
		byteCount := positiveInt(fields["bytes"])
		noPipe := positiveInt(fields["no_pipe"])
		dropped := positiveInt(fields["dropped"])
		postedChunks, postedBytes := splitCountPair(fields["posted"])
		switch {
		case writeCount > 0 || byteCount > 0:
			return map[string]string{
				"status": "bridging",
				"detail": fmt.Sprintf("vendor_monopipe bytes observed bytes=%d writes=%d posted=%d/%d", byteCount, writeCount, postedChunks, postedBytes),
			}, index
		case noPipe > 0 && (postedChunks > 0 || postedBytes > 0):
			return map[string]string{
				"status": "waiting_remote_submix",
				"detail": fmt.Sprintf("vendor_monopipe waiting for remote_submix pipe no_pipe=%d posted=%d/%d dropped=%d", noPipe, postedChunks, postedBytes, dropped),
			}, index
		default:
			return map[string]string{
				"status": "probing",
				"detail": fmt.Sprintf("vendor_monopipe attached, waiting for remote_submix activity posted=%d/%d dropped=%d", postedChunks, postedBytes, dropped),
			}, index
		}
	}
	return nil, -1
}

func parseFridaConnectionStatus(lines []string) (map[string]string, int) {
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if strings.Contains(line, "bridge_start") || strings.Contains(line, "hidden_voice_track_discovered") || strings.Contains(line, "bridge_tick") {
			return nil, -1
		}
		if (strings.Contains(line, "frida_remote_output_probe_failed") || strings.Contains(line, "hidden_voice_probe_failed")) && strings.Contains(line, "connection closed") {
			return map[string]string{
				"status": "frida_disconnected",
				"detail": "frida-server connection closed; supervisor should restart/reuse Android frida-server",
			}, index
		}
	}
	return nil, -1
}

func readLogTail(path string, maxBytes int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	stat, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := stat.Size() - maxBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return string(raw)
}

func nonEmptyLines(text string) []string {
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func lineFields(line string) map[string]string {
	fields := map[string]string{}
	for _, match := range lineFieldPattern.FindAllStringSubmatch(line, -1) {
		if len(match) == 3 {
			fields[match[1]] = match[2]
		}
	}
	return fields
}

func splitCountPair(value any) (int, int) {
	parts := strings.SplitN(clean(value), "/", 2)
	left := 0
	right := 0
	if len(parts) > 0 {
		left = positiveInt(parts[0])
	}
	if len(parts) > 1 {
		right = positiveInt(parts[1])
	}
	return left, right
}

func appendRoot(roots []string, root string) []string {
	if strings.TrimSpace(root) == "" {
		return roots
	}
	resolved, err := filepath.Abs(root)
	if err != nil {
		return roots
	}
	if symlinkResolved, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = symlinkResolved
	}
	resolved = filepath.Clean(resolved)
	if !containsString(roots, resolved) {
		roots = append(roots, resolved)
	}
	return roots
}

func appendPathCandidate(candidates []string, path string) []string {
	if strings.TrimSpace(path) == "" {
		return candidates
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return candidates
	}
	resolved = filepath.Clean(resolved)
	if !containsString(candidates, resolved) {
		candidates = append(candidates, resolved)
	}
	return candidates
}

func pathIsInside(path string, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative))
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
