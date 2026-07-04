# Milestones

本文档给出新的产品里程碑：可独立运行的 Go + Next.js 高性能、高可用 IM 项目。阶段编号保留 0-12，便于延续现有计划节奏，但含义转为独立实现和产品化建设。

## 阶段 0：战略重置

目标：

- 删除过渡期项目叙事的文档。
- 明确 Go + Next.js 是项目的目标运行面。
- 建立 connector/provider 中立原则。
- 标记需要清理的过渡资产。

退出标准：

- README 和 docs 不再把其他代码库作为目标参照。
- 新增产品路线、架构、里程碑和发布就绪文档。
- 后续 PR 以独立 IM 产品目标评审。

## 阶段 1：独立启动骨架

目标：

- Go API、Next.js、MySQL、Redis、基础 worker 能在本仓库独立启动。
- 本地和 CI 具备统一 gate。
- 现有 `phase1` 命名保留为过渡实现，不继续扩展语义。

退出标准：

- `go test ./...`、`go vet ./...`、`npm run test`、`npm run build` 通过。
- Docker compose 可启动 API、Web、Redis 和核心 worker。
- README 只描述本项目自己的验证方式。

## 阶段 2：核心领域模型

目标：

- 收敛 `Tenant`、`User`、`Agent`、`Contact`、`Conversation`、`Message`、`OutboundTask`、`DeliveryReceipt`、`AutomationRun`。
- 清理把通道名、设备名或供应商名写入 core 的字段和接口。

退出标准：

- Core package 不依赖具体消息平台或 RPA provider。
- 领域模型有单元测试、契约 fixture 和数据库 schema 变更脚本说明。

## 阶段 3：通道中立消息接收

目标：

- 建立 connector contract。
- 用 internal/webhook fake connector 跑通 inbound message。
- 将企微接收链路降级为 connector adapter。

退出标准：

- `InboundEvent` 到 message/projection/outbox/realtime 的链路可重放。
- 新 connector 不需要修改 IM core。
- 真实通道失败不影响 fake connector 测试。
- `connector-inbound-event.schema.json`、`internal/connector` 和 incoming worker 的 connector event 测试形成基础证据。

## 阶段 4：通道中立消息发送

目标：

- 发送入口只创建标准 `OutboundTask`。
- Connector worker 负责外部发送、receipt 和失败分类。
- 客服工作台不感知具体通道发送协议。

退出标准：

- 文本、图片、文件、语音等 payload 有统一 contract。
- 发送任务支持幂等、重试、超时、DLQ 和人工补偿。
- 至少一个 fake connector 和一个真实 connector 通过同一套 conformance tests。
- `connector-outbound-message.schema.json`、`connector-delivery-receipt.schema.json`、task-to-connector 适配测试、fake outbound connector dispatch conformance test 和 `GO_SEND_CONNECTOR_MODE=fake` runtime 测试形成基础证据。

## 阶段 5：实时网关与投影

目标：

- outbox relay、WebSocket gateway、gap replay 和页面回源刷新稳定化。
- 投影可重建、可回放、可诊断。

退出标准：

- `message.received`、`message.sent`、`message.delivery_updated`、`task.status` 形成端到端 smoke。
- WS 断线重连、乱序、重复投递和延迟事件有测试。
- 关键投影有重建命令或维护入口。

## 阶段 6：管理台与配置面

目标：

- 管理台覆盖账号、角色、分配、内容配置、敏感词、AI 配置、审计和诊断。
- 高风险设置有权限和审计。

退出标准：

- 管理 API 有 contract tests。
- 管理页面只消费 Go API，不重建领域逻辑。
- 配置变更能触发可观测事件。

## 阶段 7：Provider 中立自动化

目标：

- 建立 automation provider contract。
- 将魔云腾/MytRpc、设备控制、浏览器自动化等实现隔离到 provider。
- 默认部署使用 fake/HTTP provider 完成验证。

退出标准：

- `AutomationRun` 状态机独立于供应商。
- provider health、capacity、error classification 可观测。
- 自动化失败不影响 IM core 写路径。

## 阶段 8：高可用任务与 worker

目标：

- 统一 worker pool、重试、DLQ、backpressure、锁和幂等。
- 发送、归档、媒体、自动化、同步任务采用一致运行模型。

退出标准：

- Worker 可水平扩缩容。
- 队列堆积、失败率、重试次数和处理耗时可观测。
- 关键 worker 有容器集成和 replay tests。

## 阶段 9：归档、媒体与对象存储

目标：

- 归档接收、媒体下载、对象存储、语音转写和冷存储产品化。
- 供应商配置进入 provider，不进入 core。

退出标准：

- 媒体任务可重试、可补偿、可限流。
- 对象引用稳定，外部 provider 故障可降级。
- 归档链路有一致性检查和修复入口。

## 阶段 10：外部业务集成

目标：

- 订单、预约、客户、门店等外部系统通过 integration provider 接入。
- 主 IM 工作台不强依赖任一外部业务平台。

退出标准：

- integration provider 可关闭、可替换、可降级。
- 外部业务失败不会阻断消息核心链路。
- 高风险写操作有审计和回滚。

## 阶段 11：完整产品体验

目标：

- 客服工作台、管理台、诊断台形成可连续使用的产品体验。
- 登录、搜索、会话、发送、实时、任务、配置和诊断有端到端 smoke。

退出标准：

- 桌面与移动主要 viewport 无明显布局和交互问题。
- 前端错误、API 错误、WS 错误和版本信息可观测。
- 发布前 smoke 可由 CI 或预发环境稳定执行。

## 阶段 12：发布就绪与规模化

目标：

- 建立 release readiness profile、SLO、容量预算、备份恢复和故障演练。
- 移除过渡期命名、临时 sidecar 和不再需要的候选开关。

退出标准：

- profile 使用 release/readiness 命名，不再使用历史发布语境。
- API、worker、Web、Redis、DB 和对象存储有容量与告警基线。
- 独立生产部署不依赖外部代码仓库。
- 不合理功能已删除、降级为 integration，或明确进入后续产品路线。
