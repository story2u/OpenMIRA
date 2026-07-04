# Cutover Profile Runbooks

本文档用于承接 `phase1-summary.mjs` 的 profile 链接，帮助把 cutover readiness 的失败项转成下一步动作。详细 route、flag、env、service 和 golden 清单以 Go 源码中的 `internal/cutover.DefaultProfiles()` 为准，可用命令生成完整版本：

```bash
cd go
go run ./cmd/cutover-readiness -all -format runbook
go run ./cmd/cutover-readiness -profile <profile> -format markdown
go run ./cmd/cutover-readiness -profile <profile> -strict
```

通用顺序：

1. 先修 route、golden、service 失败，这些通常是仓库或部署清单缺口。
2. 再配置 profile 必需 env/secrets。
3. 跑对应 golden/live/shadow gate。
4. 最后开启 `GO_ENABLE_*` 候选开关并跑 `-strict`。
5. 进入 canary 前保留 Python 回滚路径和旧 Docker 运行角色。

## session-access

范围：Session login, impersonation, refresh, logout, and current-user access for Next.js cutover.

重点动作：
- 配置 `CLOUD_DB_DSN`、`SESSION_JWT_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`。
- 跑 phase2 session golden/live 对账。
- 开启 session 相关 `GO_ENABLE_SESSION_*` 开关后再执行 `-strict`。

```bash
go run ./cmd/cutover-readiness -profile session-access -format markdown
go run ./cmd/cutover-readiness -profile session-access -strict
```

## admin-observability

范围：Admin and Next observability surfaces: audit logs, system logs, runtime dashboard, client logs, and stats.

重点动作：
- 配置 DB/JWT，并确认 `go-api`、`go-web` 服务存在。
- 核对 audit/system log、stats、observability dashboard golden。
- 开启观测类 `GO_ENABLE_*` 前确认 dashboard 不伪造运行态。

```bash
go run ./cmd/cutover-readiness -profile admin-observability -format markdown
go run ./cmd/cutover-readiness -profile admin-observability -strict
```

## incoming-ingest

范围：Queue-first incoming message ingestion through Go API, Redis Stream worker, durable message write, and outbox relay.

重点动作：
- 配置 `CLOUD_DB_DSN` 和 `CLOUD_EVENTBUS_REDIS_URL`。
- 确认 `go-incoming-worker`、`go-outbox-worker`、Redis 服务存在。
- 用低副作用 incoming golden 和 replay/shadow 数据验证入站消息不丢 outbox。

```bash
go run ./cmd/cutover-readiness -profile incoming-ingest -format markdown
go run ./cmd/cutover-readiness -profile incoming-ingest -strict
```

## task-status

范围：Generic task create/list/detail/status/retry routes, task.status realtime events, and agent callback authentication.

重点动作：
- 配置 DB、JWT、`AGENT_API_TOKEN` 和 eventbus Redis。
- 确认 task create/status golden 通过。
- 切流前用 agent callback 低副作用样本验证鉴权和终态同步。

```bash
go run ./cmd/cutover-readiness -profile task-status -format markdown
go run ./cmd/cutover-readiness -profile task-status -strict
```

## workbench-read

范围：Next.js workbench read model, conversation panels, message history, search, and contact profile helper routes.

重点动作：
- 配置 DB/JWT，并确认 `go-api`、`go-web` 服务存在。
- 先验证会话列表、消息历史、panel snapshot、搜索和 contact profile golden。
- 搜索仍需注意 projection 字段覆盖边界，切流前不要把 ES/消息正文增强视为已完全替换。

```bash
go run ./cmd/cutover-readiness -profile workbench-read -format markdown
go run ./cmd/cutover-readiness -profile workbench-read -strict
```

## admin-accounts

范围：Admin account, customer-service user, and cached contact read/write surfaces used by the Next.js management console.

重点动作：
- 配置 DB/JWT/WS Redis。
- 验证账号、客服、联系人缓存 golden。
- 写入口切流前确认 audit log、read-model cache 失效和 realtime 事件可观察。

```bash
go run ./cmd/cutover-readiness -profile admin-accounts -format markdown
go run ./cmd/cutover-readiness -profile admin-accounts -strict
```

## admin-assignments

范围：Assignment configuration, workload reads, manual claim/release, purge, and auto-assignment control plane.

重点动作：
- 配置 DB/JWT、WS Redis、cache Redis。
- 验证 assignment config、claim/release、purge、auto assign golden。
- 切流前关注 `assign:*` Redis key、pool state 和 read-model cache 失效一致性。

```bash
go run ./cmd/cutover-readiness -profile admin-assignments -format markdown
go run ./cmd/cutover-readiness -profile admin-assignments -strict
```

## admin-config-content

范围：Management-console configuration and content surfaces: sensitive words, reply scripts, AI config, knowledge base, and enterprises.

重点动作：
- 配置 DB/JWT、WS Redis、`KNOWLEDGE_UPLOAD_ROOT`。
- 验证敏感词、快捷话术、AI 配置、知识库和企业绑定 golden。
- 知识库写入口切流前确认上传目录权限和回滚策略。

```bash
go run ./cmd/cutover-readiness -profile admin-config-content -format markdown
go run ./cmd/cutover-readiness -profile admin-config-content -strict
```

## sop-operations

范围：SOP flow/policy configuration, analytics, dispatch inspection/resend, media helpers, and platform test surfaces.

重点动作：
- 配置 DB/JWT/WS Redis、SDK sidecar token、lock/cache Redis 和 archive media 上传/签名参数。
- 验证 SOP flow/policy、analytics、dispatch、media golden。
- 写入口切流前确认媒体对象上传和签名访问链路可回滚。

```bash
go run ./cmd/cutover-readiness -profile sop-operations -format markdown
go run ./cmd/cutover-readiness -profile sop-operations -strict
```

## admin-diagnostics

范围：Admin diagnostic maps, conversation/contact integrity checks, archive sync snapshots, archive outbox replay, and historical timezone dry-run maintenance.

重点动作：
- 配置 DB/JWT、WS Redis、cache Redis。
- 验证诊断和 archive missing outbox check/replay golden。
- 历史时区修复当前保持受控 dry-run，切流前不要把批量写修复视为已接管。

```bash
go run ./cmd/cutover-readiness -profile admin-diagnostics -format markdown
go run ./cmd/cutover-readiness -profile admin-diagnostics -strict
```

## send-dispatch

范围：Manual send, media send, group invite, and conversation reply paths with the temporary Python SDK executor sidecar.

重点动作：
- 配置 DB/JWT、SDK executor token、eventbus/lock/cache Redis。
- 验证 reply、send text/media、group invite golden。
- 该 profile 仍依赖 Python SDK executor sidecar，canary 前必须确认 sidecar 回滚路径。

```bash
go run ./cmd/cutover-readiness -profile send-dispatch -format markdown
go run ./cmd/cutover-readiness -profile send-dispatch -strict
```

## workbench-actions

范围：Workbench conversation write actions: read state, AI handoff, resend, revoke, calls, and admin transfer.

重点动作：
- 配置 DB/JWT、SDK executor token、eventbus/lock/cache Redis。
- 验证 mark-read、AI 开关、resend、revoke、call、transfer golden。
- 切流前关注设备离线守卫、send target 解析和 RPA safe 边界。

```bash
go run ./cmd/cutover-readiness -profile workbench-actions -format markdown
go run ./cmd/cutover-readiness -profile workbench-actions -strict
```

## contact-sync

范围：Manual and scheduled WeWork contact cache sync through the Go API and contact-sync worker.

重点动作：
- 配置 DB/JWT，并确认 `go-contact-sync-worker` 服务存在。
- 验证 external/full/refresh-stale golden。
- 切流前确认企微凭证、头像持久化 fallback 和调度间隔。

```bash
go run ./cmd/cutover-readiness -profile contact-sync -format markdown
go run ./cmd/cutover-readiness -profile contact-sync -strict
```

## wework-events

范围：WeWork friend-added and customer-contact notify callbacks with durable outbox delivery.

重点动作：
- 配置 DB/JWT/eventbus Redis。
- 验证 friend-added 和 notify callback golden。
- 切流前确认 ACK 语义、outbox 持久化和首次加微 SOP side effect。

```bash
go run ./cmd/cutover-readiness -profile wework-events -format markdown
go run ./cmd/cutover-readiness -profile wework-events -strict
```

## platform-proxy

范围：Platform read/write proxy routes and device sidebar commands backed by durable SDK tasks.

重点动作：
- 配置平台 base/token/default identity、DB、SDK sidecar token、lock/cache Redis。
- 验证 platform read/write/sidebar golden。
- 侧边栏任务切流前确认 durable task、SDK executor 和平台错误映射。

```bash
go run ./cmd/cutover-readiness -profile platform-proxy -format markdown
go run ./cmd/cutover-readiness -profile platform-proxy -strict
```

## ai-outreach

范围：Platform-agent AI outreach conversation reads and durable send task creation.

重点动作：
- 配置 DB、agent token、平台配置、SDK sidecar token 和 Redis。
- 验证 AI outreach golden。
- 切流前确认外部平台代理、发送任务和回滚路径。

```bash
go run ./cmd/cutover-readiness -profile ai-outreach -format markdown
go run ./cmd/cutover-readiness -profile ai-outreach -strict
```

## archive-pipeline

范围：Archive admin reads, manual pulls, callbacks, SDK bridge, media sync, and media download surfaces.

重点动作：
- 配置 DB/JWT、archive bridge/self-decrypt/media 参数、eventbus/lock/cache Redis。
- 确认 archive sync、ingest、media、outbox worker 服务存在。
- 验证 archive read、callback、SDK bridge、media run/download golden。

```bash
go run ./cmd/cutover-readiness -profile archive-pipeline -format markdown
go run ./cmd/cutover-readiness -profile archive-pipeline -strict
```

## archive-voice-transcription

范围：Archive voice transcription retry API and worker credentials.

重点动作：
- 配置 DB/JWT/eventbus Redis。
- 配置 Coze API token 或 JWT OAuth 组合凭据之一。
- 验证 voice retry golden 和 `go-voice-transcription-worker` 服务。

```bash
go run ./cmd/cutover-readiness -profile archive-voice-transcription -format markdown
go run ./cmd/cutover-readiness -profile archive-voice-transcription -strict
```

## archive-cold-storage

范围：Worker-only archive cold storage export from encrypted_messages to Parquet/object storage before hot-row pruning.

重点动作：
- 配置 DB、本地冷归档 export root、对象上传 URL/token。
- 确认 archive sync worker、Redis/cache Redis 服务存在。
- 该 profile 没有 HTTP route 或 golden fixture，切流证据应来自 worker shadow/canary 产物。

```bash
go run ./cmd/cutover-readiness -profile archive-cold-storage -format markdown
go run ./cmd/cutover-readiness -profile archive-cold-storage -strict
```

## device-ops

范围：Device inventory, P1 screen helpers, WeWork login tasks, SDK controls, LiveKit RTC control, and media preparation.

重点动作：
- 配置 DB/JWT、agent token、SDK sidecar token、P1/RTC/LiveKit/media/env。
- 验证 P1、devices、wework login、SDK control、RTC/media golden。
- 切流前必须保留真机 shadow/canary 和 Python device side rollback。

```bash
go run ./cmd/cutover-readiness -profile device-ops -format markdown
go run ./cmd/cutover-readiness -profile device-ops -strict
```

## realtime-workbench

范围：Workbench websocket gateway, channel list, replay, and snapshot recovery.

重点动作：
- 配置 JWT 和 WS Redis。
- 验证 stream channels、replay、snapshot golden/replay。
- 切流前确认 WS fanout、cursor、replay 和旧前端事件消费兼容。

```bash
go run ./cmd/cutover-readiness -profile realtime-workbench -format markdown
go run ./cmd/cutover-readiness -profile realtime-workbench -strict
```
