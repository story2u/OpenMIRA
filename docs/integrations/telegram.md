# Telegram 原生连接

> 状态：部分实现（P0 可用，P1 需平台配置，P2 未部署） · 最后核验：2026-07-11

## 用户可见连接方式

`/settings/telegram` 是唯一的默认入口。页面不会收集或显示 API ID、API Hash、手机号、验证码、
2FA 密码、Session String 或手填 chat ID。

| 方式 | 优先级 | 当前状态 | 用途 |
| --- | --- | --- | --- |
| Bot 群组/频道 | P0 | 已实现 | 选择并监听已添加平台 Bot 的群组或频道 |
| Telegram Business | P1 | 部分实现 | 通过 Business connection 接收授权范围内的私聊 |
| 普通账号 QR | P2 | 未部署 | 未来使用平台统一 MTProto 凭据和常驻 QR worker |

旧 `TelegramUserConfig` / `TelegramMonitor` 仍由现有 listener 读取。它们不会被本次迁移解密、
复制或删除；新页面只显示非敏感的“旧监听仍在运行”提示。

## P0：Bot 群组/频道流程

1. 已登录用户点击“连接群组或频道”。服务器创建十分钟有效、只保存 SHA-256 哈希的连接令牌。
2. 用户打开 Bot deep link；`/start connect_<token>` 将连接尝试绑定到该 Telegram 账号。
3. Bot 在私聊中发送 `request_chat` 选择器，分别请求群组与频道。用户不输入 chat ID。
4. 收到 `chat_shared` 后，服务调用 `getChat` 与 `getChatMember`，确认类型正确且 Bot 仍是成员。
5. 服务在 owner 隔离的 `TelegramConnection` / `TelegramSource` 中保存来源。消息 webhook 只摄取已启用、
未因套餐暂停的来源；未选择的 chat 会被忽略。

Bot 必须已加入目标群/频道。群消息需要在 BotFather 中关闭 Privacy Mode，或为 Bot 配置足以接收所需
消息的权限；这是 Telegram 平台限制，不是应用内权限开关。

每个 Telegram update 在 `telegram_webhook_events.update_id` 上去重，且在解析 payload 前校验
`X-Telegram-Bot-Api-Secret-Token`。事件表只保存 payload 哈希和处理状态，不保存原始 webhook。
消息仍使用既有 `(channel, external_message_id)` 约束作第二层去重。

## P1：Business 私聊

点击开始会先要求用户在私聊中确认自己的 Telegram Business 账号，再引导其到 Telegram Business 设置
添加 Bot。只有后续 `business_connection.user.id` 与已确认账号匹配时，系统才创建 owner 连接；未匹配
connection 不会被任意用户认领。`business_message` 按 business connection ID 路由到该 owner。

是否可用由平台 Bot 配置决定。外发能力仍受 `IM_SEND_ENABLED`、Business rights、独立审批和审计约束，
本连接功能不会自动代用户发私信。

## P2：普通账号 QR

QR 登录需要平台全局 `TELEGRAM_MTPROTO_API_ID` / `TELEGRAM_MTPROTO_API_HASH`，加密 session 存储和
常驻 worker 来维持 QR 状态及后续监听。当前 VPS 尚未部署专用 QR worker，因此 API 与 UI 明确返回
“未启用”，不会退回到手填 API Hash/Session 表单。

## 生产配置与发布

GitHub **Secrets**：

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_MTPROTO_API_HASH`（仅未来 P2）

GitHub **Variables**：

- `TELEGRAM_BOT_USERNAME`（不含 `@`）
- `TELEGRAM_MTPROTO_QR_ENABLED`、`TELEGRAM_MTPROTO_QR_WORKER_ENABLED`、`TELEGRAM_MTPROTO_API_ID`（仅未来 P2）

webhook secret 不需要 GitHub Secret：release 工作流会在服务器首次发布时生成随机值（1–256 位，仅
`A-Z a-z 0-9 _ -`）并持久化在运行时 `.env`，后续发布保持不变。生产 compose 默认使用
`https://im.story2u.xyz/api/v1/webhooks/telegram`、`TELEGRAM_INTEGRATION_MODE=live` 和
`TELEGRAM_CONNECT_TTL_SECONDS=600`；如需覆盖这些默认值，可直接在服务器运行时 `.env` 中设置。

release 部署会在 Bot token、secret 与 webhook URL 全部存在时注册 `setWebhook`，并限制 update 类型到
连接和消息所需集合；未完整配置时会明确跳过注册。生产环境绝不能设置 `TELEGRAM_INTEGRATION_MODE=mock`。
本地默认 mock adapter 只创建名为“本地测试群（Mock）”的测试来源，不会调用 Telegram。

## 运维与故障排查

- 健康/能力状态：`GET /api/v1/integrations/telegram/health`（需登录，不返回秘密）。
- 用户连接：`GET /api/v1/integrations/telegram/connections`。
- webhook 返回 `duplicate: true` 表示 update 已完成，不会重复连接或摄取。
- 连接失败时检查 Bot username、webhook URL/secret、Bot 是否在目标 chat、Privacy Mode/权限，以及套餐群额度。
- 删除连接只删除本系统连接与来源记录；用户需在 Telegram 内自行移除 Business Bot 或群成员。
