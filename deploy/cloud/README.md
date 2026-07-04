# Standalone Cloud Compose

This directory contains the cloud compose baseline for the standalone Go + Next.js IM project. It is intended for development, staging, and controlled production rollout of this repository as an independent system.

## Usage

```bash
cd go/deploy/cloud
cp .env.example .env
docker compose --env-file .env config
docker compose --env-file .env up -d --build go-api go-web go-redis
```

Set `GO_IMAGE_TAG`, `GO_WEB_VERSION`, `GO_WEB_COMMIT`, and `GO_WEB_BUILD_TIME` before building release images when the tag, visible Next.js version marker, `/version.txt`, and container runtime metadata must point at the same artifact.

Start workers only for the product surface being validated:

```bash
docker compose --env-file .env up -d --build \
  go-outbox-worker \
  go-incoming-worker \
  go-contact-sync-worker \
  go-send-dispatcher \
  go-archive-sync-worker \
  go-archive-ingest-worker \
  go-archive-media-worker \
  go-voice-transcription-worker
```

## Release Readiness

Generate a readiness report from the Go project root and keep the artifact with the deployment change:

```bash
go run ./cmd/release-readiness -all -format markdown
go run ./cmd/release-readiness -profile session-access -format markdown
go run ./cmd/release-readiness -profile incoming-ingest -format markdown
go run ./cmd/release-readiness -profile send-dispatch -format markdown
```

The command checks route metadata, runtime flags, required settings, compose services, and fixture coverage. The release readiness model is documented in `docs/release-readiness.md`.

Use `-strict` in a release gate so disabled flags or missing settings fail before traffic reaches a product surface.

## Minimum Required Settings

- `CLOUD_DB_DSN`
- `SESSION_JWT_SECRET`
- `CLOUD_WS_REDIS_URL` and `CLOUD_EVENTBUS_REDIS_URL` when using realtime or queues
- `CLOUD_CACHE_REDIS_URL` when using locks, provider leases, or cache-backed diagnostics
- Object storage upload URL/token when media or archive workers are enabled

Provider-specific settings are optional unless the matching connector or provider is enabled.

`go-send-dispatcher` is part of the standalone worker set, but it does not assume a bundled device/RPA implementation. Leave `GO_SEND_PROVIDER_BASE_URL` empty for core/runtime validation. Set it only when an HTTP-compatible send provider is enabled for real outbound delivery.

## Runtime Roles

Core roles:

- `go-api`: stateless HTTP API and realtime gateway.
- `go-web`: Next.js web console.
- `go-outbox-worker`: durable event relay.
- `go-incoming-worker`: inbound connector event consumer.
- `go-send-dispatcher`: outbound task dispatcher.
- `go-redis` / `go-cache-redis`: eventbus, realtime, locks, pending queues and cache.

Optional roles:

- `go-contact-sync-worker`: contact synchronization.
- `go-archive-sync-worker`: archive cursor and sync jobs.
- `go-archive-ingest-worker`: archive message ingest.
- `go-archive-media-worker`: archive media preparation/download/upload.
- `go-voice-transcription-worker`: voice transcription retry.
- External connector/provider services: message channels, automation providers, media providers and platform integrations.

## Connector And Provider Policy

Message platforms are connectors. Automation backends are providers. The compose baseline should not make any single connector or provider mandatory for the IM core.

Practical rules:

- Keep core API/Web/Redis/DB deployable without a specific message platform.
- Keep `go-send-dispatcher` deployable with fake or HTTP providers for validation.
- Do not add provider sidecars to the default compose graph; use explicit overrides or external services for provider-specific deployments.
- Put provider secrets behind dedicated env names and avoid leaking them into core service assumptions.
- Prefer one provider service per capability boundary instead of embedding device or vendor logic in `go-api`.
- Document every temporary bridge with an owner, replacement path and removal condition.

## Existing Transition Flags

The current compose file still contains `GO_ENABLE_*_CANDIDATE` flags and some profile names from the Phase 1 implementation. Treat them as deployment controls, not long-term architecture.

Follow-up milestones will:

- Rename readiness artifacts to release terminology.
- Replace supplier-specific service names with connector/provider roles.
- Remove temporary bridge services once native providers exist.
- Delete product surfaces that do not fit the standalone IM roadmap.

## Validation

```bash
cd go
go test ./...
go vet ./...
SKIP_NPM_CI=1 bash scripts/phase1_gate.sh

cd web
npm run test
npm run build
```

For staging deployments, also verify:

- API `/healthz`, `/readyz`, and `/metrics`.
- Web `/version.txt`.
- Worker logs and queue lag.
- Outbox relay delivery.
- Connector/provider health endpoints for any enabled integration.
