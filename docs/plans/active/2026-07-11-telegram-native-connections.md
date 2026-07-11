# Telegram 原生连接重构

> 状态：active · Owner：Codex · 创建：2026-07-11 · 更新：2026-07-11

## 目标与用户价值

将 Telegram 设置从要求用户填写 MTProto 凭据的表单重构为统一连接中心。用户能通过受控的
Bot 引导连接群组/频道，查看连接和来源的真实状态；未来 Business 与 MTProto QR 复用同一数据模型。

## 非目标

- 不在本次变更中启用个人账号主动发送消息。
- 不把 P2 QR 登录伪装成已经可用的功能；没有常驻 worker 与平台凭据时明确显示不可用。
- 不删除现有 `TelegramUserConfig`、`TelegramMonitor` 或 listener，避免中断已连接用户。

## 背景与当前行为

`/settings/telegram` 当前收集 API ID、API Hash、手机号、验证码、2FA、Session 和手填 chat ID。
`/webhooks/telegram` 已有 secret 校验和消息摄取，生产为 VPS Docker，拥有常驻
`telegram_listener`，不是 serverless。旧 MTProto 模型保持为兼容路径。

## 验收标准

- [x] 默认 UI 不展示或收集 API Hash、手机号、验证码、Session String 或手填 chat ID。
- [x] P0 Bot 群/频道流程有 owner 隔离、短期一次性令牌、webhook 验签和 update 幂等记录；成功后创建连接和来源并复用消息摄取。
- [x] 所有连接 API 都不返回 token、session 或其它秘密；本地 mock 仅在非生产环境可用且清楚标记。
- [x] 旧监听配置继续运行，schema 迁移不解密或删除历史 session。
- [x] P1/P2 的状态和前置条件在 UI/API 中如实呈现。

## 影响面与风险

数据库新增连接、来源、连接尝试和 webhook 事件表；API/webhook 需要严格 owner/幂等处理；Bot
调用和 MTProto 未来会影响外部服务。连接令牌仅保存哈希并有 TTL。部署需设置 Bot username、模式和
webhook 地址；生产禁止 mock。订阅限额需要同时统计旧 monitor 和新 source。

## 实施步骤

- [x] 审计现有页面、路由、认证、listener、迁移、部署、队列和官方 Telegram API。
- [x] 新增统一数据模型、迁移、仓储、DTO 与安全令牌。
- [x] 完成 P0 Bot 群/频道连接、验证、事件幂等和摄取编排。
- [x] 接入 P1 Business 连接状态；实现 P2 能力探测与明确禁用状态。
- [x] 重写设置页、补测试、配置文档、ADR、功能地图并完整验证。

## 进度日志

- 2026-07-11：开始。已确认部署具备常驻 Python listener；P0 采用 Bot API `request_chat`，而不是用户输入 chat ID。
- 2026-07-11：完成 P0/P1 后端与设置页。release workflow 仅从 GitHub 同步 Bot token 与 Bot username；首次发布在服务器生成并持久化 webhook secret，生产 URL/live/600 秒 TTL 使用 compose 默认值，并在配置完整时注册 webhook。
- 2026-07-11：P2 保持明确禁用，直到专用 QR worker、加密 session 生命周期和运维监控作为后续计划完成；不以旧手填秘密表单替代。

## 发现日志

- Telegram 的 `chat_shared` 只返回 chat ID，且 Bot 可能不可访问该 chat；因此回传后必须调用 `getChat` 和 `getChatMember` 验证。
- Bot API Business connection 的用户 ID 可以安全地与已在私聊确认的连接尝试匹配；未匹配事件不能被任意用户认领。

## 决策日志

- 2026-07-11：旧 MTProto 表继续作为 legacy 运行路径，新表不在 Alembic 中解密或复制 session；这样可回滚且不会因密钥轮换造成迁移失败。
- 2026-07-11：生产运行时禁止 mock；本地 mock adapter 仅用来验证 API/UI 完整链路，不表示真实 Telegram 接入。
- 2026-07-11：`update_id` 的迁移列是 PostgreSQL `BIGINT`，SQLModel 也必须显式使用 `BigInteger`；仓储集成测试发现并修正了该绑定漂移。
- 2026-07-11：空的 `TELEGRAM_MTPROTO_API_ID` 是常见部署值，配置层将空字符串规范化为未设置，避免迁移容器启动失败。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make check` | 通过 | harness、后端 48 passed / 5 skipped、pi runtime、前端 lint/typecheck/build 全部通过 |
| 连接令牌、owner、重复 webhook 单测 | 通过 | 本地单测 9 passed / 2 skipped；临时 PostgreSQL 仓储测试 2 passed |
| Alembic upgrade/downgrade/upgrade | 通过 | 隔离 Compose PostgreSQL：`head → 202607110002 → head` |
| production Compose config | 通过 | 使用非敏感临时 Cloudflare token 渲染 `docker-compose.prod.yml` |

## 回滚与恢复

新表和 API 可独立停用，旧 listener 不读取它们。应用回滚前保留新增表；若需要数据库降级，先确认
没有依赖新表的运行中 webhook。失效或泄漏的连接尝试令牌可等待过期或由用户取消，令牌原文不持久化。

## 结果与剩余风险

P0 已完成；P1 已具备连接确认、状态更新和消息 owner 路由，需真实 Telegram Business 权限做隔离环境
冒烟。P2 QR worker 仍是后续工作，页面/API 已 fail-closed，避免重新引入用户手填秘密。
