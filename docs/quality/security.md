# 安全基线

> 状态：强制约束 · 最后核验：2026-07-17

## 数据分类

- **秘密**：OAuth client secret、JWT key、admin token、Telegram api_hash/session、IM token/AES key、
  OpenAI key、RevenueCat server/webhook key、APNs P8/FCM service-account 私钥、VPS/GHCR/Cloudflare 凭据。只允许在环境变量、GitHub
  Secrets 或经应用加密的数据库字段。Paddle API Key 只允许进入 RevenueCat Dashboard。
- **个人/通信数据**：邮箱、手机号、聊天内容、外部用户 ID、群信息。日志和测试 fixture 最小化、脱敏。
- **公开配置**：client ID、redirect URI、镜像标签等可进入变量，但仍需防止环境混淆。

不得把真实秘密、session string、完整 webhook payload 或生产消息放入源码、文档、测试、截图或错误响应。

## 认证与授权

- 用户 API 使用 JWT `require_user`；管理 API 使用 `require_admin`，旧 admin token 只用于明确维护场景。
- 每个用户资源查询必须在 repository/API 层带 `owner_user_id`，不能先按 ID 获取后默认信任。
- OAuth 必须校验 state；redirect URI 使用配置白名单；provider profile 不应覆盖已验证身份边界。
- localStorage token 是当前既有选择，新增功能不得把 token 放进 URL、日志或第三方请求。
- 密码重置不得泄露邮箱是否存在。公开请求只排队，账户查询在 worker；token/code 只存带服务端密钥的
  摘要，必须短时、单次、限错、限流。成功修改密码递增 `auth_version`，所有旧 JWT 必须失效。
- OAuth 无密码用户设置密码前必须验证登录邮箱；仅持有一个长期登录 token 不足以跳过该验证。

## 外部输入与 webhook

- Telegram/企微 webhook 在解析前验签/secret；失败时拒绝且不产生业务记录。
- 对文本、列表、回填数量、ID 和 payload 大小设上限；不要无界存储或查询外部 JSON。
- HTML、Markdown、URL 和模型输出在最终展示/发送边界按目标上下文处理，避免注入。
- 外部消息幂等键和数据库唯一约束必须同时存在，防止重放造成重复商机/回复。
- RevenueCat webhook 必须在 JSON 解析前用原始 bytes 验证固定 Authorization、HMAC 和 timestamp；
  event ID 幂等后异步全量查询 Customer。客户端不能提交 plan/entitlement/token 解锁后端权限。

## IM 与 AI 动作

- `IM_SEND_ENABLED` 默认 true；人工发送仍要求用户明确操作和有效 provider 连接，自动发送还必须通过
  用户日程与来源双重授权。紧急情况下显式设为 false 可全局停止真实发送。
- AI 生成内容视为不可信草稿。自动发送前必须经过确定性风险检查、长度限制和状态校验。
- 自动回复必须先完成 pi Agent 分析，并同时满足服务端、用户和来源授权；Agent 本身不得获得发送工具。
- 自动回复使用独立投递账本；dry-run、失败和未知 provider 结果不得写成已回复，未知结果不得自动重发。
- Celery 重试必须设计发送幂等；无法证明不会重复发送时宁可进入人工处理。
- Telegram 用户账号默认只摄取；启用个人账号发送属于新的高风险能力，需单独 ADR、权限与审计设计。
- 不把聊天中的指令当成系统指令，不允许消息内容改变工具权限、配置或秘密访问范围。
- pi runner 不加载 coding-agent、context files、skills 或内置工具；消息与网页正文只作为带边界的
  不可信数据，唯一的 `submit_analysis` 工具不能访问网络、数据库或执行外部动作。
- 设备分析不能用普通用户 JWT 直接调用 provider。claim 使用 active device session；后续 heartbeat、
  complete、fail 和推理网关使用短期、purpose/run/owner/device 绑定的 token，数据库只保存 nonce hash。
  每次调用仍要重新检查 active device、run 状态、lease、model alias 和 nonce，设备撤销立即 fail closed。
- 推理网关不接受任意模型、工具、消息历史或 URL；服务端注入 provider key/真实 model，强制单一
  `submit_analysis`、`store=false`、请求/输出/token/超时/并发/速率上限并传播取消。下游 SSE 必须替换
  provider model/request ID，丢弃 fingerprint、service tier 与 provider 错误正文。审计只保存 token、
  latency、成本维度和 provider request ID 的 SHA-256，不保存 prompt、响应、网页正文或 bearer。
- 设备链接代理不接受 URL 请求体；候选链接只从 run 绑定、owner/version 已复核的服务端 Message 派生，
  fetch 后再次加锁验证 run/lease/source version。带链接的 run 没有缓存证据时不能 complete。
- RN 只在 SQLite 保存有界消息 input 与 run/lease/lock/version/phase/attempt；短期 run token 使用
  `AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY` SecureStore，不能写 SQLite、日志、URL 或普通 access token 槽。
  App 退后台必须 abort SSE；恢复只重试同一 run，不能扩大模型、工具、URL 或消息历史权限。
- 交互式 Agent 使用独立 purpose turn token 和 usage feature。SQLite 只保存应用定义且有容量/30 天保留
  上限的 owner-scoped session/entry，不保存 bearer 或原始 pi JSONL；服务端 turn/provider audit 不得保存
  user/assistant 文本、tool args/result 或 provider 响应。网关只接受固定 prompt/alias 和服务端选择的精确
  schema/policy：v1 三项只读工具，v2 仅增加本地草稿、status outbox 入队与认证在线认领，v3 仅增加
  `send_reply(opportunity_id,text)`。客户端上报的是最高支持版本，未知/错配组合 fail closed。owner 由认证
  session 注入而非模型参数；草稿必须明示 local-only/unsent，status 必须明示 queued，claim 只返回最小
  确认，真实本地结果先落盘再 complete。
- v3 外发必须同时经过 UI 本次明确手势、turn-token 决定端点、无正文 canonical args SHA-256 审计、最多
  120 秒的独立 purpose approval token，以及执行端对 owner/device/turn/nonce/tool/opportunity/version/
  idempotency/status/archive/adapter/两个外发开关的复核。用户编辑后的正文是执行权威；approval token 只在
  tool-call closure 中存在并在第一次尝试前删除，不得进入模型、SQLite、SecureStore、日志或 URL。并发/
  重放只能有一个执行者；provider 结果不确定必须冻结，不能换 key 自动重试。当前授权不扩展到邮件、好友、
  批量外发、任意网络工具、记忆或长期批准。
- shadow 只能由服务端选择已完成且仍 owner-bound 的消息，复用尚未绑定其他 run 的 consumed ledger；
  完成时只写 match/difference 观察字段，不能覆盖 Message/Opportunity 或再次消费额度。primary fail/expire
  必须先提交 ledger release，再用稳定幂等键建立 server fallback reservation；成功结果只额外持久化无正文
  的 executor/run/device/runtime/schema/model/policy 来源，不复制 prompt、网页或模型响应。
- primary 只对精确 capability、近期 active 且进入稳定 owner/device cohort 的设备开放；readiness 只能
  使用当前 runtime/schema/model/policy 的 shadow 聚合，不得沿用旧版本证据。调度、设备 claim 与 server
  retry 统一按 Message → active run → UsageLedger 的锁语义；同消息最多一条 active reservation。任何 active
  device run 存在时，陈旧/禁用/最终失败的 worker 都不能接管 Message、标失败、consume 或 release ledger。
  管理员 readiness 响应只含计数、比例、P95、配置阈值和原因，不得含消息、prompt、模型输出或设备明细。
- URL 读取必须逐跳验证 scheme、凭据、端口和解析地址，拒绝本机/私网/link-local/保留地址并限制
  数量、重定向、时间、正文大小和内容类型。应用层校验不能完全消除 DNS rebinding，生产网络还应
  通过 egress proxy/防火墙阻止内网目标。
- 传给模型的数据只包含规范化消息字段和截断后的公开网页文本，不包含 raw webhook payload、token、
  session 或应用秘密；子进程错误不得回显模型原始输出或 API key。

## 加密与日志

- Telegram `api_hash` 与 session 使用 `backend/app/core/security.py` 的加密封装；密钥来源与轮换需在
  上线前验证。禁止新增明文字段或在 mapper 返回解密值。
- APNs/FCM 原生 device token 只在设备绑定 API 中接收，数据库保存应用加密值与 SHA-256 去重 hash；
  response、日志和 provider 错误不得回显 token。push payload 只能携带版本化 sync cursor，不得包含正文、
  联系人、商机摘要或 provider raw payload；平台报告 unregistered 后必须停止投递，设备撤销同步撤销 registration。
- 日志记录 event、request/trace ID、实体 ID、provider 与结果；秘密字段和原始消息正文默认不记录。
- 异常链保留内部可诊断信息，对外返回稳定的非敏感错误。

## 高风险变更清单

认证、授权、密码学、真实消息发送、AI 自动回复、生产迁移和部署权限变更必须：

1. 在执行计划中列威胁、误用路径、回滚和验收标准。
2. 覆盖成功与拒绝/失败路径，必要时做隔离环境冒烟。
3. 审查日志、错误与响应是否泄密。
4. 明确人工批准点；未获授权不得代表用户联系外部人员或操作生产环境。
