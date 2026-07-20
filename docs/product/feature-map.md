# 功能地图

> 状态：当前事实 · 最后核验：2026-07-20

本地图用成熟度区分“仓库里有 UI/数据结构”和“用户端到端可用”。AI 开发前必须先确认目标
能力所在行，避免沿着 mock 或占位 DTO 继续实现。

## 成熟度定义

- **已实现**：有真实运行路径，并有与风险相称的自动化验证。
- **部分实现**：部分链路真实，仍有明确断点、降级或验证缺口。
- **仅演示**：只在浏览器状态、mock、timer 或默认映射中存在，不是生产能力。

## 用户能力

| 能力 | 前端入口 | 后端/API | 成熟度 | 备注 |
| --- | --- | --- | --- | --- |
| Google / Apple OAuth 登录 | Web/RN `/login` | `/api/v1/auth/oauth/*`、`/auth/me` | 部分实现 | Web 与移动端均使用真实 OAuth/JWT 路径，仍依赖外部 provider 配置且缺少真实账号端到端测试。RN 的 `POST /auth/oauth/{google\|apple}/native` 只以严格 ID token 换站内 JWT：Apple 使用系统控件，Google 在 Android 使用 Credential Manager、iOS 使用 GoogleSignIn 9.x，取消非错误，provider token 不持久化或记录；双平台 Release 构建和 iOS 登录页视觉核对已通过。audience 经 `GOOGLE/APPLE_NATIVE_CLIENT_IDS` 钳制，为空即关闭；真实 audience、首次建号和 Android 运行态仍待隔离环境验证 |
| iOS/RN 邮箱密码登录 | iOS/RN 登录页 | `POST /auth/password/login`、`/auth/me` | 已实现 | 仅登录已有密码账户，不含注册/重置；PBKDF2 校验、统一失败响应、Redis 登录限流；原生 iOS JWT 存 Keychain，RN dev client 存 SecureStore 并已用本地假账号验证 Release 登录/恢复/离线重试。RN production 变体已用 `com.codeiy.im` 完成双平台 Release 构建，并在全新 iOS 模拟器冷启到登录页；但无签名构建不能使用真实 Keychain，尚未安装同商店签名候选包、读取真实旧 token 或执行生产账号 E2E |
| RN 中英文界面 | RN 全部生产页面 | 无 | 已实现 | 类型化 `zh-CN`/`en` catalog 覆盖 auth、看板、详情/消息、设置/Telegram、订阅和安全错误；按系统首选语言选择，不支持的语言显式回退简体中文。iOS/Android 原生支持语言声明、catalog key/插值一致性、生产 TSX 硬编码中文审计及双平台 production Release 构建已验证；系统语言切换视觉和 VoiceOver/TalkBack 仍需真机 QA |
| 信息胃口教学 | RN 首页/机会详情/`/teaching` | 本地 SQLite；可选 `/sync/signal-appetite/events` | 部分实现 | 真实 owner-scoped 本地消息支持固定物理左滑 positive / 右滑 negative、按钮/读屏等价操作、原因、跳过、连续撤销、Session 总结、历史试跑和明确 apply；首次三步引导可在设置重播。Reanimated UI-thread 手势、haptic 与 Reduce Motion 已接入，53 files / 174 tests 和双平台 Hermes export 通过；真机触感、RTL、大字体、TalkBack/VoiceOver 与 P90 55fps 未验收 |
| 信息胃口首页、意图地图与安静区 | RN 首页、`/intent-map`、`/quiet-zone`、Pi Tab | 本地 SQLite；Pi schema v4 | 部分实现 | 首页显示当前胃口与四类投递统计。确定性 SVG 地图最多 30 节点，支持平移/缩放/节点解释、线性语义替代、融入式一天时间轴、临时关注和 Shadow 当前/候选对比。安静区保留本地可抽查副本并显示原因、证据、位置、置信度与 evaluator；地图/安静区/商机可带无正文上下文进入 Pi。严格立即删除、RN 独立工作机会列表、运行截图和真机 UI/性能仍未交付 |
| Mira 今日简报与消息管家 | RN `/(tabs)/home`、`/(tabs)/dashboard`、`/(tabs)/agent` | 本地 SQLite Briefing/AttentionSnapshot；interactive schema v5 | 部分实现 | RN 新体验默认在 dev 开启，底部导航为“今天 / 消息 / Mira”，设置降为首页头像入口，并可用 `EXPO_PUBLIC_MIRA_CONCIERGE_UI_ENABLED=false` 回到经典首页/看板/Account Tab。Today 读取本地 AttentionSnapshot 与 Briefing 列表，展示 Mira 状态头、今日只需看数量、一句话、三时段简报和 Reduce Motion 友好的 MiraOrb；Messages 默认显示智能分类，原看板列表通过分类进入。简报可按本地日程到点生成，同日同一时段去重，L2 summarizer 失败降级为本地结构化简报。真机动效、截图 golden、通知实际投递仍待验收 |
| 信息胃口事件同步 | RN 后台同步 | `GET/POST /sync/signal-appetite/events`、设备 capability | 部分实现 | SQLite v7 append-only event + 投影覆盖 preference/intent/example/session/decision/shadow/focus/UI state；服务端 additive PostgreSQL 流按 owner/device 隔离，content-free、64 KiB、有冲突检测和独立 cursor。客户端先 push pending 再 pull/fold；`SIGNAL_APPETITE_SYNC_ENABLED=false` 且 capability 默认关闭，跨真机并发与长时间离线验收前不开放 |
| 设备身份、增量同步与离线副本 | RN 后台注册（尚无设备管理页） | `/devices/*`、`GET /sync/bootstrap`、`GET /sync/changes`、`POST /sync/ack` | 部分实现 | P3 S1-S6 已完成可撤销设备会话、事务内 change feed、RN SQLite 原子消费/只读回退、内部状态有界 outbox，以及原生 APNs/FCM token 轮换和 cursor-only 推送提示。服务端短租约扫描已提交 change、失败退避并回收无效 token；payload 无正文/商机数据。启动、前台、手动刷新和确认的离线→在线转换触发同一 single-flight sync；不领先于 durable cursor 的推送提示会去重，丢推送仍由这些生命周期入口补拉。文件型 SQLite 已覆盖 1 万条 change、断网后关闭/重开并从 cursor 继续的宿主机回归，但这不替代设备性能。人工回复/好友申请等外部动作始终在线。两个 rollout 默认 false，真机飞行模式/kill-reopen/真实 provider 证据前 `syncAvailable/pushAvailable=false` |
| 商机列表、筛选、分页 | `/` | `GET /opportunities` | 已实现 | 登录后每 30 秒轮询；查询按 owner 隔离；默认排除归档记录，可按 `archive=active\|archived\|all` 查询 |
| 商机归档与恢复 | `/`、`/opportunity/[id]` | `POST .../archive`、`POST .../restore`、`POST /bulk-archive` | 已实现 | 单条/最多 100 条批量归档、归档视图与恢复；保留原状态、消息和分析；归档项禁止业务写操作并跳过 AI 自动回复 |
| 商机看板（服务端筛选/排序/分页） | iOS/Android/RN 消息 Tab | `GET /opportunities/dashboard` | 部分实现 | 后端聚合端点已实现：status/platform/source/时间范围/trust/sop/keyword 多维筛选 + 4 种排序 + total/pendingCount/attentionItems/keywordOptions，全部 SQL 完成并按 owner 隔离，有 Postgres 门控筛选矩阵测试；RN dev 包已接入共享严格 decoder、全量服务端筛选/排序/分页、重大提示、Abort/竞态保护及 loading/error/empty/retry。2026-07-20 起 RN 默认“消息”入口先展示 Mira 智能分类，旧看板列表通过 `view=list` 和 feature flag 继续可达。P3 S4 已实现同语义 SQLite 只读查询，但生产 capability 默认关闭且真机飞行模式待 S7。SwiftUI/Compose 双 Tab 已接入但未在 CI 外做真机 E2E。iOS 在生产后端滚动升级期间会把默认/status/platform 查询降级到旧 `/opportunities`，并明确停用无法等价支持的高级筛选。Web `/` 仍走客户端 `applyFilters` 内存筛选，但列表请求已迁移到共享严格 API 边界 |
| 商机语义识别 | 无独立页 | 摄取用例 + `OpportunityDetector` + LiteLLM | 已实现 | 高置信规则直通；启用 AI 后对其余非空消息结合 owner 隔离的有限会话历史、来源和 AI hint 复核；模型新发现只进人工审核，provider 失败回退规则 |
| 商机详情 | Web/RN `/opportunity/[id]` | `GET /opportunities/{id}` | 已实现 | Web 与 RN 均通过共享严格 decoder 独立读取，不依赖列表预热；API 额外返回 `opportunityType=business\|job`，RN 根据类型展示商业机会/职位机会模板。商业详情保留回复框和审批链；职位详情不显示回复框，提供保存、准备建议、查看来源、不感兴趣路径。正式 UI 只展示 Mira 结论、关键事实、相关理由、风险和建议下一步，不直接铺 raw JSON/debug 字段。P3 S4 已实现 capability 门控的跨启动 SQLite 详情回退，生产灰度与真机证据仍待完成 |
| 消息历史 | Web 详情页 SOP、RN 详情页 | `GET /messages/page`；兼容 `GET /messages` | 已实现 | 新端点 owner 隔离且按时间正序分页，单页硬上限 200；Web 每页 200、RN 每页 20，均有 incoming/outgoing、空态、加载更多和安全失败态。RN 已实现 capability 门控的 SQLite 消息分页回退；旧数组端点保留给 SwiftUI/Compose，但只返回最近 500 条并按时间正序展示 |
| 人工回复 | Web/RN 详情页回复框 | `POST /opportunities/{id}/manual-reply/result`；兼容 `/manual-reply` | 已实现 | Web/RN 共用严格动作 API，并使用稳定 UUID 幂等键；SwiftUI/Compose 兼容端也在失败重试期间复用同一键。通用 `manual_reply_deliveries` 账本按 owner + key 唯一，绑定商机/正文哈希；Telegram/企微 provider 成功后才创建 outgoing Message 和流转状态，投影失败以同键恢复而不重发；发送结果不确定返回 502 并冻结该键等待人工核验，`IM_SEND_ENABLED=false` 返回 503 且不伪造消息。新结果端点同时返回服务端商机、消息和精确消息总数；旧响应保持兼容 |
| AI 回复草稿 | Web/RN 详情页回复框 | `POST /opportunities/{id}/ai-draft` | 已实现 | Web/RN 调用真实生成端点，显示 loading/error，草稿可编辑且不会自动发送；AI 未启用或 provider/输出失败均显式 503，不再以 timer 或固定文案冒充 AI |
| 非工作时间 AI 自动回复 | 无独立页 | Celery `ai.reply` | 已实现 | 受 `AI_ENABLED`、`IM_SEND_ENABLED` 与 provider 配置控制 |
| 商机状态更新/认领 | Web/RN 详情页 | `PATCH .../status`、`POST .../claim` | 已实现 | Web/RN 均读取服务端返回真相，不做本地伪成功；状态迁移与认领使用行锁，非法/并发/归档写入返回稳定 409。开启 sync capability 后，RN 只允许状态变更在网络/5xx 时进入有界 outbox，按 base version + 幂等回执重放；冲突用户可见。认领和其他动作仍不离线排队；身份只来自认证用户，旧 `operator_id` 查询参数仅保留兼容且被忽略 |
| 回复模板读取 | Web/RN 详情页、`/templates` | `GET /templates` | 已实现 | 必须登录，服务端最多返回 200 条；Web/RN 共享严格 decoder，覆盖 loading/empty/error/retry，空结果保持为空，不静默回退 mock 模板 |
| 回复模板编辑 | `/templates` | `POST/PATCH /templates` | 部分实现 | 后端 admin API 已有；前端新增/编辑只改本地 store |
| Telegram 原生连接中心 | Web/RN `/settings/telegram` | `/integrations/telegram/*` | 部分实现 | P0 Bot 群/频道有真实连接、来源、验签和 webhook 幂等；Business 需平台配置；P2 依赖平台 MTProto 凭据。Web/RN 已共用严格 health/connection/source 契约；RN 可读取空态、套餐暂停并启停现有连接，新建外部握手仍只在 Web，失败不做本地伪成功 |
| Telegram Bot 群组/频道 | `/settings/telegram` | `POST /integrations/telegram/connect/bot-chat`、`POST /webhooks/telegram` | 已实现 | 使用短期 token 和 `request_chat`，回调后验证 chat/Bot membership；未配置来源的 chat 不摄取 |
| Telegram Business 私聊 | `/settings/telegram` | `POST /integrations/telegram/connect/business`、`POST /webhooks/telegram` | 部分实现 | 私聊确认后按 Business connection owner 路由并创建默认关闭的来源授权；发送时携带 `business_connection_id`；需可用的平台 Bot、Telegram Business 权限与真实隔离 E2E |
| Telegram 普通账号 QR | `/settings/telegram` | `POST /integrations/telegram/connect/mtproto-qr`、dialogs/sources API | 部分实现 | 平台统一凭据、二维码、加密 session、独立 QR worker 和只读 listener；需生产 Telegram 隔离冒烟 |
| Telegram 旧 MTProto 监听 | 无默认表单入口 | 独立 legacy listener | 已实现（兼容） | 已授权 session 继续监听；旧用户凭据采集 API 已删除，新页面不展示或迁移秘密 |
| 企业微信自建应用 | `/settings/wecom` | `/integrations/wecom/*`、`GET/POST /webhooks/wecom/{connection_id}` | 部分实现 | 用户级加密凭据、专属回调、验签/解密、幂等 Celery 摄取和成员私聊人工回复已实现；待真实企业联调，不支持普通群聊监听 |
| 企微会话内容存档 | `/settings/wecom` | `/integrations/wecom/archive-connections/*`、Celery `wecom_archive` queue | 部分实现 | 企业级连接、成员 binding、Finance SDK 拉取/解密、幂等 cursor、owner 投影、群配额和只读商机链路已实现；默认关闭，需企业购买存档、管理员合规授权、出口 IP 白名单、官方 SDK 挂载及真实企业 E2E，不是普通成员个人授权 |
| 规则管理（全局） | `/settings` | CRUD `/rules` | 部分实现 | 后端 admin API 已有；面向普通用户的识别偏好改走用户级 `/settings/detection` |
| 用户级识别规则（关键词 + AI 语义） | Web/iOS/Android/RN 设置 | `GET /settings/me`、`PATCH /settings/detection` | 已实现 | `user_detection_preferences` 表按 owner 隔离；关键词去空格/大小写去重/限长限量；已接入 `ingest_message` 摄取（用户关键词叠加全局规则、AI 语义开关生效）。Web/RN 共用严格契约，RN 失败恢复服务端旧值；有 owner 隔离、规范化与 iOS Release fixture 证据 |
| 用户级工作时间 | Web `/settings/working-hours`、iOS/Android/RN | `GET /settings/me`、`PATCH /settings/work-schedule` | 已实现 | `user_work_schedules` 表按 owner；IANA 时区 + 空/任意多段人工审核时段，跨午夜需拆段；`WorkScheduleService` 接入摄取决定人工/AI，无用户设置回退全局默认。Web/RN 共用严格契约，RN 失败回滚；旧全局 `/configs/work-mode` 保留但普通用户不再改全局 |
| 用户级通知偏好 | Web/iOS/Android/RN 设置 | `GET /settings/me`、`PATCH /settings/notifications` | 部分实现 | `user_notification_preferences` 表按 owner 持久化，Web/RN 共用严格契约并在失败时恢复旧值。RN 已提供用户显式授权的 cursor-only 同步唤醒开关，并新增 Mira 简报式通知文案生成器：只含计数和简报入口，不含消息正文、原始链接或模型调试信息。当前仍不发送可见的“新商机/AI 回复/每日摘要”通知；provider/灰度未配置时 UI 明确不可用，不声称偏好已经投递 |
| 统计摘要 | 无独立展示 | `GET /stats/summary` | 部分实现 | API 已有，前端未消费 |
| pi Agent 消息后处理 | 看板提醒、详情页 SOP | Celery `agent.analyze_message`、`POST .../agent-analysis`、`/agent/runs/*`、`/agent/gateway/v1/chat/completions` | 部分实现 | 服务端 runner 已支持新消息异步分析、补判商机和结构化建议，依赖有效 provider key 并执行套餐月额度。P4 已增加默认关闭的 owner/device-bound claim/lease/finalize API、受限 OpenAI-compatible SSE 网关，以及只从服务端 Message 派生 URL 的 SafeLinkInspector 代理；run token、单 ledger、单 active provider stream、正文零审计和错误脱敏已有 PostgreSQL/fake provider 测试。RN 已接共享 `submit_analysis`-only pi harness，SQLite v5 + SecureStore 支持续租、前后台取消、重启重试、过期与明确 fail。同步后先 `claim-next` primary、无候选再取一条 shadow；shadow 可独立于 primary rollout，只复用服务端已消费账本并记录投影差异。primary 仅在当前 runtime/schema/model/policy 的 shadow 成功率/一致率/P95 达标后，对稳定百分比 cohort 或白名单中的近期设备开放；同一 Celery job 延迟领取窗口兜底，active run 与同消息 reservation 唯一约束阻止陈旧 worker 接管或双扣。管理员 readiness API 只返回无正文聚合。成功结果持久化 server/device 与版本来源；双平台 Hermes production export 通过。真实 provider、双真机 kill/reopen、按设备真实流量灰度尚未验收，故 rollout、shadow、gateway、fallback 仍不可在生产开放 |
| 交互式 Agent | RN capability 门控 Mira Tab | `/agent/interactive/turns*`、`/agent/interactive/gateway/v1/chat/completions`、`/agent/interactive/actions/*` | 部分实现 | P5 beta 工具集单调演进：v1 本地只读；v2 草稿/status outbox/认领；v3 增加独立外发 Approval Gate；v4 增加 15 个信息胃口 inspect/teaching/propose/simulate/apply/shadow/explain/correct/focus/schedule/undo/compare 工具；v5 增加 6 个简报工具（summarize_time_window/get_attention_snapshot/list_priority_items/list_category_items/get_quiet_summary/update_brief_schedule），TS 与 Python 网关契约 golden 对齐，RN 设备注册上报 `agent.interactiveSchema=5`。`apply_appetite_change` 有与外发批准隔离的一次性 UI 确认，capture/propose/simulate 不会激活偏好；外部发送仍走 Approval Gate。token/正文不进入模型持久化、SQLite 审批材料、日志或 URL；生产 rollout、真实 provider、授权 IM 沙箱、双真机外发、记忆和跨设备会话仍未验收，因此标为部分实现 |
| 统一订阅、购买与用量 | Web/RN `/settings/subscription`、SwiftUI/Compose 套餐页 | `/subscriptions/{plans,catalog,me,sync,management}`、RevenueCat webhook | 部分实现 | RevenueCat 聚合 Paddle/App Store/Play，服务端投影执行权益。Web/RN 共用严格目录、用量与管理契约；RN 使用官方 SDK、认证 UUID、平台 Public Key 和指定 Offering，支持购买/恢复后服务端 sync、取消非错误与重复操作保护，双平台 Release 已构建。外部 Dashboard 商品、真实 Sandbox/Test Store E2E 与用户级企微群配置仍待完成，不能视作生产支付已开通 |
| 链接安全核验 | 详情页 SOP | SafeLinkInspector + pi Agent | 已实现 | 公网/DNS/端口/逐跳重定向/类型/大小/超时限制、结果持久化、可重跑；设备 Agent 请求不能携带任意 URL，只能读取 run 绑定消息的服务端派生链接。不是恶意软件扫描器，生产仍需受控 egress |
| 联系方式提取 | 详情页 SOP | pi Agent 结果投影 | 部分实现 | 消息/公开网页中的联系方式可持久化；详情页手工编辑仍只更新浏览器状态 |
| 后续行动建议 | 详情页发现步骤 | pi Agent 结构化 actions | 已实现 | 可建议邮件、加好友、私信和内部提醒；外部动作强制标记需人工批准，不会自动执行 |
| 重大商机提醒 | 商机看板 | Opportunity attention projection | 已实现 | Agent 判定紧急/高影响或建议通知时展示 owner 隔离的看板提醒 |
| 好友申请状态流转 | 详情页 SOP Step 4 | `POST /opportunities/{id}/friend-request` | 部分实现 | 发送/通过/被拒/重试真实持久化并推进 SOP（pending→friend_requested、accepted→ready_to_chat），非法流转 409，owner 隔离；已移除 4 秒假自动通过 timer。"已通过/已拒绝"由操作员在 IM 内确认后手动回填——平台无自动发送好友申请的 IM 能力，实际发送仍是人工动作 |
| 通知偏好/每日摘要 | Web/iOS/Android/RN 设置 | `/settings/notifications` | 部分实现 | 偏好真实持久化（见上“用户级通知偏好”）；Mira 简报 copy 已可由本地 Briefing/AttentionSnapshot 组合并通过测试保证不泄露消息/机会 id 或正文。P3 push 仍只唤醒同步且无正文，可见提醒和每日摘要实际投递仍未实现，不能声称偏好已产生用户通知 |

## 后端能力入口

| 领域 | 关键代码 |
| --- | --- |
| 路由注册 | `backend/app/api/v1/router.py` |
| 认证与授权 | `backend/app/api/v1/routes/auth.py`、`backend/app/api/deps.py`、`backend/app/core/security.py` |
| 摄取编排 | `backend/app/application/use_cases/ingest_message.py` |
| 识别规则 | `backend/app/domain/services/detection_policy.py` |
| 用户级设置 | `backend/app/api/v1/routes/settings.py`、`backend/app/infrastructure/db/repositories.py`（`UserSettingsRepository`）、`backend/app/core/time_window.py`（`WorkScheduleService`） |
| 商机状态机 | `backend/app/domain/services/opportunity_state.py` |
| 回复编排 | `backend/app/application/use_cases/manual_reply.py`、`ai_reply.py` |
| IM 适配 | `backend/app/infrastructure/im/` |
| 企业微信会话存档 | `backend/app/infrastructure/im/wecom_archive.py`、`application/use_cases/sync_wecom_archive.py` |
| AI 分类/回复 | `backend/app/infrastructure/ai/litellm_client.py` |
| 异步任务 | `backend/app/worker/tasks.py`、`queue.py` |
| pi Agent 后处理 | `backend/app/application/use_cases/analyze_message.py`、`infrastructure/agent/`、`pi-agent-runtime/` |
| 工作机会发现 | `backend/app/domain/services/job_discovery.py`、`job_matching.py`、`application/use_cases/persist_job_opportunity.py`、`api/v1/routes/jobs.py` |
| 信息胃口同步 | `backend/app/application/use_cases/signal_appetite_sync.py`、`infrastructure/db/signal_appetite_repository.py`、`api/v1/routes/sync.py` |
| 订阅与额度 | `backend/app/domain/services/subscription_policy.py`、`application/use_cases/sync_revenuecat_customer.py`、`infrastructure/billing/` |
| 持久化 | `backend/app/infrastructure/db/models.py`、`repositories.py` |

## 扩展功能时的同步清单

- 功能从“演示”升级：移除对应 mock/timer，接入 API，补失败/加载状态和端到端验证，再改成熟度。
- 新 API：更新路由、DTO、前端 client/types、认证约束、测试和本表。
- 新 IM 平台：扩展 `IMChannel`、实现 `IMAdapter`、注册 adapter、迁移枚举存储影响、补解析和发送测试。
- 新商机状态：同步领域枚举、状态迁移、数据库、mapper、前端状态及筛选/统计。
- 新持久字段：SQLModel + Alembic + repository + DTO + mapper + 前端类型，禁止只更新其中一层。
