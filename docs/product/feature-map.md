# 功能地图

> 状态：当前事实 · 最后核验：2026-07-10

本地图用成熟度区分“仓库里有 UI/数据结构”和“用户端到端可用”。AI 开发前必须先确认目标
能力所在行，避免沿着 mock 或占位 DTO 继续实现。

## 成熟度定义

- **已实现**：有真实运行路径，并有与风险相称的自动化验证。
- **部分实现**：部分链路真实，仍有明确断点、降级或验证缺口。
- **仅演示**：只在浏览器状态、mock、timer 或默认映射中存在，不是生产能力。

## 用户能力

| 能力 | 前端入口 | 后端/API | 成熟度 | 备注 |
| --- | --- | --- | --- | --- |
| Google / Apple OAuth 登录 | `/login` | `/api/v1/auth/oauth/*`、`/auth/me` | 部分实现 | 真实 OAuth 与 JWT；依赖外部 provider 配置，缺少端到端 OAuth 测试 |
| 商机列表、筛选、分页 | `/` | `GET /opportunities` | 已实现 | 登录后每 30 秒轮询；查询按 owner 隔离 |
| 商机详情 | `/opportunity/[id]` | `GET /opportunities/{id}` | 部分实现 | 页面从列表 store 查找，未独立请求详情，刷新/深链能力有限 |
| 消息历史 | 详情页 SOP | `GET /messages` | 部分实现 | API 已有；前端未调用，后端数据加载后消息 store 为空 |
| 人工回复 | 详情页回复框 | `POST /opportunities/{id}/manual-reply` | 部分实现 | 后端真实发送/落库；前端当前只写本地 store |
| AI 回复草稿 | 详情页回复框 | `POST /opportunities/{id}/ai-draft` | 部分实现 | 后端可生成并保存；前端以 timer 模拟生成 |
| 非工作时间 AI 自动回复 | 无独立页 | Celery `ai.reply` | 已实现 | 受 `AI_ENABLED`、`IM_SEND_ENABLED` 与 provider 配置控制 |
| 商机状态更新/认领 | 看板/详情部分交互 | `PATCH .../status`、`POST .../claim` | 部分实现 | 后端已实现；前端状态动作多为本地更新 |
| 回复模板读取 | `/templates` | `GET /templates` | 已实现 | 登录后加载；空结果会回退到 mock 模板，应在生产化时移除静默回退 |
| 回复模板编辑 | `/templates` | `POST/PATCH /templates` | 部分实现 | 后端 admin API 已有；前端新增/编辑只改本地 store |
| Telegram 普通账号配置 | `/settings/telegram` | `/integrations/telegram-user/*` | 已实现 | 登录码、2FA、会话、dialogs、monitor 均连接后端 |
| Telegram Bot webhook | 无 | `POST /webhooks/telegram` | 已实现 | 验签、规范化、摄取；bot 必须在目标群中 |
| Telegram MTProto 监听 | 配置页 | 独立 listener | 已实现 | 每用户监听已授权会话；默认只摄取，不用个人账号发送 |
| 企业微信 webhook | 设置页显示绑定卡 | `GET/POST /webhooks/wecom` | 已实现 | 后端验签/解密/摄取；前端绑定状态目前是静态展示 |
| 规则管理 | `/settings` | CRUD `/rules` | 部分实现 | 后端 admin API 已有；前端关键词与 AI 开关仅本地状态 |
| 工作时间配置 | `/settings/working-hours` | `/configs/work-mode`、`PATCH /configs/{key}` | 部分实现 | 后端可读写；前端编辑只在本地状态 |
| 统计摘要 | 无独立展示 | `GET /stats/summary` | 部分实现 | API 已有，前端未消费 |
| 链接安全核验 | 详情页 SOP | 无真实服务 | 仅演示 | `setTimeout` 返回固定安全/可疑结果 |
| 联系方式提取 | 详情页 SOP | DTO 返回默认空值 | 仅演示 | 表单只更新浏览器状态，数据库没有对应字段 |
| 好友申请 | 详情页 SOP | 无 | 仅演示 | timer 4 秒后自动通过 |
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
| 持久化 | `backend/app/infrastructure/db/models.py`、`repositories.py` |

## 扩展功能时的同步清单

- 功能从“演示”升级：移除对应 mock/timer，接入 API，补失败/加载状态和端到端验证，再改成熟度。
- 新 API：更新路由、DTO、前端 client/types、认证约束、测试和本表。
- 新 IM 平台：扩展 `IMChannel`、实现 `IMAdapter`、注册 adapter、迁移枚举存储影响、补解析和发送测试。
- 新商机状态：同步领域枚举、状态迁移、数据库、mapper、前端状态及筛选/统计。
- 新持久字段：SQLModel + Alembic + repository + DTO + mapper + 前端类型，禁止只更新其中一层。
