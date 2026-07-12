# 功能地图

> 状态：当前事实 · 最后核验：2026-07-11

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
| 商机列表、筛选、分页 | `/` | `GET /opportunities` | 已实现 | 登录后每 30 秒轮询；查询按 owner 隔离 |
| 商机语义识别 | 无独立页 | 摄取用例 + `OpportunityDetector` + LiteLLM | 已实现 | 高置信规则直通；启用 AI 后对其余非空消息结合 owner 隔离的有限会话历史、来源和 AI hint 复核；模型新发现只进人工审核，provider 失败回退规则 |
| 商机详情 | `/opportunity/[id]` | `GET /opportunities/{id}` | 部分实现 | 页面从列表 store 查找，未独立请求详情，刷新/深链能力有限 |
| 消息历史 | 详情页 SOP | `GET /messages` | 部分实现 | API 已有；前端未调用，后端数据加载后消息 store 为空 |
| 人工回复 | 详情页回复框 | `POST /opportunities/{id}/manual-reply` | 部分实现 | 后端真实发送/落库；前端当前只写本地 store |
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
| 企业微信 webhook | 设置页显示绑定卡 | `GET/POST /webhooks/wecom` | 已实现 | 后端验签/解密/摄取；前端绑定状态目前是静态展示 |
| 规则管理 | `/settings` | CRUD `/rules` | 部分实现 | 后端 admin API 已有；前端关键词与 AI 开关仅本地状态 |
| 工作时间配置 | `/settings/working-hours` | `/configs/work-mode`、`PATCH /configs/{key}` | 部分实现 | 后端可读写；前端编辑只在本地状态 |
| 统计摘要 | 无独立展示 | `GET /stats/summary` | 部分实现 | API 已有，前端未消费 |
| pi Agent 消息后处理 | 看板提醒、详情页 SOP | Celery `agent.analyze_message`、`POST .../agent-analysis` | 部分实现 | 新消息异步分析、补判商机、结构化建议；默认开启，依赖有效 provider key，并执行套餐月额度 |
| 统一订阅、购买与用量 | `/settings/subscription`、iOS/Android 套餐页 | `/subscriptions/{plans,catalog,me,sync,management}`、RevenueCat webhook | 部分实现 | RevenueCat 聚合 Paddle/App Store/Play，本地投影执行权益；三端 Offering/购买/恢复代码和 webhook 已实现。外部 Dashboard、真实 Sandbox E2E 与用户级企微群配置仍待完成，不能视作生产支付已开通 |
| 链接安全核验 | 详情页 SOP | SafeLinkInspector + pi Agent | 已实现 | 公网/重定向/大小限制、结果持久化、可重跑；不是恶意软件扫描器，生产需受控 egress |
| 联系方式提取 | 详情页 SOP | pi Agent 结果投影 | 部分实现 | 消息/公开网页中的联系方式可持久化；详情页手工编辑仍只更新浏览器状态 |
| 后续行动建议 | 详情页发现步骤 | pi Agent 结构化 actions | 已实现 | 可建议邮件、加好友、私信和内部提醒；外部动作强制标记需人工批准，不会自动执行 |
| 重大商机提醒 | 商机看板 | Opportunity attention projection | 已实现 | Agent 判定紧急/高影响或建议通知时展示 owner 隔离的看板提醒 |
| 好友申请执行 | 详情页 SOP | 无 | 仅演示 | Agent 只提供建议；现有按钮仍是 timer 演示，不能视为真实发送 |
| 通知偏好/每日摘要 | `/settings` | 无用户通知偏好 API | 仅演示 | switches 只存在页面 state |

## 后端能力入口

| 领域 | 关键代码 |
| --- | --- |
| 路由注册 | `backend/app/api/v1/router.py` |
| 认证与授权 | `backend/app/api/v1/routes/auth.py`、`backend/app/api/deps.py`、`backend/app/core/security.py` |
| 摄取编排 | `backend/app/application/use_cases/ingest_message.py` |
| 识别规则 | `backend/app/domain/services/detection_policy.py` |
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
