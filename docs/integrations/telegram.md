# Telegram 原生连接

> 状态：部分实现（P0 可用，P1 需平台配置，P2 需生产凭据验证） · 最后核验：2026-07-12

## 用户可见连接方式

`/settings/telegram` 是唯一的默认入口。页面不会收集或显示 API ID、API Hash、手机号、验证码、
2FA 密码、Session String 或手填 chat ID。

| 方式 | 优先级 | 当前状态 | 用途 |
| --- | --- | --- | --- |
| Bot 群组/频道 | P0 | 已实现 | 选择并监听已添加平台 Bot 的群组或频道 |
| Telegram Business | P1 | 部分实现 | 通过 Business connection 接收授权范围内的私聊 |
| 普通账号 QR | P2 | 已实现 | 平台统一 MTProto 凭据、用户扫码、选择群组/频道并只读监听 |

旧 `TelegramUserConfig` / `TelegramMonitor` 仍由兼容 listener 读取，旧用户配置 API 暂时保留用于管理既有连接；
它们不会被本次迁移解密、复制或删除。

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

QR 登录使用平台全局 `TELEGRAM_MTPROTO_API_ID` / `TELEGRAM_MTPROTO_API_HASH`。`telegram_mtproto_qr_worker`
仅在服务端生成、等待和刷新 QR 登录；`telegram_mtproto_listener` 用加密 session 对用户已选择的群组/频道
做只读监听。页面只会展示二维码、连接状态和可选群组，绝不采集或显示用户 API Hash、手机号、验证码、
两步验证密码或 Session。

如账号启用了两步验证，当前流程会明确失败而不索取密码；用户可关闭该账号的两步验证后重新扫码。QR URL 是
短期登录凭据，以加密形式存储且只在所属用户的轮询响应中返回；取消、过期或 worker 重启都会使其失效，需重新生成。

## 生产配置与发布

GitHub **Secrets**：

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_MTPROTO_API_HASH`（P2，平台凭据）

GitHub **Variables**：

- `TELEGRAM_BOT_USERNAME`（不含 `@`）
- `TELEGRAM_MTPROTO_QR_ENABLED=true`、`TELEGRAM_MTPROTO_QR_WORKER_ENABLED=true`、`TELEGRAM_MTPROTO_API_ID`（P2）

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
- QR 连接失败时检查 P2 三项平台配置、`telegram_mtproto_qr_worker` / `telegram_mtproto_listener` 是否运行、
账号是否启用两步验证以及套餐群额度。不要在服务器或浏览器中填入用户 API Hash、手机号、验证码或 Session。
- 添加来源后 listener 最迟在一个 15 秒轮询周期内按 connection revision 重载；额度已满时 API 返回 403，
  页面展示套餐限制，不会创建或启用超额来源。
- 删除连接只删除本系统连接与来源记录；用户需在 Telegram 内自行移除 Business Bot 或群成员。
