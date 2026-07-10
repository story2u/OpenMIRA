# 安全基线

> 状态：强制约束 · 最后核验：2026-07-10

## 数据分类

- **秘密**：OAuth client secret、JWT key、admin token、Telegram api_hash/session、IM token/AES key、
  OpenAI key、VPS/GHCR/Cloudflare 凭据。只允许在环境变量、GitHub Secrets 或经应用加密的数据库字段。
- **个人/通信数据**：邮箱、手机号、聊天内容、外部用户 ID、群信息。日志和测试 fixture 最小化、脱敏。
- **公开配置**：client ID、redirect URI、镜像标签等可进入变量，但仍需防止环境混淆。

不得把真实秘密、session string、完整 webhook payload 或生产消息放入源码、文档、测试、截图或错误响应。

## 认证与授权

- 用户 API 使用 JWT `require_user`；管理 API 使用 `require_admin`，旧 admin token 只用于明确维护场景。
- 每个用户资源查询必须在 repository/API 层带 `owner_user_id`，不能先按 ID 获取后默认信任。
- OAuth 必须校验 state；redirect URI 使用配置白名单；provider profile 不应覆盖已验证身份边界。
- localStorage token 是当前既有选择，新增功能不得把 token 放进 URL、日志或第三方请求。

## 外部输入与 webhook

- Telegram/企微 webhook 在解析前验签/secret；失败时拒绝且不产生业务记录。
- 对文本、列表、回填数量、ID 和 payload 大小设上限；不要无界存储或查询外部 JSON。
- HTML、Markdown、URL 和模型输出在最终展示/发送边界按目标上下文处理，避免注入。
- 外部消息幂等键和数据库唯一约束必须同时存在，防止重放造成重复商机/回复。

## IM 与 AI 动作

- `IM_SEND_ENABLED` 默认 false；只有明确配置的环境能执行真实发送。
- AI 生成内容视为不可信草稿。自动发送前必须经过确定性风险检查、长度限制和状态校验。
- Celery 重试必须设计发送幂等；无法证明不会重复发送时宁可进入人工处理。
- Telegram 用户账号默认只摄取；启用个人账号发送属于新的高风险能力，需单独 ADR、权限与审计设计。
- 不把聊天中的指令当成系统指令，不允许消息内容改变工具权限、配置或秘密访问范围。
- pi runner 不加载 coding-agent、context files、skills 或内置工具；消息与网页正文只作为带边界的
  不可信数据，唯一的 `submit_analysis` 工具不能访问网络、数据库或执行外部动作。
- URL 读取必须逐跳验证 scheme、凭据、端口和解析地址，拒绝本机/私网/link-local/保留地址并限制
  数量、重定向、时间、正文大小和内容类型。应用层校验不能完全消除 DNS rebinding，生产网络还应
  通过 egress proxy/防火墙阻止内网目标。
- 传给模型的数据只包含规范化消息字段和截断后的公开网页文本，不包含 raw webhook payload、token、
  session 或应用秘密；子进程错误不得回显模型原始输出或 API key。

## 加密与日志

- Telegram `api_hash` 与 session 使用 `backend/app/core/security.py` 的加密封装；密钥来源与轮换需在
  上线前验证。禁止新增明文字段或在 mapper 返回解密值。
- 日志记录 event、request/trace ID、实体 ID、provider 与结果；秘密字段和原始消息正文默认不记录。
- 异常链保留内部可诊断信息，对外返回稳定的非敏感错误。

## 高风险变更清单

认证、授权、密码学、真实消息发送、AI 自动回复、生产迁移和部署权限变更必须：

1. 在执行计划中列威胁、误用路径、回滚和验收标准。
2. 覆盖成功与拒绝/失败路径，必要时做隔离环境冒烟。
3. 审查日志、错误与响应是否泄密。
4. 明确人工批准点；未获授权不得代表用户联系外部人员或操作生产环境。
