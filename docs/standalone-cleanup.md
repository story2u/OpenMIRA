# Standalone Cleanup Plan

本文档把战略重置转成可执行的清理清单。目标不是一次性删除所有现有能力，而是让每个能力都明确归属：进入 IM core、移到 connector/provider/integration，或从默认产品面移除。

当前仓库能力的具体归属和下一步动作见 [capability-inventory.md](capability-inventory.md)。

## 判断状态

每个功能域只能落入以下状态之一：

- `Core`：独立 IM 必需能力，默认构建、默认测试、默认部署。
- `Connector`：消息通道适配能力，通过 connector contract 进入 core。
- `Provider`：自动化、媒体、设备或 AI 执行能力，通过 provider contract 进入 core。
- `Integration`：订单、预约、客户、门店等外部业务平台能力，可关闭、可替换、可降级。
- `Remove`：不符合独立 IM 目标，或成本高于产品价值，进入删除队列。
- `Transition`：短期保留的命令、artifact、flag 或兼容别名，必须有替代路径和下线条件。

## 保留为 Core

- 认证、会话、租户、账号、角色和权限。
- 消息端工作台、管理台、诊断台的基础 API 和 Next.js 页面。
- 联系人、会话、消息、任务、投影、审计和系统日志。
- outbox、realtime gateway、WebSocket、Redis Stream、任务状态机和 worker runtime。
- 幂等、重试、DLQ、回滚、可观测和 release readiness profile。

这些能力必须能在本仓库独立构建、测试和部署。

## 降级为 Connector

- 企微回调、联系人同步、登录状态、用户信息、好友添加和通知能力。
- 任何消息平台的 webhook 签名、回调结构、外部 ID 绑定和 receipt 映射。
- 发送侧的文本、图片、文件、语音、撤回和 delivery receipt 外部协议。

要求：

- core 只接收 `InboundEvent`、`OutboundMessage`、`DeliveryReceipt` 和 `ContactIdentity`。
- 默认 CI 和本地开发使用 fake connector。
- 真实 connector 的故障不阻断 core contract、fake smoke 和基础发布检查。

## 降级为 Provider

- 魔云腾/MytRpc、设备 SDK、RTC、屏幕、音频桥接、点击输入、上传下载和浏览器自动化。
- 语音转写、媒体处理、对象存储和外部 AI 执行后端。
- 任何需要供应商凭证、设备租约或外部 runtime 的执行能力。

要求：

- `AutomationRun` 状态机独立于供应商。
- provider 必须声明 capability、health、capacity、version 和 error classification。
- 默认 compose 不强制启动 provider sidecar。
- 设备或浏览器控制必须有租户、权限、租约、审计和超时。

## 降级为 Integration

- 订单、预约、客户资料、门店、sidebar task、平台代理读写和类似业务系统能力。
- SOP、AI outreach 和主动触达中的外部业务取数能力。

要求：

- integration provider 可关闭、可替换、可降级。
- integration 失败不阻断会话、消息、发送任务和实时推送。
- 主工作台只展示 core 已归一化的数据，不直接绑定外部业务平台协议。

## 删除或下线队列

优先清理以下资产：

- 需要其他代码仓库才能解释、构建或测试的路径。
- 只服务阶段性对照或候选发布命名的临时资产。
- 以早期阶段、候选或发布切换语义作为长期产品语义的脚本、artifact、命令和包名。
- 没有 owner、替代路径和下线条件的 bridge、sidecar 和兼容别名。
- 默认产品面中低频、高耦合、强设备依赖且没有清晰 IM 价值的操作。

删除前需要确认：

- 是否仍被 compose、CI、readiness profile 或文档引用。
- 是否有 fake connector/provider 或标准 contract 替代。
- 是否影响默认独立启动路径。
- 是否需要数据变更、兼容 alias 或发布公告。

## 包与命名规则

- `internal/*` core package 不应直接依赖具体 connector/provider 的 SDK、凭证或协议对象。
- 新 env 名应使用 `CONNECTOR`、`PROVIDER`、`INTEGRATION`、`RELEASE`、`READINESS` 等中性词。
- 供应商名只允许出现在 adapter/provider/integration package、fixture 名称和明确的可选部署文档中。
- 新增代码不得扩大供应商、设备 SDK、候选发布或发布切换语义作为核心领域名的使用范围。

## 近期执行顺序

1. 文档和 roadmap 已切换到 standalone 叙事，继续禁止新增外部项目对照文档。
2. 将早期阶段、候选和发布切换命名从发布语义中剥离，先保留兼容命令，新增 release/readiness 中性入口。
3. 把发送、接收、联系人、自动化和设备能力按 connector/provider 边界拆出 core 依赖。
4. 默认 compose 只启动 API、Web、Redis、DB 和 incoming worker；发送、归档、联系人同步、provider sidecar 后续通过明确的 compose overlay 加入。
5. 对低价值高耦合能力做删除 PR；保留能力必须有 contract、fixture、observability 和 rollback 证据。

## 验证命令

```bash
cd go
rg -n "<外部项目对照词>|<发布切换旧命名>|<单一供应商名>" README.md docs deploy/README.md
rg -n "<供应商或设备核心命名>" internal cmd deploy
go test ./...
go vet ./...
go run ./cmd/release-readiness -all -format markdown
```

搜索结果不是零才算失败；真正的失败是命中项仍把供应商、过渡命名或外部项目当成 core 约束。每次保留命中都要能解释为 connector/provider/integration、兼容入口或明确的下线项。
