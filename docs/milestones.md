# Milestones

本文档给出精简后的里程碑：可独立运行的 Go + Next.js 高性能、高可用 IM 集成平台。

## 阶段 0：旧代码清理

目标：

- 删除历史 worker、历史 runtime、过渡配置和历史 task/agent contract。
- 保留 `cmd/api`、`internal/integrationhub`、`web`、PostgreSQL schema 和部署基线。
- GitHub Actions 只构建当前可运行的 API/Web 镜像。

退出标准：

- `go test ./...` 只覆盖新内核。
- `docker compose config` 使用最小服务图。
- 文档不再指导使用已删除的旧命令或旧模块。

## 阶段 1：独立 API 与控制台

目标：

- API 服务 `/api/v1/*` 覆盖前端需要的所有页面数据。
- PostgreSQL migration 和 seed 支持空库启动。
- Next.js 控制台通过同源 `/api/v1/*` 访问 Go API。

退出标准：

- `go test ./...`
- `go vet ./...`
- `npm run build`
- `docker build` for `im-api` and `im-web`

## 阶段 2：通道中立消息接收

目标：

- Connector callback 统一解析为 `InboundEvent`。
- `InboundEvent` 落库后生成标准 `Conversation` 和 `Message`。
- 企微只作为 `connectors/wecom` adapter。

退出标准：

- 同一套 service 可处理 fake connector 和 WeCom adapter。
- 入站事件具备幂等键、trace id、审计和失败原因。
- 前端消息端能看到真实入站消息。

## 阶段 3：通道中立消息发送

目标：

- 前端发送动作只创建 `OutboundCommand`。
- Connector dispatcher 消费出站命令并调用 adapter。
- Delivery receipt 回写 message/outbox 状态。

退出标准：

- 出站命令支持 pending、requires_approval、sending、sent、failed、canceled。
- 重试、审批、取消均有审计。
- Fake connector 和至少一个真实 connector 通过同一套测试。

## 阶段 4：SOP 与 AI 协同

目标：

- SOP workflow 可绑定通道和会话。
- AI policy 可参与分类、草稿、风险识别和人工交接。
- SOP/AI 只生成标准 message 或 outbound command。

退出标准：

- SOP 状态与消息流一致。
- AI 失败不阻断消息接收。
- 所有自动动作可追溯、可回放、可关闭。

## 阶段 5：运行时与高可用

目标：

- 引入新的 worker runtime、队列、DLQ、realtime 和观测指标。
- API stateless，worker 可水平扩展。
- 关键数据可备份、恢复和一致性检查。

退出标准：

- 入站、出站、SOP worker 均可独立部署。
- 队列堆积、失败率、延迟和重试可观测。
- 生产部署具备健康检查、回滚和容量基线。
