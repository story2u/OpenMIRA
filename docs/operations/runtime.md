# 运行与运维

> 状态：当前事实 · 最后核验：2026-07-10

## 服务拓扑

本地 `backend/docker-compose.yml` 启动 PostgreSQL、Redis、迁移、API、Celery worker、Celery beat
和 Telegram listener。生产 `backend/docker-compose.prod.yml` 另含 frontend 与 cloudflared，并从
GHCR 拉取镜像。`.github/workflows/docker.yml` 构建镜像，`deploy.yml` 通过 SSH 同步 compose/env 并
部署到 VPS。

后端镜像固定复制官方 uv 二进制，使用 `uv sync --locked --no-dev` 从 `pyproject.toml` 与
`uv.lock` 构建环境；`.venv` 不进入 Docker context。CI 使用相同锁文件和 `uv run --locked`。

## 配置分组

以 `backend/.env.example` 和 `backend/app/core/config.py` 为字段真相源。

| 分组 | 关键变量 | 说明 |
| --- | --- | --- |
| 基础 | `APP_ENV`、`DEBUG`、`DATABASE_URL`、`REDIS_URL` | API 与持久化 |
| 队列 | `CELERY_BROKER_URL`、`CELERY_RESULT_BACKEND` | 默认使用 Redis DB 1/2 |
| Web/Auth | `FRONTEND_BASE_URL`、`CORS_ORIGINS`、`JWT_SECRET_KEY`、`ADMIN_API_TOKEN` | 生产必须使用强随机值 |
| OAuth | `GOOGLE_*`、`APPLE_*` | 至少配置一个 provider；Apple secret 可由 key material 生成 |
| 工作模式 | `DEFAULT_TIMEZONE`、`DEFAULT_WORKDAYS`、`DEFAULT_WORK_START/END`、`PENDING_HUMAN_SLA_MINUTES` | 决定人工/AI 路由与超时 |
| Telegram | `TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`TELEGRAM_FREE_MONITOR_LIMIT` | Bot webhook 和用户监听限制 |
| 企业微信 | `WECOM_CORP_ID`、`WECOM_AGENT_ID`、`WECOM_SECRET`、`WECOM_TOKEN`、`WECOM_AES_KEY` | webhook 验签、解密与发送 |
| AI/发送 | `AI_ENABLED`、`LITELLM_MODEL`、`OPENAI_API_KEY`、`IM_SEND_ENABLED` | 两个功能开关默认关闭 |

`JWT_SECRET_KEY` 在首次 VPS 部署缺失或仍为占位值时由 workflow 生成并保留。秘密放 GitHub Secrets，
非敏感配置放 Variables；不得把生产 `.env` 回写仓库。

## 队列

worker 监听 `default,im,ai`。关键任务定义在 `backend/app/worker/tasks.py`：AI 回复和人工 SLA
超时扫描。新增任务时需要明确 queue、超时、重试、幂等键和可观测字段，并同步 compose/部署配置。

## 健康与启动顺序

1. PostgreSQL/Redis 通过 healthcheck。
2. `migrate` 执行 `alembic upgrade head` 并成功退出。
3. API、worker、beat、listener 启动。
4. frontend/cloudflared 对外提供入口。

健康端点：根 `/healthz` 与 API 前缀下 `/api/v1/healthz`、`/api/v1/readyz`。当前 readyz 只返回
静态状态，尚未证明数据库、Redis 或外部 provider 可用；不要把它解释成完整依赖就绪检查。

## 发布与回滚

- CI 先执行 harness、backend、frontend；镜像 workflow 对 PR 构建但不推送，对 tag/main 事件按规则推送。
- 部署版本可为 branch、tag 或 sha tag；生产优先使用不可变 tag/SHA，不依赖漂移的 `latest`。
- schema 变更上线前评估旧/新应用兼容窗口。不可向后兼容的迁移需要分阶段 expand/migrate/contract。
- 回滚应用前确认数据库仍兼容旧版本；不能安全降级时以前向修复为主并在计划写明。

## 运维缺口

当前仓库没有完整的集中日志/指标/trace、外部告警、备份恢复演练或依赖型 readiness。相关建设记录在
[技术债](../plans/tech-debt.md)，不能仅凭容器 `running` 判断服务健康。
