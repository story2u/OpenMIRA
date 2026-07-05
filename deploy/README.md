# Standalone Deploy Compose

This directory contains the minimal Docker Compose baseline for validating message receive on the standalone Go + Next.js IM project.

## Usage

```bash
cd go/deploy
cp .env.example .env
docker compose --env-file .env config
docker compose --env-file .env up -d --build \
  go-postgres \
  go-redis \
  go-cache-redis \
  go-api \
  go-web \
  go-incoming-worker \
  cloudflared
```

The compose file intentionally includes only Postgres, Redis, API, Web, the incoming message worker, and Cloudflare Tunnel for public ingress. Add sending, archive, contact sync, or transcription workers later through explicit compose overlays.

## GitHub Actions VPS Deploy

The `Deploy to VPS` workflow deploys the GHCR images built by `Docker Build & Push`. Configure these repository or environment values before running it:

- Variables or secrets: `VPS_HOST`, `VPS_USER`, optional `VPS_PORT`, optional `VPS_DEPLOY_DIR`, optional `VPS_API_URL`, optional `VPS_WEB_URL`.
- Secret: `VPS_SSH_KEY`, the private key used by the workflow to SSH into the VPS.
- Secret: `CLOUDFLARE_TUNNEL_TOKEN`, the token from Cloudflare Tunnel. Required when `cloudflared` is included in `VPS_COMPOSE_SERVICES`.
- Optional secret: `VPS_ENV_FILE`, the production `.env` content to write to the VPS deploy directory. Use the repository root `.env.example` as the VPS template and replace every `change-me` value before saving the secret.
- Optional variable: `VPS_COMPOSE_SERVICES`, defaults to `go-postgres go-redis go-cache-redis go-api go-web go-incoming-worker cloudflared`.
- Optional variable/secret: `GHCR_USERNAME` / `GHCR_TOKEN` when the package registry requires a token other than the workflow token.

The SSH user must be able to write `VPS_DEPLOY_DIR` and run `docker compose`. On a fresh Ubuntu VPS, install Docker and add the deploy user to the `docker` group, or use a restricted root login dedicated to deployment.

The workflow copies `deploy/docker-compose.yml` and `deploy/.env.example` to the VPS, preserves an existing `.env`, and overwrites `.env` only when `VPS_ENV_FILE` is set. It exports GHCR image names such as `ghcr.io/story2u/wework-api:main` at deploy time, so the compose file pulls release images instead of building locally.

## Cloudflare Tunnel Routes

Use a remotely managed Cloudflare Tunnel and point public hostnames to services on the Docker network:

- `app.example.com` path `/api/*` -> `http://go-api:9000`
- `app.example.com` path `/ws/*` -> `http://go-api:9000`
- `app.example.com` root path -> `http://go-web:3000`
- `api.example.com` root path -> `http://go-api:9000`

Keep the path-specific `app.example.com` routes ahead of the root Web route. The Web console uses same-origin `/api/v1/...` and `/ws/...` requests, while `api.example.com` is convenient for external callbacks such as `https://api.example.com/api/v1/notify/event/{enterprise_id}`.

## Release Readiness

Generate a readiness report from the Go project root and keep the artifact with the deployment change:

```bash
go run ./cmd/release-readiness -all -format markdown
go run ./cmd/release-readiness -profile session-access -format markdown
go run ./cmd/release-readiness -profile incoming-ingest -format markdown
```

The command checks route metadata, runtime flags, required settings, compose services, and optional fixture coverage. The release readiness model is documented in `docs/release-readiness.md`.

Use `-strict` in a release gate so disabled flags or missing settings fail before traffic reaches a product surface.

## Minimum Required Settings

- `SESSION_JWT_SECRET`
- `POSTGRES_PASSWORD` and `CLOUD_DB_DSN` only when changing the bundled Postgres password or using an external database.

The receive callback route is enabled by default in compose with `GO_ENABLE_CONNECTOR_NOTIFY_CALLBACK_CANDIDATE=1`. The admin login route is enabled by default with `GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE=1`; it bootstraps `root` / `1234567890` in the database and requires the password to be changed after first login. Browser client log ingestion is enabled by default with `GO_ENABLE_CLIENT_ERRORS_CANDIDATE=1`.

## Runtime Roles

Core roles:

- `go-postgres`: bundled PostgreSQL for VPS validation.
- `go-api`: stateless HTTP API and connector callback endpoint.
- `go-web`: Next.js web console.
- `go-incoming-worker`: inbound connector event consumer.
- `cloudflared`: outbound-only Cloudflare Tunnel connector for public API and Web hostnames.
- `go-redis` / `go-cache-redis`: eventbus, realtime, locks, pending queues and cache.

Other workers are intentionally out of this baseline until their product surfaces are enabled.

## Connector And Provider Policy

Message platforms are connectors. Automation backends are providers. The compose baseline should not make any single connector or provider mandatory for the IM core.

Practical rules:

- Keep core API/Web/Redis/DB deployable without a specific message platform.
- Do not add provider sidecars to the default compose graph; use explicit overrides or external services for provider-specific deployments.
- Put provider secrets behind dedicated env names and avoid leaking them into core service assumptions.
- Prefer one provider service per capability boundary instead of embedding device or vendor logic in `go-api`.
- Document every temporary bridge with an owner, replacement path and removal condition.

## Validation

```bash
cd go
go test ./...
go vet ./...

cd web
npm run test
npm run build
```

For staging deployments, also verify:

- API `/healthz`, `/readyz`, and `/metrics`.
- Web `/`.
- Worker logs and queue lag.
- Connector/provider health endpoints for any enabled integration.
