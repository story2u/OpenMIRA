# 功能地图

> 状态：当前事实 · 最后核验：2026-07-14

本地图用成熟度区分“仓库里有 UI/数据结构”和“用户端到端可用”。AI 开发前必须先确认目标
能力所在行，避免沿着 mock 或占位 DTO 继续实现。

## 成熟度定义

- **已实现**：有真实运行路径，并有与风险相称的自动化验证。
- **部分实现**：部分链路真实，仍有明确断点、降级或验证缺口。
- **仅演示**：只在浏览器状态、mock、timer 或默认映射中存在，不是生产能力。

## 用户能力

| 能力 | 前端入口 | 后端/API | 成熟度 | 备注 |
| --- | --- | --- | --- | --- |
| Google / Apple OAuth 登录 | `/login` | `/api/v1/auth/oauth/*`、`/auth/me` | 部分实现 | 真实 OAuth 与 JWT；依赖外部 provider 配置，缺少端到端 OAuth 测试。移动端原生登录 `POST /auth/oauth/{google\|apple}/native`（id_token 换 JWT）已实现并有路由测试，audience 经 `GOOGLE/APPLE_NATIVE_CLIENT_IDS` 配置，为空即关闭 |
| iOS 邮箱密码登录 | iOS 登录页 | `POST /auth/password/login`、`/auth/me` | 已实现 | 仅登录已有密码账户，不含注册/重置；PBKDF2 校验、统一失败响应、Redis 登录限流、JWT 存 Keychain；DEBUG/Release 均无粘贴 token 旁路 |
| 商机列表、筛选、分页 | `/` | `GET /opportunities` | 已实现 | 登录后每 30 秒轮询；查询按 owner 隔离；默认排除归档记录，可按 `archive=active\|archived\|all` 查询 |
| 商机归档与恢复 | `/`、`/opportunity/[id]` | `POST .../archive`、`POST .../restore`、`POST /bulk-archive` | 已实现 | 单条/最多 100 条批量归档、归档视图与恢复；保留原状态、消息和分析；归档项禁止业务写操作并跳过 AI 自动回复 |
| 商机看板（服务端筛选/排序/分页） | iOS/Android 商机 Tab | `GET /opportunities/dashboard` | 部分实现 | 后端聚合端点已实现：status/platform/source/时间范围/trust/sop/keyword 多维筛选 + 4 种排序 + total/pendingCount/attentionItems/keywordOptions，全部 SQL 完成并按 owner 隔离，有 Postgres 门控筛选矩阵测试；移动端 iOS(SwiftUI)/Android(Compose) 双 Tab 已接入但未在 CI 外做真机 E2E。Web `/` 仍走客户端 `applyFilters` 内存筛选，未切到该端点 |
| 商机语义识别 | 无独立页 | 摄取用例 + `OpportunityDetector` + LiteLLM | 已实现 | 高置信规则直通；启用 AI 后对其余非空消息结合 owner 隔离的有限会话历史、来源和 AI hint 复核；模型新发现只进人工审核，provider 失败回退规则 |
| 商机详情 | `/opportunity/[id]` | `GET /opportunities/{id}` | 部分实现 | 页面从列表 store 查找，未独立请求详情，刷新/深链能力有限 |
| 消息历史 | 详情页 SOP | `GET /messages` | 部分实现 | API 已有；前端未调用，后端数据加载后消息 store 为空 |
| 人工回复 | 详情页回复框 | `POST /opportunities/{id}/manual-reply` | 已实现 | Web 已调用真实 API；Telegram/企微按适配器发送，企微用户连接强制幂等键和人工审批，provider 成功后才落库并更新状态 |
| AI 回复草稿 | 详情页回复框 | `POST /opportunities/{id}/ai-draft` | 部分实现 | 后端可生成并保存；前端以 timer 模拟生成 |
| 非工作时间 AI 自动回复 | 无独立页 | Celery `ai.reply` | 已实现 | 受 `AI_ENABLED`、`IM_SEND_ENABLED` 与 provider 配置控制 |
| 商机状态更新/认领 | 看板/详情部分交互 | `PATCH .../status`、`POST .../claim` | 部分实现 | 后端已实现；前端状态动作多为本地更新 |
| 回复模板读取 | `/templates` | `GET /templates` | 已实现 | 登录后加载；空结果会回退到 mock 模板，应在生产化时移除静默回退 |
| 回复模板编辑 | `/templates` | `POST/PATCH /templates` | 部分实现 | 后端 admin API 已有；前端新增/编辑只改本地 store |
| Telegram 原生连接中心 | `/settings/telegram` | `/integrations/telegram/*` | 部分实现 | P0 Bot 群/频道有真实连接、来源、验签和 webhook 幂等；Business 需平台配置；P2 依赖平台 MTProto 凭据 |
| Telegram Bot 群组/频道 | `/settings/telegram` | `POST /integrations/telegram/connect/bot-chat`、`POST /webhooks/telegram` | 已实现 | 使用短期 token 和 `request_chat`，回调后验证 chat/Bot membership；未配置来源的 chat 不摄取 |
| Telegram Business 私聊 | `/settings/telegram` | `POST /integrations/telegram/connect/business`、`POST /webhooks/telegram` | 部分实现 | 私聊确认后按 Business connection owner 路由；需可用的平台 Bot 和 Telegram Business 权限 |
| Telegram 普通账号 QR | `/settings/telegram` | `POST /integrations/telegram/connect/mtproto-qr`、dialogs/sources API | 部分实现 | 平台统一凭据、二维码、加密 session、独立 QR worker 和只读 listener；需生产 Telegram 隔离冒烟 |
| Telegram 旧 MTProto 监听 | 无默认表单入口 | 独立 legacy listener | 已实现（兼容） | 已授权 session 继续监听；旧用户凭据采集 API 已删除，新页面不展示或迁移秘密 |
| 企业微信自建应用 | `/settings/wecom` | `/integrations/wecom/*`、`GET/POST /webhooks/wecom/{connection_id}` | 部分实现 | 用户级加密凭据、专属回调、验签/解密、幂等 Celery 摄取和成员私聊人工回复已实现；待真实企业联调，不支持普通群聊监听 |
| 企微会话内容存档 | 无 | 仅数据模型预留 | 未实现 | 内部/外部群聊读取需企业购买会话存档、合规授权和 Finance SDK；当前不用自建应用回调冒充 |
| 规则管理（全局） | `/settings` | CRUD `/rules` | 部分实现 | 后端 admin API 已有；面向普通用户的识别偏好改走用户级 `/settings/detection` |
| 用户级识别规则（关键词 + AI 语义） | Web/iOS/Android 设置 | `GET /settings/me`、`PATCH /settings/detection` | 已实现 | `user_detection_preferences` 表按 owner 隔离；关键词去空格/去重/限长限量；已接入 `ingest_message` 摄取（用户关键词叠加全局规则、AI 语义开关生效），三端共享同一数据源，有 owner 隔离与规范化测试 |
| 用户级工作时间 | `/settings/working-hours`、iOS/Android | `GET /settings/me`、`PATCH /settings/work-schedule` | 已实现 | `user_work_schedules` 表按 owner；IANA 时区 + 任意人工审核时段；`WorkScheduleService` 接入摄取决定人工/AI，无用户设置回退全局默认；三端共享，有时区/时段判定测试。旧全局 `/configs/work-mode` 保留但普通用户不再改全局 |
| 用户级通知偏好 | Web/iOS/Android 设置 | `GET /settings/me`、`PATCH /settings/notifications` | 部分实现 | `user_notification_preferences` 表按 owner 持久化，三端共享，有测试；推送通道尚未开发，UI 明确标注"将在推送服务启用后生效"，`capabilities.pushAvailable=false` |
| 统计摘要 | 无独立展示 | `GET /stats/summary` | 部分实现 | API 已有，前端未消费 |
| pi Agent 消息后处理 | 看板提醒、详情页 SOP | Celery `agent.analyze_message`、`POST .../agent-analysis` | 部分实现 | 新消息异步分析、补判商机、结构化建议；默认开启，依赖有效 provider key，并执行套餐月额度 |
| 统一订阅、购买与用量 | `/settings/subscription`、iOS/Android 套餐页 | `/subscriptions/{plans,catalog,me,sync,management}`、RevenueCat webhook | 部分实现 | RevenueCat 聚合 Paddle/App Store/Play，本地投影执行权益；三端 Offering/购买/恢复代码和 webhook 已实现。外部 Dashboard、真实 Sandbox E2E 与用户级企微群配置仍待完成，不能视作生产支付已开通 |
| 链接安全核验 | 详情页 SOP | SafeLinkInspector + pi Agent | 已实现 | 公网/重定向/大小限制、结果持久化、可重跑；不是恶意软件扫描器，生产需受控 egress |
| 联系方式提取 | 详情页 SOP | pi Agent 结果投影 | 部分实现 | 消息/公开网页中的联系方式可持久化；详情页手工编辑仍只更新浏览器状态 |
| 后续行动建议 | 详情页发现步骤 | pi Agent 结构化 actions | 已实现 | 可建议邮件、加好友、私信和内部提醒；外部动作强制标记需人工批准，不会自动执行 |
| 重大商机提醒 | 商机看板 | Opportunity attention projection | 已实现 | Agent 判定紧急/高影响或建议通知时展示 owner 隔离的看板提醒 |
| 好友申请状态流转 | 详情页 SOP Step 4 | `POST /opportunities/{id}/friend-request` | 部分实现 | 发送/通过/被拒/重试真实持久化并推进 SOP（pending→friend_requested、accepted→ready_to_chat），非法流转 409，owner 隔离；已移除 4 秒假自动通过 timer。"已通过/已拒绝"由操作员在 IM 内确认后手动回填——平台无自动发送好友申请的 IM 能力，实际发送仍是人工动作 |
| 通知偏好/每日摘要 | Web/iOS/Android 设置 | `/settings/notifications` | 部分实现 | 偏好真实持久化（见上"用户级通知偏好"）；每日摘要与推送投递未实现，不能声称已生效 |

## 后端能力入口

| 领域 | 关键代码 |
| --- | --- |
| 路由注册 | `backend/app/api/v1/router.py` |
| 认证与授权 | `backend/app/api/v1/routes/auth.py`、`backend/app/api/deps.py`、`backend/app/core/security.py` |
| 摄取编排 | `backend/app/application/use_cases/ingest_message.py` |
| 识别规则 | `backend/app/domain/services/detection_policy.py` |
| 用户级设置 | `backend/app/api/v1/routes/settings.py`、`backend/app/infrastructure/db/repositories.py`（`UserSettingsRepository`）、`backend/app/core/time_window.py`（`WorkScheduleService`） |
| 商机状态机 | `backend/app/domain/services/opportunity_state.py` |
| 回复编排 | `backend/app/application/use_cases/manual_reply.py`、`ai_reply.py` |
| IM 适配 | `backend/app/infrastructure/im/` |
| AI 分类/回复 | `backend/app/infrastructure/ai/litellm_client.py` |
| 异步任务 | `backend/app/worker/tasks.py`、`queue.py` |
| pi Agent 后处理 | `backend/app/application/use_cases/analyze_message.py`、`infrastructure/agent/`、`pi-agent-runtime/` |
| 订阅与额度 | `backend/app/domain/services/subscription_policy.py`、`application/use_cases/sync_revenuecat_customer.py`、`infrastructure/billing/` |
| 持久化 | `backend/app/infrastructure/db/models.py`、`repositories.py` |

## 扩展功能时的同步清单

- 功能从“演示”升级：移除对应 mock/timer，接入 API，补失败/加载状态和端到端验证，再改成熟度。
- 新 API：更新路由、DTO、前端 client/types、认证约束、测试和本表。
- 新 IM 平台：扩展 `IMChannel`、实现 `IMAdapter`、注册 adapter、迁移枚举存储影响、补解析和发送测试。
- 新商机状态：同步领域枚举、状态迁移、数据库、mapper、前端状态及筛选/统计。
- 新持久字段：SQLModel + Alembic + repository + DTO + mapper + 前端类型，禁止只更新其中一层。
