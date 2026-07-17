# React Native 功能对齐矩阵

> 状态：P0 基线 · Owner：bruce / Codex · 创建：2026-07-17 · 更新：2026-07-17
>
> 隶属：[Agent-native + React Native 系统重构总计划](2026-07-17-agent-native-system-refactor.md) P0/P2。
> 当前事实以代码、[功能地图](../../product/feature-map.md)、iOS/Android README 为准。

## 标记

- `已实现`：当前客户端有真实 API/平台路径和相称测试或构建证据。
- `部分`：有真实代码，但缺外部配置、真机 E2E、平台另一端或完整异常路径。
- `无`：当前客户端没有该能力；不代表后端没有。
- RN 列在实现前保持 `待开发`；只有对应验收证据写入本表后才能改为完成。

## P2 商店换装必需能力

| 能力 | 后端/平台真相 | iOS SwiftUI | Android Compose | RN 目标 | 验收证据 |
| --- | --- | --- | --- | --- | --- |
| 邮箱密码登录 | `POST /auth/password/login` | 已实现 | 已实现 | 已实现（dev 包） | 共享严格解码；安全错误映射；iOS Release 用固定 `.test` 假账号完成登录，token 不进日志；RN 回归测试覆盖 401/429/原生存储错误脱敏 |
| Apple 原生登录 | `POST /auth/oauth/apple/native` | 已实现 | 不适用 | 部分（iOS 真实代码/Release 构建） | 使用 Apple 系统按钮和 `expo-apple-authentication`，本地 state 关联、取消非错误、严格 id_token 换站内 JWT，token 不落日志/SQLite；iOS Release 已安装并核对登录页，真实 Apple 账号、错误 audience 与首次建号 E2E 仍是外部发布闸门 |
| Google 原生登录 | `POST /auth/oauth/google/native` | 无 | 已实现 | 部分（双端真实代码/Release 构建） | Android 使用 Credential Manager 显式 Google 按钮流，iOS 使用 GoogleSignIn 9.x，界面采用官方品牌资产；只把严格 ID token 交给后端并安全归一错误。双平台 Release 构建通过；真实 provider 账号/错误 audience E2E 与 Android 运行态未运行 |
| 会话恢复/401/登出 | `GET /auth/me` | 已实现 | 已实现 | 已实现（dev 包） | iOS Release 冷启恢复、离线 fail closed/重试；401 与登出清 token，按 owner 清本地表；SecureStore 失败不伪装成功 |
| 旧 App token 升级 | Keychain / EncryptedSharedPreferences | 旧格式源 | 旧格式源 | 部分 | 迁移事务单测、iOS `.dev` 签名新装与 Android 原生模块编译通过；production 同 ID 无签名模拟器会因 Keychain 不可用显示重新登录，不能替代真实迁移证据。同包名、商店同签名备份真机原位升级仍是发布闸门 |
| 主导航与深链 | bundle/app links | 部分 | 已实现 | 部分 | 登录守卫与看板/账户双 Tab、详情 UUID 路由、非法参数 fail closed、登录后安全 returnTo 已实现并有单测；production 变体复用 Android 既有 `opportunity-radar` scheme 且双平台 Release 可构建。详情从看板进入已在 iOS Release 验证，外部链接冷启动、HTTPS app links 与通知冷启动仍待真机/P3 |
| 服务端商机看板 | `GET /opportunities/dashboard` | 已实现 | 已实现 | 已实现（在线切片） | 共享严格 decoder；全量服务端筛选/排序/分页查询、Abort/竞态、空/失败/重试单测；iOS Release 模拟器逐项验收；P3 前不承诺离线副本 |
| 重大商机提示 | dashboard `attentionItems` | 已实现 | 已实现 | 已实现（在线切片） | 后端 owner 隔离/分页外聚合测试；共享 decoder；iOS Release 验证提示独立于当前筛选结果展示 |
| 商机详情独立加载 | `GET /opportunities/{id}` | 已实现 | 已实现 | 已实现（在线切片） | 共享严格 decoder；UUID 先校验，详情/首批消息并行；404/归档/刷新/错误重试与 stale request reducer 有测试；iOS Release 验证独立详情、Agent 发现和既有草稿；P3 前无离线副本 |
| 消息历史 | `GET /messages/page`；兼容 `/messages` | 已实现 | 已实现 | 已实现（在线切片） | 服务端 owner 隔离、时间正序、单页上限 200；RN 每页 20，覆盖 incoming/outgoing、空态、去重、分页失败与加载更多；旧数组客户端保留最近 500 条兼容窗口 |
| 人工回复 | `POST .../manual-reply/result`；兼容 `/manual-reply` | 已实现 | 已实现 | 已实现（在线切片） | Web/RN 共享严格结果契约；客户端稳定 UUID 幂等键、服务端 owner-scoped 投递账本与 provider 后投影恢复；同键异文/并发/归档/禁用/结果不确定均 fail closed；iOS Release fixture 验证发送后消息 `20/55 → 21/56` 且状态进入跟进中 |
| AI 草稿 | `POST .../ai-draft` | 已实现 | 已实现 | 已实现（在线切片） | 真实后端请求、loading/error、可编辑且绝不自动发送；AI 关闭或 provider 失败返回稳定 503，不以 timer/固定文案伪造；iOS Release fixture 已验收 |
| 模板只读选用 | `GET /templates` | 已实现 | 已实现 | 已实现（在线切片） | 登录必需、最多 200 条严格解码、空/加载/失败/重试，不回退 mock；iOS Release fixture 验证模板加载与选用 |
| 状态流转 | `PATCH .../status` | 已实现 | 已实现 | 已实现（在线切片） | Web/RN 使用服务端状态机与行锁；非法迁移、归档和并发变化稳定拒绝；iOS Release fixture 验证待人工处理进入跟进中 |
| 商机认领 | `POST .../claim` | 已实现 | 已实现 | 已实现（在线切片） | 身份只取认证用户，服务端行锁保证单一认领者；Web/RN 显示本人/他人认领，冲突不做本地乐观伪造；iOS Release fixture 验证“已由你认领” |
| Agent 分析展示 | Opportunity Agent fields | 已实现 | 已实现 | 已实现（只读） | 严格状态/action enum；错误不透传 provider detail；链接、联系人和需审批动作有显示上限与 iOS Release fixture 证据；重新分析写操作留在后续切片 |
| 订阅与用量 | `/subscriptions/*` + RevenueCat | 部分 | 部分 | 部分（真实代码/Release 构建） | Web/RN 共用严格套餐、用量和管理契约；RN 使用官方 SDK，以认证 UUID identify，支持指定 Offering、Free 用户购买、用户触发恢复、取消非错误、操作后后端 sync、重复操作保护和原渠道管理。iOS/Android Release 原生模块构建通过；Dashboard 商品、真实 Sandbox/Test Store 账号与支付 E2E 未配置，因此不能标为生产开通 |
| 识别偏好 | `/settings/me`、`PATCH /settings/detection` | 已实现 | 已实现 | 已实现（在线切片） | Web/RN 共享严格设置契约；边界去空格、大小写去重、64 字/200 个上限，服务端 owner 隔离；RN 写失败恢复服务端旧值，iOS Release fixture 已验收 |
| 工作时间 | `PATCH /settings/work-schedule` | 已实现 | 已实现 | 已实现（在线切片） | 支持 IANA 时区、空时段、同日任意多段；跨午夜必须拆段且客户端/服务端拒绝反向端点，写失败回滚；iOS Release fixture 展示 3 段含周一午休 |
| 通知偏好 | `PATCH /settings/notifications` | 已实现，推送无 | 已实现，推送无 | 已实现（偏好 + cursor 唤醒代码） | 四类偏好读取/写入与失败回滚；用户可显式启停无正文的 APNs/FCM sync wake-up。可见提醒/每日摘要与真实 provider 双真机验收仍无，rollout 默认 false，不伪装已通知 |
| Telegram 连接读取/启停 | `/integrations/telegram/*` | 已实现 | 已实现 | 已实现（管理切片） | health/空态/来源/quota pause/启停均用严格服务端真相；401 清会话、错误日志脱敏、provider detail 不展示；新建外部握手留在 Web，iOS Release fixture 已验证启停 |
| 深色/动态字体/无障碍 | 平台 UI | 部分 | 部分 | 部分 | auth/看板/订阅关键状态和控件已有语义、progressbar 与可缩放文本；VoiceOver/TalkBack、字体放大和对比度仍需真机验收 |
| 中英文 UI | 平台资源 | 部分 | 部分 | 已实现（跟随系统） | 类型化 `zh-CN`/`en` catalog 覆盖生产 TSX，按系统首选语言解析并显式回退简体中文；iOS 声明 `en`/`zh-Hans`，Android 声明 `en`/`zh-CN`；catalog key/插值和硬编码中文审计纳入 23 files/65 tests，双平台 production Release 可构建。系统语言切换视觉、VoiceOver/TalkBack 仍待真机验收 |
| 错误/崩溃日志脱敏 | 客户端边界 | 部分 | 部分 | 部分 | auth/看板/详情/消息/回复/状态/设置/Telegram/订阅只记录动作、错误类别与 HTTP 状态，Keychain、服务端 detail、消息正文、幂等键、RevenueCat key/CustomerInfo 和 Agent/provider 错误不进日志/错误 UI |

## P3 之后新增，不阻塞 P2 换装

| 能力 | 当前状态 | 计划阶段 | 完成条件 |
| --- | --- | --- | --- |
| APNs/FCM 注册与推送深链 | RN/服务端代码完成、生产通道默认关闭 | P3 S7 | 已实现签名环境上报、token 轮换、权限拒绝、失效回收、cursor-only 无正文 payload、前台/点击/background sync；仍需真实 APNs/FCM 凭据与双真机丢推送/冷启动验收 |
| 增量同步/SQLite 副本 | 代码完成、production rollout 默认关闭 | P3 S7 | bootstrap/cursor/gap/reset 自动化通过；文件型 SQLite 已覆盖 1 万条 change、断网后关闭/重开并续传。293ms 是宿主机回归证据，仍需双真机 kill-reopen/内存/性能证据 |
| 飞行模式只读 | 代码完成、真机门禁待验 | P3 S7 | SQLite 看板/详情/消息查询、账号切换全表清理和确认 offline→online 自动恢复有自动化；仍需双真机飞行模式运行态证据 |
| 离线命令 outbox | 内部状态命令代码完成、production rollout 默认关闭 | P3 S7 | 幂等、base version、7 天 expiry、稳定 409、冲突可见与离线→在线自动 drain 入口通过自动化；外部回复/好友申请不入队 |
| 端上 pi 分析 | 无 | P4 | 双平台 release gate + lease/fallback/quota 一致 |
| Agent 对话与 session | 无 | P5 | 持久化、取消、保留期、用量边界明确 |
| 领域工具和审批 | 仅服务端建议展示 | P5 | 白名单 + 用户批准 + 服务端二次钳制 |
| 端侧 L0/L1 分诊 | 无 | 独立后续 | 设备覆盖/降级/金标指标先定义 |
| sqlite-vec 记忆 | 无 | 独立后续 | 隐私评审、存储预算、删除/导出 |
| macOS/CLI 常驻节点 | 无 | P6 | 凭据迁移、后台可靠性、用户可撤销 |
| E2EE 信封/端上权威 | 无 | P6 | 独立威胁模型、加入/撤销/恢复演练 |

## 明确不从旧客户端照搬

- Web `AppStoreProvider` 中的 mock/timer/静默回退。
- 没有平台执行能力的自动好友申请暗示。
- 运行时动态代码、通用 shell/文件/HTTP Agent 工具。
- 把 404/422/provider 错误统一显示成“网络错误”的兼容行为。

## 更新纪律

每个 RN 纵向切片完成时：链接测试或真机证据，更新 RN 状态，并同步功能地图。P2 结束前重新核对
Swift/Kotlin 的最后发布版本；若旧端新增了安全/契约能力，本矩阵必须补行，不允许以冻结为理由漏迁。
