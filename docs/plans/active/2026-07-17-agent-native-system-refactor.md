# Agent-native + React Native 系统重构总计划

> 状态：active（P5 S0-S5 完成，生产默认 v1/关闭） · Owner：bruce / Codex · 创建：2026-07-17 · 更新：2026-07-18
>
> 方向输入：[Agent-native 产品架构蓝图](2026-07-16-agent-native-architecture.md)、
> [React Native + Pi Agent 移动端重构方案](2026-07-16-rn-pi-agent-refactor.md)。两份输入仍是提案，
> 本计划把它们收敛为可验证、可回滚的实施顺序；代码与当前事实仍以
> [架构总览](../../architecture/overview.md)、[功能地图](../../product/feature-map.md)和仓库实现为准。

## 目标与用户价值

在不让消息摄取、人工审核、真实回复和订阅链路失效的前提下，逐步把产品从“服务端业务管道 +
三套瘦客户端”演进为：

1. 一套 React Native/Expo 移动端代码库，先完整替代现有 SwiftUI/Compose 客户端；
2. 设备端 SQLite 可离线读取、增量同步并可靠重放用户命令；
3. 对在线设备，pi 分析 harness 在端上运行，模型密钥、配额裁决、链接代理和平台发送凭据仍留在
   服务端；设备不可用时保留显式、可观测的服务端 fallback；
4. Agent 的只读能力、草稿、审批和外部动作逐级开放，任何外发都不能由模型自行提权；
5. 只有在同步、设备撤销和密钥恢复经过独立安全评审后，才进入 E2EE 信封与服务端数据收缩阶段。

最终体验是：用户打开 App 即看到最新商机，断网仍可阅读并排队安全操作；在线设备优先完成分析，
跨设备状态一致；服务端只承担公网接入、身份/计费、同步/推送、推理网关及必须持有平台凭据的动作。

## 非目标

- 不在一个版本中同时切换 RN、事件溯源、端上 Agent、E2EE 和服务端数据删除。
- 不在 P0-P4 删除现有 PostgreSQL 业务表、Celery 分析管道或原生 App；旧路径是迁移回滚面。
- 不把本项目改造成通用 Agent 平台、动态插件市场、A2A 网络或微服务集群。
- 不向移动端开放 shell、任意文件、任意 HTTP、运行时下载代码或未经审核的 pi 工具。
- 不在第一版加入 BYOK、向量记忆、端侧小模型或 macOS 常驻节点；它们必须在核心闭环稳定后分别立项。
- 不把 Expo Push Service、EAS 云构建或 OTA 更新设为不可替代依赖；是否采用需单独记录运维与隐私取舍。
- 不声称 Telegram Bot/企微 webhook 对服务端“密码学不可见”：公网 relay 在接收瞬间必然接触明文。
  E2EE 阶段的可验证目标是正文不落服务端业务库/日志，进入同步存储前立即按设备密钥加密。

## 背景与代码核对结论

### 当前事实

- Web 使用 Next.js 16/React 19；`frontend/pnpm-workspace.yaml` 只覆盖自身，仓库根还不是 JS monorepo。
- iOS 与 Android 已有真实登录、看板、详情、设置、订阅和 API 客户端，不是空壳；两端都仍缺推送。
  iOS token 使用 Keychain service `com.codeiy.im.jwt`，Android 使用
  `radar_secure_prefs/access_token`，RN 换装必须迁移或明确要求重新登录。
- 后端以 PostgreSQL 业务模型为唯一真相，没有设备、推送、同步游标、领域事件日志或推理网关端点。
  `notify_reviewers` 当前为空实现，移动端仍靠轮询/刷新。
- 当前所谓 LiteLLM 是 Python 进程内 `ChatLiteLLM` 调用，不是可供 App 访问的 OpenAI 兼容代理。
- pi 0.80.6 由 Celery 启动受限 Node 子进程；唯一工具是 `submit_analysis`，Python 再做 Pydantic
  校验与 `project_agent_result` 确定性钳制。
- `usage_ledger` 的已接受语义是“一次顶层 pi 分析计一次”，不是“一次 provider 请求计一次”。
  端上运行不能静默改变 ADR-0004 的计费合同。
- `MessageRepository` / `OpportunityRepository` 多处自行 commit。若直接做跨聚合“事件双写”，会出现
  非原子或半完成事件；第一版同步必须采用可容忍中间态的变更日志，或先收敛事务边界。

### 对两份提案的必要修正

- 截至 2026-07-17，官方稳定矩阵是 Expo SDK 57 + React Native 0.86 + React 19.2.3；不使用提案中的
  “RN 0.8x / Expo >=53”模糊基线。所有版本仍以 P0 创建项目时的官方稳定矩阵和锁文件为准。
- 当前 pi 的 `getModel(provider, modelId)` 没有第三个 `baseURL` 参数；`Agent.streamFn` 接收的是 pi
  stream function，也不能直接传 `expo/fetch`。Expo 57 已把 `expo/fetch` 设为移动端全局 fetch，
  但 OpenAI SDK、pi import graph、SSE、abort 和 tool-call 在 Hermes 上仍必须用真机 release spike 证明。
- `pi-agent-core` 和 `pi-ai` 的 package engine 声明为 Node >=22.19；“主入口没有静态 `node:` import”
  不是 Hermes 兼容证明。P0 不通过时继续用服务端 runner，不阻塞 RN 客户端重构。
- 首选 `expo-sqlite` 做端上数据库：官方版本已支持事务、预编译语句和可选 sqlite-vec 扩展，可减少
  `op-sqlite + sqlite-vec` 的额外原生依赖。只有性能基准证明不满足要求时才通过 ADR 替换。
- P0-P4 的同步单元定义为“可版本化的服务端变更日志 + 客户端投影”，而不是立即把业务真相改成
  通用事件溯源。等客户端写入、冲突和恢复被生产数据验证后，再决定是否让事件日志成为权威。
- Python 服务端钳制与 TS 端上钳制无法天然共享同一个函数实现；共享的是 schema、规范化规则和
  golden fixtures。服务端对所有由其执行的动作仍以 Python 策略为最终权威。

## 迁移期间的权威边界

| 数据/动作 | P0-P2 | P3-P5 | P6（条件性） |
| --- | --- | --- | --- |
| 身份、设备撤销、订阅、套餐、用量 | 服务端权威 | 服务端权威 | 服务端权威 |
| 消息、商机、设置 | PostgreSQL 权威 | PostgreSQL 权威；SQLite 是可重建副本 | 通过独立 ADR 决定是否改为端上事件权威 |
| 增量同步 | 无 | 服务端游标变更日志 + 客户端幂等投影 | E2EE 信封中转，不存正文投影 |
| pi 分析 | 服务端 runner | 在线设备优先、服务端超时 fallback | 角色化设备/节点 + 显式托管 fallback |
| 模型密钥与配额 | 服务端 | 服务端推理网关/分析运行授权 | 服务端推理网关/分析运行授权 |
| Bot/企微发送 | 服务端适配器 | 服务端二次钳制后执行 | 服务端二次钳制后执行 |
| 用户 MTProto session | 服务端兼容路径 | 原样保留 | 有常驻节点后才可迁出 |

任何阶段都禁止两个权威同时接受同一写入而没有版本/幂等仲裁。阶段切换必须由服务端 capability flag
按用户/设备灰度，默认关闭，并能在不回滚数据库的情况下退回上一条运行路径。

## 总体验收标准

- [ ] RN App 使用原 bundle id/package name 升级现有安装；iOS/Android 登录凭据迁移或可解释地失效，
      账号切换和登出会清除 token、本地数据库、Agent session、outbox 与设备绑定。
- [ ] RN 功能对齐矩阵覆盖现有双端真实能力；原生 App 未删除前始终能从同一后端继续工作。
- [ ] 同步支持有界 bootstrap、单调游标、分页、幂等重放、tombstone、断点续传和 gap/reset；跨用户访问
      一律拒绝，推送 payload 只含事件类型/资源 ID/游标提示。
- [ ] 飞行模式可读取已同步的看板、详情、消息和 Agent 历史；允许的离线命令恢复联网后按幂等键重放，
      冲突显示给用户而不是静默覆盖。
- [ ] 在线设备认领的消息由端上 pi 完成分析；租约过期、App 被杀、网关超时或版本不兼容时服务端
      fallback 能恢复，同一消息不会双跑生效、重复扣额或产生冲突投影。
- [ ] 模型 provider key 不进 App；网关不记录 prompt/响应正文，限制 body、token、并发、速率、超时，
      支持取消并记录不含正文的 run/device/model/usage/latency 观测字段。
- [ ] 顶层分析仍按 ADR-0004 计一次；内部模型轮次只记成本指标。交互式 Agent 若收费，必须定义新的
      feature/套餐语义，不能复用分析额度而不写 ADR。
- [ ] 外部动作需要服务端校验的短期批准凭证，绑定 user/device/tool/canonical-args-hash/resource-version/
      expiry/idempotency-key；批准被拒、过期、重放或状态已变化时不得发送。
- [ ] 所有阶段都覆盖成功、拒绝、超时、重试、崩溃恢复、版本不兼容、owner 隔离和日志脱敏；生产能力
      不静默回退到 mock。
- [ ] 架构、功能地图、运行配置、验证命令、ADR、活动/完成计划与代码保持一致。

## 发布策略与全局安全阀

服务端 capability 建议至少包含：`rnClientSupported`、`syncAvailable`、`pushAvailable`、
`deviceAgentAvailable`、`agentToolsAvailable`、`hostedFallbackAvailable`、`e2eeAvailable`。能力必须由服务端
按用户/设备版本返回，不能只靠 App 本地开关。

每阶段遵循同一发布序列：

1. expand schema/API，旧客户端可继续使用；
2. 内部账号 + dev bundle id 真机验证；
3. 仅观测/shadow，不改变业务结果；
4. 1% → 10% → 50% → 100% 灰度，并设置错误率、重复扣额、重复发送、同步 gap、fallback 延迟阈值；
5. 至少跨两个 App 版本和一个约定观察期后，才 contract 旧字段/路径；
6. 任一阈值越界先关 capability，保留数据用于诊断，不做破坏性回滚。

## 分阶段实施

### P0：决策、契约和真机可行性闸门（约 1-2 周）

此阶段不得改变生产行为，是后续投入的硬前置。

- [x] 新增 ADR-0012：RN/Expo 单代码库，supersede 尚未 accepted 的 ADR-0006；记录同 bundle id 换装、
      dev bundle id、原生目录保留期、构建/OTA 边界。
- [x] 新增 ADR-0013：端上分析 + 服务端网关/fallback；明确 ADR-0002 继续约束服务端 fallback，
      ADR-0004 的顶层分析计量不变。
- [x] 新增 ADR-0014：先采用同步变更日志/客户端投影，不立即切事件溯源；定义游标、版本、tombstone、
      bootstrap、保留期和未来升级为端上权威的复审信号。
- [x] 制作 RN parity 清单：逐项对照 Web、SwiftUI、Compose、真实 API、成熟度与 mock 边界；锁定
      首发必须项和明确不迁移项。
- [ ] 创建最小 Expo 57 dev-client spike（独立 dev bundle id），在 iOS/Android release 构建验证：
      `expo/fetch` 流式读取、AbortController、App 前后台切换、崩溃恢复。
- [ ] 用固定 pi 0.80.6 + faux provider 在 Hermes 跑一次 `Agent.prompt → submit_analysis → terminate`；
      再经本地无付费模型的 SSE fixture 验证 tool-call 增量。记录 bundle import graph、polyfill、包体、
      TTI、内存和错误，不使用真实 provider key。
- [ ] 验证正确的 pi gateway adapter：不得使用提案示例中的第三个 `getModel` 参数或把 fetch 直接当
      `streamFn`；必要时使用自定义 `Model.baseUrl` + 审查过的 provider stream，或提交最小上游补丁。
- [ ] 对 `expo-sqlite` 做迁移、事务、并发、kill/reopen、万级事件 fold 基准；暂不启用 sqlite-vec。
- [ ] 做登录迁移 spike：iOS 按旧 service/account 读取 Keychain；Android 通过一次性 Expo Module 读取
      旧 EncryptedSharedPreferences 后写入新存储并删除旧值。覆盖升级、全新安装、错误密钥、登出。
- [ ] 决策闸门：若 pi/Hermes release 双平台任一不稳定，P1-P3 继续，P4 保持服务端 Agent；不得为
      “按计划端上”引入不受控 polyfill、Node runtime 或远程代码执行。

P0 退出证据：三份 ADR 状态明确；双平台 release spike 可复现；兼容性问题和 go/no-go 决策写回本计划。

P0 当前判定（2026-07-17）：

- 已完成三份 accepted ADR、功能对齐矩阵和 `mobile/radar` 独立 P0 工程；iOS/Android Release 均可构建。
- iOS Release 模拟器已实际跑通 Hermes/pi 结构化工具调用、10,000 条 SQLite change 重放、经本地 fixture
  的 `expo/fetch` UTF-8 分块读取与 AbortController 取消，以及全新安装无旧 token 的 SecureStore 路径。
- pi 0.80.6 的默认入口会经 `pi-ai` 加载变量动态 import，Metro 无法打包；当前 spike 只收窄到
  `pi-agent-core/dist/agent.js` 并提供最小 RN `pi-ai/compat` seam。该 seam 足以证明 `Agent` 核心循环，
  但尚未证明真实 provider/gateway stream adapter，因此 P4 暂不通过生产闸门。
- Android 自定义旧 token Expo Module 与 Release APK 已编译通过；本机没有 Android Emulator system image，
  也没有连接真机，Android 运行态明确未运行。
- 旧 token 的真实升级迁移必须使用 `com.codeiy.im`、商店同签名与备份真机；dev bundle id 无权读取生产
  App 沙箱。现阶段仅完成迁移事务单测、iOS 新装路径和 Android 原生实现编译。
- Go/no-go：P1-P3 可以继续；P4 维持 `deviceAgentAvailable=false`，直到真实网关 SSE adapter、双平台
  Hermes 运行、前后台/被杀恢复与同签名 token 升级矩阵全部通过。

### P1：Monorepo、共享契约与 RN 骨架（约 2-3 周）

- [x] 在根增加 `package.json`、`pnpm-workspace.yaml` 和唯一根 `pnpm-lock.yaml`，纳入 `frontend`、
      `mobile/radar`、`packages/*`；先保证 Web 构建零行为变化，再删除 frontend 局部 workspace/lock。
- [x] `backend/pi-agent-runtime` 暂保留 npm 精确锁和 Docker 构建入口；若消费共享本地包，明确
      `file:` 依赖、Docker COPY 顺序与锁文件复现。除非 ADR 批准，不顺手改成第二套包管理迁移。
- [x] 建立 `packages/radar-contracts`：后端 OpenAPI/JSON Schema 为契约源，生成/校验 TS 类型；CI
      检查生成物无漂移。先覆盖 auth/dashboard/detail/messages/settings/subscription，再扩展同步/Agent。
- [x] 建立 `packages/radar-core`：只放平台无关的枚举、事件 envelope、规范化、状态/信任/SOP 纯规则；
      不导入 React Native、Next、数据库或网络。
- [x] 建立 `packages/radar-api`：鉴权、错误映射、幂等 header、runtime schema 校验；Web 和 RN 逐端点
      迁移，禁止一次性重写全部 `frontend/lib/api.ts`。P1 以 auth 纵向切片完成 Web/RN 双端迁移与严格
      响应解码；dashboard/detail/messages 等端点随 P2 业务切片继续迁移。
- [x] 建立 `packages/radar-agent`：Analysis schema、system prompt、输入序列化、端上钳制镜像、golden
      fixtures；Node runner 与 RN 共同消费。Python Pydantic/策略仍在 CI 用相同 fixtures 做一致性测试。
- [x] 创建 `mobile/radar`：Expo Router、主题 token、错误边界、日志脱敏、auth/session、API 注入、
      secure storage、SQLite schema migration 和 capability client。真实密码登录/恢复/登出 shell 与
      owner-scoped inbox/projection/outbox schema 已完成；看板等业务路由进入 P2 纵向切片。
- [x] 增加 `make rn-check` 与 CI：frozen pnpm install、lint、tsc、unit、schema drift、Expo doctor、
      Android release assemble、iOS simulator build/test；缓存和 secrets 不进入构建产物。

P1 退出证据：Web、后端 runner、RN 同时消费共享契约且现有行为不变；根锁文件可复现；双平台认证壳
可构建，且 iOS 模拟器已安装并跑通登录/恢复/离线重试。以上证据已于 2026-07-17 满足，P1 退出。

### P2：RN 功能对齐与原生 App 换装（约 4-6 周）

- [x] 完成登录/恢复/登出纵向切片：共享严格契约、SecureStore、401/离线/重试、脱敏和 iOS Release 验收。
- [x] 完成在线看板纵向切片：共享严格契约、全量服务端筛选/排序/分页、Abort/竞态、
      loading/error/empty/retry、重大商机提示、无障碍语义和 iOS Release 验收。P3 前不读写 SQLite projection，
      避免把未交付的同步/离线副本描述成现有能力。
- [x] 完成在线详情/消息纵向切片：Web/RN 共用严格详情与分页消息契约；RN 覆盖 UUID 深链、并行首屏、
      Agent 发现、归档/404、incoming/outgoing、空态、加载更多、失败重试、无障碍与 iOS Release 验收。
      P3 前不读写 SQLite projection。
- [x] 完成回复/状态纵向切片：Web/RN 共享严格动作契约、通用持久化投递账本、稳定幂等键、真实 AI 草稿
      与模板、服务端行锁认领/状态流转、loading/error/retry/无障碍和 iOS Release fixture 验收；旧
      SwiftUI/Compose 仅补稳定幂等键兼容。
- [x] 完成设置/Telegram 纵向切片：Web/RN 共享严格 settings/Telegram 契约；RN 支持识别偏好、IANA
      时区与任意多段工作时间、通知能力文案，以及 Telegram health/连接/来源/quota pause 读取和启停；
      401 清会话、失败回滚、日志脱敏、loading/error/empty/retry、无障碍与 iOS Release fixture 已验收。
- [x] 完成订阅纵向切片：Web/RN 共用严格套餐/用量/管理契约；RN 使用官方 RevenueCat SDK，以认证
      UUID identify，支持 Offering、Free 用户购买、用户触发恢复、取消非错误、后端 sync、重复操作保护、
      原渠道管理、loading/error/retry、无障碍和脱敏。双平台 Release 构建通过；外部 Dashboard 商品与
      真实 Sandbox/Test Store E2E 仍是发布闸门，不把未配置状态描述成生产支付已开通。
- [x] 完成 Apple/Google 原生登录代码切片：RN 复用严格 native token 换站内 JWT 契约；Apple 使用系统
      登录控件，Google 的 Android 路径使用 Credential Manager、iOS 路径使用 GoogleSignIn 9.x，界面使用
      官方品牌资产。取消不显示错误，provider/native detail 不进入 UI 或日志，身份 token 不持久化；双平台
      Release 构建通过。真实 provider 账号、Dashboard/audience 和首次建号 E2E 仍属于发布闸门。
- [x] 完成中英文 UI 切片：类型化 `zh-CN`/`en` catalog 覆盖全部 RN 生产页面与安全错误，按系统首选
      语言选择并显式回退简体中文；iOS/Android 声明各自 BCP-47 支持语言，catalog key/插值一致性及
      生产 TSX 硬编码中文审计进入测试。双平台 production Release 构建通过；真机视觉与无障碍仍是 QA 闸门。
- [ ] 保留原生 App 只做安全/契约兼容修复；在 RN 达到内部 beta 前不删除 Swift/Kotlin，不再把新功能
      同时实现三遍。推送和端侧 Agent 只落 RN。
- [ ] 复用现有 bundle id `com.codeiy.im` / application id `com.codeiy.im` 发布升级包；开发/并行安装用
      后缀 ID。验证 Apple/Google OAuth audience、RevenueCat app user id/entitlement、深链和版本号。
- [ ] 执行 P0 的 token 迁移；迁移成功后删除旧 token，失败只要求重新登录，不得把密文写日志。
- [ ] 用现有 iOS/Android 实现和功能地图维护 parity matrix；移除 RN 中任何看似真实的 mock/timer。
- [ ] TestFlight/Play 内测灰度，记录 crash-free、冷启/TTI、看板滚动、详情/回复成功率；达到门槛后才
      用 RN 顶替商店版，保留上一个原生版本的服务端兼容窗口。

P2 退出证据：RN 覆盖现有真实移动能力并可原位升级；原生 App 可回退安装；尚未声称离线/推送/端上 Agent。

#### P2 回复/状态切片安全契约

本切片包含真实 IM 外发，按高风险变更处理：

- 重复点击、网络超时或进程在 provider 成功后崩溃，可能造成重复外发。新增持久化手动回复投递账本，
  幂等键绑定 owner、商机和正文哈希；`SENDING` 状态不自动重发，provider 已成功但本地投影未完成时
  从 `DELIVERED` 恢复消息/状态而不再次联系对方。
- 同一幂等键换正文/商机、并发认领、非法状态迁移、归档写入和非 owner 访问必须拒绝；客户端禁用
  重复提交只是体验层，服务端唯一约束、条件更新和状态机才是最终边界。
- `IM_SEND_ENABLED=false` 时必须显式失败，不得写入看似已发送的消息或状态；AI 未启用或 provider
  失败时不得以本地 timer/固定文案冒充 AI 草稿，也不得把 provider detail、消息正文或 token 写日志。
- provider 超时可能发生在远端已接受之后，因此投递记为 `UNCERTAIN` 并返回 502；同一键永久禁止自动
  重发，必须由操作员核验。仅明确的本地发送禁用记为可重试 `FAILED`，恢复开关后才允许同键重试。
- AI 只产生可编辑草稿，不触发发送；模板只读选择不授予外部动作。所有外发仍由用户点击发送，
  本地 fixture 只用于 dev bundle 的确定性验收。

验收标准：同键同正文重复请求最多外发一次；同键异正文、发送中、归档、非法迁移和 claim 冲突返回
稳定 409/422；provider 禁用/失败不伪造消息或状态；成功后详情与分页消息来自服务端真相；401 清会话；
Web/RN/旧双端使用稳定幂等键；新结果端点返回 outgoing Message 与精确消息总数；日志只含错误类别/
HTTP 状态。回滚时可先关闭客户端写入口或保持
`IM_SEND_ENABLED=false`，新增账本与响应字段为 additive，旧客户端数组/请求体继续兼容，表数据保留审计。

### P3：设备身份、推送、增量同步与离线副本（约 4-6 周）

- [x] 先建 `Device`/`DeviceCredential`/`PushRegistration`/`SyncChange` 模型与 Alembic expand 迁移；
      token 只存哈希或平台要求的最小值，支持轮换、撤销、最后活跃和 capability/version。
- [x] 为后台同步设计可撤销的设备会话/refresh token 轮换与复用检测；不能长期依赖当前 7 天 access
      token。认证成功、撤销、过期、丢机、跨用户和日志脱敏路径均需测试。
- [x] 定义同步契约：每用户单调 cursor、event id、aggregate type/id/version、operation、schema version、
      createdAt、payload/tombstone；所有列表有上限，客户端必须能识别 schema 不兼容并触发 reset。
- [x] 先覆盖 Message、Opportunity 与用户设置；模板因只服务在线外部动作不进入首批离线必要数据。每次
      业务写在同一数据库事务内追加变更记录；
      对现有多 commit 用例，发送 aggregate upsert 事件并允许中间态，客户端按版本幂等覆盖，不假装是
      原子领域事件。必要的事务收敛单独切片，不横向重写全部 repository。
- [x] 新增 bounded bootstrap snapshot、`GET /sync/changes?after=`、ack/observability；测试并发游标、分页、
      重放、乱序、重复、删除、过期 cursor、恢复和 owner 隔离。
- [x] RN SQLite 保存原始 change inbox、物化表、sync cursor、command outbox；应用变更和推进 cursor 在
      同一事务。账号切换清库；数据库损坏可从服务端 bootstrap 重建。
- [x] 第一批离线命令只含可证明幂等的内部状态操作；外部回复/好友动作默认不离线自动重放。命令包含
      idempotency key、base version 和过期时间，409 冲突进入用户可见队列。
- [x] `expo-notifications` 仅负责客户端 native APNs/FCM token 和交互；服务端保持直连 APNs/FCM 的
      可替换 adapter，payload 不含正文。token rollover、失效回收、权限拒绝和深链均需覆盖。
- [ ] 推送只作为“有新游标”的提示，丢推送仍能在前台/后台补同步；在 gap/延迟指标达标后逐步移除
      30 秒轮询，保留手动刷新和 capability 回退。

P3 退出证据：飞行模式可读，断网内部命令可安全恢复；推送丢失不丢数据；PostgreSQL 仍是唯一业务权威。

### P4：推理网关与端上 pi 分析（约 4-6 周）

- [x] 定义“分析运行”资源，而不是让 App 用普通 JWT 任意调用模型：claim 时校验 owner、message 状态、
      device capability 和套餐，原子预留一次 usage，返回短期 run token/lease/model alias。
- [x] 增加 claim/heartbeat/complete/fail/expire API；complete 接受结构化结果，服务端重新做 Pydantic、
      `project_agent_result`、状态/version/lease 校验后才投影并 consume；失败/超时 release。
- [x] 构建 FastAPI 内的最小推理网关模块，不先拆微服务。v1 只支持 spike 已验证的 OpenAI-compatible
      streaming contract/model alias；限制 request/response、并发、速率、超时，转发取消，强制 provider
      `store=false`（若支持），禁止 prompt/响应日志。多 provider 只在契约测试通过后增加。
- [x] 网关记录 provider request/token/cost/latency 作为成本维度，但同一个 analysis run 只消费一条
      `usage_ledger`。交互式 Agent 调用不得混入该额度。
- [x] 提供受 run token 约束的链接 fetch 代理，复用 `SafeLinkInspector` 的逐跳 SSRF、大小、内容类型、
      超时规则；端上默认不直连不可信链接。
- [x] RN 首先只运行现有单工具 `submit_analysis`；无 session、无外部动作工具。持久化 run 状态，App
      重启可 fail/续租；后台限制导致无法完成时让 lease 过期并由服务端接管。
- [ ] 先 shadow 对比端上/服务端结果，不双重生效、不双重扣额；再按设备灰度“端上优先”。所有结果带
      `executedBy`、runtime/schema/model/policy version，便于回放和回滚。默认关闭的实现已完成：shadow
      可独立运行，primary 按稳定设备 cohort 和同版本 shadow readiness 开门；真实流量灰度证据未完成。
- [ ] 服务端 Celery fallback 在观察期内保持完整；只有端上成功率、P95 时延、重复运行、quota 漂移和
      结果一致性达标后，才从即时执行改为 SLA 延迟兜底。代码已用同一 job + 领取窗口实现条件延迟并在
      active run 时拒绝接管；生产开关和真实指标仍未开放。

P4 退出证据：活跃兼容设备的新分析端上优先，设备失联可恢复；provider key 不进 App；配额与投影无漂移。

### P5：Agent 对话、领域工具、审批与本地记忆（约 4-6 周）

- [x] 先定义交互式 Agent 的产品边界、独立用量 feature、30 天保留期和隐私说明；未决定计费前仅内部 beta。
- [x] SQLite 保存 session/entry；与同步业务事件分表、分版本，不直接把 pi JSONL 当整个领域事件模型。
- [x] 完成本地只读 v1 与内部工具 v2：搜索/详情/消息、本地未发送草稿、幂等 status outbox 和认证在线
      claim 都有独立 capability、schema、超时、取消、审计与拒绝测试。
- [x] 外部发送仅开放一个 v3 `send_reply`；已完成批准凭证、canonical args hash、资源版本与服务端二次复核。
- [x] 端上 `beforeToolCall` 做白名单/会话 capability/用户审批；服务端动作端点再次检查批准凭证、owner、
      状态机、归档状态、IM adapter 能力、`IM_SEND_ENABLED` 与幂等键。模型输出不能构造批准凭证。
- [x] 审批 UI 展示目标、正文、渠道、风险、有效期和可编辑内容；用户编辑后以编辑值重新计算 args hash。
- [ ] `remember/recall`、sqlite-vec、联系人/日历原生模块均后置到独立隐私评审；默认不把聊天正文写入
      向量索引，也不做运行时动态 extension 下载。

P5 退出证据：Agent 可对话并使用审查过的领域工具；外部动作只有明确用户批准且服务端复核后执行一次。

### P6：端上权威、E2EE 信封、常驻节点与服务端收缩（条件性，至少 8-12 周）

只有 P3-P5 稳定且完成独立威胁模型/隐私评审后才创建子计划；本阶段当前不是已承诺能力。

- [ ] 写 E2EE/设备信任 ADR：设备加入、跨 iOS/Android 密钥分发、恢复码、丢机撤销、密钥轮换、历史
      重加密、备份、元数据泄露、服务端 relay 瞬时明文和客服恢复边界。禁止只写“平台钥匙串同步”。
- [ ] 先做密文信箱 shadow：relay 收到 webhook 后在内存规范化并按设备公钥扇出，持久层只存 envelope
      metadata/ciphertext/TTL；证明消息可在离线设备最终取得、撤销设备无法取得新事件。
- [ ] 通过 expand/migrate/contract 把业务读写从服务端投影迁到端上事件日志；明确多设备冲突、命令仲裁、
      快照、compaction、恢复与 schema 升级。未证明前不得删除 Postgres 正文。
- [ ] 增加可选 macOS/CLI 常驻节点后，才迁移 MTProto session 和长任务；凭据在节点安全存储，服务器
      旧 session 迁移需用户重新授权或经过审计的导出流程。
- [ ] 服务端按职责重组为 relay/gateway/sync/id 模块但保持单一部署，先停止写旧投影，再只读观察，最后
      在备份、导出、回滚窗口和数据删除审批完成后删除明文业务表/旧 Celery 管道。
- [ ] Web 只在移动端/节点覆盖率满足产品阈值后降级为控制台；不能让仅使用 Web 的现有用户突然失去能力。

P6 退出证据：持久化服务端不能读取同步正文；跨平台设备加入/撤销/恢复可演练；旧业务库和任务管道有
可审计的数据迁移、删除与回滚记录。

## 影响面与风险

| 影响面 | 主要变化 | 首要风险/控制 |
| --- | --- | --- |
| 仓库/构建 | 根 pnpm workspace、RN dev client、共享包、双平台 CI | 锁文件/Docker 漂移；P1 先保持 Web 零行为变化 |
| 移动端 | 原生双端换装、SQLite、推送、Agent | token/RevenueCat/深链升级回归；dev ID + 内测灰度 |
| API/Auth | 设备会话、同步、分析运行、网关、批准凭证 | token 盗用/重放；哈希、轮换、撤销、速率限制 |
| 数据库 | Device/SyncChange/AgentRun/Approval 等新表 | 大表回填、游标竞态、双写遗漏；expand/contract + DB 集成测试 |
| Agent | Hermes 运行、版本化 schema/policy、fallback | Node 假设、后台中断、双跑；P0 硬闸门 + lease |
| 计费 | run 级预留，provider 请求级成本 | 重复扣额/逃逸调用；短期 run token + 账本不变量 |
| IM/外发 | server 复核批准后执行 | 重放/状态变化/重复发送；args hash + version + idempotency |
| 隐私/E2EE | 密文信箱、设备密钥、正文删除 | 丢钥、跨平台恢复、错误承诺；独立 ADR/评审/演练 |
| 运维 | capability 灰度、push、网关指标、fallback SLA | 缺少现成观测闭环；每阶段先补最小指标和告警 |

最大未知量是 pi/Hermes 兼容性、当前 repository 的事务颗粒度、后台设备会话、跨平台 E2EE 密钥恢复。
这些未知量分别在 P0、P3、P3、P6 解决，不允许把风险推迟到商店换装或数据删除时才暴露。

## 验证矩阵

| 阶段 | 必跑检查/场景 | 完成证据 |
| --- | --- | --- |
| 所有阶段 | `make harness-check` + 受影响最小单测 | 命令、退出码、测试数写入验证记录 |
| P0 | iOS/Android release spike、Hermes faux/SSE/tool/abort、token upgrade | 真实设备/模拟器矩阵与基准报告 |
| P1-P2 | `make check`、`make rn-check`、iOS/Android build/test、Web production build | frozen lock、双平台产物、parity matrix |
| P3 | Alembic upgrade/downgrade/upgrade；Postgres 并发/owner/gap/reset；飞行模式/kill/reopen | 临时 PostgreSQL + 双真机 push/sync 演练 |
| P4 | faux gateway、SSE cancel、lease 竞态、重复 complete、quota reserve/consume/release | 无真实 key 单测 + 授权隔离环境 smoke |
| P5 | 审批拒绝/过期/篡改/重放、归档/状态变化、IM 发送幂等 | `IM_SEND_ENABLED=false` 自动测 + 授权沙箱 smoke |
| P6 | 设备加入/撤销/恢复/轮换、密文落库审计、旧库导出/删除/恢复 | 威胁模型签字 + 恢复演练记录 |

未实际运行的检查必须写“未运行”与原因；真实 APNs/FCM、RevenueCat、Telegram/企微、App Store/Play
只在用户授权的隔离环境冒烟，不使用生产消息或真实用户数据作 fixture。

## 回滚与恢复

- P0-P2：删除/关闭 RN dev 路径即可；服务端和原生 App 无行为变化。商店换装后保留上一个原生版本的
  API 兼容，必要时提交回退二进制。
- P3：关闭 `syncAvailable/pushAvailable`，RN 回到 REST + 手动刷新/轮询；SyncChange/Device 表保留，
  不阻塞旧客户端。数据库 downgrade 只在确认无新客户端依赖后执行。
- P4：关闭 `deviceAgentAvailable`，未完成 lease 到期并 release，Celery fallback 恢复即时执行；网关
  路由不可用时不得把 provider key 下发客户端。
- P5：关闭 `agentToolsAvailable`，保留只读 session；撤销未执行批准凭证，服务端动作 API 继续 fail closed。
- P6：采用 expand/migrate/contract；旧明文投影只在密文路径稳定且备份/恢复演练完成后删除。删除后若
  不能安全降级，以前向恢复为主，计划中必须记录数据不可逆点和人工批准。

## 进度日志

- 2026-07-17：完成 P5 内部只读交互 Agent 代码切片。新增 SQLite v5 本地 session/entry、共享三项只读
  工具、独立 turn usage/lease/purpose token、无正文 provider 审计和受限 SSE gateway；RN pi host 仅在
  用户提交时加载，完整 tool pair 有界回放，流式瞬态局部更新，最终 entry 批量落库后才 complete。
  Agent Tab、i18n、取消/失败/删除与生产默认关闭的 capability/部署门禁已接入。自动化使用 faux provider、
  临时 PostgreSQL 和双平台 Hermes export；真实 provider、双真机/同签名升级和 allowlist 灰度仍未运行，
  draft、内部写、批准凭证、外发和记忆继续留在后续独立高风险切片。
- 2026-07-18：完成 P5 S5 内部工具切片。schema v2 只增加 local-only draft、queued status outbox 与
  authenticated claim；客户端最高支持版本与服务端实际 turn 版本分离，v1 可回退。部署只接受两组审核过的
  schema/policy，生产默认仍为 v1 且 beta/gateway/额度/allowlist 关闭；外发继续不存在。
- 2026-07-18：完成 P5 S6 单次批准外发实现。schema v3 只增加 `send_reply`；UI 编辑值绑定无正文 hash 与
  短期 purpose token，服务端复核版本/状态/设备/门禁后复用 ManualReplyDelivery，并发/不确定结果 fail
  closed。生产仍保持 v1、external-actions=false、IM send=false，真实 provider/IM 沙箱/双真机待发布验收。
- 2026-07-17：完成 P2 Apple/Google 原生登录代码与双平台构建。共享 auth API 新增严格、有界的 native
  id_token 请求；RN 会话层只把 provider token 交换为站内 JWT，后者继续走既有 SecureStore 事务，前者
  不持久化、不记录。Apple 使用系统按钮和 state 关联；Google 新增最小 Expo Module，Android 使用
  Credential Manager `GetSignInWithGoogleOption`，iOS 使用 GoogleSignIn 9.2，并由 config plugin 写入
  client ID/URL callback。Google 界面使用官方深色 pill 资产。iOS Release 已安装并完成登录页视觉核对，
  Android Release APK 已打包；仅用格式合法的假 client ID 做构建，未触发真实登录、未使用真实账号。
- 2026-07-17：完成 P4 S1-S5.5 与 S6 本地验证。analysis run、受限推理/链接网关、RN Hermes 恢复、
  shadow/readiness/稳定设备 cohort/primary 领取窗口/fallback/expire 和无正文执行来源均已实现，生产
  开关默认关闭。PostgreSQL 010 fresh upgrade/downgrade/re-upgrade、后端 272 tests、RN 126 tests、
  共享 API 54 tests 和 clean 双平台
  production Release 通过；产物均为 `com.codeiy.im`、`0.2.0 (2)`。真实 provider、双真机 kill/reopen、
  同签名升级和逐设备灰度仍是外部门禁，不能由构建或 fake 替代。
- 2026-07-17：增加 RN 显式 build variant。默认 development 保持 `com.codeiy.im.dev` 与本机 fixture
  例外；production 恢复既有双平台 `com.codeiy.im` 和 `opportunity-radar` deep link，强制显式 semver/
  build number，并移除 iOS ATS 本地例外和 Android cleartext。单测与 Expo config 同时验证 dev/prod
  身份和缺版本 fail closed；clean 模拟器首次启动发现 production 缺 API origin 仍可生成不可用产物，
  随即把严格 HTTPS `EXPO_PUBLIC_API_BASE_URL` 也提升为构建期必填。CI 的双平台 Release job 以 `.test`
  origin/占位版本构建 production 变体。商店同签名覆盖安装、真实版本/API、OAuth/RevenueCat 与旧 token
  读取仍未因此完成。
- 2026-07-17：完成 RN 中英文 UI。新增类型化 `zh-CN`/`en` catalog、系统 locale provider 和显式中文
  fallback；auth、看板、详情/消息、设置/Telegram、订阅、错误与无障碍标签全部迁移。测试检查 catalog
  key/插值参数并扫描生产 TSX 的硬编码中文；Expo 原生语言声明生成 iOS `en`/`zh-Hans` 与 Android
  `en`/`zh-CN`。production Android Release APK 和无签名 generic iOS device Release 均通过；本次 iOS
  simulator 构建被本机 CoreSimulator 的 Swift 系统库策略拒绝，未作为代码失败或视觉通过记录。
- 2026-07-17：完成 P2 订阅纵向切片代码与原生构建。`radar-contracts` / `radar-api` 新增 subscriptions
  直接子路径、TypeBox 严格有界解码和跨字段不变量，Web 删除最后一组手写订阅网络契约；RN 接入官方
  `react-native-purchases` 10.2.0，使用平台 Public Key、认证用户 UUID、指定 Offering 和服务端目录映射，
  支持只读用量、购买、恢复、取消、同步、原渠道管理以及重复付费/付款问题/周期末取消提示。fixture
  只提供 Free 目录/用量并让 sync 明确 503，不模拟 Offering 或交易成功。iOS/Android Release 均完成
  RevenueCat 原生编译；外部 Dashboard、商店商品、Sandbox/Test Store 账号与真实交易 E2E 未配置。
- 2026-07-17：完成 P2 设置/Telegram 纵向切片。`radar-contracts` / `radar-api` 新增 settings 与 Telegram
  直接子路径、TypeBox 严格有界解码、IANA/时段/关键词写入规范化；Web 删除对应手写网络契约，RN 新增
  设置中心、识别偏好、任意多段工作时间、通知偏好与 Telegram 健康/连接/来源管理。所有写入只以服务端
  响应替换真相，失败保留或恢复旧值；401 清会话，日志不记录服务端/provider detail。iPhone 17 Pro Max
  iOS Release dev 包以固定 `.test` fixture 验收设置导航、周一双时段、推送未开放文案、quota pause 与
  Telegram 启停；未连接生产 API、真实 Telegram 或用户数据。
- 2026-07-17：完成 P2 回复/状态纵向切片。新增通用 `manual_reply_deliveries` Alembic 表与 owner-scoped
  投递状态机，覆盖同键恢复、异文冲突、并发发送、明确禁用重试和 provider 结果不确定冻结；新
  `/manual-reply/result` 返回服务端商机、outgoing Message 与精确消息总数，旧端点保持兼容。Web/RN
  迁移到共享严格 actions/templates API，AI 草稿、模板、认领和状态均使用真实服务端结果；SwiftUI/
  Compose 只补失败期间稳定幂等键。临时 PostgreSQL 16 验证迁移 upgrade/downgrade/upgrade、通用账本
  与并发认领；iPhone 16e 的 iOS Release dev 包以本地内存 fixture 验证模板、AI 草稿、认领、状态和
  回复闭环，消息计数从 20/55 精确更新到 21/56，未连接生产 API 或真实 IM。
- 2026-07-17：完成 P2 在线详情/消息纵向切片。新增 owner 隔离的 `GET /messages/page`，单页上限 200；
  旧数组端点改为最近 500 条兼容窗口。共享 `radar-api` 对详情与消息做严格、有界解码，Web 独立加载
  详情并支持每 200 条继续分页；RN 详情深链并行首屏、每 20 条分页，展示归档、Agent 发现、既有草稿
  与双向消息，覆盖非法 UUID、404、过期响应、去重、空态和分页失败。iOS Release 模拟器使用本地
  fixture 验证详情与长消息滚动；服务下线重试、空态和加载更多证据在本切片最终复跑中记录。
- 2026-07-17：完成 P2 在线看板纵向切片。`radar-api` 新增 opportunities/dashboard 严格 runtime decoder
  与确定性查询序列化，Web 商机列表迁移到同一共享边界；RN 接入服务端全量筛选、排序、分页、重大商机
  提示、下拉刷新、Abort/过期响应抑制和 loading/error/empty/retry。iOS Release 模拟器验证三条富卡片、
  高级筛选/筛选计数、单条结果、组合空态、fixture 下线失败态及恢复后重试；登录原生存储错误统一脱敏。
  当前看板仍是在线 REST，SQLite 只保留 P3 schema 基础；Android 运行态和商店同 ID 升级仍未验证。
- 2026-07-17：完成 RN 首个真实纵向切片：共享 auth API 严格解码，Web OAuth callback 先验证再持久化，
  RN 密码登录、SecureStore 回读、会话恢复、离线保留、登出清理和版本化 SQLite schema。iOS Release
  模拟器通过本地假账号完成登录，强制终止后恢复，fixture 下线时 fail closed 且保留 token，恢复后重试
  返回首页；旧凭据先销毁再接受新 token，登出也按 legacy → current 顺序清理，避免残留 token 在下次
  启动复活。最终 iOS/Android Release 均重新构建通过。真实生产账号/后端未用于验证；P1 据此退出。
- 2026-07-17：完成 P1 第一批基础设施：根 pnpm workspace/唯一 lock、OpenAPI 契约生成与漂移检查、
  `radar-core`/`radar-api`/`radar-agent`、Node runner 的共享 Agent 契约、Expo Router 骨架、fail-closed
  capability、session/token 恢复、日志脱敏及双平台 CI。Router 版 iOS/Android Release 本地构建通过；
  `radar-api` 的 Web 端点迁移、RN 生产 SQLite schema 与真实业务页面仍未完成，P1 尚未退出。
- 2026-07-17：完成 P0 的 ADR、RN parity matrix 与 Expo/Hermes spike；双平台 Release 构建通过，iOS
  模拟器运行通过 pi、SQLite、真实 SSE/abort 和全新安装 token 路径。Android 运行态与同签名升级 token
  迁移因缺少对应设备/签名条件保留为闸门；P1-P3 可继续，P4 暂不开放。
- 2026-07-17：阅读两份方向提案、仓库入口、架构/功能/安全/测试/运维文档，并核对当前移动端、
  pi runtime、Agent 用例、数据模型、配额、构建与 CI；创建本总计划。下一步是评审 P0 范围，
  然后只执行 ADR 与可行性 spike。

## 发现日志

- 2026-07-17：两份方向输入是用户未提交的新文件，本计划不修改其内容，只记录实施修正。
- 2026-07-17：现有 RN 提案代码片段与固定 pi 0.80.6 API 不完全匹配：`getModel` 无 baseURL 参数，
  `streamFn` 也不是 fetch；Hermes 兼容必须从“判断”升级为 release 构建证据。
- 2026-07-17：Expo 官方当前稳定矩阵已到 SDK 57/RN 0.86；SDK 57 的 `expo/fetch` 是移动端全局 fetch，
  monorepo/pnpm 受官方支持；版本和 Metro 配置应按当前官方方式生成，不能照抄旧 SDK 53 方案。
- 2026-07-17：Expo SQLite 已有可选 sqlite-vec 扩展，优先使用官方存储可减少一个高风险原生依赖。
- 2026-07-17：当前后端没有 LiteLLM proxy、设备 refresh token、push 或 sync；它们是新能力，不能被
  描述成“移动调用点搬家”。
- 2026-07-17：现有 iOS/Android token 存储实现不同；iOS 可按 Keychain service 兼容读取，Android
  需要一次性原生迁移或重新登录策略。
- 2026-07-17：E2EE 的跨 iOS/Android 密钥加入/恢复不能只依赖平台钥匙串；这是 P6 的独立设计问题。
- 2026-07-17：pi 0.80.6 的默认 import graph 会从 `pi-ai` 认证模块触发变量动态 import，Metro 双平台
  bundle 均失败；收窄到 `Agent` 实现并替换最小 compat 后可在 Hermes Release 运行。P1 应把 seam 封装在
  `radar-agent` adapter，不能让业务代码依赖 Metro alias。
- 2026-07-17：`expo-modules-jsi@57.0.3` 在 Xcode 26.3 / Swift 6 C++ interop 中对 `abs` 解析歧义；P0
  以一行 `Swift.abs` pnpm patch 解除构建阻塞，升级依赖时必须先验证并移除补丁。
- 2026-07-17：Node 22 单测支持 `Array.prototype.toSorted`，当前 Hermes/RN 0.86 Release 不支持；SQLite
  migration 首次模拟器运行因此在建表前回滚。已改为 `Array.from(...).sort(...)` 并以原生数据库表、
  migration marker、WAL 与 integrity check 作为回归证据，不能只依赖 Node 测试判断 Hermes 兼容。

## 决策日志

- 2026-07-17：采用“RN 先换装 → 服务端变更日志/端上副本 → 端上分析 → Agent 工具 → 条件性 E2EE”
  顺序。原因是每步可独立交付、可回滚，并避免同时出现客户端、数据权威和安全边界三重切换。
- 2026-07-17：P3 前不把事件日志设为业务真相。先用 aggregate change feed 验证同步语义，避免在
  repository 事务尚未收敛时制造不可恢复的伪事件溯源。
- 2026-07-17：端上分析与交互式 Agent 分开计量/发布。前者延续 ADR-0004，后者需要新的产品和计费决策。
- 2026-07-17：服务端 fallback、Python 二次钳制和 IM 发送安全阀在迁移全程保留；端上成功率达标只会
  改变默认执行者，不会自动扩大 Agent 权限。
- 2026-07-17：P0 条件性放行 P1-P3，P4 保持关闭。原因是核心 `Agent` 已在 iOS Hermes 运行，但真实
  gateway/provider stream 与 Android Hermes 运行尚无证据；不得把最小 compat seam 描述成完整 pi 支持。
- 2026-07-17：P4 S1-S2 已建立默认关闭的 analysis run 与受限 Chat Completions SSE 网关。run token
  绑定 purpose/owner/device/run，单条 ledger 与 active run/provider stream 由 PostgreSQL 约束；网关只接
  model alias 和 `submit_analysis`，服务端注入真实 provider 配置并做无正文成本审计。当前证据来自本地
  fake provider 和 PostgreSQL，尚不满足真实 provider、RN adapter、Android Hermes 与生命周期生产闸门。
- 2026-07-17：P4 S3-S5 完成 run-bound SafeLink、RN Hermes adapter/恢复以及默认关闭的 shadow/fallback
  仲裁。shadow 复用 server consumed ledger 且只记录投影差异；primary fail/expire 提交 release 后才用稳定
  键调度 Celery，Message 保存无正文执行来源。PostgreSQL 约束/并发测试证明不双投影、不新增 shadow
  配额和单 fallback reservation；真实 provider、双真机 kill/reopen 与按设备灰度仍是生产门禁。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make check` | 通过（2026-07-17，中英文 UI 后最终复跑） | harness 52 个 Markdown/81 个后端 Python 文件；后端 145 passed/29 skipped；独立 Node runner 4 tests；共享 API 9 files/34 tests；Web production build；OpenAPI drift；RN 23 files/65 tests 与双平台 Expo export 全部通过 |
| `make harness-check` | 通过（2026-07-17） | 52 个 Markdown 文件链接、81 个后端 Python 文件；harness 回归 8 tests passed |
| P4 S5 `make harness-check` | 通过（2026-07-17） | 54 个 Markdown 文件、102 个 backend Python 文件与 8 个 harness 单测通过 |
| P4 S5.5 `make backend-check`（临时 PostgreSQL 16） | 通过（2026-07-17） | 272 passed；010 fresh upgrade/downgrade/re-upgrade，AnalysisRun PostgreSQL 17 tests；无本次 model drift，仅保留既有 FK/Text/index 漂移 |
| P4 S5.5 `make rn-check` | 通过（2026-07-17） | OpenAPI/shared 无漂移；radar-api 12 files/54 tests，RN 38 files/126 tests；iOS/Android Hermes production export 通过 |
| P4 S5 clean 双平台 production Release | 通过（2026-07-17） | iOS simulator `xcodebuild` exit 0，118 MB App 含 `main.jsbundle`；Android 905 tasks，`BUILD SUCCESSFUL in 4m 52s`，104.1 MB APK 含 Hermes bundle；两端 manifest/plist 均为 `com.codeiy.im`、`0.2.0 (2)` |
| P5 S5 `make check` | 通过（2026-07-18） | backend 221 passed/76 skipped；RN 43 files/145 tests；OpenAPI/shared/Web/pi runner/双平台 Hermes export/harness 全部通过；PostgreSQL 目标 9 tests |
| `pnpm install && pnpm typecheck && pnpm test`（`mobile/radar`） | 通过（2026-07-17） | pnpm 10.25 frozen 基线；4 个测试文件，8 tests passed |
| `expo install --check` + 双平台 `expo export` | 通过（2026-07-17） | 依赖矩阵无漂移；iOS/Android Hermes bundle 均为 2.7 MB |
| `expo prebuild --clean` | 通过（2026-07-17） | iOS Pods 安装成功；Android 自动链接 `radar-legacy-token` |
| iOS Release simulator build + launch | 通过（2026-07-17） | P0.app 56 MB；iOS 17 Pro 模拟器输出 pi `callCount=1`、SQLite `10000/10000 ready`、SSE `第一条/second`、`aborted=true`、token `no-legacy-token` |
| Android `:app:assembleRelease` | 通过（2026-07-17） | 552 tasks，48 秒；APK 74 MB；`radar-legacy-token:compileReleaseKotlin` 通过 |
| `make backend-check` | 通过（2026-07-17） | compileall/ruff 通过；145 passed、29 skipped，1 条上游弃用 warning；包含通用回复账本、AI fail-closed、适配器禁用、详情/消息 owner 隔离与分页路由测试 |
| `make frontend-check` | 通过（2026-07-17） | ESLint/typecheck、3 files/7 tests、Next.js production build 通过 |
| `make rn-check` | 通过（2026-07-17） | 由最终 `make check` 调用；根 frozen install、OpenAPI drift、共享 core/API/Agent、RN 23 files/65 tests、双平台 Expo export 通过；原生登录、build variant 与中英文 catalog/copy audit 均纳入复跑 |
| `pnpm --dir packages/radar-api check`（P2 设置/Telegram） | 通过（2026-07-17） | 8 files / 28 tests；严格 settings/Telegram 响应、关键词/IANA/跨午夜、重复 ID、能力字典和写请求契约 |
| `pnpm --dir mobile/radar typecheck && test`（P2 设置/Telegram） | 通过（2026-07-17） | 17 files / 43 tests；追加设置服务端真相/失败回滚与 Telegram 启停 reducer，fixture 脚本语法及路由 smoke 通过 |
| RN 设置/Telegram iOS Release simulator | 通过（2026-07-17） | iPhone 17 Pro Max；Release native build 0 errors/4 warnings；固定 `.test` fixture 验证设置导航、3 段工作时间、推送 capability、Telegram health/quota pause 与真实响应启停；未连接生产 API/IM |
| `pnpm --dir packages/radar-api check`（P2 订阅） | 通过（2026-07-17） | 9 files / 32 tests；严格套餐/用量/管理响应、重复/错配 package、跨字段用量、HTTPS 管理入口和无 body sync 契约 |
| `pnpm --dir mobile/radar typecheck && test`（P2 订阅） | 通过（2026-07-17） | 19 files / 46 tests；追加 RevenueCat package 白名单、购买/恢复互斥和取消非错误 reducer；fixture 三个只读订阅路由 smoke 通过，sync 明确未配置 |
| P2 订阅双平台 Expo export | 通过（2026-07-17） | `expo install --check` 无漂移；iOS 1893 modules / 5.2 MB HBC，Android 2030 modules / 5.5 MB HBC，均包含官方 RevenueCat SDK JS 边界 |
| P2 订阅 iOS Release simulator build | 通过（2026-07-17） | iPhone 17 Pro Max；RevenueCat/PurchasesHybridCommon/RNPurchases、隐私清单、Hermes bundle 完成编译、链接、签名、安装与启动，0 errors/4 warnings；Mac 锁屏导致本次订阅页面视觉核对未运行 |
| P2 订阅 Android `:app:assembleRelease` | 通过（2026-07-17） | 2030 modules、RevenueCat Kotlin/Java、四 ABI CMake、lintVital 与 Release APK 打包通过；868 tasks，`BUILD SUCCESSFUL in 4m 31s`；首次依赖下载遇 TLS 握手中断，重启 Gradle daemon 后通过；运行态未运行 |
| `pnpm --dir packages/radar-api check`（P2 原生登录） | 通过（2026-07-17） | 9 files / 34 tests；追加 Google/Apple native endpoint、精确 request body，以及空白、超长、CRLF token 在发请求前失败 |
| `pnpm --dir mobile/radar typecheck && test`（P2 原生登录） | 通过（2026-07-17） | 20 files / 52 tests；追加 provider 配置、Apple state 关联、Google 原生结果白名单、取消、错误脱敏和单动作互斥边界 |
| P2 原生登录 Expo config/prebuild/export | 通过（2026-07-17） | `expo install --check` 无漂移；假 client ID introspect 生成 Apple entitlement、Google client/server ID 与反向 URL scheme；Pods 安装 GoogleSignIn 9.2/AppCheckCore；iOS 1904 modules / 5.2 MB HBC，Android 2041 modules / 5.5 MB HBC |
| P2 原生登录 iOS Release simulator | 通过（2026-07-17） | iPhone 17 Pro Max；GoogleSignIn/AppCheckCore/RadarGoogleAuth、ExpoAppleAuthentication、RevenueCat 完成编译、链接、签名、安装和启动，0 errors/4 个既有 warning；干净模拟器截图核对 Apple 系统按钮、Google 官方按钮、邮箱表单无溢出。未点击 provider、未使用真实账号 |
| P2 原生登录 Android `:app:assembleRelease` | 通过（2026-07-17） | Credential Manager/googleid/RadarGoogleAuth、2041 modules、四 ABI CMake、lintVital 与 Release APK 打包通过；905 tasks，`BUILD SUCCESSFUL in 3m 34s`；异常脱敏补丁另以 `:radar-google-auth:compileReleaseKotlin` 通过。运行态未运行 |
| RN production build variant config | 通过（2026-07-17） | 单测 4 项覆盖 dev 默认、production 身份、未知 variant、缺失/越界版本与 API；Expo public config 验证 `com.codeiy.im`、`opportunity-radar`、`0.2.0(2)` 占位版本、Android cleartext=false；iOS prebuild 产物无 ATS local/Bonjour keys；缺版本或严格 HTTPS API origin 均 config exit 1 |
| RN production iOS Release build/runtime | 部分通过（2026-07-17） | production `com.codeiy.im` 无签名 simulator build 完成；GoogleSignIn、Apple、RevenueCat、legacy token 与 1904-module Hermes bundle 均进入产物，xcodebuild exit 0。全新临时 iPhone 17 Pro 模拟器安装/冷启到登录页，旧 scheme 打开系统“在商机雷达中打开”确认框；随后模拟器已删除。因无商店签名/Keychain entitlement，页面显示旧凭据迁移失败，不能据此验收真实 token 或确认框后的 returnTo |
| RN production Android `:app:assembleRelease` | 通过（2026-07-17） | production `com.codeiy.im`、`opportunity-radar`、versionCode 2 与 `.test` HTTPS origin 占位配置，Google Credential Manager、RevenueCat、legacy token、2041 modules 与四 ABI 全部打包；首次 clean 905 tasks / 9m20s，API guard 后重打包 25s 均成功。未安装、未证明商店同签名升级 |
| RN production CI 编译边界 | 已配置（待远端运行） | `rn-ios` / `rn-android` job 使用 production 变体、占位版本和成对非敏感假 Google client ID，使 prebuild/Release build 覆盖商店身份、config plugin 及双平台原生模块；真实 provider E2E 不由该假配置替代 |
| Apple/Google 真实 provider E2E | 未运行 | 需要隔离的 Apple/Google Console 配置、真实 client ID/audience、测试账号和后端环境；当前构建只使用格式合法的假 client ID 且没有触发授权 UI，不能据此声称首次建号或错误 audience 已端到端通过 |
| `pnpm --dir mobile/radar typecheck && test`（P2 中英文 UI） | 通过（2026-07-17） | 23 files / 65 tests；覆盖系统首选语言顺序、中文区域归一、显式 fallback、插值、catalog key/占位符一致性、英文安全错误与生产 TSX 硬编码中文审计 |
| P2 中英文 UI production Expo config/prebuild | 通过（2026-07-17） | public config 为 `com.codeiy.im`、严格 HTTPS、iOS `en`/`zh-Hans`、Android `en`/`zh-CN`；prebuild 后 `Info.plist`、Android manifest/`locales_config.xml`/resourceConfigurations 均包含对应原生声明 |
| P2 中英文 UI Android production `:app:assembleRelease` | 通过（2026-07-17） | 905 tasks，`BUILD SUCCESSFUL in 6m 1s`；APK manifest 为 `com.codeiy.im` 1.0.0(1)、cleartext=false 且引用 locale config；包含 ExpoLocalization、Google Credential Manager、RevenueCat、legacy token 与四 ABI native 产物。未安装运行 |
| P2 中英文 UI iOS production generic device Release | 通过（2026-07-17） | `CODE_SIGNING_ALLOWED=NO` generic iOS device Release build 成功；产物为 `com.codeiy.im` 1.0.0(1)，`CFBundleLocalizations` 为 `en`/`zh-Hans`，包含 ExpoLocalization、GoogleSignIn、RadarGoogleAuth 与 RevenueCat。本次 simulator build 仅因本机 CoreSimulator 无法加载系统 `libswiftDispatch.dylib` 被策略拒绝，故未运行语言切换视觉验收 |
| `make pi-agent-check` | 通过（2026-07-17） | Node runner `npm ci` 与 4 tests 通过，共享 `radar-agent` 为本地精确依赖 |
| 最终认证版 iOS Release simulator build | 通过（2026-07-17） | 最新 auth/SQLite bundle、Expo Router/worklets 与本地 token module 完成原生链接；`xcodebuild` exit 0 |
| 最终认证版 Android `:app:assembleRelease` | 通过（2026-07-17） | 831 tasks；1956 modules、四 ABI CMake、lintVital 与 Release APK 打包通过；`BUILD SUCCESSFUL` |
| RN auth iOS Release simulator | 通过（2026-07-17） | 固定 `.test` 假账号经本地 fixture 登录；SecureStore 跨进程恢复；离线保留 token，服务恢复后重试成功；未使用真实账号/生产 API |
| RN 在线看板 iOS Release simulator | 通过（2026-07-17） | 本地确定性 fixture；3 条富卡片、重大提示、全量高级筛选、筛选计数、单条/空态、服务下线失败和恢复后重试均通过；未使用生产 API/数据 |
| P2 看板 iOS `expo run:ios --configuration Release` | 通过（2026-07-17） | 正常签名 dev bundle 完成 1828 modules 编译、安装和启动，SecureStore/Keychain entitlement 可用；0 errors、4 个上游/脚本 warning |
| P2 看板 Android `:app:assembleRelease` | 通过（2026-07-17） | 1965 modules、四 ABI CMake、lintVital 与 Release APK 打包通过；831 tasks，`BUILD SUCCESSFUL`；运行态仍未运行 |
| `pnpm --dir mobile/radar typecheck && test`（P2 详情/消息） | 通过（2026-07-17） | 15 files / 38 tests；追加详情竞态 reducer、分页去重/失败、安全错误映射与 returnTo/UUID 深链校验 |
| `pnpm --dir packages/radar-api check`（P2 详情/消息） | 通过（2026-07-17） | 4 files / 16 tests；追加详情/消息严格有界响应、重复 ID、时间顺序、分页元数据、非法 UUID/查询与 AbortSignal |
| `pnpm --dir packages/radar-api check`（P2 回复/状态） | 通过（2026-07-17） | 6 files / 21 tests；追加回复结果、AI 草稿、状态、认领、模板严格解码与请求契约 |
| RN 详情/消息 iOS Release simulator | 通过（2026-07-17） | 最新签名 dev bundle 1835 modules、0 errors/4 warnings；验证详情/Agent 发现/既有草稿、20/55 首批消息、加载后第 21/22 条、0/0 空态、服务下线失败态及恢复重试；未使用生产 API/数据 |
| P2 详情/消息 Android `:app:assembleRelease` | 通过（2026-07-17） | 1972 modules、四 ABI CMake、lintVital 与 Release APK 打包通过；831 tasks，`BUILD SUCCESSFUL in 25s`；运行态仍未运行 |
| P2 回复/状态 PostgreSQL 16 | 通过（2026-07-17） | 临时容器 fresh upgrade 到 `202607170001`、定向 4 tests、downgrade 到 `202607140001` 后再 upgrade head 全通过；容器已删除 |
| RN 回复/状态 iOS Release simulator | 通过（2026-07-17） | iPhone 16e；Release native 二进制 + 带本地 origin 的 Hermes bytecode；固定 `.test` fixture 验证模板、AI 草稿、认领、状态和幂等回复，20/55 精确更新为 21/56；未连接生产 API/IM |
| P2 回复/状态 RN Android `:app:assembleRelease` | 通过（2026-07-17） | 1981 modules、Expo Crypto、四 ABI native、lintVital 与 Release APK 打包通过；903 tasks，`BUILD SUCCESSFUL in 1m 35s`；运行态仍未运行 |
| RN SQLite v1 原生迁移 | 通过（2026-07-17） | `schema_migrations=1`；inbox/projection/outbox/sync_state 齐全；WAL、`integrity_check=ok`；schema 无 token/password 字段 |
| `docker compose -f backend/docker-compose.yml config --quiet` | 通过（2026-07-17） | 根 build context 与 `backend/Dockerfile` 配置有效 |
| P3 S7 最终本地回归 | 通过（2026-07-17） | PostgreSQL backend 224 tests；radar-api 47 tests；RN 36 files/117 tests、Expo compatibility、双平台 Hermes export 与 clean production 原生 Release build（iOS 1995 modules，Android 2134 modules/905 tasks）；production iOS Release 模拟器冷启动到登录页且无 Expo Network 错误；文件型 SQLite 1 万条 change 在 cursor 1500 断网关闭/重开后续传完成（293ms 宿主机回归）；harness、Web、pi runtime、diff check 与 release workflow YAML 解析通过 |
| 后端 Docker 镜像构建 | 未运行 | 本机 Docker daemon 不可用；compose 配置与 runner `npm ci` 已分别验证，镜像 COPY 边界仍由 CI 验证 |
| Android Release 运行态 | 未运行 | 本机没有 Emulator system image，且无连接真机；不能用“可构建”代替 Hermes/native runtime 结论 |
| 同包名/同签名 token 覆盖升级 | 未运行 | 必须在备份真机以商店签名执行；dev id `com.codeiy.im.dev` 无权读取 `com.codeiy.im` 沙箱 |

## 结果与剩余风险

P0 已产生独立开发包、兼容性代码和可复现证据；P1 已退出，交付共享包、根工作区、契约漂移检查、RN
认证 shell 与生产 SQLite 基础 schema。P2 已完成 auth、在线看板、详情/消息、回复/状态、设置/Telegram、
订阅、Apple/Google 原生登录及跟随系统的中英文 UI 代码切片；共享 API 已覆盖 auth、opportunity list/dashboard/detail/actions、
messages、templates、settings、Telegram 与 subscriptions。下一焦点是同 bundle/application id 的原位换装、
真实 token 迁移、provider/支付外部 E2E 与内测灰度；这些需要隔离账号、商店签名和控制台配置。
当前最大剩余风险是完整 pi provider stream 的 Metro/Hermes 边界、Android 运行态、移动前后台/被杀恢复，
以及同签名 token 覆盖升级。P1-P3 可以按计划推进；P4 在这些证据补齐前保持关闭，后续工期仍是量级
判断，不是上线承诺。

P3 S1-S6 代码已完成：设备会话、change feed、SQLite 只读副本、内部命令 outbox 和 cursor-only 原生
APNs/FCM 提示均由默认关闭的 capability 门控。S7 本地全量回归、1 万条宿主机重启续传、推送 cursor
去重及 offline→online 自动恢复通过；真实 provider、双真机飞行模式、kill/reopen、通知冷启动和设备性能
证据未完成，因此 `syncAvailable/pushAvailable` 生产默认仍为 false。
