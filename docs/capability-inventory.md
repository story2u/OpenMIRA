# Capability Inventory

本文档把当前仓库能力映射到独立 Go + Next.js IM 的目标架构。它用于后续 PR 判断：保留到 core、移入 connector/provider/integration、作为过渡资产限期替换，或从默认产品面删除。

## 状态定义

| 状态 | 含义 | 默认部署 | 默认测试 |
| --- | --- | --- | --- |
| `Core` | IM 必需能力，表达稳定领域模型和运行面 | 必须启动 | 必须覆盖 |
| `Connector` | 消息通道适配，负责外部消息平台协议 | 可选启动 | fake connector 必须覆盖 |
| `Provider` | 自动化、媒体、设备、AI 或执行后端 | 可选启动 | fake/HTTP provider 必须覆盖 |
| `Integration` | 订单、预约、客户、门店等外部业务系统 | 可关闭 | 不阻断 core gate |
| `Transition` | 暂存命名、兼容别名、阶段性 gate 或桥接路径 | 有下线条件 | 有替代入口 |
| `Review` | 需要产品和运维价值复核，可能删除或降级 | 默认关闭 | 不进入 core gate |

## Core

这些能力是独立 IM 的主干，后续应继续加强契约、性能、可观测和高可用。

| 能力 | 当前主要位置 | 目标动作 |
| --- | --- | --- |
| 认证、会话、权限 | `internal/auth`, `internal/session`, `internal/sessionhttp` | 保留为 core，后续把历史命名改为 session/auth 中性语义 |
| 联系人和身份 | `internal/contacts`, `internal/contactidentity`, `internal/contactshttp` | 保留为 core，只存本地身份和通道绑定 |
| 会话和消息 | `internal/messages`, `internal/messageshttp`, `internal/workbench` | 保留为 core，继续把通道字段收敛为 channel identity |
| 任务和状态机 | `internal/tasks`, `internal/taskshttp`, `internal/tasksmodule` | 保留为 core，任务 record 使用 channel/provider 中性字段 |
| outbox 和实时 | `internal/outbox*`, `internal/realtime*`, `internal/wsgateway` | 保留为 core，补充 gap replay 和重建证据 |
| 管理台和诊断台 | `internal/workbench*`, `go/web` | 保留为 core shell，供应商页面降级到 connector/provider 配置面 |
| 审计、日志、观测 | `internal/infra/systemlogwriter`, `internal/clienterrors`, `cmd/release-readiness` | 保留为 core，profile 使用 release/readiness 语义 |

## Connector

消息平台只能作为 connector 进入系统。Core 只消费 `InboundEvent`、`OutboundMessage`、`DeliveryReceipt` 和 `ContactIdentity`。

| 能力 | 当前主要位置 | 目标动作 |
| --- | --- | --- |
| 通道 contract | `contracts/v1/connector-*.schema.json`, `internal/connector` | 作为 connector 边界的权威契约继续扩展 |
| 入站消息 | `internal/incominghandler`, `internal/incomingqueue`, `cmd/incoming-worker` | 保持通道中立，默认 gate 使用 internal webhook/fake event |
| 出站发送 | `internal/outboxdispatch`, `internal/senddispatcher`, `cmd/send-dispatcher` | 统一通过 outbound connector，默认可用 fake connector 验证 |
| 企微回调和通知 | `internal/weworknotify*`, `internal/friendadded*`, connector golden fixtures | 降级为 WeWork connector adapter，不定义 core 消息模型 |
| 企微登录和用户信息 | `internal/weworklogin*`, `internal/weworkuserinfo*`, `internal/infra/weworkcontactapi` | 移到 connector 配置与健康面，默认产品启动不要求存在 |
| 联系人同步 | `internal/contactsyncscheduler`, `cmd/contact-sync-worker` | 作为 connector sync job，失败不阻断消息收发 |

## Provider

自动化和执行后端只能通过 provider contract 接入。Core 负责 `AutomationRun`、任务状态、审计、超时、重试和 DLQ。

| 能力 | 当前主要位置 | 目标动作 |
| --- | --- | --- |
| 设备 SDK 和控制 | `internal/devicesdk*`, `cmd/api` device routes | 降级为 automation provider，默认 compose 不启动 |
| 设备桥接和屏幕/音频 | `internal/devicebridge*`, cloud `RPA_CALL_AUDIO_BRIDGE_*` env | 降级为可选 provider sidecar，补 owner、租约、权限、超时和删除条件 |
| 通话和 RTC | `internal/conversationcall*`, `internal/infra/devicertcstate` | 由 provider 执行，IM core 只记录请求、状态和结果 |
| 媒体处理和对象存储 | `internal/sendmedia*`, `internal/archivemedia`, archive media workers | 作为 media provider，失败可重试、可补偿 |
| 语音转写 | `internal/voicetranscription*`, `cmd/voice-transcription-worker` | 作为 media/AI provider，凭证和限流不进入 core |
| 自动化类任务 | `rpa_*` task types, provider dispatch paths | 收敛到 `AutomationCommand` 和 `AutomationRun`，保留兼容 task type 映射 |

## Integration

这些能力可以有产品价值，但不能定义主 IM 工作台的必备运行条件。

| 能力 | 当前主要位置 | 目标动作 |
| --- | --- | --- |
| 平台代理和 sidebar task | `internal/platformproxy*`, phase10 route metadata | 降级为 integration provider，可关闭、可替换、可降级 |
| AI outreach 和主动触达 | `internal/aioutreach*` | 创建标准 outbound task 或 automation run，外部 AI 失败可重试 |
| SOP 和内容运营 | `internal/sop*`, workbench SOP routes | 保留产品能力，但发送、媒体、外部取数走 core contract/provider |
| 客户关系和外部资料 | `internal/customerrelation`, workbench customer profile | 本地只存归一化 profile，外部资料源进入 integration provider |
| 归档 integration test 和外部归档配置 | `internal/archiveintegration`, archive readiness profiles | 作为 archive/media integration 验证，不阻断普通 IM core gate |

## Transition

这些资产短期允许存在，但必须有替代入口和下线条件。

| 资产 | 当前位置 | 替代方向 |
| --- | --- | --- |
| `phase1` 脚本和 artifact 命名 | `scripts/phase1_gate.sh`, `tmp/phase1`, golden case names | 使用 `scripts/release_gate.sh` 作为推荐入口，旧脚本保留为内部执行和兼容入口 |
| `GO_ENABLE_*_CANDIDATE` | `internal/config`, `cmd/api`, cloud compose | 改为 `GO_ENABLE_*` 或 release/readiness flag，保留 env alias 一段时间 |
| 供应商字段兼容别名 | `wework_user_id`, `GO_SEND_PROVIDER_BASE_URL` 等 | 新写路径使用 `channel_user_id`、`connector_*`、`provider_*` |
| 供应商命名 task type | `wework_login_*`, `wework_user_info`, `wework_logout` | 新创建任务使用 `connector_login_*`、`connector_user_info`、`connector_logout`，旧 task type 保留为兼容输入 |
| 以具体通道命名的 HTTP routes | `/api/v1/connectors/*`、`/api/v1/devices/{device_id}/apps/*` 主入口，`/wework/*` 和 device SDK open/stop 兼容入口，notify routes | 继续移到 connector/provider admin namespace，保留旧 route 作为 adapter-only 兼容入口并补下线条件 |
| 阶段编号 golden fixtures | `testdata/golden/phase*` | 保留为证据集合，新增按产品能力命名的 manifest |

## Review

这些能力需要先证明独立 IM 产品价值，再决定保留、降级或删除。

| 能力 | 复核问题 | 推荐默认 |
| --- | --- | --- |
| 低频设备控制操作 | 是否服务核心客服会话？是否有租约、审计和超时？ | 默认关闭，provider 专属入口 |
| 平台 sidebar 写操作 | 是否阻断消息主链路？是否可回滚？ | integration，可关闭 |
| 历史时间修正类诊断写操作 | 是否仍有生产数据价值？是否只能 dry-run？ | 只读或删除写入口 |
| 单供应商登录状态页面 | 是否应变成 connector account health？ | 改为 connector health/config |
| 无 owner 的桥接 sidecar | 是否有替代 provider 和删除条件？ | 不进入默认 compose |

## 下一批 PR 顺序

1. Connector 边界收敛：把入站、出站、联系人同步中的 WeWork 字段继续收敛为 channel identity 和 connector metadata。
2. Provider 边界收敛：把设备 SDK、RTC、音频桥接、浏览器/点击能力移出 core 路由语义。
3. 默认 compose 瘦身：core 只启动 API、Web、Redis、DB、outbox、incoming、send dispatcher 和 fake connector/provider。
4. Review 队列删除：对低价值高耦合能力逐项开 PR，删除或降级前补 readiness 影响说明。
5. 过渡命名收尾：把 artifact manifest、summary 脚本和 CI job 名称逐步切到 release/readiness 语义。

## 完成标准

- Core package 不再新增供应商协议、设备命令或外部业务 payload。
- 默认本地和 CI 不需要真实消息平台、RPA provider 或外部业务系统。
- Connector/provider/integration 都有可关闭、可替换、可观测和可回滚路径。
- 每个保留的过渡资产都有替代入口和下线条件。
