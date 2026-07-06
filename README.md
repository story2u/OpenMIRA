# Standalone Go + Next.js IM

本目录是一个独立运行的 Go + Next.js IM 集成平台。当前系统处于空系统阶段，已删除历史运行时、历史 worker、过渡开关和单一平台强耦合代码；后续开发以本仓库的新 `integrationhub` 内核为准。

## 当前内核

- Go API: `cmd/api`
- 后端核心: `internal/integrationhub`
- 前端控制台: `web`
- 数据库: PostgreSQL
- 部署基线: `deploy/docker-compose.yml`

`integrationhub` 负责通道、会话、消息、入站事件、出站命令、SOP、AI policy、审计和观测数据。企业微信只是 `internal/integrationhub/connectors` 下的一个 adapter，不是 IM core 的约束。

## 本地验证

```bash
cd go
go test ./...
go vet ./...
go build ./cmd/api
```

前端验证：

```bash
cd go/web
npm install --package-lock=false --no-audit --no-fund
npm run build
```

容器构建：

```bash
cd go
docker build -t im-api --build-arg TARGET_CMD=api .
docker build -t im-web -f web/Dockerfile web
```

## 本地部署

```bash
cd go/deploy
cp .env.example .env
docker compose --env-file .env config
docker compose --env-file .env up -d --build go-postgres go-api go-web
```

如果要通过 Cloudflare Tunnel 公开访问，配置 `CLOUDFLARE_TUNNEL_TOKEN` 后再启动 `cloudflared`。

## 工程边界

- IM core 不直接依赖具体消息平台、RPA 供应商或设备控制协议。
- 具体平台能力必须进入 connector 或 provider adapter。
- API 写路径只写本地事实和 outbox，不阻塞在外部 IM 平台调用上。
- 空系统阶段允许直接重塑 schema、接口和运行时，不保留历史兼容层。
