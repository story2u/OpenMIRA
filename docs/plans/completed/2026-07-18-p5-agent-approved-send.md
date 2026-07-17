# P5 Agent 明确批准后外发

> 状态：completed（生产发布证据待补） · Owner：Codex · 创建：2026-07-18 · 完成：2026-07-18

## 目标与用户价值

在交互式 Agent 的 v1 只读、v2 内部动作边界上增加第三个精确合约：当模型提出向某个商机发送回复时，
RN 必须暂停工具循环，向用户展示目标、渠道、完整正文、风险和有效期；只有用户本次明确批准后，服务端才
签发短期一次性凭证，并在复核 owner、设备、turn、参数哈希、商机版本、归档/状态、IM 开关和幂等键后
复用既有 `ManualReplyDelivery` 发送一次。模型不能读取、提交、持久化或自行构造批准凭证。

本计划是 [P5 端上交互式 Agent](../active/2026-07-17-p5-interactive-agent.md) 的 S6 高风险切片。两份
2026-07-16 提案只提供方向；本计划以当前人工回复账本、设备会话、交互 turn、代码与测试为行为真相。

## 非目标

- 不开放好友申请、邮件、联系人、日历、通知、任意 HTTP/SQL/文件或批量外发。
- 不允许“本会话总是批准”、自动批准、预批准、长期授权或关闭“外部动作总是询问”。
- 不把外部发送放进离线 command outbox；批准和执行都必须在线，网络不确定时不得自动换 key 重试。
- 不把批准 token、正文、模型 prompt/result 或 IM 凭据写入服务端批准表、SQLite、SecureStore、日志或 URL。
- 不替换既有人工回复 API/账本；Web、详情页人工发送和旧移动端语义保持不变。
- 不把 faux adapter、Hermes export 或 `IM_SEND_ENABLED=false` 测试描述成真实 IM 已验收。

## 冻结契约

### 工具与版本

- schema v1/v2 保持字节级兼容。schema v3/policy `interactive-approved-send-v3` 只在 v2 六工具上增加
  `send_reply(opportunity_id, text)`；`text` 为 1–4000 字符非空正文，工具不接受 token、owner、device、
  resource version、幂等键、channel、operator 或 `mark_following`。
- 客户端上报最高支持 schema v3；服务端选择实际 turn 版本。v3 还要求独立
  `INTERACTIVE_AGENT_EXTERNAL_ACTIONS_ENABLED=true`、`IM_SEND_ENABLED=true`、beta/gateway/allowlist/正数额度
  和审核过的 schema/policy 对。所有生产默认值继续停在 v1 且关闭。
- `beforeToolCall` 对 v3 白名单再检查；只有 `send_reply` 进入批准门。v1/v2 transcript 中含 v3 工具的完整
  user turn 在回退时丢弃，不能借历史重放扩大权限。

### 用户手势与参数

- 宿主先用 owner-scoped SQLite projection 重新读取 active 商机，取得 title/channel/contact 与
  `aggregateVersion`。审批卡展示完整目标、渠道、正文、外发风险和“批准后 2 分钟有效”；正文可编辑。
- 用户拒绝时工具必须被 block，服务端尽力写入无正文 `DENIED` 审计；拒绝、关闭卡片、App 退后台、取消、
  lease 丢失或组件卸载都不得执行工具。
- 用户编辑正文后，以编辑后的完整正文创建批准决定；服务端 canonical JSON/hash 随之变化。原模型 tool-call
  args 不是执行权威，实际执行参数来自该次 UI 决定，并由批准记录与一次性 token 绑定。
- idempotency key、expected resource version、tool-call ID 与 `mark_following=true` 由宿主/服务端注入，模型
  无法选择。UI Promise 只在用户按钮事件中 resolve；批准 token 仅保存在当前 host closure，并在第一次执行
  尝试前删除。

### 服务端批准与执行

- 批准决定端点只接受 active purpose-bound interactive turn token。记录仅保存 owner/device/turn/tool-call、
  tool、opportunity、expected version、idempotency key、canonical args SHA-256、状态和时间；不保存正文。
- `GRANTED` 记录使用派生 nonce 的 SHA-256 并返回最多 120 秒的 purpose approval token；`DENIED` 不签 token。
  同一 turn/tool-call/同参数重试幂等；换参数、换决定或超过每 turn 上限冲突。
- 执行端点只接受 approval token。服务端重新检查 active device、turn、schema/policy/external-action gate、nonce、
  owner、tool、args hash、幂等键、版本、归档/状态和 adapter/`IM_SEND_ENABLED`，再原子把批准从 `GRANTED`
  claim 为 `EXECUTING`。并发/重放只有一个调用取得执行权。
- `ManualReplyDelivery` 继续是 provider 外发幂等真相。批准 `CONSUMED` 表示已有确定投递；provider 结果不确定
  同时进入 `UNCERTAIN`，不得复用凭证或自动新建批准；明确发送前失败进入 `FAILED`，需要用户重新批准。
- 执行开始前商机版本变化、归档、非法状态、owner/device/turn 变化、token 过期/篡改/重放都必须在 provider
  调用前拒绝。发送开始后的并发业务变化不能改写已投递事实，沿用现有投影恢复语义。

## 状态机

```text
用户拒绝 ───────────────────────────────────────────────▶ DENIED
用户批准 ─▶ GRANTED ─▶ EXECUTING ─┬─▶ CONSUMED
             │          │          ├─▶ FAILED（确认未发送）
             │          │          └─▶ UNCERTAIN（禁止自动重试）
             └───────────┴────────────▶ EXPIRED（未执行且超时）
```

终态不可回退；`EXECUTING` 进程崩溃后按不确定结果处理，不得重新开放同一 token。

## 验收标准

- [x] 共享 v3 工具/prompt/schema 严格、v1/v2 不变；Python/TS exact contract 和 provider 流测试拒绝
      `send_reply` 走私、token 参数、未知 schema/policy。
- [x] 新增 expand-only Alembic 迁移与无正文批准模型；约束 owner/device/turn、唯一 tool-call、SHA-256、状态时间
      和一次性 nonce，fresh upgrade、downgrade、re-upgrade 与 model drift 均验证。
- [ ] 批准决定 API 只接受有效 active turn token；覆盖批准/拒绝、同键重放、异参冲突、上限、过期、撤销设备、
      stale turn/schema 和跨 owner。
- [ ] 执行 API 重新计算 canonical args hash 并复核版本/归档/状态/adapter/开关；覆盖正文/版本/key/token 篡改、
      并发重放、发送前失败、provider uncertain、投影恢复，证明 provider 最多调用一次。
- [ ] RN `beforeToolCall` 只对 v3 `send_reply` 暂停；审批 UI 展示目标、渠道、全文、风险、有效期和编辑框，
      approve/deny/cancel/background/unmount/lease-loss 路径确定，token 不进入模型或本地持久化。
- [ ] 编辑正文会得到新 hash 并以编辑值执行；拒绝和取消不调用执行 API；成功 UI 只在服务端确认后显示已发送，
      网络/不确定结果不得伪造成功。
- [x] 生产默认 v1、external-actions=false、IM send=false；Compose/部署只允许
      `3:interactive-approved-send-v3` 在两个外发开关和其他 beta 门禁全部开启时配置。
- [ ] OpenAPI/contracts、shared/backend/RN/PostgreSQL、`make check`、harness、双平台 Hermes export 通过；
      真实 provider、授权 IM 沙箱和双真机未运行时明确记录。

## 实施步骤

- [x] S6.1：共享 schema v3、domain canonical hash、配置/部署门禁与执行计划。
- [x] S6.2：批准表/迁移、purpose token、repository/service/DTO/API 与 PostgreSQL 安全测试。
- [x] S6.3：执行服务复用 ManualReplyDelivery，补版本绑定、并发/uncertain/投影恢复测试。
- [x] S6.4：RN approval coordinator、beforeToolCall、API、审批 UI、session/result 卡片和取消路径。
- [ ] S6.5：OpenAPI、双平台导出、全仓验证、权威文档与回滚证据。

## 发现日志

- 2026-07-18：现有 `ManualReplyDelivery` 已用 owner + idempotency key + 正文/目标状态 hash 保证 provider
  最多调用一次，并对 timeout 标记 `UNCERTAIN`；S6 应在它前面增加批准证明，不能另建第二套发送幂等真相。
- 2026-07-18：pi 0.80.6 的 `beforeToolCall` 只能 block，不能替换 args。为满足“用户编辑后重新 hash”，
  执行值必须保存在 host 的 tool-call-scoped closure；服务端批准记录是实际参数权威，模型原始 args 仅作提议。
- 2026-07-18：客户端普通 access token 不能充当批准证明。决定端点使用 active turn token，执行端点使用
  独立 purpose token；两者都重新检查 active device，撤销设备立即 fail closed。
- 2026-07-18：发送不确定路径中的 delivery repository rollback 会使同 session ORM 对象失效；日志上下文和
  approval ID 必须在 provider 调用前冻结，成功收尾不能无条件 rollback，否则会掩盖原始状态或返回 detached
  projection。实现已按成功/失败路径分别处理。
- 2026-07-18：按用户要求，本任务在实现、针对性验证和权威文档更新后结束，不继续后续 Agent-native
  里程碑；全仓 `make check`、双平台 Hermes export、真实 provider/授权 IM 沙箱/双真机留作发布前门禁。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| shared/backend/RN 定向测试 | 通过 | shared 7；radar-api 60；RN 149；`make backend-check` 223 passed / 85 skipped |
| PostgreSQL/迁移 | 通过（已知漂移除外） | approval 9 passed；fresh upgrade 已通过；012 downgrade→upgrade 通过；`alembic check` 无 S6 漂移，仍报告既有 archive FK、3 个 Text/AutoString 与 Telegram enabled index 差异 |
| OpenAPI/contracts | 通过 | `pnpm contracts:generate`、`pnpm contracts:check`；action DTO/paths 已生成 |
| harness / Compose | 通过 | `make harness-check`；本地及带占位 tunnel token 的生产 Compose `config -q` |
| `make check` / 双平台 export | 未运行 | 用户要求在文档更新后结束；不能用此前里程碑结果代替本次证据 |
| 真实 provider / IM 沙箱 / 双真机 | 未运行 | 发布门禁，不以 fake/export 代替 |

## 回滚与恢复

首选回滚是关闭 `INTERACTIVE_AGENT_EXTERNAL_ACTIONS_ENABLED` 并把 schema/policy 配回 v2/internal 或
v1/read-only；新 turn 不再注册 `send_reply`，已有 `GRANTED` 凭证因服务端 gate 立即失效。`EXECUTING`/
`UNCERTAIN` 不自动重放，按 ManualReplyDelivery 人工核对；`CONSUMED` 是外部已发生事实，不能用数据库回滚
伪造撤销。批准表和 API 采用 expand-only，跨兼容窗口后才考虑 contract。
