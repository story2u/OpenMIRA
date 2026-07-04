export const DEVICES_PATH = "/devices";
export const DEVICES_MANUAL_PATH = "/devices/manual";
export const DEVICES_DISCOVERY_REFRESH_PATH = "/devices/discovery/refresh";
export const DEVICES_DISCOVERY_PROBE_PATH = "/devices/discovery/probe";
export const ROOT_ROUTE_BASE_PATH = "";
export const WEWORK_LOGIN_STATUS_PATH = "/wework/login/status";
export const WEWORK_LOGIN_QRCODE_PATH = "/wework/login/qrcode";
export const WEWORK_LOGIN_VERIFY_PATH = "/wework/login/verify-code";
export const WEWORK_LOGOUT_PATH = "/wework/logout";
export const WEWORK_USER_INFO_REQUEST_PATH = "/wework/user-info/request";
export const DEVICE_RTC_CONTROL_ACTIONS = new Set(["acquire", "release", "steal"]);

export const DEVICE_SDK_ACTIONS = new Set([
  "open_wework",
  "stop_wework",
  "prepare_call_audio_output",
]);

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAdminDevices(payload = {}) {
  const devices = Array.isArray(payload?.devices)
    ? payload.devices
    : Array.isArray(payload?.data?.devices)
      ? payload.data.devices
      : Array.isArray(payload)
        ? payload
        : [];
  return devices.map(normalizeAdminDevice).filter(Boolean);
}

export function normalizeAdminDevice(device = {}) {
  const deviceId = cleanText(device?.device_id || device?.deviceId);
  const agentId = cleanText(device?.agent_id || device?.agentId);
  if (!deviceId && !agentId) return null;
  const online = parseBool(device?.online, false);
  const weworkLoggedIn = parseBool(device?.wework_logged_in || device?.weworkLoggedIn, false);
  const sdkRoute = parseBool(device?.sdk_route || device?.sdkRoute, false);
  const p1Host = cleanText(device?.p1_host || device?.p1Host);
  const p1ManagerHost = cleanText(firstDefined(
    device?.p1_manager_host,
    device?.p1ManagerHost,
    device?.manager_host,
    device?.managerHost,
  ));
  const p1DeviceIP = cleanText(firstDefined(
    device?.p1_device_ip,
    device?.p1DeviceIP,
    device?.device_ip,
    device?.deviceIP,
  ));
  return {
    agentId,
    deviceId,
    model: cleanText(device?.model),
    androidVersion: cleanText(device?.android_version || device?.androidVersion),
    online,
    onlineLabel: online ? "在线" : "离线",
    weworkLoggedIn,
    weworkLoggedInLabel: weworkLoggedIn ? "已登录" : "未登录",
    weworkStatus: cleanText(device?.wework_status || device?.weworkStatus),
    version: cleanText(device?.version),
    timestamp: cleanText(device?.timestamp || device?.updated_at || device?.updatedAt),
    sdkRoute,
    sdkConnectable: parseBool(device?.sdk_connectable || device?.sdkConnectable, false),
    p1Host,
    p1ManagerHost,
    p1DeviceIP,
    p1Slot: cleanText(device?.p1_slot || device?.p1Slot),
    p1ManagerPort: normalizePositiveInt(firstDefined(device?.p1_manager_port, device?.p1ManagerPort, device?.manager_port, device?.managerPort)),
    loginAccountName: cleanText(device?.login_account_name || device?.loginAccountName),
    loginWeWorkUserId: cleanText(device?.login_wework_user_id || device?.loginWeWorkUserId),
    loginOrganizationName: cleanText(device?.login_organization_name || device?.loginOrganizationName),
    loginAccountAvatar: cleanText(device?.login_account_avatar || device?.loginAccountAvatar),
    raw: device,
  };
}

export function buildManualDeviceUpsertMutation(options = {}) {
  const agentId = cleanText(options.agentId || options.agent_id);
  if (!agentId) return { ok: false, error: "agent_id_required" };
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const body = {
    agent_id: agentId,
    device_id: deviceId,
    online: parseBool(firstDefined(options.online, true), true),
  };
  const model = cleanText(options.model);
  if (model) body.model = model;
  const androidVersion = cleanText(options.androidVersion || options.android_version);
  if (androidVersion) body.android_version = androidVersion;
  const weworkLoggedIn = parseOptionalBool(firstDefined(options.weworkLoggedIn, options.wework_logged_in));
  if (weworkLoggedIn !== null) body.wework_logged_in = weworkLoggedIn;
  return {
    ok: true,
    method: "POST",
    path: DEVICES_MANUAL_PATH,
    body,
  };
}

export function buildManualDeviceDeleteMutation(options = {}) {
  const agentId = cleanText(options.agentId || options.agent_id);
  if (!agentId) return { ok: false, error: "agent_id_required" };
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const params = new URLSearchParams({ agent_id: agentId, device_id: deviceId });
  return {
    ok: true,
    method: "DELETE",
    path: `${DEVICES_MANUAL_PATH}?${params.toString()}`,
  };
}

export function buildDeviceDiscoveryRefreshMutation() {
  return {
    ok: true,
    method: "POST",
    path: DEVICES_DISCOVERY_REFRESH_PATH,
  };
}

export function buildDeviceDiscoveryProbeMutation(options = {}) {
  const body = {};
  const deviceIP = cleanText(firstDefined(options.deviceIP, options.device_ip));
  if (deviceIP) body.device_ip = deviceIP;
  const managerHost = cleanText(firstDefined(options.managerHost, options.manager_host));
  if (managerHost) body.manager_host = managerHost;
  const managerPort = normalizePositiveInt(firstDefined(options.managerPort, options.manager_port));
  if (managerPort > 0) body.manager_port = managerPort;
  const sdkHost = cleanText(firstDefined(options.sdkHost, options.sdk_host));
  if (sdkHost) body.sdk_host = sdkHost;
  const webrtcHost = cleanText(firstDefined(options.webrtcHost, options.webrtc_host));
  if (webrtcHost) body.webrtc_host = webrtcHost;
  const timeoutSec = normalizePositiveNumber(firstDefined(options.timeoutSec, options.timeout_sec));
  if (timeoutSec > 0) body.timeout_sec = timeoutSec;
  body.apply_on_success = parseBool(firstDefined(options.applyOnSuccess, options.apply_on_success), false);
  return {
    ok: true,
    method: "POST",
    path: DEVICES_DISCOVERY_PROBE_PATH,
    body,
  };
}

export function buildWeWorkLoginStatusRequest(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  return {
    ok: true,
    method: "GET",
    path: WEWORK_LOGIN_STATUS_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
    params: {
      device_id: deviceId,
      live: parseBool(firstDefined(options.live, false), false) ? "true" : "",
      include_qrcode: parseBool(firstDefined(options.includeQRCode, options.include_qrcode), false) ? "true" : "false",
    },
  };
}

export function buildWeWorkLoginQRCodeMutation(options = {}) {
  const body = deviceActionBody(options);
  if (!body.device_id) return { ok: false, error: "device_id_required" };
  const timeoutSeconds = normalizePositiveInt(firstDefined(options.timeoutSeconds, options.timeout_seconds));
  if (timeoutSeconds > 0) body.timeout_seconds = timeoutSeconds;
  return {
    ok: true,
    method: "POST",
    path: WEWORK_LOGIN_QRCODE_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
    body,
  };
}

export function buildWeWorkVerifyMutation(options = {}) {
  const body = deviceActionBody(options);
  if (!body.device_id) return { ok: false, error: "device_id_required" };
  const verifyCode = cleanText(options.verifyCode || options.verify_code);
  if (!verifyCode) return { ok: false, error: "verify_code_required" };
  body.verify_code = verifyCode;
  const verifyType = cleanText(options.verifyType || options.verify_type);
  if (verifyType) body.verify_type = verifyType;
  return {
    ok: true,
    method: "POST",
    path: WEWORK_LOGIN_VERIFY_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
    body,
  };
}

export function buildWeWorkLogoutMutation(options = {}) {
  const body = deviceActionBody(options);
  if (!body.device_id) return { ok: false, error: "device_id_required" };
  return {
    ok: true,
    method: "POST",
    path: WEWORK_LOGOUT_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
    body,
  };
}

export function buildWeWorkUserInfoRequestMutation(options = {}) {
  const body = deviceActionBody(options);
  if (!body.device_id) return { ok: false, error: "device_id_required" };
  return {
    ok: true,
    method: "POST",
    path: WEWORK_USER_INFO_REQUEST_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
    body,
  };
}

export function buildDeviceSDKControlMutation(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const action = cleanText(options.action).toLowerCase();
  if (!DEVICE_SDK_ACTIONS.has(action)) return { ok: false, error: "unknown_device_action" };
  const encodedDeviceID = encodeURIComponent(deviceId);
  if (action === "prepare_call_audio_output") {
    const callType = cleanText(options.callType || options.call_type || "voice");
    if (callType !== "voice" && callType !== "video") return { ok: false, error: "call_type_invalid" };
    return {
      ok: true,
      method: "POST",
      path: `/devices/${encodedDeviceID}/sdk/prepare-call-audio-output?call_type=${encodeURIComponent(callType)}`,
    };
  }
  const suffix = action === "open_wework" ? "open-wework" : "stop-wework";
  return {
    ok: true,
    method: "POST",
    path: `/devices/${encodedDeviceID}/sdk/${suffix}`,
  };
}

export function buildDeviceSDKWebRTCRequest(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const quality = normalizeRTCQuality(options.quality);
  if (quality === null) return { ok: false, error: "quality_invalid" };
  return {
    ok: true,
    method: "GET",
    path: `/devices/${encodeURIComponent(deviceId)}/sdk/webrtc`,
    params: quality ? { quality } : {},
  };
}

export function buildDeviceSDKRTCSessionRequest(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const quality = normalizeRTCQuality(options.quality);
  if (quality === null) return { ok: false, error: "quality_invalid" };
  const mode = cleanText(options.mode || "auto").toLowerCase();
  if (!["auto", "legacy", "livekit"].includes(mode)) return { ok: false, error: "rtc_mode_invalid" };
  const params = { mode };
  if (quality) params.quality = quality;
  return {
    ok: true,
    method: "GET",
    path: `/devices/${encodeURIComponent(deviceId)}/sdk/rtc-session`,
    params,
  };
}

export function buildDeviceRTCActiveMutation(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const participantIdentity = cleanText(options.participantIdentity || options.participant_identity);
  if (!participantIdentity) return { ok: false, error: "participant_identity_required" };
  return {
    ok: true,
    method: "POST",
    path: `/devices/${encodeURIComponent(deviceId)}/rtc-active`,
    body: { participant_identity: participantIdentity },
  };
}

export function buildDeviceRTCActiveListRequest() {
  return {
    ok: true,
    method: "GET",
    path: "/devices/rtc/active",
  };
}

export function buildDeviceRTCControlStateRequest(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  return {
    ok: true,
    method: "GET",
    path: `/devices/${encodeURIComponent(deviceId)}/control/state`,
  };
}

export function buildDeviceRTCControlMutation(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const action = cleanText(options.action).toLowerCase();
  if (!DEVICE_RTC_CONTROL_ACTIONS.has(action)) return { ok: false, error: "unknown_rtc_control_action" };
  const participantIdentity = cleanText(options.participantIdentity || options.participant_identity);
  if (!participantIdentity) return { ok: false, error: "participant_identity_required" };
  return {
    ok: true,
    method: "POST",
    path: `/devices/${encodeURIComponent(deviceId)}/control/${action}`,
    body: { participant_identity: participantIdentity },
  };
}

export function buildDeviceRTCControlInputMutation(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const participantIdentity = cleanText(options.participantIdentity || options.participant_identity);
  if (!participantIdentity) return { ok: false, error: "participant_identity_required" };
  const kind = cleanText(options.kind || "key").toLowerCase();
  if (!["pointer", "key", "text"].includes(kind)) return { ok: false, error: "control_input_kind_invalid" };
  const action = cleanText(options.action || (kind === "text" ? "input" : "press")).toLowerCase();
  const body = {
    participant_identity: participantIdentity,
    kind,
    action,
    x: clampRatio(firstDefined(options.x, 0.5)),
    y: clampRatio(firstDefined(options.y, 0.5)),
    delta_x: normalizeNumber(firstDefined(options.deltaX, options.delta_x)),
    delta_y: normalizeNumber(firstDefined(options.deltaY, options.delta_y)),
    ts: normalizePositiveInt(firstDefined(options.ts, Date.now())),
  };
  if (kind === "key") {
    const key = cleanText(options.key);
    if (!key) return { ok: false, error: "control_input_key_required" };
    body.key = key;
  }
  if (kind === "text") {
    const text = String(options.text || "");
    if (!text) return { ok: false, error: "control_input_text_required" };
    body.text = text;
  }
  return {
    ok: true,
    method: "POST",
    path: `/devices/${encodeURIComponent(deviceId)}/control/input`,
    body,
  };
}

export function buildDeviceRTCMediaStartMutation(options = {}) {
  const deviceId = cleanText(options.deviceId || options.device_id);
  if (!deviceId) return { ok: false, error: "device_id_required" };
  const participantIdentity = cleanText(options.participantIdentity || options.participant_identity);
  if (!participantIdentity) return { ok: false, error: "participant_identity_required" };
  const body = {
    participant_identity: participantIdentity,
    activate: false,
    camera: parseBool(firstDefined(options.camera, true), true),
    microphone: parseBool(firstDefined(options.microphone, true), true),
  };
  const streamInstance = cleanText(options.streamInstance || options.stream_instance);
  if (streamInstance) body.stream_instance = streamInstance;
  const cameraAddr = cleanText(options.cameraAddr || options.camera_addr);
  if (cameraAddr) body.camera_addr = cameraAddr;
  const whipPublishURL = cleanText(options.whipPublishURL || options.whip_publish_url);
  if (whipPublishURL) body.whip_publish_url = whipPublishURL;
  const cameraStreamType = normalizePositiveInt(firstDefined(options.cameraStreamType, options.camera_stream_type));
  if (cameraStreamType > 0) body.camera_stream_type = cameraStreamType;
  const cameraResolution = normalizePositiveInt(firstDefined(options.cameraResolution, options.camera_resolution));
  if (cameraResolution > 0) body.camera_resolution = cameraResolution;
  return {
    ok: true,
    method: "POST",
    path: `/devices/${encodeURIComponent(deviceId)}/media/start`,
    body,
  };
}

export function normalizeDeviceDiscoveryRefreshResult(payload = {}) {
  const diagnostics = payload?.diagnostics || {};
  const managerDiagnostics = diagnostics?.manager || {};
  const sdkDiagnostics = diagnostics?.sdk || {};
  return {
    success: payload?.success !== false,
    devicesDiscovered: normalizeNonNegativeInt(firstDefined(payload?.devices_discovered, payload?.devicesDiscovered)),
    managerDevices: normalizeNonNegativeInt(firstDefined(payload?.manager_devices, payload?.managerDevices)),
    sdkDevices: normalizeNonNegativeInt(firstDefined(payload?.sdk_devices, payload?.sdkDevices)),
    errors: normalizeStringList(payload?.errors),
    managerConfigured: parseBool(managerDiagnostics?.configured, false),
    managerCachedDevices: normalizeNonNegativeInt(firstDefined(managerDiagnostics?.cached_devices, managerDiagnostics?.cachedDevices)),
    sdkExecutorConfigured: parseBool(firstDefined(sdkDiagnostics?.executor_configured, sdkDiagnostics?.executorConfigured), false),
    raw: payload,
  };
}

export function normalizeDeviceDiscoveryProbeResult(payload = {}) {
  const target = payload?.target || {};
  const managerTCP = payload?.manager_tcp || payload?.managerTCP || {};
  const manager = payload?.manager || {};
  const rpa = payload?.rpa || {};
  const webrtc = payload?.webrtc || {};
  const applyErrors = normalizeStringList(firstDefined(payload?.apply_errors, payload?.applyErrors));
  const errors = uniqueStrings([
    ...normalizeStringList(manager?.errors),
    ...normalizeStringList(rpa?.errors),
    ...normalizeStringList(webrtc?.errors),
    ...applyErrors,
    ...(parseBool(managerTCP?.success, false) ? [] : normalizeStringList(managerTCP?.error)),
  ]);
  return {
    success: parseBool(payload?.success, false),
    applied: parseBool(payload?.applied, false),
    applyErrors,
    target: {
      deviceIP: cleanText(firstDefined(target?.device_ip, target?.deviceIP)),
      requestedDeviceIP: cleanText(firstDefined(target?.requested_device_ip, target?.requestedDeviceIP)),
      candidateDeviceIPs: normalizeStringList(firstDefined(target?.candidate_device_ips, target?.candidateDeviceIPs)),
      detectedDeviceIPs: normalizeStringList(firstDefined(target?.detected_device_ips, target?.detectedDeviceIPs)),
      autoDetected: parseBool(firstDefined(target?.auto_detected, target?.autoDetected), false),
      managerHost: cleanText(firstDefined(target?.manager_host, target?.managerHost)),
      managerPort: normalizePositiveInt(firstDefined(target?.manager_port, target?.managerPort)),
      sdkHost: cleanText(firstDefined(target?.sdk_host, target?.sdkHost)),
      webrtcHost: cleanText(firstDefined(target?.webrtc_host, target?.webrtcHost)),
    },
    detectedDeviceIPs: normalizeStringList(firstDefined(payload?.detected_device_ips, payload?.detectedDeviceIPs, target?.detected_device_ips, target?.detectedDeviceIPs)),
    probeCandidateCount: Array.isArray(payload?.probe_candidates) ? payload.probe_candidates.length : 0,
    managerTCPSuccess: parseBool(managerTCP?.success, false),
    managerSuccess: parseBool(manager?.success, false),
    managerDeviceCount: normalizeNonNegativeInt(firstDefined(manager?.device_count, manager?.deviceCount)),
    managerRunningCount: normalizeNonNegativeInt(firstDefined(manager?.running_count, manager?.runningCount)),
    managerMethod: cleanText(manager?.method),
    rpaSuccess: parseBool(rpa?.success, false),
    rpaTargetCount: Array.isArray(rpa?.targets) ? rpa.targets.length : 0,
    webrtcSuccess: parseBool(webrtc?.success, false),
    webrtcTargetCount: Array.isArray(webrtc?.targets) ? webrtc.targets.length : 0,
    suggestedEnv: normalizeSuggestedEnv(firstDefined(payload?.suggested_env, payload?.suggestedEnv)),
    errors,
    raw: payload,
  };
}

export function normalizeDeviceActionResult(payload = {}) {
  const data = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  const task = data?.task && typeof data.task === "object" ? data.task : {};
  return {
    success: data?.success !== false,
    status: cleanText(data?.status || data?.login_status || task?.status),
    message: cleanText(data?.message || data?.detail || data?.error),
    taskID: cleanText(data?.task_id || data?.taskId || task?.task_id || task?.taskId),
    taskType: cleanText(data?.task_type || data?.taskType || task?.task_type || task?.taskType),
    qrcode: cleanText(data?.qrcode || data?.qr_code || data?.qrcode_url || data?.qrcodeUrl),
    expiresAt: cleanText(data?.expires_at || data?.expiresAt),
    deviceID: cleanText(data?.device_id || data?.deviceId),
    raw: data && typeof data === "object" ? data : {},
  };
}

export function normalizeWeWorkLoginStatus(payload = {}) {
  const data = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  return {
    found: data?.found !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    status: cleanText(data?.status || data?.login_status || data?.loginStatus),
    accountName: cleanText(data?.account_name || data?.accountName || data?.login_account_name || data?.loginAccountName),
    weworkUserID: cleanText(data?.wework_user_id || data?.weworkUserId || data?.wework_userid),
    qrcode: cleanText(data?.qrcode || data?.qr_code || data?.qrcode_url || data?.qrcodeUrl),
    expiresAt: cleanText(data?.expires_at || data?.expiresAt),
    taskID: cleanText(data?.task_id || data?.taskId),
    raw: data && typeof data === "object" ? data : {},
  };
}

export function normalizeDeviceWebRTCResult(payload = {}) {
  const data = unwrapPayload(payload);
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    slot: normalizeSlotLabel(data?.slot),
    url: cleanText(data?.url),
    fallbackURL: cleanText(data?.fallback_url || data?.fallbackURL),
    directURL: cleanText(data?.direct_url || data?.directURL),
    managerURL: cleanText(data?.manager_url || data?.managerURL),
    webrtcTCPPort: normalizeNonNegativeInt(firstDefined(data?.webrtc_tcp_port, data?.webrtcTCPPort)),
    webrtcUDPPort: normalizeNonNegativeInt(firstDefined(data?.webrtc_udp_port, data?.webrtcUDPPort)),
    raw: data,
  };
}

export function normalizeDeviceRTCSessionResult(payload = {}) {
  const data = unwrapPayload(payload);
  const controlState = data?.control_state && typeof data.control_state === "object" ? data.control_state : {};
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    mode: cleanText(data?.mode),
    modeReason: cleanText(data?.mode_reason || data?.modeReason),
    requestedMode: cleanText(data?.requested_mode || data?.requestedMode),
    entryURL: cleanText(data?.entry_url || data?.entryURL || data?.url),
    livekitURL: cleanText(data?.livekit_url || data?.livekitURL),
    roomName: cleanText(data?.room_name || data?.roomName),
    participantIdentity: cleanText(data?.participant_identity || data?.participantIdentity),
    bridgeIdentity: cleanText(data?.bridge_identity || data?.bridgeIdentity),
    tokenTTLSeconds: normalizeNonNegativeInt(firstDefined(data?.token_ttl_sec, data?.tokenTTLSeconds)),
    token: cleanText(data?.token),
    controlState: normalizeDeviceRTCControlState(controlState),
    raw: data,
  };
}

export function normalizeDeviceRTCActiveResult(payload = {}) {
  const data = unwrapPayload(payload);
  const devices = Array.isArray(data?.devices) ? data.devices : null;
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    roomName: cleanText(data?.room_name || data?.roomName),
    participantIdentity: cleanText(data?.participant_identity || data?.participantIdentity),
    activeAt: cleanText(data?.active_at || data?.activeAt),
    expiresAt: cleanText(data?.expires_at || data?.expiresAt),
    devices: devices ? devices.map(normalizeDeviceRTCActiveDevice).filter(Boolean) : [],
    raw: data,
  };
}

export function normalizeDeviceRTCControlState(payload = {}) {
  const data = unwrapPayload(payload);
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    controllerIdentity: cleanText(data?.controller_identity || data?.controllerIdentity),
    controllerUserID: cleanText(data?.controller_user_id || data?.controllerUserID),
    controllerName: cleanText(data?.controller_name || data?.controllerName),
    controllerRole: cleanText(data?.controller_role || data?.controllerRole),
    acquiredAt: cleanText(data?.acquired_at || data?.acquiredAt),
    expiresAt: cleanText(data?.expires_at || data?.expiresAt),
    raw: data,
  };
}

export function normalizeDeviceRTCControlInputResult(payload = {}) {
  const data = unwrapPayload(payload);
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    route: cleanText(data?.route),
    sent: parseBool(data?.sent, false),
    detail: cleanText(data?.detail),
    ageMS: normalizeNumber(firstDefined(data?.age_ms, data?.ageMS)),
    slotMS: normalizeNumber(firstDefined(data?.slot_ms, data?.slotMS)),
    controlStateMS: normalizeNumber(firstDefined(data?.control_state_ms, data?.controlStateMS)),
    acquireMS: normalizeNumber(firstDefined(data?.acquire_ms, data?.acquireMS)),
    sendMS: normalizeNumber(firstDefined(data?.send_ms, data?.sendMS)),
    dispatchMS: normalizeNumber(firstDefined(data?.dispatch_ms, data?.dispatchMS)),
    totalMS: normalizeNumber(firstDefined(data?.total_ms, data?.totalMS)),
    screenWidth: normalizeNonNegativeInt(firstDefined(data?.screen_width, data?.screenWidth)),
    screenHeight: normalizeNonNegativeInt(firstDefined(data?.screen_height, data?.screenHeight)),
    raw: data,
  };
}

export function normalizeDeviceRTCMediaStartResult(payload = {}) {
  const data = unwrapPayload(payload);
  const camera = normalizeDeviceMediaStream(data?.camera);
  const audio = normalizeDeviceMediaStream(data?.audio);
  return {
    success: data?.success !== false,
    deviceID: cleanText(data?.device_id || data?.deviceId),
    roomName: cleanText(data?.room_name || data?.roomName),
    controllerIdentity: cleanText(data?.controller_identity || data?.controllerIdentity),
    camera,
    audio,
    raw: data,
  };
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function unwrapPayload(payload = {}) {
  return payload?.data && typeof payload.data === "object" ? payload.data : payload && typeof payload === "object" ? payload : {};
}

function deviceActionBody(options = {}) {
  const body = {
    device_id: cleanText(options.deviceId || options.device_id),
  };
  const agentId = cleanText(options.agentId || options.agent_id);
  if (agentId) body.agent_id = agentId;
  const source = cleanText(options.source || "admin-dashboard");
  if (source) body.source = source;
  return body;
}

function parseOptionalBool(value) {
  if (value === undefined || value === null || cleanText(value) === "") return null;
  return parseBool(value, false);
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "在线", "已登录"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "离线", "未登录"].includes(normalized)) return false;
  return fallback;
}

function normalizePositiveInt(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return Math.floor(number);
}

function normalizePositiveNumber(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return number;
}

function normalizeNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : 0;
}

function clampRatio(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0) return 0;
  if (number > 1) return 1;
  return number;
}

function normalizeNonNegativeInt(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return Math.floor(number);
}

function normalizeRTCQuality(value) {
  const quality = cleanText(value);
  if (!quality) return "";
  if (quality === "0" || quality === "1") return quality;
  return null;
}

function normalizeSlotLabel(value) {
  if (value && typeof value === "object") {
    return cleanText(firstDefined(value.slot, value.index, value.device_id, value.deviceId));
  }
  return cleanText(value);
}

function normalizeDeviceRTCActiveDevice(value = {}) {
  if (!value || typeof value !== "object") return null;
  const deviceID = cleanText(value?.device_id || value?.deviceId);
  const participantIdentity = cleanText(value?.participant_identity || value?.participantIdentity);
  if (!deviceID && !participantIdentity) return null;
  return {
    deviceID,
    roomName: cleanText(value?.room_name || value?.roomName),
    participantIdentity,
    activeAt: cleanText(value?.active_at || value?.activeAt),
    expiresAt: cleanText(value?.expires_at || value?.expiresAt),
    raw: value,
  };
}

function normalizeDeviceMediaStream(value = {}) {
  const data = value && typeof value === "object" ? value : {};
  return {
    status: cleanText(data?.status),
    transport: cleanText(data?.transport),
    addr: cleanText(data?.addr),
    streamKey: cleanText(data?.stream_key || data?.streamKey),
    playbackURL: cleanText(data?.playback_url || data?.playbackURL),
    previewURL: cleanText(data?.preview_url || data?.previewURL),
    publishURL: cleanText(data?.publish_url || data?.publishURL),
    directPublishURL: cleanText(data?.direct_publish_url || data?.directPublishURL),
    publishProtocol: cleanText(data?.publish_protocol || data?.publishProtocol),
    playbackProtocol: cleanText(data?.playback_protocol || data?.playbackProtocol),
    consumerStatus: cleanText(data?.consumer_status || data?.consumerStatus),
    detail: cleanText(data?.detail || data?.consumer_detail || data?.consumerDetail),
    raw: data,
  };
}

function normalizeStringList(value) {
  const values = Array.isArray(value) ? value : value === undefined || value === null ? [] : [value];
  return values.map(cleanText).filter(Boolean);
}

function normalizeSuggestedEnv(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => ({
      name: cleanText(item?.name),
      value: cleanText(item?.value),
      changed: parseBool(item?.changed, false),
    }))
    .filter((item) => item.name);
}

function uniqueStrings(values = []) {
  const result = [];
  values.forEach((value) => {
    const normalized = cleanText(value);
    if (normalized && !result.includes(normalized)) result.push(normalized);
  });
  return result;
}
