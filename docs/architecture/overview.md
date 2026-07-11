# 架构总览

> 状态：当前事实 · 最后核验：2026-07-11 · 代码真相源：`backend/app/`、`frontend/`

## 系统上下文

商机雷达把外部 IM 消息转换为可审核、可回复的商机。系统有四类边界：外部消息平台、Web
用户、AI 提供商和持久化/队列基础设施。

```mermaid
flowchart LR
    TG["Telegram Bot / 用户账号"] --> API["FastAPI API / 监听器"]
    WC["企业微信"] --> API
    WEB["Next.js Web"] <--> API
    API --> PG[(PostgreSQL)]
    API --> REDIS[(Redis)]
    REDIS --> CELERY["Celery worker / beat"]
    CELERY --> PG
    CELERY --> AI["LiteLLM / OpenAI"]
    CELERY --> PI["pi Agent / 受限 Node runner"]
    CELERY --> WEBLINK["公开网页 / 安全链接读取器"]
    CELERY --> TG
    CELERY --> WC
```

## 部署单元

| 单元 | 入口 | 职责 |
| --- | --- | --- |
| Frontend | `frontend/app/layout.tsx` | 认证上下文、商机看板、详情/SOP、设置与模板 UI |
| API | `backend/app/main.py` | FastAPI 路由、OAuth、查询/命令、webhook 接入 |
| Celery worker | `backend/app/worker/celery_app.py` | AI 回复与超时任务的异步执行 |
| Celery beat | 同上 | 周期调度待人工商机超时检查 |
| pi Agent runner | `backend/pi-agent-runtime/src/index.mjs` | 由 worker 按消息启动；只提交结构化分析，不持有业务动作权限 |
| Telegram listener | `backend/app/worker/telegram_listener.py` | 按用户启动 MTProto 监听器并复用消息摄取用例 |
| PostgreSQL | SQLModel + Alembic | 用户、订阅/用量账本、消息、商机、规则、配置、模板、Telegram 配置 |
| Redis | DB 0/1/2 | 应用临时状态、Celery broker、Celery result backend |

本地编排见 `backend/docker-compose.yml`；生产镜像编排见
`backend/docker-compose.prod.yml`；GitHub Actions 构建和 VPS 部署见 `.github/workflows/`。
后端 Python 版本与直接依赖声明在 `backend/pyproject.toml`，`backend/uv.lock` 是跨平台精确锁；
本地、CI 和 Docker 均通过 uv 同步同一依赖图。

## 后端分层

项目是“实用型分层 + 领域端口”，并非严格的 Clean Architecture：application 当前可直接使用
SQLModel 实体和具体 repository。新增代码必须至少保持以下单向约束，不得进一步侵蚀内层。

```mermaid
flowchart LR
    DOMAIN["domain\n枚举、端口、纯规则"]
    CORE["core\n配置、安全、时间、日志"]
    INFRA["infrastructure\nDB、IM、AI"]
    APP["application\nDTO、映射、用例"]
    ENTRY["api / worker\n传输与组合根"]

    INFRA --> DOMAIN
    INFRA --> CORE
    APP --> DOMAIN
    APP --> CORE
    APP --> INFRA
    ENTRY --> APP
    ENTRY --> INFRA
    ENTRY --> DOMAIN
    ENTRY --> CORE
```

### 层职责与禁区

- `domain/`：领域枚举、Protocol 端口、商机识别和状态迁移规则。不得导入 `api`、
  `application`、`infrastructure`、`worker` 或框架/数据库实现。
- `core/`：无业务编排的跨切面能力。不得依赖 API、application、infrastructure、worker。
- `infrastructure/`：端口适配器与持久化实现。不得依赖 API、application 或 worker。
- `application/`：用例、DTO 与映射。可协调现有基础设施，但不得依赖 API/worker。
- `api/`、`worker/`：组合根和传输适配层，可以组装上述模块；业务判断应下沉到用例或领域服务。

这些稳定禁区由 `scripts/harness_check.py` 解析 Python AST 检查。若要收紧成严格端口架构，先写
ADR 和迁移计划，不要在单个功能中半途改造。

## 核心数据模型

| 模型 | 作用 | 关键关系/约束 |
| --- | --- | --- |
| `User` / `AuthAccount` | 本地用户与 OAuth 身份 | provider + subject 唯一；资源按 `user_id` 隔离 |
| `SubscriptionAccount` | 用户付费状态与当前 billing period | user 唯一；只有 active/trialing 且在周期内的付费套餐生效 |
| `UsageLedger` | AI 功能额度的可审计账本 | user + feature + idempotency key 唯一；reserved/consumed/released |
| `Message` | 收发消息审计记录 | channel + external_message_id 幂等；保存 pi 分析状态/结果；可关联 opportunity |
| `Opportunity` | 商机聚合根 | 持有来源、检测结果、Agent 投影、状态、草稿与最终回复；source_message 唯一 |
| `Rule` | 关键词/正则/AI hint 规则 | 启用、优先级、分数驱动检测策略 |
| `AppConfig` | 运行期业务配置 | JSONB value；当前包含工作时间等配置 |
| `ReplyTemplate` | 人工回复模板 | 可启用、分类 |
| `TelegramConnection` | 新版用户 Telegram 连接 | owner、连接类型、状态、能力和非明文凭据槽；不向 API 返回秘密 |
| `TelegramSource` | 连接下的群组/频道/私聊来源 | connection + external chat 唯一；按 owner、enabled 与 quota_paused 过滤 webhook |
| `TelegramConnectionAttempt` / `TelegramWebhookEvent` | 连接握手与 webhook 审计 | 仅保存连接令牌哈希/TTL；以 Telegram update ID 去重，不保存 raw webhook |
| `TelegramUserConfig` / `TelegramMonitor` | 旧 MTProto 兼容路径 | 既有加密 session 与 listener 保持可用，直到单独的迁移计划完成 |

结构变更以 `backend/alembic/versions/` 的迁移历史为准；模型变更必须配套新迁移。

## 关键数据流

### 消息摄取与商机识别

```mermaid
sequenceDiagram
    participant IM as Telegram/企微
    participant E as Webhook 或 MTProto listener
    participant U as IngestMessageUseCase
    participant D as OpportunityDetector
    participant DB as PostgreSQL
    participant Q as Celery
    participant P as pi Agent

    IM->>E: 外部消息
    E->>U: InboundMessage
    U->>DB: 按 channel + external_message_id 去重并落 Message
    U->>D: 文本 + 已启用规则
    D-->>U: DetectionResult
    alt 非商机
        U->>DB: 标记 processed
    else 工作时间商机
        U->>DB: 创建 PENDING_HUMAN Opportunity
        U->>Q: 通知审核者（当前为队列接口）
    else 非工作时间商机
        U->>DB: 创建 AI_AUTO_REPLY Opportunity
        U->>Q: enqueue_ai_reply
    end
    U->>DB: 按 owner 锁定并预留 pi Agent 月额度
    alt owner 缺失或额度耗尽
        U->>DB: fail closed / quota_exceeded
    else 额度可用
        U->>Q: enqueue agent.analyze_message（开启且 provider 已配置）
    end
    Q->>Q: 验证 URL / 限制抓取
    Q->>P: 规范化消息 + 链接证据
    P-->>Q: submit_analysis 结构化建议
    Q->>DB: 保存审计结果并投影到商机
    opt 规则漏判且 Agent 高置信
        Q->>DB: 创建 PENDING_HUMAN 商机
    end
```

### Telegram 原生连接

```mermaid
sequenceDiagram
    participant U as 登录用户
    participant W as Next.js 设置页
    participant A as FastAPI
    participant B as Telegram Bot API
    participant T as Telegram webhook
    participant DB as PostgreSQL

    U->>W: 选择群组/频道连接
    W->>A: 创建短期连接尝试
    A->>DB: 仅保存 token hash、TTL、owner 和 request IDs
    A-->>W: Bot deep link
    U->>B: /start + 选择 chat
    B->>T: message/chat_shared update
    T->>A: 带 secret token 的 webhook
    A->>DB: 按 update_id 去重
    A->>B: getChat + getChatMember
    A->>DB: 创建 TelegramConnection/Source
    B->>T: 后续消息
    A->>DB: 只按启用 source 路由并摄取
```

### pi Agent 消息后处理

- `PI_AGENT_ENABLED` 默认开启；显式设为 `false` 时摄取链路不启动 Node 或链接网络请求。开启但
  provider key 缺失时不入队并记录非敏感配置告警，不回退到匿名或 mock provider。
- worker 从 `agent` 队列领取 message ID；重复任务通过 Message 分析状态和 source message 唯一索引
  保持幂等，失败可重试。
- Python `SafeLinkInspector` 只读取公网 HTTP(S) 文本，逐跳检查重定向并限制端口、数量、时间、
  响应字节和传给模型的文本长度。网页内容与消息文本都作为不可信数据。
- Node runner 使用 `@earendil-works/pi-agent-core` 的无持久会话 Agent，不加载 coding-agent、
  context、skills 或内置工具；唯一工具 `submit_analysis` 用 TypeBox 验证最终结构并终止 loop。
- Python 再用 Pydantic 校验并执行确定性投影：链接读取器的风险不能被模型降级；邮件、好友申请、
  私信建议强制需要人工批准；内部重大商机提醒可以直接展示。
- Agent 高置信补判商机时只创建 `PENDING_HUMAN`，不能让模型把自己路由到自动回复。
- 自动分析使用 message 级幂等键，手工重跑接受 `Idempotency-Key`；两条路径都在 enqueue 前通过
  `SubscriptionRepository` 预留额度。worker 成功结算，最终失败释放；Free 使用 UTC 自然月，付费
  用户使用有效 subscription period。无 owner 消息不运行 Agent，也不占用任何用户额度。
- Telegram 配置读取和 listener 每次刷新都重新解析有效套餐；套餐到期后，按用户保留优先级启动
  额度内 monitor，超额项标记 `quota_paused` 而不删除。用户可在设置页重新选择保留群；选择写入当前
  retention limit，升级后优先级仍保留，未来再次降级可复用。
- 新版 Bot 来源与旧 MTProto monitor 在 Telegram 套餐额度中合计统计。新来源创建时先检查总额；
  webhook 只会摄取已启用且未被额度暂停的 `TelegramSource`。

### 回复

- 人工回复：API → `ManualReplyUseCase` → IM adapter 发送 → 创建 outgoing `Message` → 状态
  进入 `FOLLOWING` 或 `REPLIED`。
- AI 草稿：API → `AIDraftUseCase` → `LiteLLMReplyGenerator` → 保存 `ai_reply_draft`，不发送。
- AI 自动回复：Celery → `AIAutoReplyUseCase` → 生成/复用草稿 → IM adapter 发送 → 记录消息 →
  状态进入 `REPLIED`。
- `IM_SEND_ENABLED=false` 是本地安全阀；适配器不得绕过它执行真实发送。

## 前端结构与状态边界

- App Router 页面位于 `frontend/app/`，复用组件位于 `frontend/components/`，基础 UI 在
  `frontend/components/ui/`。
- `AuthProvider` 负责 localStorage token 与 `/auth/me` 恢复；`AppStoreProvider` 当前混合后端
  数据与演示态本地状态。
- `frontend/lib/api.ts` 是 HTTP 访问边界，`frontend/lib/types.ts` 是前端契约。后端字段变化
  必须从 DTO → API client → types → UI 连贯更新。
- 生产功能不得继续扩大 `AppStoreProvider` 中的 timer/mock 状态；新增真实能力应先补 API client，
  再把 UI action 接到后端并处理 loading/error/rollback。

## 主要不变量

- 外部消息摄取按平台消息 ID 幂等。
- 用户查询与 Telegram 配置必须按当前认证用户隔离。
- 商机状态只能走 `domain/services/opportunity_state.py` 允许的迁移。
- 外部 payload 在传输/适配器边界解析；领域规则只接收规范化数据。
- 发送成功后要记录 outgoing Message；失败不得伪造已回复状态。
- Agent 外部动作只是建议；未经过独立审批用例不得调用 IM/邮件/好友适配器。
- URL 分析拒绝本机、私网、link-local 和保留地址；生产还应使用受控 egress 降低 DNS rebinding 风险。
- 时间均使用带时区 datetime；业务工作时间默认 `Asia/Shanghai`，可由配置覆盖。
- 秘密只来自环境或加密字段，日志和 API 响应不得暴露 token、session、api_hash。
