# ADR-0013：端上 pi Agent 优先，服务端网关与 runner 兜底

> 状态：accepted · 日期：2026-07-17

## 背景

当前 pi 0.80.6 在 Celery worker 内通过受限 Node 子进程运行，模型 key、额度预留、链接读取、结果钳制
与投影都在服务端。目标形态要求兼容设备在本地运行 harness 和会话，但移动操作系统不能保证后台常驻，
模型密钥与平台发送凭据也不能进入 App。

本地包审计确认 `pi-agent-core` 主入口没有直接加载 Node 环境入口，但包声明 Node >=22.19；当前
`getModel`/`streamFn` API 也不能按设计草图直接注入 `baseURL`/fetch。因此 Hermes 兼容必须由固定版本的
双平台 release 构建证明，不能由静态 import 搜索推断。

## 决策

- 对 capability 允许且在线的设备，端上 pi harness 是分析首选执行者；服务端现有 Node runner 继续受
  ADR-0002 约束，并作为设备离线、租约过期、版本不兼容或网关失败时的 fallback。
- 第一阶段端上 runtime 只允许现有 `submit_analysis` 工具，不加载 coding-agent、skills、context、
  shell、通用文件或通用网络工具。对话和业务工具在独立阶段逐项开放。
- 新建“分析运行”资源：设备 claim 时校验 owner、message、设备 capability、状态和套餐，原子预留一次
  `usage_ledger` 并取得短期 lease/run token。heartbeat、complete、fail、expire 都必须幂等。
- 模型调用只经过服务端推理网关。App 只持用户/设备凭据和短期 run token；provider key、模型实际名称、
  配额裁决与成本记录留在服务端。网关限制正文大小、token、并发、速率、超时并转发取消，不记录
  prompt/响应正文。
- ADR-0004 的计费语义保持不变：一次顶层分析只消费一次额度；同一 run 的内部 provider 请求只记录
  token/cost/latency。交互式 Agent 若收费，必须使用新的 feature/套餐决策。
- 端上 complete 的结果始终是不可信输入。服务端用 Pydantic 校验、版本/lease/owner 检查并重新执行
  `project_agent_result` 后才投影；由服务端执行的外部动作还需独立批准凭证与二次钳制。
- 链接默认通过受 run token 约束的服务端 fetch 代理，复用逐跳 SSRF、端口、地址、重定向、内容类型、
  数量、时间和响应大小限制；端上不得默认直连消息中的可疑 URL。
- 所有结果持久化 `executed_by`、device/runtime/schema/model/policy version 和 run id。先 shadow 对比，
  再逐设备灰度端上生效；shadow 不得双重扣额或双重投影。
- pi/Hermes P0 gate 若任一平台失败，RN/同步工作继续，`deviceAgentAvailable=false`，生产分析保持服务端。

## 备选方案

- **立即删除服务端 runner**：架构更纯，但 App 被杀或系统限制后台执行时雷达会失明，不能接受。
- **App 直接调用 provider/BYOK**：省去网关，但默认用户需要暴露 key，平台无法做统一套餐、成本和撤销。
- **远程 RPC Agent**：实现简单，却把 harness/会话继续留在云上，不能满足端上智能与离线会话目标。
- **在移动端嵌 Node/运行 coding-agent**：包体、平台政策和工具权限面均不可接受。

## 后果

端上可拥有 harness、会话和审批上下文，同时服务端保留公网/密钥/计费/发送边界。代价是需要设备身份、
租约、网关、双运行时 golden fixtures、fallback SLA 和更完整的观测；迁移期会存在两种执行者，但结果
只允许一个 run/version 成功提交。

## 验证与复审

- P0 在 iOS/Android release 环境验证 faux provider、SSE tool-call、abort、前后台切换和崩溃恢复；
  禁止使用真实 provider key 作为自动测试前提。
- 数据库/API 测试覆盖 claim 竞态、租约回收、重复 complete/fail、owner 隔离和账本 reserve/consume/release。
- 网关测试覆盖正文/速率上限、取消、provider 错误、无正文日志与模型 alias；隔离环境才做真实 provider smoke。
- 端上成功率、P95、重复 run、quota 漂移或 fallback 延迟达不到发布阈值时关闭 capability，不下发 key。
