# P4 端上 pi 分析运行与受限推理网关

> 状态：active · Owner：Codex · 创建：2026-07-17 · 更新：2026-07-17

## 目标与用户价值

让兼容且已授权的 RN 设备能够领取一次 owner-scoped 消息分析，在端上运行现有
`submit_analysis`-only pi harness，并通过不暴露 provider key 的服务端网关完成模型调用。设备失联、租约
过期或版本不兼容时，既有 Celery runner 仍能接管；一次顶层分析只预留和消费一次用户额度，结果只允许
一个执行者投影。

## 非目标

- 本计划不开放 Agent 对话、session、通用工具、外部发送或审批 UI；这些属于 P5。
- 不把 provider key、真实模型名或通用网络访问能力下发到 App。
- 不删除现有 Celery/pi runner，也不在真实双平台/provider 证据前开启 production
  `deviceAgentAvailable`。
- 不提前实现 P6 的端上业务权威、E2EE、sqlite-vec、动态 extension 或常驻节点。

## 背景与当前行为

服务端当前由 `ScheduleAgentAnalysisUseCase` 在入队前原子预留 `usage_ledger`，Celery 调用受限 Node
runner，成功后消费、最终失败后释放。RN 已能在 iOS Hermes 用 faux provider 执行结构化工具调用，并有
`expo/fetch` SSE/Abort spike，但 production capability 固定关闭，尚无可领取的 analysis run、短期 run
token、租约、网关成本记录或设备完成结果 API。

权威约束见 ADR-0013、总计划 P4、安全基线和订阅用量计划。P0 go/no-go 仍要求真实 gateway adapter、
Android Hermes 运行态、前后台/被杀恢复及同签名升级证据；因此本计划先建立默认关闭、可独立验证和可回滚
的纵向切片。

## 验收标准

- [x] active、owner-matched、带 `did` 且 capability/schema/runtime 满足要求的设备才能 claim；跨 owner、
      revoked/legacy device、非候选消息、额度耗尽和全局关闭均 fail closed。
- [x] 并发 claim 同一消息最多产生一个 active run 和一条 reserved ledger；重复 claim 返回同一有效 run，
      不重复扣额。
- [x] run token 短期、绑定 run/owner/device/purpose，不进入 URL、数据库明文、日志或响应以外的持久层；
      heartbeat、complete、fail/expire 校验 lease、状态和版本并保持幂等。
- [x] complete 只接受有界结构化 `submit_analysis` 结果；服务端重新执行 schema、policy 和
      `project_agent_result`，成功才 consume ledger；失败/过期 release，重复或迟到执行者不能二次投影。
- [x] 推理网关只接受有效 run token 和模型 alias，限制正文、token、并发、速率、超时并传播取消；不记录
      prompt/响应正文或 provider secret，provider 请求只记成本维度且不新增用户用量。
- [x] 链接 fetch 代理绑定 run token，并复用 `SafeLinkInspector` 的逐跳 SSRF、端口、大小、类型和超时
      约束；客户端没有任意 URL fetch 工具。
- [x] RN 持久化最小 run 状态，只注册 `submit_analysis`，支持重启恢复/续租/明确 fail；先 shadow，结果
      带 run/device/runtime/schema/model/policy/executedBy，且 production capability 默认关闭。
- [x] PostgreSQL 迁移可 fresh upgrade/downgrade/upgrade；API/shared/RN/backend/harness 与双平台
      production Release 检查通过；真实 provider/真机仍未运行并明确保留为生产门禁。

## 影响面与风险

- **domain/application**：新增 run 状态机、claim/lease/finalize 用例；不能让 API 层直接拼状态转换。
- **database/migration**：新增 analysis run/provider request 审计表及必要唯一/复合外键；用 PostgreSQL
  行锁和约束证明并发唯一，迁移只做 expand。
- **API/auth**：设备 claim 使用 device-bound JWT；网关使用目的受限 run token。owner、device、run、
  message、ledger 必须在同一查询/约束中绑定。
- **billing**：复用现有 `usage_ledger` 顶层计量；任何 enqueue、设备、fallback 竞态都不能双重 reserve、
  consume 或 release 已消费记录。
- **AI/隐私**：prompt、消息正文、网页正文、provider 响应和 key 均不得进日志；外部模型输出始终不可信。
- **worker/fallback**：观察期保留服务端 runner；租约回收必须可重复且不能与迟到 complete 同时生效。
- **RN/native**：Hermes stream、AppState、网络取消和 SQLite 恢复影响 production build，但 capability 默认
  关闭，核心看板/回复路径不依赖端上 Agent。
- **部署**：新增开关、alias 和 provider 限额必须默认关闭/fail closed；不改变现有 `PI_AGENT_ENABLED`
  fallback 行为。

## 实施步骤

- [x] S1：冻结 analysis run 状态机、DTO 与威胁边界；新增 PostgreSQL expand migration、repository 和
      claim/heartbeat/complete/fail/expire API，复用单条 usage ledger，并补 owner/并发/幂等测试。
- [x] S2：实现最小 FastAPI OpenAI-compatible streaming gateway、目的受限 run token、alias/限额/取消与
      provider request 成本审计；使用本地 SSE fake 覆盖成功/中断/超限/泄密边界。
- [x] S3：增加受 run token 约束的 SafeLinkInspector fetch 代理和有界契约测试。
- [x] S4：把共享 `radar-agent` harness 与真实 gateway adapter 接入 RN；SQLite 持久化 run、续租、恢复和
      fail，保持单工具与 capability 关闭。
- [x] S5：接入 shadow/fallback 仲裁、expire 扫描和观测字段；证明不双投影、不双扣额。
- [x] S5.5：补齐默认关闭的 primary 生产触发：shadow 与 primary 开关解耦；同步后先 claim-next；按
      owner/device 稳定哈希百分比或设备白名单选组；仅使用当前 runtime/schema/model/policy 的 shadow
      样本计算成功率、一致率和 P95 门槛。符合条件的近期设备获得 90 秒领取窗口，同一 Celery job 延迟
      兜底；消息锁、active run 与 ledger 锁共同阻止陈旧 worker 接管、标失败、释放或二次投影。
- [ ] S6：本地全量检查、双平台 production Release 及架构/功能/运维/总计划更新已完成；隔离 provider、
      双真机验收和按设备灰度仍待外部环境。只有全部 go/no-go 证据成立才逐设备开放 capability。

## 进度日志

- 2026-07-17：开始 P4。已确认 P3 本地恢复证据不等于 P0 的真实 gateway/Android 运行态门禁；先实现
  默认关闭的 S1 服务端分析运行闭环，不提前开放 production capability。
- 2026-07-17：完成 S1。新增 owner/device/message/ledger 复合约束、active run 唯一索引、purpose token、
  claim/heartbeat/complete/fail/expire 和单事务 consume/release/project；并发、重放、过期、nonce 与设备
  撤销均以 PostgreSQL 测试覆盖，rollout 默认 false。
- 2026-07-17：完成 S2。本地 FastAPI 网关只接受固定 alias、两条受限消息和单一 `submit_analysis`，
  服务端强制真实 model/key、`store=false`、tool choice、字节/token/超时/并发/每 run 请求上限；SSE
  替换 provider identity，非 2xx/流错误不回显正文，取消关闭上游。provider audit 不存正文，只记录
  request ID hash、token、延迟和估算成本，且不新增用户 usage。真实 provider 与 RN adapter 仍未运行。
- 2026-07-17：完成 S3。新增 run-token-only 链接代理，请求体不能携带 URL；服务端从绑定 Message 派生
  候选，逐跳复用 SafeLinkInspector，网络返回后再次锁 run/version/lease，并缓存有界证据。带链接 run
  缺证据时拒绝 complete，确定性 suspicious 不能被模型降级。
- 2026-07-17：完成 S4。RN 自定义 stream adapter 通过 `expo/fetch` 消费受限 Chat Completions SSE，真实
  pi Agent 只注册共享 `submit_analysis` 并拒绝多轮、额外工具、prose 和 provider 身份。SQLite v4 保存
  最小恢复状态/有界 input，短期 token 只进 device-only SecureStore；模型流周期续租，退后台 abort 并
  保留 run，前台/网络恢复重试，三次失败后明确 fail。production capability 仍由默认 false rollout 控制。
- 2026-07-17：完成 S5。`claim-shadow` 只选择 server-completed 且未被 run 绑定的 consumed ledger，同 owner
  并发设备只会领取一次；shadow complete 使用相同分析时间比较服务端钳制投影，只保存 match/difference，
  不改 Message/Opportunity/usage。primary fail/expire 先提交 release，再用稳定键调度现有 Celery fallback；
  beat 周期扫描失联 lease。Message 另存无正文 server/device 执行来源；RN 每次恢复最多领取一条 shadow。
  shadow/fallback/rollout/gateway 仍全部默认关闭。
- 2026-07-17：完成 S6 的本地验证部分。production 变体 clean prebuild 后，iOS 无签名 simulator Release 与
  Android `assembleRelease` 均成功；两个产物均为 `com.codeiy.im`、`0.2.0 (2)`，并包含生产 Hermes bundle。
  隔离真实 provider、双真机 kill/reopen、同签名升级与逐设备灰度未运行，因此所有生产开关继续关闭。
- 2026-07-17：完成 S5.5。自动/手工分析调度先建立同一条 message reservation；只有 shadow 证据达到
  版本绑定的样本量、成功率、一致率和 P95 阈值，且近期兼容设备进入稳定 cohort 时，服务端才把同一
  Celery fallback 延迟一个领取窗口。RN 在同步/推送 cursor 收敛后先 `claim-next` primary，再尝试一条
  shadow。`usage_ledger` 增加同消息 active reservation 唯一约束；陈旧或最终失败的 server worker 只要
  发现 active device run 就不能接管、标失败或 release。管理员可从无正文的
  `GET /api/v1/agent/runs/rollout-readiness` 查看门槛证据。全部开关仍默认 false。

## 发现日志

- 2026-07-17：现有 `usage_ledger` 已具备 owner、source message、稳定幂等键和 reserved/consumed/
  released 状态，设备 run 必须复用这条顶层计量，不能另建第二套用户额度。
- 2026-07-17：当前自动/手工分析在调度时立即入 Celery。设备优先需要新的仲裁入口；在 shadow/fallback
  策略完成前，S1 API 只能由独立 rollout 开关保护，不能抢走生产任务。
- 2026-07-17：pi 0.80.6 首轮请求是两条 system/user 消息、`stream=true` 与单一 function tool；网关
  无需开放通用 Chat Completions。服务端重建 tool choice/model/store 等敏感参数，仅保留有界用户 prompt
  与共享 tool schema，减少设备扩大权限的空间。

## 决策日志

- 2026-07-17：analysis run 采用服务端 PostgreSQL 作为租约和计量权威，run token 仅作短期能力凭证；
  原因是设备进程不可常驻且 JWT 本身不能解决竞态。替代方案“App 普通 JWT 直接调用模型”会泄露权限和
  绕过额度，拒绝采用。
- 2026-07-17：provider request 单独建无正文审计，而不复用用户 ledger。provider stream ID 只存 SHA-256，
  真实 model 只在服务端成本维度，设备看到 alias 与网关生成 completion ID；这样既能观测成本，又不会
  把 provider 身份或内部错误变成客户端契约。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 以本计划最终文档状态复跑；Markdown 链接、Python 结构和 8 个 harness 单测通过 |
| PostgreSQL migration + 并发状态机 | 通过 | PostgreSQL 16 fresh upgrade → 010、downgrade 010 → 009、re-upgrade；17 个 AnalysisRun 集成测试覆盖 primary claim-next、版本绑定 readiness、shadow 并发唯一、陈旧 worker/ledger 保护、fallback release→reserve；010 无新增 model drift，仍只有仓库既有 FK/Text/index 漂移 |
| `make backend-check` | 通过 | 临时 PostgreSQL 下 272 passed；compileall/ruff 通过；1 条上游 Starlette/httpx 弃用 warning |
| `make contracts-check shared-check` | 通过 | 由 `make rn-check` 复跑，OpenAPI 无漂移；radar-core 1 test、radar-api 12 files/54 tests、radar-agent 共享契约通过 |
| `make rn-check` | 通过 | 38 files/126 tests；类型、Expo compatibility、OpenAPI/shared checks 与 iOS/Android Hermes production export 通过 |
| 双平台 production Release | 通过 | clean prebuild 后 iOS simulator `xcodebuild` exit 0，118 MB App 含 `main.jsbundle`；Android 905 tasks，`BUILD SUCCESSFUL in 4m 52s`，104.1 MB APK 含 Hermes bundle；两端均为 `com.codeiy.im`、`0.2.0 (2)` |
| 真实 provider / 双真机 | 未运行 | 外部门禁；不得由 fake 或构建替代 |

## 回滚与恢复

S1-S5 只做 expand。紧急回滚先关闭 device-agent claim/gateway/fallback 仲裁开关，保留 run、ledger 与成本
审计记录，让现有 Celery runner 继续执行。只有确认没有客户端租约、active run 或审计依赖后才能 downgrade
删除新表；已消费 ledger 不得因回滚改写，reserved run 由可重复 expire 流程释放。

## 结果与剩余风险

尚未完成。S1-S5.5 已完成服务端运行、计量、受限 SSE/链接代理、RN adapter/恢复，以及默认关闭的
shadow、版本绑定 readiness、稳定设备 cohort、primary 领取窗口和 Celery fallback 仲裁；S6 的本地全量
检查、迁移往返与双平台完整 Release 已通过。真实 provider、双真机 kill/reopen、同签名升级和真实流量
灰度阈值仍待外部环境完成，production rollout、shadow、gateway 和 fallback 必须保持 false。
