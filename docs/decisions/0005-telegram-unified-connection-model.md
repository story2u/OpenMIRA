# ADR-0005：Telegram 使用统一连接与来源模型

> 状态：accepted · 日期：2026-07-11

## 背景

原有设置流程让每个用户提供 MTProto API ID、API Hash、手机号、验证码和 Session String，既难以
使用也扩大了秘密暴露面。Bot 群/频道、Telegram Business 和未来的 MTProto QR 登录在身份、连接
状态、来源和健康检查上有共同需求。

## 决策

新增 `TelegramConnection`、`TelegramSource`、`TelegramConnectionAttempt` 与
`TelegramWebhookEvent` 作为面向用户的统一模型。连接尝试只保存随机令牌的哈希及过期时间；来源
只在平台回调验证后创建。P0 使用平台 Bot 的 `request_chat` 选择器与 webhook；P1/P2 共用模型但
必须分别满足其能力前置条件。

保留 `TelegramUserConfig`、`TelegramMonitor` 和常驻 listener 作为 legacy 兼容路径，迁移不读取、
解密或复制旧 session。生产禁用 mock adapter；本地 mock 只用于完整链路测试。

## 后果

- 新旧来源在套餐统计和运维视图中需要同时计算，直到单独规划旧 MTProto 迁移。
- webhook 处理需要先做 secret 校验和 update 级去重，再进行外部 API 调用或消息摄取。
- P2 必须由有状态常驻 worker 承担，不能放入无状态请求处理器。

## 替代方案

- 在旧表上继续增加字段：无法表示 Business/Bot 多来源连接，也延续了客户端秘密输入。
- 立刻删除旧 MTProto：会中断已授权用户，并把密钥解密/复制放进不可逆 schema 迁移。
