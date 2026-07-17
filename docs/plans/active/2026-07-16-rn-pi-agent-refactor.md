# React Native + Pi Agent 移动端重构方案

> 状态：**提案待评审** · 起草：2026-07-16 · 起草者：bruce + Claude
> 前置文档：[Agent-native 产品架构蓝图](2026-07-16-agent-native-architecture.md)（本方案是其**客户端执行形态**的落地设计，并取代蓝图 §4.3 的"原生 App 内嵌 JS 引擎"路线——RN 本身就是 Hermes 宿主，桥接问题消解）
> 本文未动任何生产代码；采纳后按 §5 路线图拆执行计划。

## 0. 三个先说清楚的事实

1. **迁移的不是 4 万行，是 4 千行。** 现有 iOS（SwiftUI ~20 文件）/ Android（Compose ~25 文件）是自述"瘦客户端"的 P0 实现：屏幕 + ViewModel + 一层薄网络。真正沉淀的资产全在 TS 侧——`frontend/lib/types.ts + api.ts + sop.ts`（DTO/语义）、`backend/pi-agent-runtime`（agent harness）、跨端 golden 夹具测试传统。**RN 重构对本仓库是顺势，不是逆势。**
2. **Pi 的包已为嵌入做好了分层**（对本地安装的 0.80.6 逐文件核实）：`pi-agent-core` 主入口零 `node:` 依赖（Node 专用代码隔离在 `./node` 子入口），session 存储自带 `memory-repo` 内存实现，harness（compaction/skills/system-prompt）齐全；`pi-ai` 按 `./providers/*` 子路径导出，走推理网关（OpenAI 兼容）后只需一条 provider 路径，Metro package exports 可摇掉 AWS/proxy 等 Node 味依赖。**Hermes 直跑没有结构性障碍。**
3. **模型 key 永不进 app**（审计确认现状 key 是服务端共享秘密）：默认走推理网关（LiteLLM 代理 + 计量配额），BYOK 是高级用户显式选项（Keychain/Keystore 存储）。

---

## 1. 整体架构设计

### 1.1 Monorepo（pnpm workspaces，仓库已用 pnpm）

```
IM/
├── pnpm-workspace.yaml
├── backend/                       # 不动（含 pi-agent-runtime 迁出前的原位）
├── frontend/                      # Next.js Web（降级为控制台，见蓝图）
├── mobile/
│   ├── ios/                       # 遗留 SwiftUI —— 冻结，仅安全修复
│   ├── android/                   # 遗留 Compose —— 冻结，仅安全修复
│   └── radar/                     # ★ 新 RN App（Expo dev client）
│       ├── app/                   # expo-router 路由
│       │   ├── (tabs)/dashboard.tsx
│       │   ├── (tabs)/agent.tsx          # Agent 对话主界面
│       │   ├── (tabs)/settings/
│       │   └── opportunity/[id].tsx
│       ├── src/
│       │   ├── features/          # dashboard / opportunity / settings / agent-chat
│       │   ├── agent/             # AgentHost、tools、approval、session 持久化
│       │   ├── data/              # SQLite 事件日志、fold、outbox、sync
│       │   └── native/            # Expo Modules 的 TS 门面
│       ├── modules/               # 自定义原生模块（Expo Modules API）
│       │   └── radar-triage/      # 端侧小模型分诊（Swift/Kotlin，见 §2.3）
│       └── app.config.ts
└── packages/
    ├── radar-core/                # ★ 领域层：DTO/枚举/sop/trust/状态机/事件类型
    │                              #   （从 frontend/lib 提升，Web 与 RN 共享）
    ├── radar-api/                 # ★ API client + zod/typebox 校验（三端同源）
    └── radar-agent/               # ★ agent harness 包装（从 backend/pi-agent-runtime 提升：
                                   #   AnalysisSchema、system prompt、投影钳制的 TS 镜像、
                                   #   AgentHost 抽象；backend runner 与 RN 共同消费）
```

```yaml
# pnpm-workspace.yaml
packages:
  - frontend
  - mobile/radar
  - packages/*
  - backend/pi-agent-runtime   # 过渡期原位保留，逐步被 packages/radar-agent 吸收
```

**为什么 monorepo 而不是独立 RN 仓**：三个 `packages/*` 是本次重构真正的复用资产，且 backend runner（服务端 fallback）与 RN 必须消费**同一份** AnalysisSchema/钳制逻辑/golden 夹具——分仓等于自愿回到"三端手抄 DTO"的旧世界。

### 1.2 RN 形态：Expo（dev client + prebuild），不是 bare RN

- RN 0.8x 最新稳定 + New Architecture（0.76 起默认）+ Hermes。
- Expo SDK（≥53）dev client 模式：原生目录由 `prebuild` 生成、config plugins 管理，**不用 Expo Go**（我们有自定义原生模块）。
- 收益：expo-router / expo-notifications / expo-secure-store / expo-sqlite / **expo/fetch（流式 fetch，SSE 关键依赖）** / EAS 或本地 fastlane 双轨构建；RN 官方文档自 2024 起把 Expo 列为默认推荐框架。

### 1.3 Pi 集成位置：**SDK 直连 in-process，不是 RPC**

| 方案 | 判定 | 理由 |
| --- | --- | --- |
| `pi --mode rpc` 子进程 | **否** | iOS 禁止 spawn 子进程；Android 可行但等于自带 Node 运行时，包体 +30MB 起 |
| RPC 到服务端跑 agent | **否**（仅作 fallback 保留） | 违背 agent-native 蓝图核心论点（harness/记忆在端上）；断网即失智 |
| **`pi-agent-core` + `pi-ai` 直接进 Hermes bundle** | **✓** | 主入口零 node: 依赖（已核实）；agent loop 本质是 async TS + 流式 fetch；`memory-repo` + 自持久化即 session 存储；与 backend runner 同库同版，golden 夹具双向约束 |
| `pi-coding-agent` 整包 | **选择性取用** | 它是面向开发机的上层封装（read/bash/edit 等文件系统工具、TUI、进程级 extensions 加载）。bash 在手机上无意义且危险；我们取其 **session JSONL 格式与 extension 形状**，工具全部换成领域工具（§3.3） |

### 1.4 状态、导航、主题

| 关切 | 选型 | 理由 |
| --- | --- | --- |
| 服务器/同步状态 | **SQLite 事件日志 = 唯一真相**（op-sqlite）+ TanStack Query 做网关请求缓存 | 对齐蓝图 P1"端侧真相"；查询=fold(事件) |
| UI/会话状态 | **Zustand**（+ immer） | agent 流式 token、审批队列这类高频小状态，Redux 仪式感过重 |
| 导航 | **expo-router**（react-navigation 7 之上） | 文件路由 + 类型化 links；deep link `radar://opportunity/[id]` 声明式继承现有清单 |
| 主题 | **NativeWind 4** | Web 端已是 Tailwind——`AppColors` 语义色 token（三端已各写一份）收敛为一份 `tailwind.config` 预设，Web/RN 共享 |
| 列表 | FlashList | 看板卡片流性能 |

---

## 2. 原生代码复用策略

### 2.1 诚实的复用清单

对现有 Swift/Kotlin 逐模块判定（复用机制见 2.2）：

| 现有模块 | 处置 | 方式 |
| --- | --- | --- |
| SwiftUI/Compose 全部屏幕与 ViewModel | **不迁移**（RN 重写 UI 是定义本身） | 其信息结构/语义色/文案已是规格，照抄进 RN 组件 |
| `TokenStore`（Keychain / EncryptedSharedPreferences） | **弃自研，换库** | expo-secure-store（够用）或 react-native-keychain（需生物识别时）。自研仅 ~50 行/端，包装成本高于重写 |
| `core/billing`（RevenueCat 双端） | **换官方 RN SDK** | react-native-purchases + 官方 config plugin，语义一一对应 |
| `ApiClient/RadarApi` + DTO 镜像 | **不迁移** | 被 `packages/radar-api`（TS 同源）取代——这正是消灭"三端手抄 DTO"的机会 |
| 推送（未落地） | RN 侧新建 | expo-notifications（APNs/FCM 统一） |
| **未来：MTProto 监听、端侧分诊模型** | **原生新写，Turbo/Expo Module 暴露** | 这才是"原生该干的活"（§2.3） |

> 结论（ponytail）：这个 app 值得桥接的存量原生代码≈0——瘦客户端的价值在 TS 域层，而它已经存在。"最大程度复用"的正确对象是 **DTO 语义、golden 夹具、设计 token、业务规则**，不是 Swift/Kotlin 文件本身。桥接机制真正的用武之地是**新增的**平台能力。

### 2.2 桥接机制（New Architecture 时代）

优先级从上到下：

1. **Expo Modules API**（Swift/Kotlin DSL）——App 级模块默认选择：声明式、自动生成 JSI 绑定、支持事件与 Promise，样板最少。
2. **Nitro Modules / 手写 Turbo Module（codegen）**——仅热路径（高频同步调用、零拷贝大数据）。本项目暂无此需求（SQLite/crypto 已被 op-sqlite/quick-crypto 覆盖）。
3. 旧 Bridge NativeModule——不再新增。

### 2.3 示范：端侧分诊模块（蓝图 L1 层的 RN 落地）

```
modules/radar-triage/
├── expo-module.config.json
├── ios/RadarTriageModule.swift        # Apple Foundation Models（iOS 26+，3B 端侧）
├── android/RadarTriageModule.kt       # Gemini Nano via AICore（可用性检测降级）
└── src/index.ts
```

```swift
// ios/RadarTriageModule.swift（骨架）
import ExpoModulesCore
import FoundationModels   // WWDC25 起的系统端侧模型框架

public class RadarTriageModule: Module {
  public func definition() -> ModuleDefinition {
    Name("RadarTriage")
    AsyncFunction("classify") { (text: String) -> [String: Any] in
      guard SystemLanguageModel.default.availability == .available else {
        return ["available": false]                    // 降级：调用方直通 L0 规则/云端
      }
      let session = LanguageModelSession(instructions: Self.triagePrompt)
      let result = try await session.respond(
        to: text, generating: TriageVerdict.self       // @Generable 结构化输出
      )
      return ["available": true,
              "isOpportunity": result.content.isOpportunity,
              "confidence": result.content.confidence]
    }
  }
}
```

```kotlin
// android/RadarTriageModule.kt（骨架）：GenerativeModel(AICore) 同形实现，
// AICore 不可用（设备不支持/未下载）时返回 available=false，调用方降级。
```

```ts
// src/agent/triage.ts —— 三级分诊编排（L0 规则 → L1 端侧模型 → L2 云端深析）
export async function triage(text: string): Promise<TriageDecision> {
  const l0 = runDetectionRules(text)                   // packages/radar-core 纯函数（移植自 detection_policy）
  if (l0.score >= 0.75) return { route: 'deep-analysis', reason: 'rules' }
  const l1 = await RadarTriage.classify(text)          // 端侧小模型，免费/离线/隐私
  if (!l1.available) return l0.score >= 0.45 ? { route: 'deep-analysis' } : { route: 'ignore' }
  return l1.isOpportunity && l1.confidence > 0.6
    ? { route: 'deep-analysis', reason: 'on-device-model' }
    : { route: 'ignore' }
}
```

---

## 3. Pi Agent 集成方案（重点）

### 3.1 AgentHost：单例宿主 + 依赖注入（隔离 SDK 细节的唯一位置）

```ts
// src/agent/host.ts
import { Agent } from '@earendil-works/pi-agent-core'
import { getModel } from '@earendil-works/pi-ai/compat'
import { fetch as streamingFetch } from 'expo/fetch'    // RN 内建 fetch 不支持流式 body，必须用它
import { SQLiteSessionStore } from './session-store'
import { buildToolRegistry } from './tools'
import { approvalGate } from './approval'

export interface AgentHostConfig {
  gatewayBaseUrl: string          // 推理网关（LiteLLM，OpenAI 兼容）
  getAccessToken: () => Promise<string>   // 网关用后端 JWT 计量配额；provider key 不在端上
  modelId: string                 // 'radar-deep' 等网关侧路由别名 → 多模型切换=改字符串
}

export function createAgentHost(cfg: AgentHostConfig) {
  const sessions = new SQLiteSessionStore()             // memory-repo 语义 + SQLite 持久化
  async function openSession(sessionId: string, opportunityId?: string) {
    const model = getModel('openai', cfg.modelId, { baseURL: cfg.gatewayBaseUrl }) // 适配层：参数名以 pin 版为准
    const agent = new Agent({
      initialState: {
        systemPrompt: RADAR_SYSTEM_PROMPT,              // packages/radar-agent，与 backend runner 同源
        model,
        thinkingLevel: 'low',
        tools: buildToolRegistry({ opportunityId }),
        messages: await sessions.load(sessionId),
      },
      getApiKey: cfg.getAccessToken,                    // Bearer JWT，网关侧换 provider key
      streamFn: streamingFetch,                         // 注入流式 fetch
      toolExecution: 'sequential',
      beforeToolCall: approvalGate,                     // ★ 审批门（§3.4）
    })
    agent.subscribe(ev => sessions.append(sessionId, ev))  // 事件流入库 → 崩溃可恢复/可回放
    return agent
  }
  return { openSession }
}
```

设计规则：**App 业务代码永远只 import `src/agent/*`，不直接 import pi 包**。pi 版本与 backend runner 锁同版（0.80.x pin），升级时只改 host/适配层。

### 3.2 移动端安全运行的四条边界

| 边界 | 措施 |
| --- | --- |
| 文件系统 | 无通用 read/write/bash 工具；agent 可触达的持久化只有 SQLite（工具化访问）与 `expo-file-system` 下 `agent-workspace/` 专用目录（附件/导出），路径校验拒绝越界 |
| 网络 | 工具出网仅两条路：推理网关、`gateway/fetch` 链接代理（沿用服务端 SSRF 防护，避免用户 IP 暴露给钓鱼页——蓝图审计结论） |
| 权限 | 通讯录/日历等系统能力先走系统权限，再包一层 agent 工具白名单（per-session capability set） |
| 动态代码 | **extensions 不做运行时动态加载**——App Store 禁止下载执行代码；扩展=编译期注册的工具包，热更走 Expo Updates（更新的是我们自己的 bundle，合规） |

### 3.3 自定义 Tools：把现有产品能力暴露给 Agent

工具 schema 用 **typebox**（backend runner 已用，保持同构）。示例——最能体现审批红线的 `send_reply`：

```ts
// src/agent/tools/send-reply.ts
import { Type } from 'typebox'

export const sendReplyTool = {
  name: 'send_reply',
  label: '发送回复',
  description: '向该商机的联系人发送一条回复。外部动作，必须经用户批准。',
  parameters: Type.Object({
    opportunity_id: Type.String(),
    text: Type.String({ minLength: 1, maxLength: 4000 }),
  }),
  requiresApproval: true as const,        // ★ 元数据；审批门据此拦截
  execute: async (_id: string, p: { opportunity_id: string; text: string }) => {
    // 经 outbox 走后端 manual-reply（bot 凭据在服务端，蓝图 §4.5：执行跟着凭据走）
    const updated = await radarApi.manualReply(p.opportunity_id, p.text)
    emitEvent({ type: 'ReplySent', opportunityId: p.opportunity_id, by: 'agent+human-approved' })
    return { content: [{ type: 'text', text: `已发送。商机状态：${updated.status}` }], details: {} }
  },
}
```

首批工具注册表（全部是现有能力的工具化，无新后端）：

| 工具 | 能力来源 | 审批 |
| --- | --- | --- |
| `search_opportunities` | 本地 SQLite fold 查询（看板同源） | 免 |
| `get_opportunity` / `get_messages` | 本地事件日志 | 免 |
| `draft_reply` | 现有 ai-draft 语义（经网关） | 免（草稿不外发） |
| `update_status` / `claim` | 领域状态机（radar-core 纯函数）+ outbox | 免（内部状态） |
| `analyze_links` | `gateway/fetch` 代理 + 链接钳制 | 免 |
| `send_reply` | manual-reply API | **必审** |
| `notify_user` | expo-notifications 本地通知 | 免（对齐现有 requires_approval=false 语义） |
| `remember` / `recall` | 本地记忆表 + sqlite-vec 向量检索 | 免 |

Extensions = 工具包分组（`ToolPack`）：`core-pack`（上表）、`calendar-pack`、`contacts-pack`……编译期静态注册，形状对齐 pi extension（名称/激活钩子），未来 pi 生态工具可低成本移植。

### 3.4 审批门与钳制：双层防线（与 backend `agent_policy` 同源）

```ts
// src/agent/approval.ts —— beforeToolCall 拦截器（形状与 backend runtime.mjs 一致）
export const approvalGate: BeforeToolCall = async ({ toolCall }) => {
  const tool = registry.get(toolCall.name)
  if (!tool) return { block: true, reason: `未注册工具：${toolCall.name}` }
  if (!sessionCapabilities().has(toolCall.name)) return { block: true, reason: '本会话未授权该工具' }
  if (tool.requiresApproval) {
    const verdict = await requestUserApproval(toolCall)   // 弹出审批卡片，Promise 挂起 agent loop
    if (!verdict.approved) return { block: true, reason: `用户拒绝：${verdict.note ?? ''}` }
    emitEvent({ type: 'ApprovalGranted', toolCall: toolCall.name, args: hash(toolCall.arguments) })
  }
  return undefined
}
```

第二层：凡由**服务端代执行**的动作（bot 发送），服务端在执行前用同一份钳制函数复核（蓝图 §4.7"端上防模型，服务端防设备"）。两层共享 golden 夹具。

### 3.5 UI 交互

- **Agent Tab（对话主界面）**：消息流 = session 事件的渲染（文本 token 流、工具调用卡片可折叠、审批卡片内嵌"批准/拒绝+备注"）。流式渲染走 Zustand store，`agent.subscribe` 直写、UI 订阅切片，避免每 token 重渲整树。
- **商机详情内嵌 Agent**：详情页"问 Agent"入口开 opportunity-scoped session（系统提示注入该商机上下文），工具自动带 `opportunity_id`。
- **Agent 控制面板**（settings/agent）：模型选择（网关别名列表）、工具包开关、审批策略（"外部动作总是询问"不可关——产品红线）、session 历史/清理、用量（网关计量回显）。

```ts
// src/features/agent-chat/useAgentStream.ts
export function useAgentStream(sessionId: string) {
  const dispatch = useAgentStore(s => s.dispatch)
  useEffect(() => {
    let agent: Agent | undefined
    ;(async () => {
      agent = await host.openSession(sessionId)
      agent.subscribe(ev => dispatch(ev))   // token / tool_call / approval_request / done
    })()
    return () => agent?.abort()             // 离开页面即取消在途循环
  }, [sessionId])
  return useAgentStore(s => s.sessions[sessionId])
}
```

### 3.6 模型配置与 Key 安全

| 层 | 方案 |
| --- | --- |
| 默认（全部用户） | 端上**只有后端 JWT**；网关（LiteLLM）持 provider keys、按用户计量进 `usage_ledger`（配额权威不动，蓝图审计 #3/#4） |
| 多模型切换 | 网关暴露别名目录（`radar-fast`/`radar-deep`/…→ 实际 provider/model 服务端可换），端上切换=改别名字符串 |
| BYOK（高级选项） | 用户自带 key 存 expo-secure-store（Keychain/Keystore，`WHEN_UNLOCKED_THIS_DEVICE_ONLY`）；`getModel(provider, id)` 直连该 provider；UI 明示"直连不计平台配额" |
| 禁止事项 | 任何 provider key 出现在 bundle/远端配置下发中；日志脱敏 key 与消息正文 |

### 3.7 Session 持久化与跨平台一致性

- 存储：`agent_sessions(id, opportunity_id, title, created_at)` + `agent_entries(session_id, seq, jsonl)`——**行格式就是 pi 的 JSONL entry**，一行一事件，天然增量同步单元。
- 一致性：iOS/Android 同一 TS 代码+同一序列化，无双端漂移面；与 backend fallback runner 的一致性靠 `packages/radar-agent` 里共享的 AnalysisSchema + golden 夹具（现有 `ModelsDecodingTest` 传统的延伸）。
- 跨设备：session JSONL 作为事件信封进蓝图同步通道（P5 起 E2EE）；compaction（pi-agent-core 内建）控制长会话上下文与存储。

---

## 4. 迁移路线图

绞杀者模式：原生双 App 冻结但在线，RN 按里程碑逐步替换，任何阶段可回退到原生 App。

### Phase 0 · 地基（~2 周）
**交付**：monorepo 改造（`packages/radar-core|api|agent` 从 frontend/backend 提升，Web 改 import 路径回归零行为变化）；Expo 骨架 app（登录：邮箱密码 + Sign in with Apple / Google Credential Manager 经 expo config plugins；推送注册；SQLite 初始化）；CI 加 RN job（tsc/eslint/jest + EAS 或本地构建）。
**风险**：Metro monorepo 解析（`unstable_enablePackageExports`/symlink 配置一次性成本）；Google/Apple 登录的原生配置迁移。

### Phase 1 · MVP 对齐现有瘦客户端（~4 周）
**交付**：看板（dashboard API 全筛选/重大商机横幅/富卡片）、详情（消息流/Agent 发现/回复/模板/状态流转）、设置中心（订阅 via react-native-purchases、Telegram 管理、识别规则/工作时间/通知）、深链。内部 TestFlight/内测轨发布；与原生 App 并行 dogfood。
**验收**：功能对齐矩阵逐行打钩（复用现有矩阵）；崩溃率 < 0.3%；冷启 < 2s（中端机）。
**风险**：RevenueCat 迁移期两套 App 同 entitlement（RC 按 app user id 聚合，低风险）；细节交互回退感。

### Phase 2 · Agent v1 + 端侧真相（~6 周）
**交付**：事件日志同步 + 推送替代 30s 轮询（蓝图 P0/P1 客户端）；pi harness 进 bundle，Agent Tab 上线（只读工具 + draft + 审批门 UI）；分析上端：claim/complete/fail API 对接（蓝图 P2），服务端管道降级 fallback、结果打 `executed_by`。
**验收**：飞行模式看板/详情/Agent 历史可用；打开 app 用户的新消息分析 100% 端上执行；golden 夹具双端（RN/backend）同过。
**风险**：expo/fetch 流式在低端 Android 的稳定性（预留 SSE 轮询降级）；pi 包在 Hermes 的未知 API 缺口（适配层 + pin 版本 + 上游 PR）。

### Phase 3 · 完整 Agent + 原生优化 + 收尾（~6 周）
**交付**：写操作工具全量（send_reply 审批链）、工具包开关、记忆（sqlite-vec）+ `remember/recall`；`radar-triage` 端侧分诊模块（L0/L1/L2 分层，云调用量可观测下降）；性能收尾（Hermes bytecode、inline requires、FlashList 调优、包体预算）；商店换装：RN App 以更新形式顶替原生 App（同 bundle id / package name），Swift/Kotlin 目录归档。
**验收**：蓝图 P2/P3 客户端判据全绿；包体 iOS ≤ 30MB / Android ≤ 25MB（下载体积）；启动 TTI ≤ 2.5s P90。
**风险**：Foundation Models / AICore 设备覆盖率（降级路径已内建）；老用户升级到 RN 版的迁移（本地无历史数据需迁——P0 起就没有本地数据，风险≈0；Keychain token 复用同 service name 可实现无感续登）。

---

## 5. 技术栈清单

| 类别 | 选型 | 备注 |
| --- | --- | --- |
| 框架 | React Native 0.8x（最新稳定）+ Expo SDK ≥53 dev client | New Architecture + Hermes 默认 |
| 语言 | TypeScript 5.x `strict`，`moduleResolution: bundler` | monorepo 内 project references |
| 包管理 | pnpm workspaces | 与仓库现状一致 |
| 导航 | expo-router | 类型化路由 + 深链声明式 |
| 状态 | Zustand + TanStack Query | UI 态 / 网关请求态分治 |
| 数据 | **op-sqlite**（支持加载 sqlite-vec）+ MMKV（轻量 KV） | 事件日志/会话/记忆；vec 为 `recall` 工具服务 |
| 安全存储 | expo-secure-store（BYOK/Token） | Keychain / Keystore |
| UI | NativeWind 4 + FlashList + expo-image | 与 Web 共享 Tailwind token |
| Agent | `@earendil-works/pi-agent-core` + `pi-ai`（与 backend 锁同版 pin） | 经 `src/agent` 适配层 |
| 工具 schema | typebox | 与 backend runner 同构 |
| 计费 | react-native-purchases | RevenueCat 官方 |
| 推送 | expo-notifications（+FCM/APNs 凭据） | |
| 质量 | jest + @testing-library/react-native；Maestro E2E；Sentry | golden 夹具跨 RN/backend 复用 |
| 构建发布 | EAS Build/Submit 或本地 fastlane 双轨；Expo Updates 做 JS 热修 | 商店合规内的 OTA |
| 调试 | Expo devtools + Hermes debugger（React Native DevTools） | Flipper 已淘汰 |

---

## 6. 潜在风险与解决方案

| 风险 | 影响 | 对策 |
| --- | --- | --- |
| **RN fetch 不支持流式响应** | Agent 流式输出的硬前提 | 强制 `expo/fetch`（WinterCG，支持 ReadableStream）注入 `streamFn`；低端机降级 SSE 分块轮询 |
| pi 包隐性 Node API 缺口 | 运行时报错 | 主入口已核实零 `node:`；仍以适配层隔离 + `metro.config` resolver 显式禁止 `./node` 子入口进 bundle + 启动冒烟测试跑一轮最小 agent loop |
| 包体积/启动 | 商店转化、体验 | Hermes bytecode + inline requires；pi-ai 仅按子路径引 openai-compat provider（摇掉 AWS/proxy 链）；体积预算进 CI（>阈值即红） |
| iOS 后台限制 | "收到推送但没打开 app，端上 agent 没跑" | 诚实产品语义（蓝图 §6）：NSE 有限时间内跑 L0/L1 分诊改写通知文案；深析等打开或用户开启服务端"离线托管" |
| Agent 执行工具的安全 | 外部动作被模型滥用 | `requiresApproval` 元数据 + beforeToolCall 双查（白名单+审批）+ 服务端代执行动作二次钳制 + `serializeUntrustedInput` 防注入（沿用现成实现）+ 审批留痕事件 |
| App Store 审核 | 动态代码/Agent 类目风险 | extensions 编译期注册；OTA 仅 Expo Updates；审核备注说明"所有外部动作需用户逐次批准"；提供无 Agent 的核心功能路径（看板/回复可独立使用） |
| 隐私合规 | App Privacy / 生成式 AI 合规 | 数据默认端上 + 网关不落盘 prompt（蓝图 PCC 原则）声明进隐私标签；若分发中国区，生成式内容功能按备案要求评估（企微用户画像大概率涉及） |
| 双 App 冻结期 | 原生 App 停止演进 | 冻结窗口明确（P1 结束前仅安全修复）；P1 交付即开始灰度换装 |
| 团队栈切换 | Swift/Kotlin → RN 学习曲线 | 原生手继续负责 modules/（分诊、未来 MTProto 节点）；RN 侧 TS 资产与 Web 同源，前端手可直接上 |
| 性能敏感清单 | 看板长列表、agent 流渲染 | FlashList + 稳定 keys；token 流写 store 切片订阅；分析在后台 JS 任务队列串行，UI 线程零阻塞 |

---

## 7. 与 Agent-native 蓝图的关系（避免文档漂移）

- 本方案**取代**蓝图 §4.3 的宿主选型：不再"原生 App 内嵌 JSC/Hermes"，而是"App 即 RN/Hermes"。蓝图其余决策（事件溯源、三原语服务端、分诊分层、租约协调、钳制双侧、E2EE 路线）原样生效，本方案是其客户端实现。
- 采纳时同步动作：蓝图 §4.3 加注取代说明 → 落 ADR（RN 单代码库 + pi in-process）→ 按 §4 路线拆 `docs/plans/active/` 执行计划 → 功能地图为 RN App 增行。
