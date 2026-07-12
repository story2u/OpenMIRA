# 运行与运维

> 状态：当前事实 · 最后核验：2026-07-12

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
| Web/Auth | `FRONTEND_BASE_URL`、`CORS_ORIGINS`、`JWT_SECRET_KEY`、`ADMIN_API_TOKEN`、`PASSWORD_LOGIN_*` | 生产必须使用强随机密钥；密码登录默认每个直连 IP + 邮箱 5 次/300 秒，状态存 Redis DB 0 |
| OAuth | `GOOGLE_*`、`APPLE_*` | 至少配置一个 provider；Apple secret 可由 key material 生成 |
| 工作模式 | `DEFAULT_TIMEZONE`、`DEFAULT_WORKDAYS`、`DEFAULT_WORK_START/END`、`PENDING_HUMAN_SLA_MINUTES` | 决定人工/AI 路由与超时 |
| Telegram | `TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`TELEGRAM_BOT_USERNAME`、`TELEGRAM_WEBHOOK_URL`、`TELEGRAM_INTEGRATION_MODE` | 原生 Bot 连接/webhook；首次 release 自动生成并持久化 secret，生产 URL/live/600 秒 TTL 均有 compose 默认值，群额度包含旧 monitor 与新 source |
| 企业微信 | `WECOM_CORP_ID`、`WECOM_AGENT_ID`、`WECOM_SECRET`、`WECOM_TOKEN`、`WECOM_AES_KEY` | webhook 验签、解密与发送 |
| AI/发送 | `AI_ENABLED`、`LITELLM_MODEL`、`OPENAI_API_KEY`、`IM_SEND_ENABLED` | 两个功能开关默认关闭；启用 AI 后，规则未高置信命中的非空消息都会进行语义复核，调用量会高于旧关键词灰区模式 |
| pi Agent | `PI_AGENT_ENABLED`、`PI_AGENT_PROVIDER`、`PI_AGENT_MODEL`、`PI_AGENT_API_KEY`、`PI_AGENT_*TIMEOUT*`、`PI_AGENT_MAX_*` | 默认开启；DeepSeek 可由 GitHub `DEEPSEEK_API_KEY` Secret 映射，OpenAI 可回退使用 `OPENAI_API_KEY` |

`JWT_SECRET_KEY` 在首次 VPS 部署缺失或仍为占位值时由 workflow 生成并保留。秘密放 GitHub Secrets，
非敏感配置放 Variables；不得把生产 `.env` 回写仓库。

商机语义复核复用 `LITELLM_MODEL`，不需要独立模型服务。每次只传当前 owner 同会话最近 6 条、合计
不超过 4000 字符的规范化历史以及最多 20 条 AI hint；模型输出非法或 provider 失败时回退到规则结果。
模型补判不会获得自动发送权限，只创建待人工审核商机。需要控制成本时关闭 `AI_ENABLED` 即可恢复纯
规则路径；后续应在有金标数据后增加本地语义候选层，以降低全量复核调用。

## 队列

worker 监听 `default,im,ai,agent`。关键任务定义在 `backend/app/worker/tasks.py`：AI 回复、pi 消息
后处理和人工 SLA 超时扫描。`agent.analyze_message` 入队前在 PostgreSQL 原子预留用户额度，成功后
转为 consumed，最终重试失败或入队失败转为 released；最多自动重试 3 次，子进程另有硬超时。
Message 状态、usage ledger 幂等键和 source message 唯一索引共同提供重复任务保护。新增任务时需要
明确 queue、超时、重试、幂等键和可观测字段，并同步 compose/部署配置。

Telegram listener 每轮加载配置时按有效套餐重算 monitor 配额。降级超额项仅设置
`quota_paused/quota_reason` 并停止对应 listener，不删除群配置；用户通过设置页选择保留项后，下一轮
刷新会停止旧 listener、启动新选择。升级会恢复容量内暂停项，同时保留用户选择优先级。

Telegram 原生连接由 API webhook 处理：先验证 Bot secret，再按 `update_id` 写入幂等事件，随后处理
连接握手或已配置 source 的消息。release 部署在 Telegram token、secret 和 webhook URL 完整时执行
`setWebhook` 注册；具体 GitHub Secrets/Variables 和本地 mock 规则见
[Telegram 原生连接](../integrations/telegram.md)。普通账号 QR 由 `telegram_mtproto_qr_worker` 与
`telegram_mtproto_listener` 两个独立常驻服务处理，生产环境已纳入默认部署与迁移前停机列表。

## 健康与启动顺序

1. PostgreSQL/Redis 通过 healthcheck。
2. 发布时先停止 API、worker、beat、Telegram listener 等数据库客户端，避免旧连接耗尽 PostgreSQL
   连接槽；`migrate` 执行 `alembic upgrade head` 并成功退出。
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
- 迁移失败时数据库客户端保持停止状态，先检查 `migrate` 日志并修复或回滚，再恢复运行服务；不要让旧
  应用继续访问可能已部分变更的 schema。
- 回滚应用前确认数据库仍兼容旧版本；不能安全降级时以前向修复为主并在计划写明。

## 运维缺口

当前仓库没有完整的集中日志/指标/trace、外部告警、备份恢复演练或依赖型 readiness。相关建设记录在
[技术债](../plans/tech-debt.md)，不能仅凭容器 `running` 判断服务健康。
