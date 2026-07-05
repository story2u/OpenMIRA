# Product Roadmap

本文档定义独立 Go + Next.js IM 项目的产品方向和取舍原则。它作为后续实现、裁剪和发布决策的上层依据。

## 产品目标

打造一个可独立运行、可水平扩展、可观测、可恢复的高性能高可用 IM 平台：

- Go 后端负责稳定的消息、任务、实时、投影、审计和 worker 体系。
- Next.js 前端负责消息端工作台、管理台和运维诊断。
- 消息接入使用 connector 模型，支持多个通道并存。
- RPA 和自动化使用 provider 模型，支持不同执行后端。
- 关键链路以本仓库的契约、测试和发布证据为准。

## 产品边界

必须保留：

- 会话、联系人、消息、任务、消息端分配和管理能力。
- API、WebSocket、outbox、worker、Redis Stream、DB transaction 和投影的一致性。
- 审计、诊断、可观测和发布就绪能力。
- 高风险写路径的幂等、重试、DLQ、回滚和告警。

必须弱化或移除：

- 把某个消息平台当成唯一消息模型的代码和文档。
- 把某个 RPA 供应商当成自动化默认前提的 API、env、worker 和数据模型。
- 需要外部代码仓库才能解释、构建或测试的路径。
- 只为过渡期存在的 candidate flag、sidecar 和历史发布命名。
- 未形成产品价值、又显著增加部署和运维复杂度的设备控制功能。

## Core 与 Adapter

IM core 只表达稳定领域对象：

- `Tenant`
- `User`
- `Agent`
- `Conversation`
- `Message`
- `Contact`
- `OutboundTask`
- `DeliveryReceipt`
- `AutomationRun`
- `AuditEvent`

外部差异进入 adapter：

- 消息通道 connector：企微、Web chat、短信、邮件、内部测试通道。
- RPA provider：浏览器自动化、设备自动化、HTTP automation、人工审批。
- 媒体 provider：对象存储、转码、语音转写、缩略图。
- 平台代理 provider：订单、预约、客户、门店等外部业务系统。

Core 不应知道 provider 的内部协议、凭证字段、回调签名或设备命令。

## 裁剪原则

1. 不能独立运行的能力先进入隔离层，再决定重写或删除。
2. 只服务单个供应商的字段不能进入 core schema。
3. 没有测试、观测、回滚和告警的写路径不能默认开启。
4. 低频且高耦合的设备操作要拆成可选 provider，默认部署不启动。
5. 与 IM 产品目标无关的平台代理能力要降级为可选 integration，不进入主工作台。
6. 新能力优先服务真实消息端会话、消息可靠性、管理效率和稳定性。

具体能力状态、删除队列和命名规则见 [standalone-cleanup.md](standalone-cleanup.md)，当前仓库能力归属见 [capability-inventory.md](capability-inventory.md)。

## 优先级

P0：项目能独立启动、独立测试、独立发布。

P1：通道中立的消息收发和实时推送。

P2：provider 中立的自动化和任务执行。

P3：高可用运行面、可观测、容量规划和故障恢复。

P4：多通道、多租户、插件化集成和管理体验完善。

## 决策记录要求

涉及以下事项时，需要在 PR 或对应 docs 中记录原因：

- 删除或降级现有功能。
- 把供应商能力移出 core。
- 新增 connector/provider 接口。
- 引入新的运行时依赖、队列、缓存或数据库对象。
- 改变 API、事件、任务或投影契约。
