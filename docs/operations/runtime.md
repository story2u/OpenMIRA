# 运行与运维

> 状态：当前事实 · 最后核验：2026-07-11

## 服务拓扑

本地 `backend/docker-compose.yml` 启动 PostgreSQL、Redis、迁移、API、Celery worker、Celery beat
和 Telegram listener。生产 `backend/docker-compose.prod.yml` 另含 frontend 与 cloudflared，并从
GHCR 拉取镜像。`.github/workflows/deploy.yml` 在同一个 release workflow 内构建镜像，并通过 SSH
同步 compose/env 后部署到 VPS。

后端镜像固定复制官方 uv 二进制，使用 `uv sync --locked --no-dev` 从 `pyproject.toml` 与
`uv.lock` 构建 Python 环境；同时从固定 Node 22 镜像用 `npm ci --omit=dev --ignore-scripts` 安装
`backend/pi-agent-runtime/package-lock.json` 中的 pi runtime。`.venv` / `node_modules` 不进入
Docker context。CI 分别验证两个锁文件。

## 配置分组

以 `backend/.env.example` 和 `backend/app/core/config.py` 为字段真相源。

| 分组 | 关键变量 | 说明 |
| --- | --- | --- |
| 基础 | `APP_ENV`、`DEBUG`、`DATABASE_URL`、`REDIS_URL` | API 与持久化 |
| 队列 | `CELERY_BROKER_URL`、`CELERY_RESULT_BACKEND` | 默认使用 Redis DB 1/2 |
| Web/Auth | `FRONTEND_BASE_URL`、`CORS_ORIGINS`、`JWT_SECRET_KEY`、`ADMIN_API_TOKEN` | 生产必须使用强随机值 |
| OAuth | `GOOGLE_*`、`APPLE_*` | 至少配置一个 provider；Apple secret 可由 key material 生成 |
| 工作模式 | `DEFAULT_TIMEZONE`、`DEFAULT_WORKDAYS`、`DEFAULT_WORK_START/END`、`PENDING_HUMAN_SLA_MINUTES` | 决定人工/AI 路由与超时 |
| Telegram | `TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET` | Bot webhook；用户监听数量由订阅 entitlement 决定 |
| 企业微信 | `WECOM_CORP_ID`、`WECOM_AGENT_ID`、`WECOM_SECRET`、`WECOM_TOKEN`、`WECOM_AES_KEY` | webhook 验签、解密与发送 |
| AI/发送 | `AI_ENABLED`、`LITELLM_MODEL`、`OPENAI_API_KEY`、`IM_SEND_ENABLED` | 两个功能开关默认关闭 |
| pi Agent | `PI_AGENT_ENABLED`、`PI_AGENT_PROVIDER`、`PI_AGENT_MODEL`、`PI_AGENT_API_KEY`、`PI_AGENT_*TIMEOUT*`、`PI_AGENT_MAX_*` | 默认开启；DeepSeek 可由 GitHub `DEEPSEEK_API_KEY` Secret 映射，OpenAI 可回退使用 `OPENAI_API_KEY` |

`JWT_SECRET_KEY` 在首次 VPS 部署缺失或仍为占位值时由 workflow 生成并保留。秘密放 GitHub Secrets，
非敏感配置放 Variables；不得把生产 `.env` 回写仓库。

## 队列

worker 监听 `default,im,ai,agent`。关键任务定义在 `backend/app/worker/tasks.py`：AI 回复、pi 消息
后处理和人工 SLA 超时扫描。`agent.analyze_message` 入队前在 PostgreSQL 原子预留用户额度，成功后
转为 consumed，最终重试失败或入队失败转为 released；最多自动重试 3 次，子进程另有硬超时。
Message 状态、usage ledger 幂等键和 source message 唯一索引共同提供重复任务保护。新增任务时需要
明确 queue、超时、重试、幂等键和可观测字段，并同步 compose/部署配置。

Telegram listener 每轮加载配置时按有效套餐重算 monitor 配额。降级超额项仅设置
`quota_paused/quota_reason` 并停止对应 listener，不删除群配置；用户通过设置页选择保留项后，下一轮
刷新会停止旧 listener、启动新选择。升级会恢复容量内暂停项，同时保留用户选择优先级。

## 健康与启动顺序

1. PostgreSQL/Redis 通过 healthcheck。
2. `migrate` 执行 `alembic upgrade head` 并成功退出。
3. API、worker、beat、listener 启动。
4. frontend/cloudflared 对外提供入口。

健康端点：根 `/healthz` 与 API 前缀下 `/api/v1/healthz`、`/api/v1/readyz`。当前 readyz 只返回
静态状态，尚未证明数据库、Redis 或外部 provider 可用；不要把它解释成完整依赖就绪检查。

## 发布与回滚

- 只有向 `release/v<major>.<minor>.<patch>` 分支推送提交才启动自动化；例如
  `release/v1.0.0`。普通分支、`main`、PR、tag 和手动 dispatch 都不会触发这些 workflow。
- 发布链固定为 `CI` 成功后触发生产 `Deploy to VPS`：workflow 内先并行构建两个镜像，两个构建 job
  都成功后才执行部署与健康检查。它直接使用 CI 的 `head_sha` 和 release 分支，避免二级
  `workflow_run` 将运行上下文切回默认分支。
- 镜像使用去掉 `release/` 前缀的版本号（如 `v1.0.0`）和 `sha-<short-sha>` 双标签；部署使用
  版本标签，不使用含 `/` 的分支名或漂移的 `latest`。
- schema 变更上线前评估旧/新应用兼容窗口。不可向后兼容的迁移需要分阶段 expand/migrate/contract。
- 回滚应用前确认数据库仍兼容旧版本；不能安全降级时以前向修复为主并在计划写明。

## 运维缺口

当前仓库没有完整的集中日志/指标/trace、外部告警、备份恢复演练或依赖型 readiness。相关建设记录在
[技术债](../plans/tech-debt.md)，不能仅凭容器 `running` 判断服务健康。
