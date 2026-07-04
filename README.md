# IM Slim

This branch is a strategic reset of the Go + Next.js IM project.

The retained product surface is intentionally small:

- message intake: `POST /api/v1/messages/incoming`
- text send: `POST /api/v1/send/text`
- message read: `GET /api/v1/conversations/{conversation_id}/messages`
- SOP flows, policies, dispatch tasks, and platform checks
- a single Next.js console for messages and SOP

Everything else from the broad transitional system has been removed from this
branch: platform-specific adapters, device control, automation-driver bindings,
content-generation management, operations dashboards, account administration,
workers, replay harnesses, and legacy deployment assets.

## Run

```bash
go run ./cmd/api
```

The API listens on `:8080` by default. Override it with `ADDR`.

```bash
cd web
npm install
npm run dev
```

## Verify

```bash
go test ./...
go vet ./...
npm --prefix web test
npm --prefix web run build
```

Or run the slim gate:

```bash
bash scripts/slim_gate.sh
```

## Current Tradeoff

The reset branch favors a clear, tiny product core over preserving historical
compatibility. The in-memory backend proves the retained API and frontend shape;
the next step is adding durable storage and queueing only for this reduced
message/SOP scope.
