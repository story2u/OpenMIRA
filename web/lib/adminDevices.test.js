import assert from "node:assert/strict";
import test from "node:test";

import {
  ROOT_ROUTE_BASE_PATH,
  WEWORK_LOGIN_QRCODE_PATH,
  WEWORK_LOGIN_STATUS_PATH,
  WEWORK_LOGIN_VERIFY_PATH,
  WEWORK_LOGOUT_PATH,
  WEWORK_USER_INFO_REQUEST_PATH,
  DEVICES_DISCOVERY_PROBE_PATH,
  DEVICES_DISCOVERY_REFRESH_PATH,
  DEVICES_MANUAL_PATH,
  buildDeviceSDKControlMutation,
  buildDeviceSDKRTCSessionRequest,
  buildDeviceSDKWebRTCRequest,
  buildDeviceRTCActiveListRequest,
  buildDeviceRTCActiveMutation,
  buildDeviceRTCControlInputMutation,
  buildDeviceRTCControlMutation,
  buildDeviceRTCControlStateRequest,
  buildDeviceRTCMediaStartMutation,
  buildDeviceDiscoveryProbeMutation,
  buildDeviceDiscoveryRefreshMutation,
  buildManualDeviceDeleteMutation,
  buildManualDeviceUpsertMutation,
  buildWeWorkLoginQRCodeMutation,
  buildWeWorkLoginStatusRequest,
  buildWeWorkLogoutMutation,
  buildWeWorkUserInfoRequestMutation,
  buildWeWorkVerifyMutation,
  normalizeDeviceActionResult,
  normalizeDeviceDiscoveryProbeResult,
  normalizeDeviceDiscoveryRefreshResult,
  normalizeDeviceRTCActiveResult,
  normalizeDeviceRTCControlInputResult,
  normalizeDeviceRTCControlState,
  normalizeDeviceRTCMediaStartResult,
  normalizeDeviceRTCSessionResult,
  normalizeDeviceWebRTCResult,
  normalizeAdminDevices,
  normalizeWeWorkLoginStatus,
} from "./adminDevices.js";

test("normalizeAdminDevices keeps device list fields", () => {
  const devices = normalizeAdminDevices({
    devices: [
      {
        agent_id: "sdk:slot-18",
        device_id: "slot-18",
        online: true,
        wework_logged_in: true,
        wework_status: "normal",
        model: "设备位 18",
        version: "sdk-manager",
        sdk_route: true,
        p1_host: "192.168.1.30",
        p1_manager_host: "100.64.0.18",
        p1_device_ip: "192.168.1.18",
        p1_manager_port: 83,
        p1_slot: 18,
        login_account_name: "客服一",
        login_wework_user_id: "dy1801",
        login_organization_name: "黛伊",
        login_account_avatar: "https://example.com/avatar.png",
      },
      { model: "missing ids" },
    ],
  });

  assert.equal(devices.length, 1);
  assert.equal(devices[0].agentId, "sdk:slot-18");
  assert.equal(devices[0].deviceId, "slot-18");
  assert.equal(devices[0].onlineLabel, "在线");
  assert.equal(devices[0].weworkLoggedInLabel, "已登录");
  assert.equal(devices[0].sdkRoute, true);
  assert.equal(devices[0].p1Host, "192.168.1.30");
  assert.equal(devices[0].p1ManagerHost, "100.64.0.18");
  assert.equal(devices[0].p1DeviceIP, "192.168.1.18");
  assert.equal(devices[0].p1ManagerPort, 83);
  assert.equal(devices[0].p1Slot, "18");
  assert.equal(devices[0].loginAccountName, "客服一");
  assert.equal(devices[0].loginWeWorkUserId, "dy1801");
  assert.equal(devices[0].loginOrganizationName, "黛伊");
  assert.equal(devices[0].loginAccountAvatar, "https://example.com/avatar.png");
});

test("buildManualDeviceUpsertMutation mirrors legacy manual payload", () => {
  const mutation = buildManualDeviceUpsertMutation({
    agentId: " agent-1 ",
    deviceId: " device-1 ",
    online: false,
    weworkLoggedIn: "true",
    model: " Pixel ",
    androidVersion: " 14 ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, DEVICES_MANUAL_PATH);
  assert.deepEqual(mutation.body, {
    agent_id: "agent-1",
    device_id: "device-1",
    online: false,
    wework_logged_in: true,
    model: "Pixel",
    android_version: "14",
  });
});

test("buildManualDeviceDeleteMutation mirrors legacy query delete", () => {
  const mutation = buildManualDeviceDeleteMutation({ agentId: "agent/1", deviceId: "device 1" });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${DEVICES_MANUAL_PATH}?agent_id=agent%2F1&device_id=device+1`);
});

test("manual device mutations validate required ids", () => {
  assert.equal(buildManualDeviceUpsertMutation({ deviceId: "device-1" }).error, "agent_id_required");
  assert.equal(buildManualDeviceUpsertMutation({ agentId: "agent-1" }).error, "device_id_required");
  assert.equal(buildManualDeviceDeleteMutation({ deviceId: "device-1" }).error, "agent_id_required");
  assert.equal(buildManualDeviceDeleteMutation({ agentId: "agent-1" }).error, "device_id_required");
});

test("buildDeviceDiscoveryRefreshMutation mirrors legacy refresh route", () => {
  const mutation = buildDeviceDiscoveryRefreshMutation();

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, DEVICES_DISCOVERY_REFRESH_PATH);
});

test("buildDeviceDiscoveryProbeMutation mirrors legacy probe payload", () => {
  const mutation = buildDeviceDiscoveryProbeMutation({
    deviceIP: " 192.168.1.30 ",
    managerHost: " 100.64.0.30 ",
    managerPort: "83",
    sdkHost: " 100.64.0.31 ",
    webrtcHost: " 100.64.0.32 ",
    timeoutSec: "0.5",
    applyOnSuccess: true,
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, DEVICES_DISCOVERY_PROBE_PATH);
  assert.deepEqual(mutation.body, {
    device_ip: "192.168.1.30",
    manager_host: "100.64.0.30",
    manager_port: 83,
    sdk_host: "100.64.0.31",
    webrtc_host: "100.64.0.32",
    timeout_sec: 0.5,
    apply_on_success: true,
  });
});

test("normalizeDeviceDiscoveryRefreshResult keeps counters and diagnostics", () => {
  const result = normalizeDeviceDiscoveryRefreshResult({
    success: false,
    devices_discovered: 1,
    manager_devices: 1,
    sdk_devices: 0,
    errors: ["sdk: SDK executor is not configured"],
    diagnostics: {
      manager: { configured: true, cached_devices: 1 },
      sdk: { executor_configured: false },
    },
  });

  assert.equal(result.success, false);
  assert.equal(result.devicesDiscovered, 1);
  assert.equal(result.managerDevices, 1);
  assert.equal(result.sdkDevices, 0);
  assert.equal(result.managerConfigured, true);
  assert.equal(result.managerCachedDevices, 1);
  assert.equal(result.sdkExecutorConfigured, false);
  assert.deepEqual(result.errors, ["sdk: SDK executor is not configured"]);
});

test("normalizeDeviceDiscoveryProbeResult keeps target, probes, suggestions and errors", () => {
  const result = normalizeDeviceDiscoveryProbeResult({
    success: true,
    applied: false,
    apply_errors: ["runtime_target: apply_on_success is not available in the Go candidate"],
    target: {
      device_ip: "192.168.1.30",
      requested_device_ip: "192.168.1.30",
      candidate_device_ips: ["192.168.1.30"],
      detected_device_ips: ["192.168.1.30"],
      manager_host: "100.64.0.30",
      manager_port: 83,
      sdk_host: "100.64.0.31",
      webrtc_host: "100.64.0.32",
    },
    detected_device_ips: ["192.168.1.30"],
    probe_candidates: [{ success: true }],
    manager_tcp: { success: true },
    manager: { success: true, method: "manager_cache", device_count: 2, running_count: 1, errors: [] },
    rpa: { success: true, targets: [{ host: "100.64.0.31", port: 7100 }] },
    webrtc: { success: false, targets: [] },
    suggested_env: [{ name: "P1_MANAGER_DEVICE_IPS", value: "192.168.1.30", changed: true }],
  });

  assert.equal(result.success, true);
  assert.equal(result.target.deviceIP, "192.168.1.30");
  assert.equal(result.target.managerPort, 83);
  assert.equal(result.managerDeviceCount, 2);
  assert.equal(result.managerRunningCount, 1);
  assert.equal(result.rpaTargetCount, 1);
  assert.equal(result.webrtcTargetCount, 0);
  assert.equal(result.suggestedEnv[0].name, "P1_MANAGER_DEVICE_IPS");
  assert.deepEqual(result.errors, ["runtime_target: apply_on_success is not available in the Go candidate"]);
});

test("wework login mutations use root legacy routes", () => {
  const status = buildWeWorkLoginStatusRequest({ deviceId: " device-1 ", includeQRCode: false });
  assert.equal(status.ok, true);
  assert.equal(status.method, "GET");
  assert.equal(status.path, WEWORK_LOGIN_STATUS_PATH);
  assert.equal(status.basePath, ROOT_ROUTE_BASE_PATH);
  assert.deepEqual(status.params, {
    device_id: "device-1",
    live: "",
    include_qrcode: "false",
  });

  const qrcode = buildWeWorkLoginQRCodeMutation({ deviceId: " device-1 ", agentId: " sdk:a ", timeoutSeconds: "30" });
  assert.equal(qrcode.path, WEWORK_LOGIN_QRCODE_PATH);
  assert.equal(qrcode.basePath, ROOT_ROUTE_BASE_PATH);
  assert.deepEqual(qrcode.body, {
    device_id: "device-1",
    agent_id: "sdk:a",
    source: "admin-dashboard",
    timeout_seconds: 30,
  });

  const verify = buildWeWorkVerifyMutation({ deviceId: "device-1", verifyCode: " 123456 ", verifyType: " sms " });
  assert.equal(verify.path, WEWORK_LOGIN_VERIFY_PATH);
  assert.deepEqual(verify.body, {
    device_id: "device-1",
    source: "admin-dashboard",
    verify_code: "123456",
    verify_type: "sms",
  });

  const logout = buildWeWorkLogoutMutation({ deviceId: "device-1" });
  assert.equal(logout.path, WEWORK_LOGOUT_PATH);
  assert.deepEqual(logout.body, { device_id: "device-1", source: "admin-dashboard" });

  const userInfo = buildWeWorkUserInfoRequestMutation({ deviceId: "device-1" });
  assert.equal(userInfo.path, WEWORK_USER_INFO_REQUEST_PATH);
  assert.deepEqual(userInfo.body, { device_id: "device-1", source: "admin-dashboard" });
});

test("device sdk control mutations encode device paths", () => {
  const open = buildDeviceSDKControlMutation({ deviceId: "slot/18", action: "open_wework" });
  assert.equal(open.ok, true);
  assert.equal(open.method, "POST");
  assert.equal(open.path, "/devices/slot%2F18/sdk/open-wework");

  const stop = buildDeviceSDKControlMutation({ deviceId: "slot 18", action: "stop_wework" });
  assert.equal(stop.path, "/devices/slot%2018/sdk/stop-wework");

  const prepare = buildDeviceSDKControlMutation({ deviceId: "slot-18", action: "prepare_call_audio_output", callType: "video" });
  assert.equal(prepare.path, "/devices/slot-18/sdk/prepare-call-audio-output?call_type=video");
});

test("device action helpers report invalid fields", () => {
  assert.equal(buildWeWorkLoginStatusRequest({}).error, "device_id_required");
  assert.equal(buildWeWorkLoginQRCodeMutation({}).error, "device_id_required");
  assert.equal(buildWeWorkVerifyMutation({ deviceId: "device-1" }).error, "verify_code_required");
  assert.equal(buildWeWorkLogoutMutation({}).error, "device_id_required");
  assert.equal(buildWeWorkUserInfoRequestMutation({}).error, "device_id_required");
  assert.equal(buildDeviceSDKControlMutation({ action: "open_wework" }).error, "device_id_required");
  assert.equal(buildDeviceSDKControlMutation({ deviceId: "device-1", action: "bad" }).error, "unknown_device_action");
  assert.equal(buildDeviceSDKControlMutation({ deviceId: "device-1", action: "prepare_call_audio_output", callType: "screen" }).error, "call_type_invalid");
});

test("device rtc helpers mirror legacy routes", () => {
  const webrtc = buildDeviceSDKWebRTCRequest({ deviceId: "slot/18", quality: "0" });
  assert.equal(webrtc.ok, true);
  assert.equal(webrtc.method, "GET");
  assert.equal(webrtc.path, "/devices/slot%2F18/sdk/webrtc");
  assert.deepEqual(webrtc.params, { quality: "0" });

  const session = buildDeviceSDKRTCSessionRequest({ deviceId: "slot 18", mode: "livekit", quality: "1" });
  assert.equal(session.path, "/devices/slot%2018/sdk/rtc-session");
  assert.deepEqual(session.params, { mode: "livekit", quality: "1" });

  const active = buildDeviceRTCActiveMutation({ deviceId: "slot-18", participantIdentity: " viewer-1 " });
  assert.equal(active.path, "/devices/slot-18/rtc-active");
  assert.deepEqual(active.body, { participant_identity: "viewer-1" });

  const activeList = buildDeviceRTCActiveListRequest();
  assert.equal(activeList.path, "/devices/rtc/active");

  const state = buildDeviceRTCControlStateRequest({ deviceId: "slot-18" });
  assert.equal(state.path, "/devices/slot-18/control/state");

  const acquire = buildDeviceRTCControlMutation({ deviceId: "slot-18", action: "acquire", participantIdentity: "viewer-1" });
  assert.equal(acquire.method, "POST");
  assert.equal(acquire.path, "/devices/slot-18/control/acquire");
  assert.deepEqual(acquire.body, { participant_identity: "viewer-1" });

  const input = buildDeviceRTCControlInputMutation({
    deviceId: "slot-18",
    participantIdentity: "viewer-1",
    kind: "key",
    action: "press",
    key: "Arrow_Left",
    x: 1.5,
    y: -0.2,
    ts: 123,
  });
  assert.equal(input.method, "POST");
  assert.equal(input.path, "/devices/slot-18/control/input");
  assert.deepEqual(input.body, {
    participant_identity: "viewer-1",
    kind: "key",
    action: "press",
    x: 1,
    y: 0,
    delta_x: 0,
    delta_y: 0,
    ts: 123,
    key: "Arrow_Left",
  });

  const textInput = buildDeviceRTCControlInputMutation({
    deviceId: "slot-18",
    participantIdentity: "viewer-1",
    kind: "text",
    text: "hello",
    ts: 456,
  });
  assert.equal(textInput.path, "/devices/slot-18/control/input");
  assert.deepEqual(textInput.body, {
    participant_identity: "viewer-1",
    kind: "text",
    action: "input",
    x: 0.5,
    y: 0.5,
    delta_x: 0,
    delta_y: 0,
    ts: 456,
    text: "hello",
  });

  const media = buildDeviceRTCMediaStartMutation({
    deviceId: "slot-18",
    participantIdentity: "viewer-1",
    streamInstance: "manual-stream",
    camera: true,
    microphone: false,
    cameraStreamType: "2",
    cameraResolution: "1",
  });
  assert.equal(media.path, "/devices/slot-18/media/start");
  assert.deepEqual(media.body, {
    participant_identity: "viewer-1",
    activate: false,
    camera: true,
    microphone: false,
    stream_instance: "manual-stream",
    camera_stream_type: 2,
    camera_resolution: 1,
  });
});

test("device rtc helpers validate required fields and enums", () => {
  assert.equal(buildDeviceSDKWebRTCRequest({}).error, "device_id_required");
  assert.equal(buildDeviceSDKWebRTCRequest({ deviceId: "slot-18", quality: "2" }).error, "quality_invalid");
  assert.equal(buildDeviceSDKRTCSessionRequest({ deviceId: "slot-18", mode: "direct" }).error, "rtc_mode_invalid");
  assert.equal(buildDeviceRTCActiveMutation({ deviceId: "slot-18" }).error, "participant_identity_required");
  assert.equal(buildDeviceRTCControlStateRequest({}).error, "device_id_required");
  assert.equal(buildDeviceRTCControlMutation({ deviceId: "slot-18", action: "input", participantIdentity: "viewer" }).error, "unknown_rtc_control_action");
  assert.equal(buildDeviceRTCControlMutation({ deviceId: "slot-18", action: "acquire" }).error, "participant_identity_required");
  assert.equal(buildDeviceRTCControlInputMutation({ deviceId: "slot-18", participantIdentity: "viewer", kind: "bad" }).error, "control_input_kind_invalid");
  assert.equal(buildDeviceRTCControlInputMutation({ deviceId: "slot-18", participantIdentity: "viewer", kind: "key" }).error, "control_input_key_required");
  assert.equal(buildDeviceRTCControlInputMutation({ deviceId: "slot-18", participantIdentity: "viewer", kind: "text" }).error, "control_input_text_required");
  assert.equal(buildDeviceRTCMediaStartMutation({ deviceId: "slot-18" }).error, "participant_identity_required");
});

test("device rtc normalizers keep WebRTC, LiveKit, lease and media fields", () => {
  const webrtc = normalizeDeviceWebRTCResult({
    success: true,
    device_id: "slot-18",
    slot: { slot: 18 },
    url: "https://cloud.example/webplayer/play.html?q=1",
    direct_url: "/webplayer/play.html?q=1",
    webrtc_tcp_port: 20018,
    webrtc_udp_port: 20018,
  });
  assert.equal(webrtc.deviceID, "slot-18");
  assert.equal(webrtc.slot, "18");
  assert.equal(webrtc.webrtcTCPPort, 20018);
  assert.equal(webrtc.url, "https://cloud.example/webplayer/play.html?q=1");

  const session = normalizeDeviceRTCSessionResult({
    success: true,
    device_id: "slot-18",
    mode: "livekit",
    entry_url: "https://cloud.example/devices/slot-18/rtc",
    livekit_url: "wss://livekit.example",
    room_name: "device-slot-18",
    participant_identity: "user-admin-slot-18",
    token_ttl_sec: 120,
    control_state: { controller_identity: "user-admin-slot-18", controller_role: "admin" },
  });
  assert.equal(session.mode, "livekit");
  assert.equal(session.roomName, "device-slot-18");
  assert.equal(session.controlState.controllerIdentity, "user-admin-slot-18");

  const active = normalizeDeviceRTCActiveResult({
    success: true,
    devices: [{ device_id: "slot-18", participant_identity: "viewer-1", room_name: "device-slot-18" }],
  });
  assert.equal(active.devices.length, 1);
  assert.equal(active.devices[0].participantIdentity, "viewer-1");

  const state = normalizeDeviceRTCControlState({
    success: true,
    device_id: "slot-18",
    controller_identity: "viewer-1",
    controller_name: "管理员",
    controller_role: "admin",
  });
  assert.equal(state.controllerName, "管理员");

  const input = normalizeDeviceRTCControlInputResult({
    success: true,
    device_id: "slot-18",
    route: "rpa-provider",
    sent: true,
    acquire_ms: 6,
    send_ms: 2,
    total_ms: 10,
    screen_width: 720,
    screen_height: 1280,
  });
  assert.equal(input.sent, true);
  assert.equal(input.route, "rpa-provider");
  assert.equal(input.screenHeight, 1280);
  assert.equal(input.totalMS, 10);

  const media = normalizeDeviceRTCMediaStartResult({
    success: true,
    device_id: "slot-18",
    room_name: "device-slot-18",
    controller_identity: "viewer-1",
    camera: {
      status: "prepared",
      playback_url: "webrtc://p1/live/slot-18-input",
      preview_url: "https://relay/live/slot-18-input/whep",
      publish_url: "https://relay/live/slot-18-input/whip",
      stream_key: "slot-18-input",
    },
    audio: { status: "disabled" },
  });
  assert.equal(media.camera.playbackURL, "webrtc://p1/live/slot-18-input");
  assert.equal(media.camera.streamKey, "slot-18-input");
  assert.equal(media.audio.status, "disabled");
});

test("device action normalizers keep status and task fields", () => {
  const action = normalizeDeviceActionResult({
    success: true,
    status: "waiting",
    task: { task_id: "task-1", task_type: "wework_login_qrcode" },
    qrcode: "qr-data",
    expires_at: "2026-07-02T10:00:00Z",
  });
  assert.equal(action.success, true);
  assert.equal(action.status, "waiting");
  assert.equal(action.taskID, "task-1");
  assert.equal(action.taskType, "wework_login_qrcode");
  assert.equal(action.qrcode, "qr-data");
  assert.equal(action.expiresAt, "2026-07-02T10:00:00Z");

  const status = normalizeWeWorkLoginStatus({
    device_id: "device-1",
    status: "normal",
    account_name: "客服一",
    wework_user_id: "wm-1",
    task_id: "task-status",
  });
  assert.equal(status.deviceID, "device-1");
  assert.equal(status.status, "normal");
  assert.equal(status.accountName, "客服一");
  assert.equal(status.weworkUserID, "wm-1");
  assert.equal(status.taskID, "task-status");
});
