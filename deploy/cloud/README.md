# Go Cloud Compose

This directory contains the Go/Next.js cloud compose baseline for shadow or canary runs.
It does not replace `Python/deploy/cloud/docker-compose.yml` by default.

## Usage

```bash
cd go/deploy/cloud
cp .env.example .env
docker compose --env-file .env config
docker compose --env-file .env up -d --build go-api go-web
```

Set `GO_IMAGE_TAG`, `GO_WEB_VERSION`, `GO_WEB_COMMIT`, and
`GO_WEB_BUILD_TIME` before building release images when the tag, visible Next.js
version marker, `/version.txt`, and container runtime metadata must point at the
same artifact.

Start workers only when the matching route or worker cutover is being tested:

```bash
docker compose --env-file .env up -d --build \
  go-outbox-worker \
  go-incoming-worker \
  go-contact-sync-worker \
  python-sdk-executor-sidecar \
  go-send-dispatcher \
  go-archive-sync-worker \
  go-archive-ingest-worker \
  go-archive-media-worker \
  go-voice-transcription-worker
```

Before enabling a traffic slice, generate the local readiness report from the Go
root and keep the artifact with the deployment change:

```bash
go run ./cmd/cutover-readiness -profile session-access -format markdown
go run ./cmd/cutover-readiness -profile admin-observability -format markdown
go run ./cmd/cutover-readiness -profile incoming-ingest -format markdown
go run ./cmd/cutover-readiness -profile task-status -format markdown
go run ./cmd/cutover-readiness -profile workbench-read -format markdown
go run ./cmd/cutover-readiness -profile admin-accounts -format markdown
go run ./cmd/cutover-readiness -profile admin-assignments -format markdown
go run ./cmd/cutover-readiness -profile admin-config-content -format markdown
go run ./cmd/cutover-readiness -profile realtime-workbench -format markdown
go run ./cmd/cutover-readiness -profile contact-sync -format markdown
go run ./cmd/cutover-readiness -profile workbench-actions -format markdown
go run ./cmd/cutover-readiness -profile wework-events -format markdown
go run ./cmd/cutover-readiness -profile platform-proxy -format markdown
go run ./cmd/cutover-readiness -profile ai-outreach -format markdown
go run ./cmd/cutover-readiness -profile archive-pipeline -format markdown
go run ./cmd/cutover-readiness -profile archive-voice-transcription -format markdown
go run ./cmd/cutover-readiness -profile device-ops -format markdown
go run ./cmd/cutover-readiness -profile send-dispatch -format markdown
```

The command checks Go route metadata, `.env` candidate flags, required secrets,
compose services, and golden fixtures. Add `-strict` in the actual cutover gate
so disabled flags or missing secrets fail before traffic moves.
`scripts/phase1_gate.sh` also checks that every `GO_ENABLE_*` candidate flag
referenced by the Go runtime is present in both this compose file and
`.env.example`.

`go-api` exposes only probe routes unless candidate flags such as
`GO_ENABLE_SESSION_ME_CANDIDATE` or
`GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE` are explicitly enabled.
The session cutover slice uses
`GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE`,
`GO_ENABLE_SESSION_LOGIN_CANDIDATE`,
`GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE`,
`GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE`,
`GO_ENABLE_SESSION_ME_CANDIDATE`, `GO_ENABLE_SESSION_REFRESH_CANDIDATE`, and
`GO_ENABLE_SESSION_LOGOUT_CANDIDATE`. Set `SESSION_JWT_SECRET`,
`CLOUD_DB_DSN`, `ADMIN_USERNAME`, and `ADMIN_PASSWORD` before moving login
traffic. `ALLOW_PASSWORDLESS_LOGIN` remains `0` unless the legacy passwordless
route is intentionally enabled.
`GO_ENABLE_AUDIT_LOGS_CANDIDATE=1`,
`GO_ENABLE_SYSTEM_LOGS_CANDIDATE=1`,
`GO_ENABLE_OBSERVABILITY_DASHBOARD_CANDIDATE=1`,
`GO_ENABLE_CLIENT_ERRORS_CANDIDATE=1`, and the `GO_ENABLE_STATS_*` /
`GO_ENABLE_AI_REPLY_LOGS_CANDIDATE` flags mount the admin observability
surface used by the Next.js management console.
`GO_ENABLE_WS_GATEWAY_CANDIDATE=1`,
`GO_ENABLE_STREAM_CHANNELS_CANDIDATE=1`,
`GO_ENABLE_REALTIME_REPLAY_CANDIDATE=1`, and
`GO_ENABLE_REALTIME_SNAPSHOT_CANDIDATE=1` mount the workbench realtime gateway
and recovery reads.
`GO_ENABLE_INCOMING_MESSAGES_CANDIDATE=1` mounts the queue-first
`POST /api/v1/messages/incoming` candidate. For cutover, keep
`CLOUD_EVENTBUS_REDIS_URL`, `CLOUD_DB_DSN`, `go-incoming-worker`,
`go-outbox-worker`, and `go-redis` present so API fast-ack, durable message
write, and realtime outbox relay stay in the same evidence chain.
`GO_ENABLE_TASKS_CANDIDATE=1` mounts the generic task create/list/detail/status
and retry routes. Set `SESSION_JWT_SECRET`, `AGENT_API_TOKEN`,
`CLOUD_DB_DSN`, and `CLOUD_EVENTBUS_REDIS_URL`; run `go-outbox-worker` with
`go-api` and `go-redis` so task.status events remain durable.
For the Next.js workbench read cutover profile, enable the `GO_ENABLE_WORKBENCH_*`
read flags plus `GO_ENABLE_CONVERSATION_LIST_CANDIDATE`,
`GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE`, panel/account-stats flags, and the
conversation contact-profile helper flags together with `CLOUD_DB_DSN` and
`SESSION_JWT_SECRET`; keep `go-web` and `go-api` on the same release artifact.
For the admin accounts cutover profile, enable the account, CS user, and cached
contact read/write flags together. It requires `CLOUD_DB_DSN`,
`SESSION_JWT_SECRET`, and `CLOUD_WS_REDIS_URL`; run `go-api`, `go-web`, and
`go-redis` so account/user updates can keep the management console refreshed.
For the admin assignments cutover profile, enable assignment config, workload,
list/detail, claim/release, purge, and auto-assign flags together. It requires
`CLOUD_DB_DSN`, `SESSION_JWT_SECRET`, `CLOUD_WS_REDIS_URL`, and
`CLOUD_CACHE_REDIS_URL`; run `go-api`, `go-web`, `go-redis`, and
`go-cache-redis` so assignment events and pool runtime resets are available.
For the admin config/content cutover profile, enable sensitive-word, reply
script, AI config, knowledge, and enterprise flags together. It requires
`CLOUD_DB_DSN`, `SESSION_JWT_SECRET`, `CLOUD_WS_REDIS_URL`, and
`KNOWLEDGE_UPLOAD_ROOT`; the default compose path is `/app/data/uploads/knowledge`
on the shared `go-data` volume.
`GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE=1`,
`GO_ENABLE_CONTACT_SYNC_FULL_CANDIDATE=1`, and
`GO_ENABLE_CONTACT_SYNC_REFRESH_STALE_CANDIDATE=1` mount the manual contact
sync endpoints. Start `go-contact-sync-worker` when scheduled full and stale
refresh sync is part of the cutover slice; it requires `CLOUD_DB_DSN` and
enterprise contact secrets in the shared database.
`GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE=1` mounts the first friend-added event
candidate, and `GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE=1` mounts the
WeWork customer-contact notify callback GET/POST candidates. Enable them only
with `CLOUD_DB_DSN`, `SESSION_JWT_SECRET`, `CLOUD_EVENTBUS_REDIS_URL`,
`go-api`, `go-outbox-worker`, and `go-redis` present so callback receipt,
dedupe, and realtime side effects remain durable.
`GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE=1` mounts the archive callback GET/POST
candidate and requires `CLOUD_DB_DSN`; Go writes callback receipt state for the
candidate path.
`GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE=1` mounts
`GET /api/v1/archive/callback/receipts`; it requires `CLOUD_DB_DSN` and
`SESSION_JWT_SECRET`, and keeps compensation/prune endpoints on Python.
For the archive pipeline cutover profile, enable the archive read/admin,
manual pull, callback, SDK bridge, media run/prepare/download, and events
notify flags together only with `ARCHIVE_BRIDGE_TOKEN`,
`ARCHIVE_SELF_DECRYPT_PULL_URL`, `ARCHIVE_SELF_DECRYPT_PULL_TOKEN`,
`ARCHIVE_MEDIA_OBJECT_UPLOAD_URL`, `ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN`,
`ARCHIVE_MEDIA_SIGNING_KEY`, DB, eventbus/lock/cache Redis, `go-outbox-worker`,
`go-archive-sync-worker`, `go-archive-ingest-worker`, and
`go-archive-media-worker` present. Voice transcription retry has separate Coze
credential choices and is not part of this profile.
`GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE=1` mounts manual archive
voice retry. Its readiness profile accepts either a Coze API token
(`VOICE_TRANSCRIPTION_COZE_API_KEY`, `VOICE_TRANSCRIPTION_COZE_TOKEN`,
`COZE_WORKFLOW_API_KEY`, or `COZE_API_KEY`) or the JWT OAuth client/public-key
ID plus inline private key or private-key path. Start `go-voice-transcription-worker`
with `go-api` and `go-redis` for the full retry/worker path.
`GO_ENABLE_CONVERSATION_REPLY_CANDIDATE=1` mounts the text reply candidate
`POST /api/v1/conversations/{conversation_id}/reply`; it requires
`SESSION_JWT_SECRET`, creates a `send_text` task, and does not yet replace the
Python route's message-table write.
`GO_ENABLE_SEND_TEXT_CANDIDATE=1`, `GO_ENABLE_SEND_IMAGE_CANDIDATE=1`,
`GO_ENABLE_SEND_VIDEO_CANDIDATE=1`, `GO_ENABLE_SEND_VOICE_CANDIDATE=1`,
`GO_ENABLE_SEND_FILE_CANDIDATE=1`, and `GO_ENABLE_GROUP_INVITE_CANDIDATE=1`
mount the legacy direct send and group invite candidates. They require
`SESSION_JWT_SECRET`, create durable SDK tasks, and share the in-process device
send limiter configured by `RATE_LIMIT_WINDOW_SEC`, `RATE_LIMIT_MAX_SENDS`,
`RATE_LIMIT_BURST`, and `RATE_LIMIT_BURST_WINDOW`. When `CLOUD_DB_DSN` is
configured, they also block fresh `devices.online=false` snapshots for
`DEVICE_OFFLINE_BLOCK_MAX_AGE_SEC` seconds before falling through like Python.
`GO_ENABLE_STAGE6_HEALTH_CANDIDATE=1` mounts the admin/supervisor
`GET /healthz/stage6` observability health candidate; it requires
`CLOUD_DB_DSN` and `SESSION_JWT_SECRET` and currently reuses the dashboard
stage6 status provider.
`GO_ENABLE_P1_SCREEN_CANDIDATE=1` mounts the read-only `/api/p1/screen/*` and
`/api/p1/slots/ports` URL/port helpers; it does not require `CLOUD_DB_DSN`.
Set `P1_INTERNAL_IP` and optional `P1_WEBRTC_TCP_PORT` /
`P1_WEBRTC_UDP_PORT` to mirror the Python P1 screen settings.
`GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE=1` mounts the LiveKit device-control
lease and input endpoints. By default `/control/input` stays unavailable after
lease validation; set `P1_RTC_CONTROL_EXECUTOR_BASE_URL` to a trusted internal
Python backend or dedicated P1 control bridge to forward validated input to
MytRpc. The bridge should share the same `CLOUD_CACHE_REDIS_URL` /
`CLOUD_CACHE_REDIS_PREFIX` control-state keys, and `P1_RTC_CONTROL_EXECUTOR_TOKEN`
can override `AGENT_API_TOKEN` for bridge auth. Optional
`P1_RTC_CONTROL_SCREEN_WIDTH` and `P1_RTC_CONTROL_SCREEN_HEIGHT` override the
manager-cache P1 coordinate size.
For the full device operations cutover profile, also configure
`P1_MANAGER_CACHE_FILE`, `MYT_CALL_AUDIO_BRIDGE_*` files, `RTC_MEDIA_*`
templates, `LIVEKIT_WS_URL`, `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`,
`CLOUD_BACKEND_BASE_URL`, `AGENT_API_TOKEN`, `SDK_EXECUTOR_API_TOKEN`, and the
lock/cache Redis URLs, then run `go-send-dispatcher` with
`python-sdk-executor-sidecar`.
`GO_ENABLE_SCRIPT_GENERATE_CANDIDATE=1` mounts
`POST /api/v1/scripts/generate`; it requires `CLOUD_DB_DSN`,
`SESSION_JWT_SECRET`, and either `system_settings.ai.api_key` or `AI_API_KEY`.
`GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE=1` mounts read-only
`/api/v1/platform/*` proxy endpoints for options, category price, customer info,
stores, orders, schedule hours, collections, and user appid; it uses
`PLATFORM_*` settings and does not require `CLOUD_DB_DSN`.
`GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE=1` mounts the fixed 501 platform login
placeholder plus non-device platform proxy mutations such as customer mobile,
order, schedule, prepay, and store video proxy calls.
`GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE=1` mounts the device sidebar command
task entrypoint; it normalizes the legacy sidebar payload and creates an
accepted SDK task, but does not execute SDK/MytRpc work inside the API process.
For the full platform proxy cutover profile, configure `PLATFORM_BASE_URL`,
`PLATFORM_API_TOKEN`, platform identity defaults, `CLOUD_DB_DSN`,
`SDK_EXECUTOR_API_TOKEN`, and the lock/cache Redis URLs, then run
`go-send-dispatcher` with `python-sdk-executor-sidecar` so sidebar tasks reach
the existing SDK executor bridge.
`GO_ENABLE_AI_OUTREACH_CANDIDATE=1` mounts
`/api/v1/platform-agent/ai-outreach/*` for platform-agent conversation context
and outreach send task creation. Set `AGENT_API_TOKEN`, the same `PLATFORM_*`
settings, `CLOUD_DB_DSN`, eventbus/lock/cache Redis URLs, and
`SDK_EXECUTOR_API_TOKEN`; run `go-outbox-worker`, `go-send-dispatcher`, and
`python-sdk-executor-sidecar` for the full outreach send path.
For the full SOP operations cutover profile, enable the `GO_ENABLE_SOP_*`
candidate flags for flows, policies, analytics, dispatch tasks/resend, media
local/upload, and platform test. Configure `CLOUD_DB_DSN`,
`SESSION_JWT_SECRET`, `CLOUD_WS_REDIS_URL`, `ARCHIVE_MEDIA_OBJECT_UPLOAD_URL`,
`ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN`, `ARCHIVE_MEDIA_SIGNING_KEY`,
`SDK_EXECUTOR_API_TOKEN`, and the lock/cache Redis URLs; run `go-web`,
`go-send-dispatcher`, and `python-sdk-executor-sidecar` so manual SOP resend
tasks leave the API process and reach the existing SDK bridge.
For the admin diagnostics cutover profile, enable the `GO_ENABLE_DIAGNOSTIC_*`
candidate flags for device/account maps, orphan/forked conversations, dirty
contacts, archive sync status, archive missing-outbox check/replay, and
historical timezone dry-run maintenance. Configure `CLOUD_DB_DSN`,
`SESSION_JWT_SECRET`, `CLOUD_WS_REDIS_URL`, and `CLOUD_CACHE_REDIS_URL`; run
`go-outbox-worker` when replaying archive missing-message outbox events so
repaired canonical events update projections and realtime subscribers.

## Minimum Required Settings

- `CLOUD_DB_DSN`
- `SESSION_JWT_SECRET`
- `CLOUD_WS_REDIS_URL` and `CLOUD_EVENTBUS_REDIS_URL` when using realtime or queues
- `ARCHIVE_SELF_DECRYPT_PULL_URL` for archive sync/media pull
- `ARCHIVE_MEDIA_OBJECT_UPLOAD_URL` for archive media upload
- `VOICE_TRANSCRIPTION_COZE_API_KEY` or the Coze JWT OAuth key fields for voice transcription

For Go send-dispatcher canary runs, start `python-sdk-executor-sidecar` with
`go-send-dispatcher`. The sidecar is a temporary bridge around the existing
Python `SdkTaskExecutor`; it exposes only `/devices`, `/execute`, and
`/execute-batch` on the internal compose network. Set `SDK_EXECUTOR_API_TOKEN`
to a non-empty shared value and leave
`GO_SDK_EXECUTOR_BASE_URL=http://python-sdk-executor-sidecar:9107` unless the
executor runs outside this compose stack.

For multi-enterprise archive workers, set `ARCHIVE_SYNC_ALL_ENTERPRISES=1` or
`ARCHIVE_WORKER_ALL_ENTERPRISES=1`.
`ARCHIVE_WORKER_SCOPE_CONCURRENCY` controls multi-enterprise archive ingest,
archive media, and voice transcription worker fan-out; the default is `1`.

`go-contact-sync-worker` runs the Go replacement for Python
`ContactSyncScheduler`. It requires `CLOUD_DB_DSN` and enterprise contact
secrets in the shared `enterprises` table. `CONTACT_SYNC_FULL_INTERVAL_SEC`
defaults to `86400` with a minimum of `3600`; `CONTACT_SYNC_REFRESH_INTERVAL_SEC`
defaults to `300` with a minimum of `60`; `CONTACT_SYNC_REFRESH_LIMIT` defaults
to `50`. Startup delays default to `CONTACT_SYNC_FULL_STARTUP_DELAY_SEC=180`
and `CONTACT_SYNC_REFRESH_STARTUP_DELAY_SEC=30`. Avatar references use the same
archive media settings as archive workers; set `ARCHIVE_MEDIA_OBJECT_UPLOAD_URL`
and `ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN` when inline/base64 contact avatars should
be persisted as object references instead of display-safe inline fallbacks.

Archive sync uses the lock Redis client for `archive-sync:lock:{enterprise_id}|{source}`;
`ARCHIVE_SYNC_LOCK_TTL_SEC` defaults to `30` and is renewed every
`ARCHIVE_SYNC_LOCK_RENEW_SEC` seconds.
`ARCHIVE_SYNC_SCOPE_CONCURRENCY` controls how many enterprises a catch-up tick
may pull concurrently; the default is `1`, and each enterprise still runs its
catch-up rounds sequentially.
The archive sync worker also runs archive retention maintenance every
`CLOUD_STAGE4_GOVERNANCE_INTERVAL_SEC` seconds, pruning archive raw rows,
terminal callback receipts, successful ingest tasks, completed compensation
tasks, and published outbox events older than their retention settings.
Set `GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE=1` to also export expired
`encrypted_messages` rows to snappy Parquet before hot-row pruning; this requires
`CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT`, `ARCHIVE_MEDIA_OBJECT_UPLOAD_URL`, and
`ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN` because the Go path intentionally does not
use the Python direct OSS fallback yet. Run `go-archive-sync-worker`; the cold
storage cutover readiness profile is worker-only and has no HTTP route.
Archive media workers use the lock Redis client for
`archive-media:lock:{enterprise_id}|{source}`; `ARCHIVE_MEDIA_LOCK_TTL_SEC`
defaults to `30` and is renewed every `ARCHIVE_MEDIA_LOCK_RENEW_SEC` seconds.
Signed archive media downloads can be shadow-mounted with
`GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE=1`; the API validates the same
`ARCHIVE_MEDIA_SIGNING_KEY` token used by generated `access_url` values, reads
`archive_media_tasks`, serves `local://archive_media/...` files from
`PYTHON_PROJECT_ROOT/backend/data`, and proxies object paths through
`ARCHIVE_MEDIA_OBJECT_INTERNAL_BASE_URL` (default `http://object-storage:9102`).

Outbox enqueue publishes a best-effort Redis wakeup to `CLOUD_OUTBOX_NOTIFY_CHANNEL`
when `CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED=1`; durable relay state still lives in SQL.
`archive.sync.requested` enqueue also publishes a best-effort wakeup to
`ARCHIVE_SYNC_NOTIFY_CHANNEL` when `ARCHIVE_SYNC_REDIS_NOTIFY_ENABLED=1`; archive
pull state still lives in `outbox_events` and `archive_sync_cursors`.
Archive ingest task enqueue publishes a best-effort wakeup to
`ARCHIVE_INGEST_NOTIFY_CHANNEL` when `ARCHIVE_INGEST_REDIS_NOTIFY_ENABLED=1`;
staged ingest task state still lives in `archive_ingest_tasks`.
Archive media task enqueue publishes a best-effort wakeup to
`ARCHIVE_MEDIA_NOTIFY_CHANNEL` when `ARCHIVE_MEDIA_REDIS_NOTIFY_ENABLED=1`; media
task state still lives in `archive_media_tasks`.
Voice transcription task enqueue publishes a best-effort wakeup to
`VOICE_TRANSCRIPTION_NOTIFY_CHANNEL` when
`VOICE_TRANSCRIPTION_REDIS_NOTIFY_ENABLED=1`; transcription task state still
lives in `voice_transcription_tasks`.
