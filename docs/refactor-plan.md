# Python 到 Go + Next.js 重构总体方案

> 本文档记录 `Python/` 旧系统迁移到 `go/` 新实现的总体方案。
> 事实来源以当前代码、`Python/docs/ai/`、`Python/contracts/v1/` 和 `go/cmd/inventory` 扫描结果为准。
> 本文档只描述迁移策略，不代表旧系统接口、数据流或部署语义已经改变。
> 具体阶段拆解见 `go/docs/phased-plan.md`。
> 可复用的 Codex 长任务提示见 `go/docs/codex-refactor-goal-prompt.md`。

## 1. 目标与边界

### 1.1 总目标

将现有 Python/FastAPI 后端与 React/Vite 前端，分阶段迁移为 Go 后端与 Next.js + Tailwind CSS 前端，同时保持线上外部行为稳定。

### 1.2 迁移原则

- 旧系统 `Python/` 在切换完成前仍是生产事实源。
- Go 与 Next.js 新实现必须先对齐契约，再接管流量。
- 每个阶段只迁移一个清晰职责边界，阶段内可验证、可回滚。
- 默认不修改 API 路径、请求参数、返回结构、WebSocket 事件、Redis key、DB schema、task contract。
- 前端不承担业务过滤、权限裁剪、状态归类、统计口径或事实推断。
- 热点运行时 `incoming-worker` 与 `send-dispatcher` 的独立部署和重启边界必须保留。

### 1.3 当前第一阶段状态

`go/` 已建立第一阶段骨架：

- Go API: `/`、`/healthz`、`/readyz`、`/metrics`
- Go inventory: 只读扫描旧路由、契约、功能文档、Docker 服务
- Next.js: `/` 客服入口、`/admin` 管理入口
- 验证命令：`go test ./...`、`go run ./cmd/inventory -python-root ../Python`、`npm run build`

## 2. 当前系统模块梳理

### 2.1 后端入口与运行角色

旧后端主入口是 `Python/backend/app/api/server.py`，通过 FastAPI 挂载健康检查、会话、客服工作台、设备、账号、任务、存档、分配、管理、统计、审计、平台代理、实时回放、企微通知和 WebSocket 路由。

运行角色由 `CLOUD_RUNTIME_ROLE` 和 `context_bootstrap_profile.py` 控制：

| 角色 | 现有职责 | 迁移要求 |
| --- | --- | --- |
| `api` | HTTP API、认证、页面数据读取、任务创建 | 先迁移只读 API，再迁移写入口 |
| `worker` / `outbox_worker` | outbox relay、搜索投影增量同步 | 保持事件驱动，禁止同步请求内串行广播 |
| `incoming-worker` | Redis Stream 入站消息消费 | 只消费入站队列，不引入重后台任务 |
| `send-dispatcher` | durable SDK 发送任务认领与执行 | 保持跨设备并发、同设备串行、fail-closed 门禁 |
| `archive-sync-worker` | 会话存档拉取 | 保持游标、锁、批量、幂等 |
| `archive-media-worker` | 存档媒体拉取、OSS、语音转写触发 | 保持快慢车道和对象存储直链 |
| `maintenance-worker` | 维护任务、投影重建、校准 | 不得进入收发热点链路 |
| `automation-worker` | SOP platform pull 等自动化拉取 | 只创建 durable 发送任务，不 inline 调 SDK |
| `ws-gateway` | 浏览器实时通道 | 保持 `/ws/{channel}` 和 Redis Pub/Sub 语义 |

### 2.2 功能域

当前主功能域按 `Python/docs/ai/01_SYSTEM_INDEX.md` 维护：

| 功能域 | 旧入口 | 迁移优先级 |
| --- | --- | --- |
| 客服工作台 | `web/src/apps/cs`、`routes/chat.py` | 高 |
| 管理后台 | `web/src/apps/admin`、admin routes | 中 |
| 账号与设备管理 | `routes/accounts.py`、`routes/devices.py` | 高 |
| 会话分配 | `routes/assignments.py` | 高 |
| 消息发送与分发 | `actions_send_routes.py`、`context_tasks.py` | 极高风险，后置 |
| AI 自动回复 | `ai_reply_service.py`、`sop_service.py` | 后置 |
| SOP 与知识库 | `admin_config.py`、`knowledge.py` | 中后 |
| 会话存档 | `archive.py`、archive services | 高风险，后置 |
| 设备网关与 SDK 控制 | P1/MytRpc/RPA 相关模块 | 极高风险，后置 |
| 实时推送 | `api/ws.py`、`realtime/*` | 高 |
| 通讯录同步与身份资料 | contact sync services | 中 |
| 认证与权限 | `routes/session.py`、`api/deps.py` | 高，先迁移兼容层 |
| 统计与审计 | `routes/stats.py`、`audit_logs.py` | 中 |
| AI 会话质检每日分析 | `entrypoints/ai_quality_daily.py` | 低，可独立迁移 |
| 系统侧主动触达 | `routes/ai_outreach.py` | 高风险，需契约测试 |

### 2.3 核心数据流

#### 消息接收

企微回调、存档或设备实时事件进入后端后，低延迟入口写 Redis Stream；`incoming-worker` 消费后写入 `messages`、`conversations`、投影和 outbox。实时事件由 outbox relay 发布到 Redis Pub/Sub，再推送浏览器。

Go 迁移要求：

- 保持 `POST /api/v1/messages/incoming` 默认只写持久队列并快速返回。
- Redis 不可写时继续 fail-closed，不能退回内存队列假成功。
- `conversation.message.received` 必须先于自动回复请求可认领。

#### 消息发送

客服、AI、SOP、主动触达入口创建任务，任务状态先进入 `accepted`，再由 inline 或 `send-dispatcher` 执行 SDK/P1 发送。终态回写 `tasks.status`、`messages.send_status`、`task.status` WS 事件和 AI/SOP attempt。

Go 迁移要求：

- 保持 `task-create.schema.json` 和 `task-status.schema.json`。
- 保持 Redis `lock:sdk-device:{device_id}` 和同设备 UI 串行。
- 保持 `_send_policy`、过期保护、停用保护、提交后不确定失败等待存档确认。
- 不得把二维码登录、截图、设备控制、通话或收消息误放入 durable 发送队列。

#### 实时推送

浏览器连接 `/ws/{channel}`；后端通过 Redis Pub/Sub，默认 topic `cloud_ws_events`。前端建连前必须先续期 JWT，避免旧 token 握手。

Go 迁移要求：

- 保持 WS 路径和鉴权语义。
- 保持事件名、payload、cursor、replay、snapshot 语义。
- `send-dispatcher`、`automation-worker` 等角色只允许 publish-only，不订阅全局 fanout。

#### 会话存档与媒体

会话存档回调触发拉取，归一化后写消息和 canonical outbox。媒体任务拉取文件、上传对象存储、生成签名 URL；语音可进入转写任务。

Go 迁移要求：

- 保持存档消息幂等、去重和游标。
- 媒体链路优先对象存储直链或签名 URL，不走后端大文件代理。
- 语音转写和重试接口保持状态字段兼容。

## 3. 目标架构

### 3.1 Go 后端分层

建议目录：

```text
go/
  cmd/
    api/
    ws-gateway/
    incoming-worker/
    send-dispatcher/
    archive-sync-worker/
    archive-media-worker/
    maintenance-worker/
    automation-worker/
    inventory/
  internal/
    api/             # HTTP 路由、参数校验、响应序列化
    app/             # 用例编排，不直接操作 DB
    domain/          # 领域模型和值对象
    infra/           # MySQL/Redis/OSS/HTTP/P1/SDK 适配
    realtime/        # WS hub、broker、replay
    contracts/       # JSON schema 加载和兼容校验
    observability/   # 日志、metrics、trace
    workers/         # worker loop 与 role profile
  web/
```

分层约束：

- `internal/api` 不直接调用 DB/Redis。
- `internal/app` 编排用例，不持有 HTTP request。
- `internal/infra` 只做数据访问和外部服务适配，不写业务判断。
- `internal/realtime` 不直接查询数据库。
- worker 入口只做生命周期和 loop 编排。

### 3.2 Next.js 前端分层

建议目录：

```text
go/web/
  app/
    page.jsx
    admin/page.jsx
  src/
    api/
    components/
    features/
      chat/
      admin/
      devices/
      assignments/
      archive/
    hooks/
    lib/
    styles/
```

前端约束：

- `/` 保持客服工作台入口，`/admin` 保持管理后台入口。
- API client 只封装传输、鉴权、错误、重试，不做业务过滤。
- 首屏、局部刷新、提交中必须有 loading 或骨架屏。
- 实时 hook 必须复用 token refresh 和 replay/snapshot 补偿。

### 3.3 数据与基础设施兼容面

必须保持兼容：

- API: `/api/v1/**`、`/ws/{channel}`、`/healthz`、`/readyz`、`/metrics`
- Contract: `Python/contracts/v1/*.schema.json`
- DB: 现有 migrations、表名、字段、索引、时区落库口径
- Redis: Stream、Pub/Sub topic、锁、dedup、projection key、deferred key
- Docker: 现有运行角色、容器职责、热路径重启边界
- 对象存储: OSS/对象引用、签名 URL、媒体直链口径

## 4. 分阶段计划索引

完整阶段拆解见 `go/docs/phased-plan.md`。阶段顺序为：

| 阶段 | 主题 | 迁移性质 |
| --- | --- | --- |
| 0 | 基线冻结与清单化 | 只读对账 |
| 1 | Go/Next 骨架与契约护栏 | 已完成基础骨架，继续补测试 |
| 2 | 认证、配置、观测与只读基础设施 | 低风险只读 |
| 3 | 客服工作台只读链路 | 只读页面与读模型 |
| 4 | 管理后台只读与低风险写入口 | 管理端渐进替换 |
| 5 | 实时网关与事件回放 | WS 协议兼容 |
| 6 | 任务创建与发送状态读写 | 写链路前置 |
| 7 | send-dispatcher 与 SDK/P1 执行边界 | 高风险发送链路 |
| 8 | 消息接收、outbox 与 projection | 高并发入站链路 |
| 9 | 会话存档、媒体与语音转写 | 存档和媒体链路 |
| 10 | AI、SOP、知识库与主动触达 | 自动化业务 |
| 11 | Next.js 完整替换 | 前端接管 |
| 12 | 灰度、切流与旧实现下线 | 发布与收尾 |

## 5. 验证体系

### 5.1 每阶段通用验证

```bash
cd go
go test ./...
go run ./cmd/inventory -python-root ../Python -pretty

cd web
npm run build
npm run test
npm run test:e2e
node ../scripts/next-routes.mjs app --check --markdown
```

### 5.2 兼容验证

- API golden diff：同请求分别打 Python 和 Go，对比状态码、字段、错误结构。
- Contract validation：任务 payload 与状态事件必须通过旧 JSON schema。
- WS replay：用旧事件样本验证 Go WS 输出。
- Redis integration：验证 Stream、Pub/Sub、锁、dedup、projection key。
- DB integration：只读查询先跑真实 schema，写入链路必须有回滚或幂等保护。

### 5.3 性能验证

- 热点接口必须分页、增量、缓存。
- 发送认领必须验证并发 dispatcher 不重复 claim。
- 入站队列必须验证 pending reclaim 和 DLQ。
- WS 必须验证多客户端、断线重连、gap replay。
- 媒体链路必须验证对象存储直链，禁止后端代理大文件。

## 6. 回滚策略

- 任何阶段 Go 接管前，Python 仍保留可用入口。
- 反向代理按 route group 切换，出现问题按组回退。
- 写链路迁移必须具备幂等键或终态保护。
- Worker 迁移必须先单实例灰度，再多实例扩容。
- 数据结构不变更时回滚只切服务；若未来必须改 schema，需单独设计向前兼容迁移和回滚脚本。

## 7. 风险清单

| 风险 | 表现 | 控制措施 |
| --- | --- | --- |
| API 不兼容 | 前端字段缺失、外部调用失败 | golden diff 和 contract test |
| WS 事件偏差 | 会话不刷新、发送状态卡住 | 事件样本 replay 和旧前端 smoke test |
| 发送重复或串发 | 同设备并发、联系人定位错误 | Redis 锁、claim 过滤、真机回归 |
| 入站消息丢失 | Stream pending 卡住、DLQ 缺失 | XAUTOCLAIM、幂等、DLQ 验证 |
| projection 口径漂移 | 待回复/未读/分配统计不一致 | 只读口径测试和禁止全扫兜底 |
| 前端承担业务逻辑 | 权限和筛选与后端不一致 | API 参数化查询，前端只展示 |
| 热点 worker 被误重启 | 收发延迟尖刺 | 部署差异分组和热路径保护 |
| RPA 行为不等价 | 真机 UI 失败或重复提交 | 保留 Python executor，逐 flow 迁移 |

## 8. 下一步建议

优先推进阶段 0 和阶段 1 的补强项：

1. 扩展 `cmd/inventory`，输出 route prefix、权限依赖、response model。
2. 增加 API golden test harness。
3. 建立 WS 事件样本目录。
4. 建立 Redis key 与 DB 表清单。
5. 给 CI 增加 `go test ./...`、`npm run build`、inventory 检查。

这些工作不会接管业务流量，但会显著降低后续迁移误差。
