# P5 端上交互式 Agent、只读领域工具与本地会话

> 状态：active（S0-S5 代码切片完成，生产默认 v1/关闭） · Owner：Codex · 创建：2026-07-17 · 更新：2026-07-18

## 目标与用户价值

在不扩大模型外部动作权限、不把对话正文持久化到服务端、也不混用消息分析额度的前提下，让内部 beta
用户能在 RN App 中创建本地 Agent 会话，询问自己的商机，并由端上 pi harness 调用审查过的本地只读工具
搜索商机、读取详情和消息。会话在设备 SQLite 中可恢复、可清除、默认 30 天过期；模型调用通过短期 turn
token 访问受限推理网关，provider key 和真实 model 仍只在服务端。

本计划是[系统重构总计划](2026-07-17-agent-native-system-refactor.md) P5 的执行计划；两份 2026-07-16
提案仍是方向输入，代码、迁移、测试和当前文档是行为真相。

## 非目标

- 首个切片不开放 `draft_reply`、状态更新、claim、链接读取、通知、联系人、日历、记忆或任何外部发送。
- 不把 pi JSONL 直接当领域事件或同步契约；会话记录使用本计划定义的有界、版本化本地 entry。
- 不跨设备同步会话，不引入 E2EE、sqlite-vec、embedding、BYOK、动态 extension 或运行时下载代码。
- 不改变 `PI_AGENT_ANALYSIS` 的套餐语义；交互式调用使用独立 feature，未完成产品定价前仅内部 beta。
- 不删除 P4 分析 run、服务端 Celery fallback 或现有看板/详情/人工回复路径。

## 背景与当前行为

- RN 已包含 pi 0.80.6、Hermes SSE adapter、SQLite v5、设备会话与 capability；P4 单工具
  `submit_analysis` 分析与本计划的本地交互会话/三项只读工具分开，`agentToolsAvailable` 默认 false。
- SQLite 已有 owner-scoped Opportunity/Message 投影，可离线读取；读取逻辑通过严格 decoder 校验，但尚无
  面向 Agent 的有界查询 facade。
- P4 推理网关要求 AnalysisRun purpose token、两条消息和唯一 `submit_analysis` 工具，不能拿普通 JWT
  改造成通用聊天代理。
- `UsageFeature.INTERACTIVE_AGENT_TURN` 已与 `PI_AGENT_ANALYSIS` 分开；provider request 只做无正文成本
  审计，不混入顶层消息分析额度。
- 人工回复已有服务端幂等账本，但模型不能直接调用；外发审批凭证、args hash 和二次服务端钳制尚未建立。

## 产品、计量与隐私边界

- **发布范围**：`INTERACTIVE_AGENT_BETA_ENABLED=false` 默认关闭；仅 active、owner-bound、精确上报
  RN/SQLite/runtime/schema/streaming/tool schema 的 allowlisted 设备获得 `agentToolsAvailable=true`。
- **计量单位**：新增 `interactive_agent_turn` feature；一次用户提交形成一个顶层 turn reservation。turn
  完成后 consume，明确失败/过期 release；同一 owner/session/idempotency key 不重复分配。内部 beta 月额度
  由服务端配置，默认 0；正式套餐映射必须另行产品决策和 ADR 更新。
- **本地保留**：session/entry 只保存在 owner-scoped SQLite，默认保留 30 天；每 owner 最多 100 个会话，
  每会话最多 500 个 entry，单 entry JSON 不超过 64 KiB。用户可立即清除；登出/账号切换必须清除。
- **服务端最小化**：只保存 turn lease/status、owner/device、usage ledger、runtime/schema/model/policy 和
  provider token/cost/latency 审计；不保存 user/assistant 文本、tool args/result 或完整 provider 响应。
- **模型可见性**：provider 会看到本次用户输入、被选中的本地历史和只读工具结果；UI/隐私说明必须明确。
  server gateway 流经正文但不得记录或落库。
- **工具边界**：首批只注册 `search_opportunities`、`get_opportunity`、`get_messages`。工具参数严格 TypeBox
  校验、owner 从 session 注入而非模型提供、查询有上限；没有任意 SQL、HTTP、文件或秘密访问。
- **S5 内部工具**：第二个精确合约只增加 local-only `draft_reply`、queued `update_status` 和认证在线
  `claim_opportunity`。客户端上报最高支持 schema v2，服务端仍可签发 v1；生产默认保持 v1。没有
  `send_reply`，外发仍需 S6 批准凭证和服务端二次复核。

## 验收标准

- [x] SQLite v5 建立版本化 `agent_sessions` / `agent_entries`；owner 隔离、顺序唯一、JSON/长度/类型约束、
      30 天清理、容量上限、删除和登出清理均有真实文件型 SQLite 测试。
- [x] 只读工具 registry 只有三项；未知/未授权工具 fail closed，参数非法、跨 owner、归档/删除、上限与
      损坏投影路径有确定性测试，结果使用共享契约而非拼接未验证 JSON。
- [x] 新增独立 interactive turn/无正文 provider audit 与 Alembic expand migration；claim/heartbeat/
      complete/fail/expire、quota、owner/device/token/lease/幂等和并发唯一由 PostgreSQL 测试证明。
- [x] 交互网关只接受 turn token、固定 alias、受限消息历史和只读工具 schema；限制请求/响应、token、
      每 turn 模型请求数、并发、速率、超时并传播取消，不泄露真实 model/provider ID/错误正文。
- [x] RN host 只从 `src/agent/interactive/*` 引入 pi SDK；按 session capability 注入三项工具，执行前再次
      白名单校验。会话先持久化用户 entry，再 claim turn；流式中断可恢复或明确失败，不能伪造完成。
- [x] Agent Tab 仅在 capability true 时可进入；覆盖 loading/streaming/error/empty/cancel/delete、键盘与
      无障碍。流式瞬态使用局部状态/ref，已提交 entry 批量落库，避免每 token 写 SQLite 或重渲整棵树。
- [x] 所有开关默认关闭；OpenAPI/shared/RN/backend/harness、迁移往返、双平台 production export 通过；
      真实 provider/双真机仍需隔离门禁并明确记录“未运行”。

## 影响面与风险

- **domain/application**：新增 turn 状态机、计量 policy 和本地只读工具契约；分析 run 与交互 turn 不共用
  状态或额度。
- **database**：服务端只存无正文 turn/audit；RN SQLite 存通信数据。两个存储都要 owner 约束和清理路径。
- **API/auth**：claim 用 device-bound access token，网关用短期 purpose token；普通 JWT 不能直接调模型。
- **AI/隐私**：历史与工具结果会发给 provider；上下文必须显式选择、截断，日志/错误不得含正文。
- **RN/performance**：流式事件频率高；遵循局部订阅、批量持久化、避免 waterfall 和条件加载 pi host。
- **计费**：turn 顶层计一次；内部 provider 多轮只做成本审计。崩溃、重放、expire 不能双扣或漏 release。
- **未来外发**：本计划不实现批准凭证。任何 `send_reply` 工具进入 registry 前必须另建高风险切片。

## 实施步骤

- [x] S0：核对提案与现状，冻结内部 beta、独立 turn 计量、30 天本地保留、服务端无正文和只读工具边界。
- [x] S1：实现 SQLite v5 session/entry repository、容量/保留/清理，以及三个 owner-bound 只读工具和测试。
- [x] S2：新增 interactive turn 状态机、独立 usage feature、provider audit、迁移与设备 capability。
- [x] S3：实现受 turn token 约束的交互 SSE gateway、成本审计和 faux provider 边界测试。
- [x] S4：接入 RN pi host、恢复/取消和 Agent Tab；补共享 schema、i18n、无障碍与双平台 export。
- [x] S5：完成 draft、内部状态 outbox 与认领切片；仍不开放外部发送。
- [x] S6：独立高风险切片实现一次性批准凭证、args hash、服务端复核与唯一外部工具 `send_reply`；
      生产门禁仍关闭，真实 IM/真机证据留作发布前验证。
- [ ] S7：全量验证、文档、真实 provider/真机灰度与回滚演练。

## 进度日志

- 2026-07-17：开始 P5。审计确认现有 P4 gateway 不是通用聊天入口，`agentToolsAvailable` 固定 false，
  SQLite schema 为 v4 且没有会话表。先建立 S1 本地数据/只读工具边界；服务端 turn 与网关后续独立实现。
- 2026-07-17：完成 S1-S2。SQLite v5、应用定义 session/entry、30 天清理和三项 owner-bound 只读工具已
  落地；服务端新增独立 usage feature、turn/无正文 audit、purpose token、租约状态机、设备 capability、
  011 expand migration 和 PostgreSQL 并发/幂等/额度/撤销测试。
- 2026-07-17：完成 S3-S4 本地代码切片。交互 gateway 固定 alias/prompt/三工具并重写 provider 身份；
  RN pi host 按需加载、截断完整 turn/tool pair、流式局部更新、最终 entry 批量落库。Agent Tab 仅在
  capability true 时显示，并覆盖新建/切换/删除、空态、取消和失败；OpenAPI 悬空 discriminator mapping
  在生成检查中被发现并修正。
- 2026-07-18：完成 S5。共享/服务端/RN 以 schema v2 + `interactive-internal-v2` 精确增加本地草稿、
  status outbox 和认证 claim；v1 transcript 回退过滤、provider tool-call 白名单、UI 诚实状态卡片、部署
  成对校验和 OpenAPI 生成均已验证。生产默认继续 v1 且开关关闭。
- 2026-07-18：完成 S6 实现切片。schema v3 只增加 `send_reply`，RN 本次明确批准与编辑正文后，服务端以
  无正文 hash、短期 purpose token、资源版本和既有 ManualReplyDelivery 复核执行一次。生产默认仍为 v1，
  external-actions 与 IM send 关闭；真实 provider、授权 IM 沙箱和双真机仍是发布前门禁。

## 发现日志

- 2026-07-17：2026-07-16 RN 提案建议直接保存 pi JSONL，但总计划已修正为“与业务事件分表、分版本”。
  为避免 SDK 升级把本地/同步格式锁死，本计划保存应用定义的 entry envelope；pi message 只在 host 边界映射。
- 2026-07-17：已有 offline repository 能严格解码 owner-scoped 投影，首批工具应复用同一 decoder/查询语义，
  不为 Agent 新开任意 SQL 或在线 API 旁路。
- 2026-07-17：交互式 Agent 会在一个用户 turn 中产生多次 provider 请求；按 provider request 计用户额度会
  让工具循环改变产品价格，因此顶层 turn 计量与 request 成本审计必须分离。

## 决策日志

- 2026-07-17：首个公开边界选择“内部 beta + 默认 0 额度 + 设备白名单”，不猜测正式套餐价格；正式商业化
  只能通过显式 entitlement/ADR 更新，不允许生产静默无限调用。
- 2026-07-17：本地 transcript 默认 30 天、100 sessions/owner、500 entries/session；这是可回滚的 beta
  默认值，UI 提供立即删除。替代方案“永久保留”扩大通信数据风险，“仅内存”无法满足恢复与离线历史。
- 2026-07-17：先开放三个本地只读工具；draft、内部写、外发按权限单调增加。这样每个 capability 都有独立
  schema、拒绝测试和回滚面，模型不能借未实现审批链扩大权限。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 55 Markdown links、110 backend Python files、8 harness tests |
| SQLite v5/session/tools | 通过 | RN 全量 41 files / 136 tests；其中交互式目标 3 files / 8 tests |
| PostgreSQL turn/migration | 通过 | 011 fresh upgrade 与 011→010→011；turn/gateway 5 tests |
| faux interactive gateway | 通过 | 后端目标 6 tests；真实 pi tool loop + faux SSE 已纳入 RN 测试 |
| OpenAPI/shared/RN | 通过 | contracts、radar-agent、radar-api 58 tests、RN typecheck/test/dependency check |
| iOS/Android Hermes export | 通过 | 两个平台 production JS/Hermes bundle 均生成 |
| `make check` | 通过 | backend 218 passed/76 skipped；pi runner、Web、shared、RN 41 files/136 tests、双平台 export 全部通过 |
| S5 shared/backend/RN | 通过 | radar-agent 6；后端交互/设备目标 27；RN 43 files/145 tests；PostgreSQL 9 tests |
| S5 `make check` | 通过 | backend 221 passed/76 skipped；OpenAPI、pi runner、Web、shared、RN 与双平台 export 全部通过 |
| 真实 provider / 双真机 | 未运行 | 外部门禁，不用 fake/export 替代 |

## 回滚与恢复

首选回滚是关闭 `INTERACTIVE_AGENT_BETA_ENABLED`，服务端返回 `agentToolsAvailable=false`，RN 隐藏入口并
停止新 turn；本地 session 保留到用户删除或 30 天清理，不影响看板/同步/P4 分析。active turn 由 expire
释放独立 reservation。数据库与 SQLite 迁移均只做 expand；只有确认没有 beta 客户端依赖后才 downgrade，
provider audit 与已消费 ledger 不因回滚改写。

## 结果与剩余风险

S0-S6 代码切片与针对性自动化证据已完成，所有生产开关、额度和 allowlist 保持关闭，生产合约默认仍为
v1。P5 生产仍未退出：记忆、跨设备 session/E2EE、正式套餐，以及真实 provider、授权 IM 沙箱、双真机、
键盘与无障碍人工灰度均未交付；不得把 fake provider 或默认关闭的 v3 配置误报为生产外发已验收。
