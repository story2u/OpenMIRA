# Agent-native 产品架构蓝图（提案）

> 状态：**提案待评审** · 起草：2026-07-16 · 起草者：bruce + Claude
> 定位：产品形态与系统架构的方向性重构提案。采纳后需按阶段拆分执行计划并落 ADR；
> 本文不是执行计划，未动任何生产代码。

## 0. 一句话

把商机雷达从「SaaS + 三个瘦客户端」重构为「**长在用户设备上的个人 Agent** + 三个瘦服务端原语（中继 / 推理网关 / 同步）」：harness、记忆、数据、审批都在端上，服务端只保留物理上无法下放的东西。

---

## 1. 现状：一个标准的 Web 时代架构

当前系统是教科书式的服务端中心架构（代码真相源：`backend/app/`、`docs/architecture/overview.md`）：

```
Telegram/企微 ──webhook/MTProto──▶ FastAPI ──▶ Postgres（唯一真相）
                                     │
                              Redis + Celery
                                     │
                    ┌────────────────┼──────────────────┐
              agent.analyze     ai.reply          billing/sweep
              （pi runner）    （LiteLLM）
                                     ▲
Web / iOS / Android ──30s 轮询───────┘   （瘦客户端，READMEs 自述"瘦客户端"）
```

关键事实（已逐文件核实）：

| 组件 | 现状 | 文件 |
| --- | --- | --- |
| Agent runtime | **纯 TS/ESM**，`pi-agent-core` harness，单工具 `submit_analysis`，stdin/stdout JSON 契约，唯一外部依赖是模型 API | `backend/pi-agent-runtime/src/runtime.mjs` |
| Agent 调用方式 | Celery task → Node 子进程（受限 env、超时、输出上限） | `backend/app/infrastructure/agent/pi_client.py` |
| 链接证据 | 服务端 `SafeLinkInspector` **预取**后作为输入喂给 agent（agent 本身不出网） | `backend/app/infrastructure/agent/link_inspector.py` |
| 分析编排 | claim 租约（防双跑）→ 链接预取 → agent → 纯函数投影 → 落库 | `backend/app/application/use_cases/analyze_message.py` |
| 配额 | `usage_ledger` 按"次"预留/消费，与订阅套餐挂钩 | `repositories.py::SubscriptionRepository` |
| 常驻进程 | api、celery worker/beat、**3 个 Telegram 监听进程**、cloudflared | `backend/docker-compose.prod.yml` |
| 客户端数据 | 无本地持久化，30s 轮询全量拉取；推送未落地 | 三端 README |
| 用户记忆 | 用户级设置（关键词/工作时间/通知）在 Postgres | `user_*_preferences` 表 |

**结论**：智能、记忆、数据、审批流全部在服务端；客户端只是遥控器。这正是要反转的。

## 2. 论点检验：诚实的物理边界

「Agent 植入 app、数据存端上、只有大模型上云」这个论点，**大方向与业界收敛方向一致**（见 §3），但对本产品必须先过三条物理约束，否则会做出一个"关掉 app 就失明的雷达"：

| 约束 | 事实 | 推论 |
| --- | --- | --- |
| **公网可达性** | Telegram Bot API / 企微回调要求公网 HTTPS webhook；MTProto 账号监听要求持久 TCP 连接 | 摄取入口永远需要一个 always-on 节点。手机 app 被杀死/休眠时无法承担（iOS 后台执行以分钟/天计） |
| **算力** | 深度分析/草稿生成需要前沿大模型 | 用户已认同：推理上云。但**分诊可以端侧小模型**（Apple Foundation Models / Gemini Nano），"只有重活上云"比"所有模型调用上云"更彻底 |
| **多端一致** | 产品现有 Web/iOS/Android 三端，用户期望切设备无缝 | "数据只存单台设备"不成立；成立的是 **local-first + 端到端加密同步**：每台设备都有全量数据，服务端只做加密信封的中转 |

因此正确的表述不是"只有大模型上云"，而是：

> **服务端收缩为三个不可下放的原语：① 公网摄取中继，② 模型推理网关，③ 加密同步/推送兑换点。其余一切——harness、记忆、数据真相、工具、审批——都在端上。**

这与用户论点的精神完全一致，只是把"为什么这三样必须留下"说清楚了。

## 3. 业界参照：成熟产品怎么分层

（2026-07-16 由 5+3 个研究/复核 agent 在线核验完成；置信度与引文见附录 B。注意：调研期间非 Anthropic 站点的抓取被上游代理大面积 429，标注 `likely` 的条目基于 2026-01 前官方文档、给出规范 URL 但未实时复核）

| 产品 | 端上 | 云上 | 对本项目的启示 |
| --- | --- | --- | --- |
| **Claude Code / Codex CLI** | harness 循环、工具执行、权限门、OS 级沙箱（Seatbelt/bubblewrap）、记忆（CLAUDE.md 分层 + auto memory）、transcripts（~/.claude/projects/ 本地明文，默认 30 天）全在本地 | 仅模型推理 API（provider 可整体替换而 harness 不动）【verified，官方文档实时核验】 | **这就是目标形态的原型**：pi-agent-runtime 已经是同构的 TS harness，只差宿主从服务器换成 app |
| **Claude Remote Control / Dispatch（2026）** | **执行与文件系统留在本机**；手机发任务→桌面常驻 agent 本地执行（Dispatch）；本机只发出站 HTTPS、不开入站端口 | 云只做**消息中继 + transcript 跨设备同步**【verified】 | 我们"三原语服务端 + 手机/常驻节点角色分工"的直接产品先例；镜面对照：ChatGPT agent/Operator 走反向极点（云 VM 执行、设备只是确认界面） |
| **Cursor** | 编辑器与聊天记录本地 | AI 请求经 Cursor 自营云组装上下文再到 provider，embeddings 存服务端【likely】 | "harness 半本地半云"的中间形态——我们不取：多一层能读业务内容的自营云与 E2EE 目标冲突 |
| **Apple Intelligence + PCC** | ~3B 端侧模型做分类/摘要/提取（Foundation Models framework，免费、离线；guided generation 保证结构化输出；**4k token 上下文是硬边界**；官方明示不用于开放问答） | 重请求走 Private Cloud Compute：**无状态、无特权访问、不可定向、可验证透明、硬件信任根** | ① 分诊层用端侧模型（长消息需分块）；② 推理网关按 PCC 原则设计：不落盘 prompt、只计量 |
| **WhatsApp / Signal vs Telegram** | WhatsApp/Signal：**消息真相在设备**，多设备=每设备独立密钥+发送端逐设备加密扇出（client-fanout），服务端只见信封（sealed sender） | Telegram：云为真相 | 采用 WhatsApp/Signal 侧哲学：中继只见信封；多设备密钥走"设备信任圈 + 云端 HSM 限次托管恢复"（iCloud Keychain / WhatsApp Backup Key Vault 同构） |
| **Linear / Figma / Notion** | 本地全量/部分副本 + 乐观应用，离线可用 | **服务器仍是唯一真相**（Linear 事件流、Figma 按属性 LWW、Notion 本地 SQLite 只是缓存）【likely】 | 教的是**同步工程**而非数据主权——我们的读路径借鉴其副本/增量机制，数据主权取 Signal 侧；事件日志同步的新生代对口方案是 LiveStore（事件日志为同步单元+端上 SQLite 物化） |
| **MCP（协议）** | 工具在数据所在地声明与执行 | 远程 MCP server = OAuth 2.0 Resource Server（2025-06-18 规范定性，客户端强制 RFC 8707）【verified】 | 端上工具按 MCP 形状声明，未来可接入生态；A2A 采用仍偏厂商试点，社区普遍把对端 agent 包成 MCP server——继续不采 A2A |

模式总结——2026 年成熟 AI 产品的收敛形态：**"智能在端、算力在云、云无状态"**；多设备协调的主流出货是"云端集中会话+薄客户端"（Codex cloud / Copilot coding agent），而"本地执行+云中继"有 Claude Remote Control 这一验证先例——我们因隐私叙事选后者，并清楚它是较少被走的路。

## 4. 目标架构

### 4.1 拓扑

```
                 ┌──────────────── 用户的设备群（数据与智能所在地）────────────────┐
                 │                                                                  │
                 │  📱 手机 App（iOS/Android）        💻 常驻节点（macOS app / CLI）  │
                 │  ┌──────────────────────┐        ┌──────────────────────────┐   │
                 │  │ Radar Agent Runtime  │◀─sync─▶│ Radar Agent Runtime      │   │
                 │  │ (pi-core, JSC/Hermes)│        │ (pi-core, Node)          │   │
                 │  │ 角色：分诊·通知·审批   │        │ 角色：重分析·长任务·      │   │
                 │  │ 端侧小模型：预分类     │        │ 可选:MTProto 监听宿主     │   │
                 │  │ SQLite: 事件日志+记忆  │        │ SQLite: 同一事件日志      │   │
                 │  └──────────┬───────────┘        └────────────┬─────────────┘   │
                 └─────────────┼───────────────────────────────── ┼────────────────┘
                               │ ①事件同步(E2EE 信封)  ②模型调用    │
                 ┌─────────────▼──────────────────────────────────▼─────────────┐
                 │                   收缩后的服务端（无业务智能）                    │
                 │  ┌────────────┐  ┌──────────────┐  ┌───────────────────────┐ │
                 │  │ 摄取中继     │  │ 推理网关       │  │ 身份·同步·推送          │ │
                 │  │ webhooks    │  │ LiteLLM 代理  │  │ auth/设备注册           │ │
                 │  │ MTProto 托管 │  │ 计量→配额     │  │ 加密事件信箱(TTL)        │ │
                 │  │ (可下放到节点)│  │ 不落盘 prompt │  │ APNs/FCM 唤醒          │ │
                 │  └────────────┘  └──────────────┘  └───────────────────────┘ │
                 └───────────────────────────────────────────────────────────────┘
                        ▲                                      ▲
                  Telegram / 企微                        RevenueCat（计费不变）
```

### 4.2 核心决策一：事件溯源作为统一地基

把领域改写为 **每用户一条可同步的事件日志**（`MessageIngested`、`OpportunityDetected`、`AnalysisCompleted`、`ApprovalGranted`、`ReplySent`、`StatusChanged`、`TaskClaimed`…）：

- 设备端状态 = `fold(事件日志)`，存 SQLite；离线可读写，回线合并。
- 现有 `claim_agent_analysis` 的 DB 租约直接翻译为 `TaskClaimed{lease}` 事件——**多设备 agent 不双跑的协调原语**，不需要引入任何分布式框架。
- 审批留痕（`ApprovalGranted`）天然是审计日志，契合"外部动作需人工批准"的产品红线。
- 服务端信箱只存**加密信封**（有 TTL），设备取走后即为端上真相；服务端从"唯一真相"降级为"暂存队列"。

这是整个重构里**唯一一个不可逆的结构性决策**，采纳时必须落 ADR。

### 4.3 核心决策二：pi-agent-runtime 上端，TS 一套跑四端

> **2026-07-16 更新**：宿主选型已被 [RN + Pi Agent 移动端重构方案](2026-07-16-rn-pi-agent-refactor.md)
> 取代——客户端整体迁移到 React Native 单代码库，App 本身就是 Hermes 宿主，
> 下表中"JSC/Hermes 桥接进原生 App"的路径不再需要。本节其余判断（TS 契约可移植、
> Node/服务端 fallback 保留、否决 Rust 双写）继续成立。

现运行时已是纯 TS + 极小契约（`f(消息+链接证据) → 结构化分析`）。宿主替换路径：

| 宿主 | 引擎 | 说明 |
| --- | --- | --- |
| iOS | **JavaScriptCore**（系统内置，零体积） | 原生桥接：fetch→URLSession、存储→SQLite、密钥→Keychain |
| Android | QuickJS / Hermes | 同上，桥到 OkHttp/Keystore |
| macOS 常驻节点 | Node（复用现有 runner 全部代码） | 还可宿主 MTProto 监听（见 4.5） |
| 服务端（过渡期 fallback） | 现有 Celery+Node 子进程，**保持不动** | 迁移期间的兜底与从未打开 app 用户的服务 |

备选方案曾考虑 Rust core（UniFFI）与 Swift/Kotlin 双写。否决理由：现有 runtime 是 TS、Web 端是 TS、pi-agent-core 生态是 TS；双写违背仓库"最小 diff"哲学，Rust 是为尚不存在的性能问题付成本。若未来端上工具数量膨胀、桥接成为瓶颈，再以 ADR 升级。

### 4.4 核心决策三：智能分层——端侧小模型分诊，云端大模型深析

```
消息事件到达设备
  → L0 确定性规则（现有 detection_policy 关键词/正则，纯函数，直接移植）
  → L1 端侧小模型分诊（Apple Foundation Models / Gemini Nano）：是否值得深析？
  → L2 云端大模型深析（经推理网关，计量配额）：现有 pi agent 分析，端上 harness 发起
```

收益：L0/L1 免费+离线+隐私（绝大多数群噪音不出设备）；L2 才计费。配额从"服务端按次预留"改为"网关按调用计量"，`usage_ledger` 语义保留、位置改变。设备覆盖的诚实预期：Apple 侧需 iPhone 15 Pro+/M 系列且上下文 4k（长消息分块）；Android 侧 Gemini Nano 截至 2026 初仅旗舰机清单可用——无端侧模型的设备 L1 自动降级（直通 L2 或按 L0 阈值决定），这是设计内行为而非缺陷。

### 4.5 核心决策四：分布式 Agent = 角色化实例 + 事件日志协调

同一用户的多个设备运行**同一个逻辑 Agent 的不同角色实例**：

| 实例 | 角色 | 认领的任务 |
| --- | --- | --- |
| 手机 | 前台：分诊呈现、通知、**审批**、快速回复 | 轻任务（收到推送即处理） |
| 常驻节点（可选） | 后台：深度分析、批量重扫、长时间监视 | 重任务 + `MTProto 监听宿主`（把"用户账号监听"从我们的服务器搬到用户自己的 Mac——Telegram Desktop 本来就是这么工作的，隐私叙事更强） |
| 服务端 fallback（可配置开关） | 兜底：所有设备离线超过 SLA 才代跑 | 仅在用户显式开启"离线托管"时 |

协调规则只有一条：**任务认领 = 带租约的事件，先到先得，过期可抢**。这是现有 DB claim 逻辑（`claim_agent_analysis` 行锁 + 10 分钟 RUNNING 过期回收，`repositories.py:780-811`）的直接翻译。调研确认（附录 B）：跨设备任务租约**没有消费级现成协议**（Handoff 只做前台接力、Syncthing 无认领语义），成熟租约实现都在服务器基础设施层（etcd/K8s Lease）——所以我们用被验证过的服务器原语（租约）+ 被验证过的产品先例（Remote Control 的云中继）组合，而不是引入 A2A 等仍在试点期的重协议。

**动作执行的归属规则**（审计发现的关键约束）：分析可以在任何实例跑，但**执行**跟着凭据走——

- 经平台 Bot / 企微应用发送（bot token、corp secret 是服务端秘密）→ 永远由服务端 relay 执行，且执行前**服务端重新钳制**（见 4.7）。
- 经用户自己账号（MTProto session）的动作 → 由持有该 session 的设备/节点执行，凭据不出设备。
- 周期性兜底（如 `sweep_pending_for_ai` 的 SLA 扫描）跟随"自动回复执行者"角色所在地：默认服务端 beat 保留，用户关闭离线托管后该语义随之关闭。

### 4.6 服务端收缩后的形态

单一 FastAPI 进程**按职责重组，不拆微服务**（ponytail：进程数不是先进性指标）：

| 模块 | 保留原因 | 变化 |
| --- | --- | --- |
| `relay/`（原 webhooks + telegram 监听） | 公网可达性物理约束 | 摄取后不再进业务管道，只写**加密信箱 + 发推送**；MTProto 监听可被用户节点接管 |
| `gateway/`（原 LiteLLM 调用点） | 算力 | 变成纯代理：转发流式推理、计量、配额裁决；**不落盘 prompt/响应**（PCC 原则） |
| `sync/`（新） | 多端一致 | 事件信封中转 + 设备注册 + APNs/FCM |
| `id/`（原 auth + billing） | 账号与收费 | 基本不变（RevenueCat 链路原样保留） |
| ~~detection/agent/reply 业务管道~~ | — | **逐阶段迁出到端**，过渡期作为 fallback 保留 |

Web 端降级为**控制台**（计费、Telegram Bot 连接向导这类必须走网页的流程）+ 营销首页；产品主战场移到 iOS/Android/macOS。

### 4.7 安全与隐私边界（重构后反而更强）

- **钳制投影双侧执行**：`agent_policy.project_agent_result` 是纯函数（链接风险不可降级、外部动作强制 `requires_approval`、补判商机强制 PENDING_HUMAN）。端上实例对自己的状态应用同一投影（防模型越权）；**服务端对一切由它代为执行的动作在执行前重新钳制**（防被攻破的客户端伪造结果——对服务端而言，端上 agent 只是不可信结果生产者）。同一份 golden 夹具约束两侧实现（仓库已有跨端夹具测试传统）。
- 事件负载端到端加密：同步服务只见 `(user, device, seq, ciphertext, ttl)`；服务端**读不了商机内容**——"数据在端上"从口号变成可验证承诺。
- 密钥：主密钥在 Keychain/Keystore，跨设备走平台钥匙串同步 + 恢复码（不自研密码学协议）。
- Telegram session：留在生成它的设备（节点托管时在用户 Mac 的 Keychain），服务器不再持有可登录凭据——比现状（服务端加密存 session，密钥派生自服务端 secret）更符合最小权限。
- **链接预取默认走网关代理**：端侧直连可疑链接会把用户 IP 暴露给钓鱼方（审计确认现有 SSRF 防护是按保护服务端内网设计的）；网关代理保留逐跳校验与大小限制，端侧直抓仅作显式选项。
- 模型 API key 永不进 app（现状 `pi_agent_api_key` 回退共享 `openai_api_key`，打进客户端等于泄露）；推理网关是唯一出口，配额记账权威（`usage_ledger` 行锁 + 幂等三段式）保留在网关侧。
- 高风险动作（发消息/加好友/邮件）：审批门在端上 harness 内强制执行，`requires_approval` 语义原样保留；模型输出永远不能自我提权（现有红线不变）。
- 推理网关沿用现有输入序列化的防注入处理（`serializeUntrustedInput`）。

## 5. 迁移路线（渐进、可回退、每阶段独立有价值）

绞杀者模式，老管道全程在线，直到新路径验证完成：

| 阶段 | 内容 | 交付判据 | 依赖 |
| --- | --- | --- | --- |
| **P0 事件地基** | 服务端内建事件日志（现有表→事件投影双写）；`GET /sync/events` 增量拉取 + APNs/FCM 推送落地 | 三端靠推送+增量同步替掉 30s 轮询 | 无（就是搁置的推送计划的超集） |
| **P1 端侧真相** | 三端 SQLite 镜像 + outbox 命令队列；离线可读、断网操作回线重放 | 飞行模式下看板/详情完整可用 | P0 |
| **P2 harness 上端** | JSC/Hermes 宿主 pi-runtime；分析在端上跑（链接预取走网关 fetch 代理）；**新增分析任务 claim/complete/fail API + 设备失联租约回收**（现状没有任何 HTTP API 能提交分析结果，全部动作只在 worker 进程内可达）；`TaskClaimed` 租约事件防双跑；服务端管道降级为 fallback，结果打标 `executed_by` | 打开 app 的用户，其新消息分析 100% 发生在端上；同一 golden 夹具约束两侧 harness 输出 | P1 |
| **P3 智能分层** | L0 规则移植 + L1 端侧模型分诊接入；网关计量替代按次预留 | 云端调用量下降可观测；无套餐用户获得离线 L0/L1 | P2 |
| **P4 常驻节点** | macOS 菜单栏 app / CLI（Node 复用 runner）；MTProto 监听可迁至节点；服务端 fallback 变成可选订阅功能"离线托管" | 重度用户的服务器依赖只剩中继+网关+同步 | P2 |
| **P5 E2EE 收口** | 信箱与同步信封端到端加密、密钥恢复流程；Web 降级为控制台 | 服务端无法读取任何商机内容 | P1-P4 稳定 |

诚实的量级判断：P0-P1 是数周级；P2-P3 是季度级；P4-P5 是第二季度。每阶段结束回写 `docs/architecture/overview.md` 与功能地图。

## 6. 风险与反对意见（自我红队）

| 风险 | 严重度 | 缓解 |
| --- | --- | --- |
| iOS 后台限制：收到推送但用户不点开，端上 agent 没机会跑 | 高 | 诚实产品语义：通知即分诊结果（L1 在推送扩展的有限时间内可跑）；深析延迟到打开时；或用户开启"离线托管"fallback |
| 多端事件合并冲突 | 中 | 事件为不可变追加 + 状态机仲裁（现有 `opportunity_state` 迁移规则就是仲裁器）；不用通用 CRDT 硬啃 |
| JS 引擎宿主的调试/性能 | 中 | 契约极小（一进一出 JSON）；golden 夹具跨宿主复用（仓库已有跨端夹具测试传统） |
| E2EE 之后服务端无法做全局功能（如跨用户统计） | 低 | 本产品本就 owner 隔离，无跨用户功能；统计走端上聚合上报 |
| 双管道过渡期的行为漂移 | 高 | 同一 golden 夹具同时约束服务端与端上 harness 输出；fallback 结果打标 `executed_by` |
| 团队栈跨度（Swift/Kotlin 桥 + TS core） | 中 | 桥面积刻意最小化（fetch/sqlite/keychain 三个原语）；先 iOS 后 Android |

## 7. 不做什么（同样重要）

- **不做** 通用 Agent 平台/插件市场——本产品的 agent 只服务商机雷达这一个 job。
- **不做** P2P 设备直连同步——信封中转已满足，P2P 是纯复杂度。
- **不做** 微服务拆分——服务端收缩是职责收缩，不是进程裂变。
- **不做** 自研密码学协议——平台钥匙串 + libsodium 级原语。
- **不做** 一步到位删除服务端管道——fallback 是"雷达不失明"的产品承诺。

## 8. 采纳流程

1. 评审本蓝图 → 方向确认后落 **ADR：事件溯源 + 端上 harness + 服务端三原语**。
2. 按 §5 拆 `docs/plans/active/` 执行计划（P0 一份即可启动）。
3. 每阶段完成回写架构总览/功能地图（本仓库知识库纪律不变）。

---

## 附录 A：仓库耦合审计（2026-07-16，已完成）

对"Agent 搬到端侧"的 12 个服务端耦合点逐一定性（独立审计 agent 产出，全部 verified，来源为 文件:行）：

| # | 耦合点 | 端侧化判定 |
| --- | --- | --- |
| 1 | Agent 触发内嵌在服务端摄取链（webhook/listener → ingest → 无条件调度分析） | 有条件：改为事件推送触发端上认领 |
| 2 | 执行载体是 Celery spawn 的 Node 子进程 | 有条件：契约可移植，宿主换 JSC/Hermes |
| 3 | 模型 API key 是服务端秘密（回退共享 `openai_api_key`） | **不能直接搬**：必须走推理网关（或用户自带 key） |
| 4 | 配额 = DB 行锁 + `usage_ledger` 幂等三段式记账 | **记账权威不能搬**：迁到网关计量 |
| 5 | 分析生命周期状态机在 DB（claim/complete/fail 仅 worker 进程内可达，**无 HTTP API**） | 有条件：P2 新增 API + 失联租约回收 |
| 6 | 信任钳制（`agent_policy` 投影：链接风险不可降级、强制审批、强制人工）在服务端 | **服务端侧必须保留**：对其代执行的动作重新钳制；端侧同函数防模型 |
| 7 | 链接证据服务端预取（SSRF 防护按保护服务端内网设计） | 有条件：默认网关代理（防用户 IP 暴露） |
| 8 | 可靠性语义由 Celery 承载（专用队列/超时/退避重试/失败释放配额） | 有条件：端侧需自行复刻，iOS 后台弱化该语义 |
| 9 | 周期任务（SLA sweep、计费对账）依赖 beat 常驻 | **不能**：跟随自动回复执行者角色；对账永驻服务端 |
| 10 | 下游动作执行依赖服务端 IM 凭据（bot token / corp secret） | **执行侧不能搬**：分析与执行拆开，见 §4.5 |
| 11 | 三个 Telegram 常驻监听进程，session 密文存 DB、密钥派生自服务端 secret | 有条件：MTProto 监听可整体迁至用户常驻节点（§4.5） |
| 12 | 前置检测层（规则+LiteLLM 分类+用户设置合成）也在服务端 | 有条件：P3 的 L0/L1 端侧化覆盖 |

审计转录：`.claude` 会话 `subagents/workflows/wf_ecc8de3c-df2/`。

## 附录 B：在线调研引文（2026-07-16 完成）

复核工作流：4 个研究方向（47 条 findings）+ 3 个独立证伪 agent（抽检关键断言，全部 holds）。方法学说明：调研期间非 Anthropic 站点的抓取被上游代理大面积 429，各 agent 已如实降级置信度——`verified` = 本次实时抓取官方原文核对；`likely` = 基于 2026-01 前官方文档、URL 规范但未实时复核；WWDC 2026（2026-06）的增量发布**未能覆盖**，采纳前建议人工补核。完整 findings 见工作流转录 `subagents/workflows/wf_ecc8de3c-df2/`。

**verified（实时核验，含独立证伪通过）**

- Claude Code 本地运行边界与数据留存："Claude Code runs locally… sends data over the network" 仅指 LLM 交互；transcripts 本地明文 `~/.claude/projects/` 默认 30 天 — https://code.claude.com/docs/en/data-usage
- 记忆体系全部本地 markdown（CLAUDE.md 分层 + auto memory MEMORY.md，机器本地不跨设备）— https://code.claude.com/docs/en/memory
- 权限门/沙箱客户端强制（Seatbelt / bubblewrap+seccomp；本地代理域名放行与凭证掩码注入）— https://code.claude.com/docs/en/sandboxing
- Remote Control：执行与文件系统留本机，云仅中继+transcript 同步，出站 HTTPS 无入站端口；Dispatch：手机发任务→桌面常驻 agent 本地执行 — https://code.claude.com/docs/en/remote-control 、https://code.claude.com/docs/en/desktop
- MCP 2025-06-18：MCP server 定性为 OAuth 2.0 Resource Server，客户端强制 RFC 8707 — https://modelcontextprotocol.io/specification/2025-06-18/changelog

**likely（官方文档口径，未实时复核）**

- Apple Foundation Models framework：~3B 端侧模型、免费离线、LanguageModelSession、guided generation（@Generable 约束解码）、4k 上下文、官方定位=分类/提取/摘要非开放问答 — https://developer.apple.com/documentation/foundationmodels
- PCC 五大保证（无状态/无特权/不可定向/可验证/硬件信任根）+ 公开 VRE 与百万美元赏金 — https://security.apple.com/blog/private-cloud-compute/
- Android Gemini Nano 经 AICore，入口 ML Kit GenAI APIs；设备覆盖限旗舰清单；Google 官方推荐"端侧优先、云端兜底" — https://developer.android.com/ai/gemini-nano
- WhatsApp 多设备：每设备独立密钥 + 发送端逐设备加密扇出，历史 E2E 直传 — https://engineering.fb.com/2021/07/14/security/whatsapp-multi-device/ ；Signal Sesame + sealed sender — https://signal.org/docs/specifications/sesame/
- 多设备密钥恢复的"云端 HSM 限次托管"模式（iCloud Keychain Cloud Key Vault / WhatsApp Backup Key Vault）— https://support.apple.com/guide/security/sec1c89c6f3b/web
- Linear 服务器权威 sync engine / Figma 属性级 LWW（明确拒绝完整 CRDT）/ Notion 本地 SQLite 仅缓存 — 各官方工程博客
- 同步引擎生产现状：PowerSync/Electric（1.0 GA，301→electric.ax 实测）= 读路径复制+写走自家 API；LiveStore = 事件日志同步（对口我们的设计）；Automerge 3 成熟、cr-sqlite 停滞（不选）；sqlite-vec v0.1.x 暴力 KNN，个人记忆量级够用
- Codex CLI 本地内核（~/.codex + AGENTS.md + Seatbelt/Landlock）+ Codex cloud 容器双形态；ChatGPT agent/Operator = 云 VM 执行反例；Cursor 请求经自营云组装 — 各官方文档/公告
- A2A：2025-06 捐 Linux Foundation、v0.3 加 gRPC 与签名 Agent Card，但真实采用偏厂商试点，社区普遍把对端 agent 包成 MCP server
- 跨设备任务租约无消费级协议（Handoff/Syncthing 均不含认领语义），成熟实现在 etcd/K8s Lease 层

**对蓝图的三处修正（已回写正文）**：① §3 Linear/Figma/Notion 行改为"云权威+端上副本"（数据主权范本改为 WhatsApp/Signal）；② §4.4 增补端侧模型设备覆盖与 4k 上下文边界；③ §4.5 协调方案的表述从"最被验证"改为"被验证原语（服务器租约）+ 被验证先例（Remote Control 云中继）的组合"。
