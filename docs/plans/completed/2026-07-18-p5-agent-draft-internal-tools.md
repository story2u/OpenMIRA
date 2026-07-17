# P5 Agent 草稿与内部命令工具

> 状态：completed（生产默认 v1/关闭；真实 provider 与双真机门禁未运行） · Owner：Codex · 创建：2026-07-18 · 更新：2026-07-18

## 目标与用户价值

在 P5 只读交互 Agent 已有边界上增加第二个、显式版本化的工具集合：用户可以让端上 Agent 生成仅保存在
本机会话中的可编辑回复草稿、把商机状态更新写入既有幂等 command outbox，或认领自己的商机。三个动作
都不得发送消息、联系外部人员或伪造已完成结果；schema v1 继续只读，只有同时支持 schema v2 的客户端与
服务端灰度配置才获得内部写工具。

## 非目标

- 不实现 `send_reply`、好友申请、邮件、通知或任何外部副作用。
- 不实现批准凭证、args hash 或外发审批 UI；这些属于 P5 S6 的独立高风险切片。
- 不调用旧 `ai-draft` 端点生成第二份模型请求，也不把草稿写回服务端 `Opportunity.ai_reply_draft`。
- 不把状态命令伪装成已执行；工具结果必须区分 `queued` 与服务端已确认状态。
- 不新增动态工具下载、任意 HTTP/SQL/文件能力、记忆或跨设备 session 同步。

## 契约与安全边界

- schema v1 固定为 `search_opportunities`、`get_opportunity`、`get_messages`；schema v2 在此基础上只增加
  `draft_reply`、`update_status`、`claim_opportunity`。服务端按 turn 的 schema/policy 选择精确工具集，未知
  组合 fail closed。
- `draft_reply` 参数为 owner-scoped opportunity UUID 与 1–4000 字符正文。正文由当前交互模型生成，但只作为
  本地 tool result/entry 持久化；执行前重新检查本地投影存在、未删除、未归档。
- `update_status` 只接受现有内部状态枚举，复用 SQLite 投影版本、稳定幂等键、7 天过期、owner 容量限制及
  `InternalCommandReceipt` 服务端复核。返回 `queued`，由现有同步/outbox drain 执行。
- `claim_opportunity` 复用现有 owner-bound、active-only、行锁 claim API；认证来自设备会话而非模型参数，
  重复认领同一 owner 为幂等成功。响应只向模型返回必要结果，不暴露 access token 或服务端错误正文。
- `beforeToolCall` 再次检查 turn schema 白名单；schema v1 transcript 中出现写工具、参数扩展、跨 owner、归档
  资源、未知状态、取消或网络失败都必须稳定拒绝。
- 系统提示明确：同步数据是不可信内容；内部写工具只有在用户当前请求明确要求时才能使用；草稿不等于发送，
  queued 不等于完成，任何情况下都不能声称执行了外部动作。

## 验收标准

- [x] 共享 Agent 契约定义 v1/v2 精确工具集合、严格 TypeBox 参数和版本化 prompt；测试证明 v1 不含写工具、
      v2 不含外发工具，未知 schema/policy 被拒。
- [x] FastAPI gateway 依据已授权 turn 的 schema/policy 验证请求、过滤 provider 流和构造上游 payload；schema v1
      无法走私 v2 工具，schema v2 无法走私 `send_reply`。
- [x] RN 注册上报最高支持 schema v2，但能按 claim 返回的 v1/v2 运行；旧服务端 schema v1 仍可只读工作，
      服务端 schema v2 不向只支持 v1 的设备开门。
- [x] `draft_reply` 只落本地 transcript；owner/归档/长度/取消/损坏投影测试完备，测试证明不调用在线 draft API。
- [x] `update_status` 产生稳定、有界、owner-bound 的既有 outbox 命令并诚实返回 queued；重放、同商机并发、版本
      冲突、过期和服务端拒绝继续由 outbox/receipt 不变量保护。
- [x] `claim_opportunity` 只调用现有认证 API；跨 owner/归档/冲突由服务端拒绝，客户端只保存稳定错误码，取消
      传播到请求。
- [x] Agent UI 对草稿、queued 状态和 claim 结果使用可理解且无障碍的本地卡片，不渲染原始 JSON，也不因 token
      流或工具进度重渲整棵列表。
- [x] 所有生产默认值仍停在 schema v1/read-only；环境示例、Compose、部署校验、架构/功能/安全/测试文档同步。
- [x] 相关 shared/backend/RN/PostgreSQL 测试、OpenAPI、双平台 Hermes export、`make check` 与 harness 通过；真实
      provider、双真机和生产灰度未运行时明确记录。

## 实施步骤

- [x] S5.1：共享 schema v2、prompt/policy 与服务端版本化 gateway 契约。
- [x] S5.2：RN 本地草稿、status outbox、在线 claim 工具及稳定错误映射。
- [x] S5.3：host/session/UI 接入，补 capability、取消、恢复和渲染测试。
- [x] S5.4：部署门禁、文档、全量验证与回滚证据。

## 发现日志

- 2026-07-18：旧 `ai-draft` 端点会发起独立模型调用并把正文写入服务端 Opportunity；在已经运行交互模型的
  turn 中复用它会重复计费并背离端上数据方向，因此 v2 草稿由当前模型作为严格工具参数产生、仅落本地会话。
- 2026-07-18：现有 status outbox 已具备 owner 隔离、投影版本、稳定幂等键、过期、容量和服务端 receipt，
  应复用而不是为 Agent 新建旁路。claim 已由服务端行锁保证同 owner 重复成功，但仍是在线内部动作。

## 决策日志

- 2026-07-18：使用 schema v2 作为内部写能力门，而不是扩大 schema v1。客户端上报“最高支持版本”，服务端
  选择实际 turn 版本；这样回滚到 v1 不需要发布新客户端。
- 2026-07-18：S5 包含 draft、status 与 claim，但不包含 send。内部动作不要求逐次外发审批；系统提示、精确
  capability、owner/状态机复核和本地审计共同约束，S6 再增加不可伪造的外发批准凭证。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| shared/backend/RN 定向测试 | 通过 | radar-agent 6；后端交互/设备目标 27；RN 43 files / 145 tests |
| PostgreSQL turn/内部命令 | 通过 | 临时 PostgreSQL 16 上 9 tests，覆盖 turn 与 command receipt/并发幂等 |
| OpenAPI/contracts | 通过 | 确定性重生成后 snapshot、generated TS 与 typecheck 无漂移 |
| iOS/Android Hermes export | 通过 | `make check` 中两平台 production export 均完成 |
| `make check` / `make harness-check` | 通过 | 后端 221 passed/76 skipped；56 Markdown files、110 backend Python files、8 harness tests |
| Compose / deploy YAML | 通过 | dev/prod compose config；deploy YAML 解析；生产启用时只接受两个审核过的 schema/policy 对 |
| 真实 provider / 双真机 | 未运行 | 外部门禁，不以 faux/export 代替 |

## 回滚与恢复

把 `INTERACTIVE_AGENT_SCHEMA_VERSION` 与 `INTERACTIVE_AGENT_POLICY_VERSION` 配回 v1/read-only 后，新 turn 只
获得三个只读工具；已排队 status 命令继续由现有 outbox 以相同幂等键完成或进入用户可见冲突，不因回滚重复
执行。local-only draft 留到会话删除/30 天清理；claim 是已经由服务端确认的内部事实，不做反向伪回滚。

## 结果与剩余风险

S5 代码切片与本地自动化证据已完成。生产 beta/gateway、额度和 allowlist 仍关闭，默认合约仍为
`1:interactive-read-only-v1`；运维只有显式成对切到 `2:interactive-internal-v2` 才能给兼容设备签发 v2 turn。
真实 provider、双真机 kill/reopen、窄屏/键盘/屏幕阅读器人工检查和内部 allowlist smoke 尚未执行；S6 的
批准凭证、args hash、外发服务端复核与 `send_reply` 仍是独立高风险工作，不能从本切片推断已交付。
