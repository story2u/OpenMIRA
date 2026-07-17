# 信息胃口（Signal Appetite）Agent-native 过滤体验

> 状态：active · Owner：bruce / Codex · 创建：2026-07-18 · 更新：2026-07-18

## 目标与用户价值

把移动端消息过滤从传统条件配置升级为可教、可解释、可试跑、可撤销的“信息胃口”。用户通过
真实本地消息的左滑有效/右滑无效样本、自然语言和意图地图教 Pi 什么值得占用注意力；单次教学只
形成候选版本，经历史试跑或影子模式并由用户明确确认后才生效。

## 非目标

- 不把关键词、AND/OR、阈值或模型参数作为普通用户的主要编辑界面。
- 不把无效消息直接删除；严格隐私立即删除模式需二次确认，且不在默认路径开启。
- 不将 provider secret、模型私有推理过程或 IM raw payload 写入 App、事件或埋点。
- 不顺带删除 SwiftUI/Compose 遗留客户端，也不改变外部发送 Approval Gate。
- 不在本功能中推翻 ADR-0011：PostgreSQL 仍是既有消息/商机同步权威；新增偏好事件采用 additive、
  owner-scoped 的同步能力，并保留 capability 回退。

## 背景与当前行为

基线为 `998bb54c`（`codex/agent-native-rn-refactor`），工作分支为
`features/agent-native-filtering-experience`。必读输入为
[Agent-native 蓝图](2026-07-16-agent-native-architecture.md)、
[RN + Pi 重构方案](2026-07-16-rn-pi-agent-refactor.md)、
[系统重构总计划](2026-07-17-agent-native-system-refactor.md)、
[架构总览](../../architecture/overview.md)和[功能地图](../../product/feature-map.md)。

| 能力 | 当前实现 | 体验问题 | 本次方案 |
| --- | --- | --- | --- |
| RN 与导航 | Expo 57 / RN 0.86 / Hermes；Dashboard、Pi、Account 三个 Tab | 首页仍以商机列表和高级筛选为中心，信息胃口不可见 | 首页变为注意力控制台；增加教学、意图地图、安静区入口，保留商机访问 |
| 本地数据 | `expo-sqlite` v5；change inbox、消息/商机/设置投影、outbox、Agent session | 没有偏好事件、过滤决定、候选版本、教学与 shadow 投影 | SQLite v6+ 增加 owner-scoped 事件日志、投影和审计 fold；账号清理覆盖新表 |
| 多设备同步 | PostgreSQL `SyncChange` + cursor；RN 幂等投影，服务端仍为业务权威 | 不能同步教学样本与偏好版本；直接覆盖 JSON 会丢失并发意图 | 新增 append-only 偏好事件同步契约，以 event id 幂等、版本仲裁和 fold 合并 |
| Pi Agent | Hermes 内本地 Host；v1 只读、v2 内部动作、v3 审批发送；SQLite session | 无信息胃口工具；Agent 不能试跑、解释或提出候选偏好 | 扩展版本化 Tool Registry；capture/simulate/propose 免审批，apply 强制显式确认 |
| 消息与机会 | RN 商机看板、详情和消息分页；工作机会仍无 RN 独立页面 | 无跨消息教学入口、无过滤解释；工作机会不是一等移动入口 | 教学卡从已同步消息选样；详情可“教 Pi”；机会信息参与意图与主动学习 |
| 主题与本地化 | 集中 `theme.ts`；类型化 `zh-CN`/`en` catalog | 缺 Signal Appetite 语义色、Reduce Motion 与专属文案 | 扩展语义 token 与双语 catalog，所有生产文案走类型化资源 |
| 动效与无障碍 | 基础 RN 动画/Pressable 语义；未安装 Gesture Handler/Reanimated | 滑动无 UI-thread 手势、替代动作、键盘和减弱动态路径 | Reanimated + Gesture Handler + haptics；accessibility actions 与按钮完全等价 |

## 验收标准

- [ ] 左滑严格生成 positive、右滑严格生成 negative；跳过不改变稳定偏好，RTL 不反转产品语义。
- [ ] 卡片使用 owner-scoped、已同步且允许教学的真实消息；主动学习兼顾边界性、变化影响与来源多样性。
- [ ] 支持最近操作与连续 10 条撤销；单次样本只写事件，不直接修改 active preference。
- [ ] 教学 Session 可总结、提出候选、历史试跑并在用户确认后应用；重大变更可观察一天或放弃。
- [ ] 首页呈现当前信息胃口、四种投递统计、临时模式、教学与安静区入口。
- [ ] 意图地图可交互，包含核心/辅助/降低节点、融入式一天时间线、过滤成果、预览前后对比和自然语言编辑。
- [ ] 安静区默认保留可抽查副本；每条决定展示原因、证据、处理位置和置信度，但不展示私有推理。
- [ ] L0/L1 离线可运行；L2 不可用时边界消息进入 inbox/digest，绝不因云端失败 suppress。
- [ ] 偏好事件支持 owner 隔离、幂等、版本化、审计、回滚和多设备同步；旧客户端忽略未知能力仍可运行。
- [ ] 滑动不是唯一操作；VoiceOver/TalkBack、键盘、Reduce Motion、深色模式、大字体和小屏路径可用。
- [ ] 埋点不含消息正文；严格删除与 apply 均有明确确认边界。
- [ ] 单元、SQLite 真实文件、Agent faux provider、手势状态机、UI/Golden 与性能预算都有可复现证据；
  未完成真机 haptic/FPS 时明确标为未验收。

## 影响面与风险

- 移动端：SQLite migration/fold、同步、Agent tools、首页/教学/地图/安静区/解释、导航、主题、本地化。
- 共享包：`radar-core` 领域事件与纯策略、`radar-agent` schema/prompt、`radar-api` 同步/API 契约。
- 后端：owner-scoped 偏好事件 append/read、幂等/版本仲裁、同步 change；需要 additive Alembic migration。
- 安全：消息正文只存在既有本地投影；事件/统计/埋点保存 ID、标签和解释摘要，不记录 raw payload。
- 性能：教学拖动全部在 UI thread；地图布局预计算且节点上限 30；列表不在手势中重渲染。
- 发布：新 capability 默认关闭，可单独关闭同步或 Agent tools；旧 SQLite 表和旧客户端路径保留。

## 实施步骤

- [x] S1：定义 core 模型/事件/fold，SQLite migration、repository、账号清理与真实文件测试。
- [x] S2：实现主动学习、L0/L1 决策、历史试跑、版本/rollback/shadow/临时关注纯逻辑。
- [x] S3：增加偏好事件服务端持久化/同步契约、API client、owner/幂等/版本/迁移测试与 capability。
- [x] S4：扩展 Pi Agent tools、Host registry 与明确 apply 审批；补 faux provider/tool loop 测试。
- [x] S5：实现首次教学、卡片栈、左右滑 UI-thread 动效、haptic、原因、撤销和 Session 总结。
- [ ] S6：实现首页注意力控制台、意图地图、时间线、预览/Shadow、安静区和消息解释。
- [ ] S7：补自然语言入口、详情“教 Pi”、本地化、无障碍、Reduce Motion、键盘与无正文埋点。
- [ ] S8：Golden/性能/双平台构建与分层检查；更新架构、功能地图、命令、ADR/计划并归档。

## 进度日志

- 2026-07-18：完成需求、分支、文档、workspace、SQLite、sync、Agent Host、导航、页面、主题和本地化入口审计；
  从 `998bb54c` 创建功能分支。下一步先以失败测试锁定事件与 fold 不变量，再做 SQLite v6 纵向切片。
- 2026-07-18：完成 S1。`radar-core` 增加类型化事件与纯 fold；RN SQLite v6 增加 append-only event log、
  七类物化投影和账号清理，repository 在同一排他事务中写事件与投影，重复 event id 不重复计数。
- 2026-07-18：完成 S2 与 Pi 工具主体。主动学习限制来源/主题重复；L0/L1 离线边界不 suppress；教学可
  连续撤销 10 条，候选版本需先模拟再确认，支持 shadow、临时关注、schedule 候选和版本回滚。交互 Agent
  新增 v4 的 15 个严格工具，Host 对 apply 使用一次性内存确认；TS/Python 网关契约有跨运行时一致性测试。
- 2026-07-18：完成 S3。服务端新增 owner/device 绑定的 content-free append-only 事件流与独立 rollout；RN
  先 push 本地 pending 事件，再按独立 cursor 拉取多设备事件并复用 SQLite fold，重复 event id 内容一致才
  幂等、内容冲突拒绝。客户端当时上报 SQLite schema 6（教学 UI 状态加入后升至 7），旧客户端继续忽略关闭的 capability。
- 2026-07-18：完成教学卡片 Commit 3 切片。增加 SQLite v7 首次引导状态、真实教学入口和显式状态机；
  左滑 positive / 右滑 negative 的 UI-thread 手势、分段语义层、阈值 haptic、下一张上浮、按钮/读屏替代、
  Reduce Motion、跳过、单步/连续撤销和真实 Session 汇总均接入事件日志。原因补充与候选提案留在 Commit 4。
- 2026-07-18：完成 Commit 4。原因 Sheet 支持正负建议与自由文本，重复 capture 只更新同一 example 的原因、
  不重复 Session 计数；教学结束后生成候选、运行本地历史试跑并保持“尚未生效”，应用前展示独立确认层，
  确认后才写 `PreferenceApplied` 和带版本的过滤决定。

## 发现日志

- 当前移动数据库是 `expo-sqlite` schema v5，而不是提案中的 op-sqlite；沿用已验证实现，避免第二套数据库。
- 当前根 workspace 已包含 `frontend`、`mobile/radar`、`packages/*`，共享包与 RN 已在同一锁文件。
- `mobile/radar` 尚未依赖 Reanimated、Gesture Handler、SVG 或 Haptics，需要用 Expo 兼容版本增加依赖。
- 当前 Pi 工具契约最高 v3，唯一外部动作 `send_reply` 已有双层审批；信息胃口工具需新 schema，不能绕开原边界。
- RN 当前没有独立工作机会页面；本任务先让工作机会语义参与教学/地图，是否补全独立列表需以现有 API/模型审计决定。
- ADR-0011 明确现阶段消息/商机由 PostgreSQL 权威；需求中的“端侧业务真相”对新增信息胃口采用本地事件优先，
  但不借机迁移既有聚合权威。

## 决策日志

- 2026-07-18：选择 `expo-sqlite` + append-only `attention_events` + materialized projections；原因是当前运行时、
  迁移与恢复路径已经验证。替代方案 op-sqlite 会引入第二套原生数据库和迁移风险。
- 2026-07-18：把教学 capture 与稳定 preference apply 分成不同事件和工具；apply 只能消费明确的候选版本与
  用户确认 nonce，避免 Agent 自行提权。
- 2026-07-18：地图首版使用 `react-native-svg` + 确定性布局，不引入 Skia；30 节点预算内 SVG 更易提供
  accessibility 替代层，性能不满足时再用基准驱动 ADR。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 待运行 | 所有切片完成后及文档更新后运行 |
| `pnpm --dir packages/radar-core check` | 通过 | S2 后 4 files / 8 tests；typecheck 通过 |
| `pnpm --dir mobile/radar test -- src/storage/migrations.test.ts src/attention/signalAppetiteStore.test.ts` | 通过 | Vitest 实际运行移动端全量：45 files / 152 tests |
| `pnpm --dir mobile/radar typecheck` | 通过 | S1 类型检查通过 |
| `pnpm --dir packages/radar-agent check` | 通过 | 8/8，覆盖 v1-v4 工具隔离与参数走私 |
| `uv run --locked pytest -q tests/test_interactive_agent_domain.py tests/test_interactive_agent_gateway.py tests/test_interactive_agent_turn_route.py` | 通过 | 20 passed；含 TS/Python v4 契约一致性 |
| `pnpm --dir mobile/radar test` | 通过 | S2 后 48 files / 161 tests |
| `pnpm --dir mobile/radar typecheck` | 通过 | S2/v4 工具类型检查通过 |
| Signal Appetite sync targeted checks | 通过 | radar-api 14 files / 62 tests；RN 49 files / 163 tests；后端 19 tests |
| `pnpm --dir mobile/radar test` | 通过 | 教学卡片与原因/候选切片后 50 files / 167 tests |
| `pnpm --dir mobile/radar export:ios` / `export:android` | 通过 | Hermes bundle 成功；不等同真机 haptic/FPS 验收 |
| `make backend-check` | 待运行 | 服务端同步切片后 |
| 双平台 Hermes export / Release | 待运行 | 不能替代真机手势、haptic 和 FPS |
| 真机 VoiceOver/TalkBack/haptic/FPS | 待运行 | 需要连接授权设备；未运行前不得声明验收 |

## 回滚与恢复

客户端通过 capability 隐藏新入口并停止上传偏好事件，继续保留本地事件供恢复；稳定 preference 用
`PreferenceReverted` 指向上一版本，不删除审计历史。服务端新表/API/SyncChange aggregate 均 additive，
关闭 capability 后旧客户端继续消费原 schema；数据库 migration 不做破坏性回滚。严格隐私模式默认关闭。

## 结果与剩余风险

开发进行中。完成后记录真实交付、测试数量、Golden、真机/外部服务未验收项和发布门槛。
