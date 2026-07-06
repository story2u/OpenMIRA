# Standalone Deploy

This directory contains the minimal Docker Compose baseline for the standalone Go + Next.js IM integration platform.

## Services

- `go-postgres`: bundled PostgreSQL for local and VPS validation.
- `go-api`: Go API powered by `internal/integrationhub`.
- `go-web`: Next.js console.
- `cloudflared`: optional Cloudflare Tunnel for public ingress.

Redis and worker services are intentionally absent from the baseline until the new connector/outbox worker runtime is implemented on top of `integrationhub`.

## Local Usage

```bash
cd go/deploy
cp .env.example .env
docker compose --env-file .env config
docker compose --env-file .env up -d --build go-postgres go-api go-web
```

Add `cloudflared` to the `up` command only after `CLOUDFLARE_TUNNEL_TOKEN` is configured.

## GitHub Actions VPS Deploy

The `Deploy to VPS` workflow deploys the GHCR images built by `Build Images`.

Required repository or environment values:

- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `CLOUDFLARE_TUNNEL_TOKEN` when `cloudflared` is included in `VPS_COMPOSE_SERVICES`

Optional values:

- `VPS_PORT`, default `22`
- `VPS_DEPLOY_DIR`, default `/opt/im`
- `VPS_API_URL`
- `VPS_WEB_URL`
- `VPS_ENV_FILE`, production `.env` content copied to the VPS
- `VPS_COMPOSE_SERVICES`, default `go-postgres go-api go-web cloudflared`

The workflow copies `deploy/docker-compose.yml` and `deploy/.env.example` to the VPS, preserves an existing `.env`, and overwrites `.env` only when `VPS_ENV_FILE` is set. It exports these GHCR image names at deploy time:

- `ghcr.io/story2u/im-api:<version>`
- `ghcr.io/story2u/im-web:<version>`

## Cloudflare Tunnel Routes

Use a remotely managed Cloudflare Tunnel and point public hostnames to services on the Docker network:

- `app.example.com` path `/api/*` -> `http://go-api:9000`
- `app.example.com` root path -> `http://go-web:3000`
- `api.example.com` root path -> `http://go-api:9000`

Keep the path-specific API route ahead of the root Web route. The Web console uses same-origin `/api/v1/...` requests.

## Validation

```bash
cd go
go test ./...
go vet ./...
docker compose --env-file deploy/.env.example -f deploy/docker-compose.yml config

cd web
npm run build
```

For VPS deployments, verify:

- API `/healthz`
- Web `/`
