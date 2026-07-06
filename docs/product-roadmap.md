# Product Roadmap

本文档定义精简后的 Go + Next.js IM 集成平台路线。当前仓库只保留新的平台内核，后续围绕 `internal/integrationhub` 扩展。

## 产品目标

打造一个可独立运行、可水平扩展、可观测、可恢复的 IM 集成平台：

- Go API 负责通道、会话、消息、SOP、AI 协同、审计和观测。
- Next.js 前端负责运营端、消息端和配置管理体验。
- 消息接入使用 connector 模型，支持多个成熟 IM 工具并存。
- 自动化能力使用 provider 模型，供应商能力不进入 IM core。
- 空系统阶段不保留旧接口兼容层，优先建立干净的产品内核。

## 当前保留能力

- `cmd/api`
- `internal/integrationhub`
- `web`
- PostgreSQL schema and seed data
- Connector contracts:
  - `contracts/v1/connector-inbound-event.schema.json`
  - `contracts/v1/connector-outbound-message.schema.json`
  - `contracts/v1/connector-delivery-receipt.schema.json`

## 当前不保留能力

- 过渡开关体系。
- 非当前产品内核模块。
- 与单一 IM 平台或 RPA 供应商绑定的 core 字段。
- 无法在当前仓库独立验证的桥接逻辑。

## 优先级

P0：精简后的 API/Web/Postgres 能稳定构建、测试和部署。

P1：入站 connector event 能生成标准 conversation/message。

P2：出站 command 能进入 connector dispatcher，并形成 delivery receipt。

P3：SOP workflow 与 AI policy 能参与真实消息流。

P4：补齐 realtime、worker、队列、DLQ、观测和容量治理。

P5：扩展多 connector、多租户、权限和插件化配置。

## 决策原则

1. 新能力先进入 `integrationhub`，不要恢复旧 runtime。
2. Core schema 只表达通道中立对象。
3. Connector/provider adapter 可以包含平台细节，但不能污染 core。
4. 写路径默认幂等、可重试、可审计。
5. 当前空系统阶段允许破坏式 schema 调整。
