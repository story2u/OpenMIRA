# OpenMIRA · Mira 消息管家体验重构（执行计划）

> 状态：**active · 代码侧完成，待真机验收** · Owner：bruce / Claude · 创建：2026-07-18
> 分支：`features/mira-ai-message-concierge-redesign`（基线 `release/v3.0.0`）
> **§3 裁决（2026-07-18，bruce）：选 A——沿用现有栈（expo-sqlite / 内建 store / theme.ts / svg）**；范围 W1-W5、节奏 S2→S5 按本计划执行。
> 方向输入：任务书"Mira AI 消息管家重构" + 仓库现状审计
> 上游依据：[Agent-native 蓝图](2026-07-16-agent-native-architecture.md)、[RN+Pi 重构方案](2026-07-16-rn-pi-agent-refactor.md)、[系统重构总计划](2026-07-17-agent-native-system-refactor.md)、[信息胃口（已完成）](../completed/2026-07-18-signal-appetite.md)

## 0. 这份计划为什么先于代码

任务书假设当前是"堆积商机列表的传统工具型 App"。**审计证明这个前提已过时**：`release/v3.0.0` 里，任务书要求的"教学、意图地图、安静区、AgentHost、Approval Gate、事件溯源、信息胃口"**已经由 `signal-appetite`（2026-07-18 completed）实现**。若照任务书从零"重构"，会重复建设并破坏已上线能力。

因此本任务的真实内容不是"重构一个传统 App"，而是**在已成熟的 Agent-native 基座上，补齐 5 块尚缺的能力，并完成品牌与信息架构的收口**。用户已确认三点：① 只做真实差距；② 交付先出本计划文档；③ 技术栈——**此处有一个必须先解决的矛盾（见 §3）**。

---

## 1. 现状审计表

| 能力 | 当前实现 | 当前体验问题 | 本次处理 |
| --- | --- | --- | --- |
| 底部导航 | `home / dashboard / agent / account` 四 Tab（`app/(tabs)/_layout.tsx`）；agent 受 capability 门控 | 命名仍是"商机/Pi/设置"系统语言；设置是一级 Tab | 改文案为 今天/消息/Mira；account 降为头像入口（保留路由兼容） |
| 首页 | `HomeScreen.tsx` 已是注意力控制台：意图地图 + 四投递统计（immediate/inbox/digest/suppress）+ 教学入口 + 安静区 + shadow 横幅 | 缺"今天只需看 N 条"当日结论、缺时间段简报、缺 Mira 状态头部/一句话 | **补** Mira 状态头 + 今日 Hero + 一句话 + 简报时间线（复用现有胃口/投递数据） |
| 左右滑教学 | `features/teaching/`（TeachingCardDeck、teaching-machine、reason sheet、useTeachingSession）+ `attention/teachingService` | **已达任务书要求**（左有效/右无效、原因、撤销、总结、试跑） | 不重写；仅接品牌文案/视觉 token（§7） |
| 意图地图 | `features/intent-map/`（IntentMapCanvas、AttentionTimeline、model、repository） | **已达要求**（核心/辅助/降低节点+时间线+过滤成果+预览对比+自然语言编辑） | 不重写；接视觉 token |
| 安静区 | `features/quiet-zone/`（Screen、model、repository） | **已达要求**（分类抽查+反馈） | 不重写；接品牌文案 |
| Pi Agent | `src/agent/interactive/`（host、sessionStore、gatewayStream、appetiteTools、internalTools、approvedSend）v1只读/v2内部/v3审批发送 | Tool Registry 无 briefing/snapshot 工具 | **新增** 简报类工具（§5），不改审批架构 |
| 本地数据 | `expo-sqlite`，20+ 表事件溯源（attention_events、preference_examples、message_filter_decisions、shadow_evaluations、temporary_focuses…）+ change_inbox/outbox/projection 同步 | **无 Briefing / AttentionSnapshot / QuietItemSummary 结构化模型** | **新增** 3 类模型 + 对应事件（§4），走既有 migration/fold/sync 范式 |
| 商机/工作机会 | `OpportunityDetailScreen.tsx`（35KB 单文件，共用）+ dashboard 看板 | 商业与工作机会共用详情结构；详情可能暴露内部字段 | **拆** 商业/工作两模板 + Mira 结论优先 + 内部字段移入 debug（§6） |
| 品牌 | 无 OpenMIRA/Mira 人格、无 MiraOrb | AI 未成为产品人格 | **新增** 品牌 + MiraOrb 状态元素 + 文案收口（§7） |
| 契约 | `packages/radar-contracts`（analysis-runs/opportunities/messages/signal-appetite-sync…） | 无 briefing 契约 | **新增** briefing 契约条目 |
| i18n | `src/i18n/catalog.ts` 类型化 zh-CN/en | 新文案未入 catalog | 全部新文案入 catalog，无硬编码 |
| feature flag | 存在 capability 门控（`capabilities.agentToolsAvailable`）+ shadow 模式 | 无本次重构总开关 | 新增 `MIRA_CONCIERGE_UI_ENABLED`（§8） |

**结论**：任务书 27 条"完成标准"里，约 15 条已由现状满足；真实待建 = **Briefing 体系 / 今日 Hero+Snapshot / 导航与机会收口 / 详情双模板 / 品牌**。

---

## 2. 只做真实差距：五个工作块

| 块 | 内容 | 依赖 | 对应任务书 commit |
| --- | --- | --- | --- |
| **W1 简报体系** | Briefing/BriefingItem/AttentionSnapshot/QuietItemSummary 数据模型 + 事件 + fold；`summarize_time_window` 等 Pi 工具；L0/L1 本地生成 + L2 语言整理；增量不重复 | 现有事件日志/同步/AgentHost | C1、C2 |
| **W2 今天首页 + 导航** | Mira 状态头 + 今日 Hero + 一句话 + 简报时间线；底部导航换名（今天/消息/Mira）；设置降为头像入口；深链别名兼容 | W1 | C3、C4 |
| **W3 消息智能视图 + 机会统一** | dashboard 升级为"你想深入了解哪类"智能视图；商机+工作机会统一为"机会"入口 + 内部细分；列表页保留上下文头部 | W2 | C5 |
| **W4 详情双模板** | 商业机会 / 工作机会分离模板；Mira 结论+相关+风险优先；内部字段（resolvedInfo/riskReasons/verifiedAt/英文原文）移入 debug inspector；工作机会空状态对话建目标 | W3 | C6、C12(部分) |
| **W5 品牌 + 视觉收口** | OpenMIRA/Mira 人格；MiraOrb 状态元素；语义色/动效/无障碍/本地化 token 落到新页面；通知简报化；golden 截图 | W1-W4 | C7-C13 |

W2/W3/W5 的教学·意图地图·安静区**已存在**，只做品牌与视觉接线，不重写。

---

## 3. ⚠ 必须你定夺的矛盾：技术栈

你在选项里选了"按任务书换新栈（op-sqlite/zustand/nativewind/skia/flashlist）"。**但这与你同时选的"只做真实差距"和架构基线第 11 条（不破坏现有同步）直接冲突**：

- 现状 6 个 feature + 20+ 张表的事件溯源/同步，全部构建在 `expo-sqlite` + RN 内建 store + `theme.ts` 之上。
- 换 op-sqlite = 重写整个存储层与 migration；换 zustand = 重写 6 个 feature 的状态；换 nativewind = 重写全部样式。这不是"差距"，是把已上线的 signal-appetite **推倒重来**。
- `system-refactor` 计划已用 ADR 明确记录：**首选 expo-sqlite 以减少 op-sqlite+sqlite-vec 原生依赖**（该文档"对两份提案的必要修正"节）。任务书的栈假设正是被这条 ADR 否决过的。

**我的建议（务实、可维护、不返工）**：沿用现有栈（expo-sqlite / 内建 store / theme.ts / react-native-svg）；仅 **MiraOrb 与意图地图**若经性能基准证明 svg 不够，才按 ADR 引入 Skia 局部使用（对齐 RN 方案原文"Skia 仅用于意图地图或 Mira Orb"）。这样新旧一致、不碰已完成工作、符合基线第 11 条。

> **需要你的裁决**：
> - 选 A（**推荐**）：沿用现有栈，本计划照此推进；"换新栈"视为误选。
> - 选 B：坚持换栈——则本任务升级为"数据/状态层大迁移"，须先单独立 ADR 推翻既有决策、评估 signal-appetite 返工面，W1-W5 全部顺延。我会据此重写本计划的工作量与风险。
>
> 在你回复前，**我不写任何代码**。默认按 A 起草了下方分阶段。

---

## 4. W1 数据模型与事件（新增，走既有范式）

SQLite migration v7+（现状最高版承接），全部 owner-scoped、幂等、可 fold、可同步、带 schema version：

- 表：`briefings`、`briefing_items`、`attention_snapshots`、`quiet_item_summaries`（字段严格按任务书 §17）。
- 事件（按任务书 §18，进 `attention_events` 或新 `briefing_events`，二选一在设计时定）：`BriefingScheduled/GenerationStarted/Generated/Opened/ItemHandled/Dismissed`、`AttentionSnapshotUpdated`、`QuietItemAdded/Restored`、`MessageDecisionExplained`、`TemporaryFocusCreated/Expired`、`MiraInputRequested/Resolved`（教学/偏好类事件**已存在**，不重复建）。
- 原则：Briefing/Snapshot 由**事件 fold 或结构化查询**生成，模型文本只做语言整理不作唯一数据源（任务书 §17/§20 硬约束）。

## 5. W1 Pi Agent 工具（新增到 Tool Registry）

按任务书 §19，TypeBox schema，只读/计算类免审批，写操作经 Application Service，apply 类需用户确认，外部动作仍走 Approval Gate：
`summarize_time_window`、`get_attention_snapshot`、`list_priority_items`、`list_category_items`、`explain_message_decision`、`get_quiet_summary`、`list_quiet_samples`、`update_brief_schedule`、`create_temporary_focus`。
（`start_teaching_session`/`capture_preference_example`/`simulate_preference_change`/`apply_preference_change`/`get_intent_map`/`propose_intent_map_change` **已存在于 appetiteTools**，复用。）

## 6. W4 详情双模板要点

- `BusinessOpportunityDetail` / `JobOpportunityDetail` 分离；共用 `MiraConclusionCard/KeyFactsSection/RelevanceSection/RiskSection/SourceEvidenceSection`。
- 内部字段（resolvedInfo/riskReasons/verifiedAt/recruiting_signal/英文原文/raw JSON）**仅** debug inspector 可见（任务书 §11 硬约束）。
- 工作机会详情不显示回复框，显示"打开投递链接/保存/不感兴趣/查看来源/求职准备建议"。
- 工作机会空状态：对话建结构化求职目标（§12）。

## 7. W5 视觉/品牌

严格按任务书 §15 语义色（清透蓝主色、暖琥珀=立即、翠绿=有效、靛蓝=摘要、石板灰=收起、玫瑰灰=无效）落到 `theme.ts` token；MiraOrb（2-3 层柔和圆环，6 状态，无机器人脸）；动效按 §16（Reanimated，Reduce Motion 全支持）；文案按 §一〜§十四 全量入 i18n catalog。

## 8. 迁移与回滚

- feature flag `MIRA_CONCIERGE_UI_ENABLED`：dev 默认开 → 内测灰度 → 新用户默认 → 老用户可切经典 → 稳定后移除（任务书 §23）。
- 旧路由（`opportunity/[id]`、`dashboard`、`settings/*`）保留 + 别名/重定向；旧数据继续可用。
- 每 commit 可构建、有数据闭环、无真实消息/secret、不破坏同步、补测试或标注未验证。

---

## 9. 分阶段交付（默认按栈选项 A）

> **进度（2026-07-18）**：
> - [x] S1 计划 + 裁决（栈 A）
> - [x] S2a `ad139955`：radar-core briefing 域（model/events/compose/fold，20 测试）+ migration v8
>   （briefing_events/briefings/briefing_items/briefing_schedules；snapshot 与 quiet summary 为派生数据
>   不建表，由事件 fold/现算——对任务书 §17 的存储决策）+ briefingStore/briefingService（增量窗口接续、
>   L2 降级不清空、事件-投影同事务、已处理状态跨简报撤销保留）
> - [x] S2b `eb1fa6aa`：interactive 契约 v5（6 简报工具，客户端先行休眠）+ 端上执行器 + host 分发 +
>   i18n 标签；radar-agent 11 测试、mobile 186 测试全绿
>   - 契约收紧：`summarize_time_window` 不接受任意窗口（增量语义自动推导，防重叠简报）
> - [x] S2c-a（2026-07-20，本轮未提交）：后端 v5 Python 镜像（`interactive_agent.py` 版本映射 +
>   `interactive_agent_gateway.py` Briefing system prompt / 6 工具 schema / 参数校验 + TS/Python
>   golden 对齐）+ RN 设备注册上报 `agent.interactiveSchema=5`
> - [x] S2c-b（2026-07-20，本轮未提交）：本地简报调度触发（到点生成、同日同一时段去重、错过时段
>   可补生成）+ L2 summarizer 从调度入口透传；失败继续降级为本地结构化简报
> - [x] S3a（2026-07-20，本轮未提交）：底部导航改为“今天 / 消息 / Mira”，Account 降为首页头像入口；
>   Today 首屏接入本地 AttentionSnapshot 与 Briefing 列表，新增 Mira 状态头、Hero、Mira 一句话、
>   时间段简报和 Reduce Motion 友好的 Mira Orb
> - [x] S3b-S5（2026-07-20，本轮未提交）：消息 Tab 默认 Mira 智能分类，旧看板列表通过分类/flag 兼容；
>   `opportunityType=business|job` 打通后端 DTO、OpenAPI、严格前端 decoder、离线 projection 兼容与
>   Agent 精简读工具；详情页按商业/职位模板分流，职位不显示回复框；通知改为简报式 copy，不暴露正文/
>   原始链接/debug；新增 `EXPO_PUBLIC_MIRA_CONCIERGE_UI_ENABLED` 回滚开关
> - [ ] 真机/视觉验收：golden 截图、haptic/FPS、VoiceOver/TalkBack、大字体、真实推送/同步通道仍需用户真机验证

| 阶段 | 交付物 | 验收判据 | 风险 |
| --- | --- | --- | --- |
| **S1** | 本计划 + 你对 §3 的裁决 | 你确认栈与范围 | — |
| **S2 (C1-2)** | W1 数据模型+事件+fold+Pi 工具+本地简报生成 | 单测：增量不重复/晚间排除已处理/云失败不清空/L2 缺失降级；SQLite 真实文件测试 | 简报增量去重逻辑复杂 |
| **S3 (C3-4)** | W2 今天首页+导航换名+深链兼容 | 首页 5 秒答 6 问；旧深链可达；golden：今天/无重要/三时段 | 与现有 HomeScreen 合并 |
| **S4 (C5-6)** | W3 消息智能视图+机会统一；W4 详情双模板 | 详情不露内部字段；商业/工作模板分流；工作空状态对话建目标 | OpportunityDetail 35KB 拆分 |
| **S5 (C7-13)** | W5 品牌+MiraOrb+视觉+通知简报化+无障碍+i18n+golden+flag | 深色/大字体/Reduce Motion/VoiceOver；14 张 golden；flag 可回滚 | 真机 haptic/FPS 未验证需标注 |

**真机验证限制**：本会话环境可跑 tsc/jest/expo-sqlite 文件测试，但**无法验证真机手势 FPS、haptic、golden 视觉**——这些按仓库纪律明确标"未验收"，交 CI/真机。

## 10. 待你确认清单（回一句即可开工）

1. §3 技术栈：**A（沿用现有栈，推荐）** 还是 B（换新栈，须先立 ADR 大迁移）？
2. 范围：确认"只做 W1-W5 五块、不重写已有教学/意图地图/安静区"？
3. 节奏：确认按 S2→S5 逐阶段、每阶段可回退？
