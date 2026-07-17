# P3 设备身份与同步基础

> 状态：active · Owner：bruce / Codex · 创建：2026-07-17 · 更新：2026-07-17
>
> 隶属：[Agent-native + React Native 系统重构总计划](2026-07-17-agent-native-system-refactor.md) P3。

## 目标与用户价值

在不改变现有在线 REST 行为的前提下，建立可撤销设备身份、可轮换设备凭据、最小推送注册和按用户
单调排序的同步变更基础；随后以 bounded bootstrap/change feed 驱动 RN SQLite 副本。完成后，移动端可在
网络恢复时从 PostgreSQL 权威状态重建本地只读数据，并为后续安全的后台同步与推送提示提供身份边界。

## 非目标

- 本计划不迁移服务端业务权威，不删除现有 REST、30 秒轮询或 SwiftUI/Compose 兼容路径。
- 不在第一迁移中签发长期设备 refresh token、注册真实 APNs/FCM token 或发送推送。
- 外部回复、好友申请和其他不可证明安全的动作不进入离线自动重放。
- 不把 proposal 中的通用事件溯源、E2EE、端上 Agent 或动态工具系统提前并入 P3。

## 背景与当前行为

- RN SQLite v1 已有 owner-scoped `change_inbox`、`opportunity_projection`、`command_outbox` 与
  `sync_state`，但 `syncAvailable=false`，生产页面不读写这些表。
- 服务端没有 `Device`、设备凭据、推送注册或 `SyncChange`；当前 access token 默认 7 天，不适合作为
  可撤销后台设备会话。
- 现有 Message、Opportunity、用户设置与模板写入分布在多个 repository/use case，部分流程会多次
  commit。P3 采用 aggregate upsert + version 幂等覆盖，不伪装成原子领域事件。
- 当前 P2 商店换装仍有同签名 token、真实 OAuth/支付和内测外部闸门；这些不阻止 expand-only P3 基础。

## 验收标准

- [x] Alembic expand 迁移创建 `devices`、`device_credentials`、`push_registrations`、`sync_changes`；
      downgrade 只删除新表/序列，不触碰现有用户、消息、商机或设置数据。
- [x] 设备归属始终来自认证用户；同一 owner 的 installation hash 唯一，跨 owner 不冲突；撤销、最后活跃、
      platform、app/build/runtime 版本和有界 capabilities 可记录。
- [x] 设备 credential 只持久化 SHA-256 hash、family/rotation/revocation/reuse 元数据，永不持久化 bearer
      明文；同一 hash 全局唯一，同一设备最多一个 active credential。
- [x] 推送 registration 只持久化发送所需的加密 token 与唯一 hash，记录 APNs/FCM 环境、轮换、失效和
      最近结果；响应、错误和日志不暴露原 token。
- [x] `SyncChange` 为不可变 owner-scoped envelope，包含唯一 event id、单调 cursor、aggregate
      type/id/version、operation、schema version、createdAt 和 payload/tombstone；约束拒绝版本 0、未知
      operation、delete 携带 payload 和 upsert 缺 payload。
- [x] PostgreSQL 迁移、约束、owner 隔离与并发唯一性测试通过；本切片四表的 SQLModel metadata 与
      Alembic 无漂移。
- [x] 设备注册/轮换 API 覆盖成功、过期、撤销、跨用户、重复使用检测和日志脱敏；设备绑定 access token
      在撤销后立即失效，旧无 `did` access token 仅保留自身过期前的兼容窗口。
- [x] bounded bootstrap/changes API 覆盖 Message、Opportunity 与三类用户设置；过期/超前 cursor 返回
      明确 reset，不允许无界列表或 owner 越权。回复模板只服务在线外部动作，不是首批离线必要数据。
- [ ] RN 在单个 SQLite 事务中写 inbox、幂等应用 projection 并推进 cursor；账号切换清库，损坏可由
      bootstrap 重建；飞行模式可读的运行态证据完成后才把 `syncAvailable` 改为 true。

## 影响面与风险

- **数据库/迁移**：新增四表和 cursor identity；只 expand。风险是唯一约束、索引或 enum/model 与
  migration 漂移，必须用 PostgreSQL fresh upgrade/downgrade/upgrade 和 metadata 比对验证。
- **认证**：后续设备 bearer 属高风险凭据。原文只生成一次，服务端仅存 hash；轮换必须事务化，reuse
  撤销整个 family，跨用户/设备拒绝。第一切片不签发 token。
- **推送**：平台 token 是必要敏感值，使用现有应用加密封装存储，hash 用于查重；第一切片不联系 APNs/FCM。
- **同步/隐私**：payload 可包含通信数据，日志不得记录；所有查询必须 owner-scoped、有上限；删除使用
  tombstone。PostgreSQL 在 P3 仍是唯一业务权威。
- **RN/性能**：应用 change 采用 SQLite 批量事务和版本比较；同步调度不得把高频 cursor 变化广播到
  整个 React 树，页面订阅投影的稳定派生状态。
- **部署**：迁移只新增对象，可先部署后端再灰度客户端；旧服务和旧客户端忽略新表。

## 实施步骤

- [x] S1：新增 P3 domain enums、四个 SQLModel 和 Alembic expand 迁移；补模型/迁移/PostgreSQL 约束测试。
- [x] S2：实现设备注册、设备列表/撤销、credential 初次签发与事务化轮换/reuse 检测 API；更新严格共享契约。
- [x] S3：给 Opportunity、Message、用户设置引入 aggregate version 和事务内 SyncChange append；新增
      bounded bootstrap、changes、reset/ack/observability；模板按非离线必要数据留在在线 API。
- [x] S4：升级 RN SQLite schema，严格解码并事务化存储 inbox/projection/cursor；接入前台/手动同步与
      只读离线看板/详情/消息，保留 capability 回退。
- [x] S5：只为内部状态命令实现有界 outbox；409 冲突用户可见，外部动作保持在线显式提交。
- [x] S6：接入 `expo-notifications` token 生命周期和服务端 APNs/FCM adapter；推送只携带新 cursor 提示。
- [ ] S7：真机飞行模式、kill/reopen、丢推送补同步、账号切换、损坏重建和性能指标；更新架构/功能/运维。

## 进度日志

- 2026-07-17：开始 S1；已核对总计划、现有 SQLite v1、SQLModel/Alembic 头与安全基线。下一步实现
  expand-only 四表迁移及 PostgreSQL 约束测试。
- 2026-07-17：S1 完成。四表 fresh upgrade、约束、owner 复合外键、partial unique、tombstone、
  downgrade/upgrade 和全量 PostgreSQL backend-check 均通过；开始 S2 设备注册与 credential 轮换 API。
- 2026-07-17：S2 完成。后端设备注册/列表/撤销、单次轮换与复用撤销已上线严格 OpenAPI/shared API；RN
  以 SecureStore 事务式持久化并在启动时轮换恢复，本地 fixture 支持同协议。开始 S3 change feed。
- 2026-07-17：S3 完成。Message、Opportunity、三类用户设置的公开投影在业务 flush 同事务 append；
  owner-bound bootstrap、changes、retention reset 和设备 ack 已进入严格共享 API。开始 S4 RN SQLite 消费。
- 2026-07-17：S4 完成。SQLite v2 在同一事务写 inbox/projection/cursor，支持 bootstrap 断点、reset、
  账号清理和逻辑损坏重建；前台/手动 single-flight 同步及看板/详情/消息/设置 online-first 回退已接入。
  服务端 capability 同时钳制 rollout、RN runtime 和 schema 2，默认关闭；开始 S5 内部状态命令 outbox。
- 2026-07-17：S5 完成。只有商机内部状态允许入队；客户端按 base version/7 天 expiry/稳定幂等键有界
  重放，服务端行锁事务内写 30 天幂等回执和 SyncChange。首次发送前先同步与比较版本，响应不确定沿用
  原键，409/过期/拒绝进入可打开/忽略的看板队列；bootstrap reset 保留待提交命令。开始 S6 推送提示。
- 2026-07-17：S6 完成。RN 用户显式授权后注册原生 token，监听 rollover、前台提示、点击和 background
  task；iOS 环境读取签名 entitlement。服务端加密 token、hash 去重，按已提交 SyncChange 最新 cursor 短租约
  扫描并直连 APNs/FCM v1，平台失效永久回收、暂时错误指数退避；payload 无正文。两个 rollout 默认关闭，
  开始 S7 真机/丢推送/损坏恢复和性能证据。
- 2026-07-17：S7 本地自动化闸门完成：PostgreSQL 全量 224 tests、RN 33 files/103 tests、双平台 production
  Hermes export、iOS/Android 原生 Release 构建、compose/harness 和账号切换全表清理均通过。真实
  APNs/FCM token、飞行模式、系统 kill/reopen、通知冷启动和设备性能仍需签名候选包与双真机，S7
  保持未完成且 rollout 继续关闭。
- 2026-07-17：S7 本地恢复/规模闸门补齐。文件型 SQLite 在 1 万条唯一消息的第 4 页注入断网，关闭并
  重开同一数据库后从 durable cursor 1500 继续余下 8500 条，不执行 bootstrap；最终 inbox/projection/
  cursor/ack 一致，宿主机回归为 293ms。RN 同时在确认 offline→online 时自动复用 enrollment/sync/outbox
  single-flight，推送提示不领先于 durable cursor 时跳过重复网络请求。RN 全量更新为 36 files/117 tests，
  production 原生 Release 重建为 iOS 1995 modules、Android 2134 modules/905 tasks。真实设备性能、飞行
  模式、系统 kill/reopen、冷启动和 provider 投递仍是 S7 外部门禁。

## 发现日志

- 2026-07-17：现有 `SyncChangeEnvelope` TypeScript 类型已包含目标 envelope 字段，但尚无 runtime schema、
  API 或服务端模型，不能把类型存在视为同步能力。
- 2026-07-17：平台 push token 无法只存 hash后仍执行发送；采用“加密最小值 + hash 查重”，而 device
  credential bearer 严格只存 hash。
- 2026-07-17：全局 PostgreSQL identity cursor 对每个 owner 的子序列仍严格单调，允许跨 owner 的正常
  gap；客户端不得把 gap 本身解释为丢事件，过期判定由服务端 stream 边界明确返回。
- 2026-07-17：PostgreSQL 16 没有 `jsonb_object_length`；首次 fresh upgrade 在事务中安全失败并回滚。
  capability 的数据库边界改为“JSON object + 序列化后最多 16 KiB”，后续注册 DTO 再限制最多 64 个键。
- 2026-07-17：SQLAlchemy JSONB 默认把 Python `None` 编码成 JSON `null`，无法满足 tombstone 的 SQL
  `NULL` 约束；`SyncChange.payload` 显式使用 `none_as_null=True`，并由 PostgreSQL 测试覆盖 delete envelope。
- 2026-07-17：启用自动 change capture 后，既有 PostgreSQL fixture 的临时用户删除被 owner FK 阻塞；这也
  暴露生产账号删除生命周期不完整。`sync_changes.owner_user_id` 改为 `ON DELETE CASCADE`，用户隐私删除时
  change feed 同步清除，全量 208 tests 恢复通过。
- 2026-07-17：全局回复模板只参与在线外部回复，而 P3 首批 outbox 明确禁止离线外发；因此模板不是首批
  离线重建必要数据，避免为全局目录向所有 owner 扇出 change。客户端仍从有界在线模板 API 读取。
- 2026-07-17：APNs sandbox/production 由签名 entitlement 决定，不能用 bundle 后缀推断；RN 通过
  `expo-application` 读取真实环境，服务端为 production/dev topic 分开配置。推送只提示 cursor，不能替代
  change feed，也不能把系统可能丢弃的后台通知当可靠队列。
- 2026-07-17：Expo notifications plugin 的默认 `mode` 会让无签名 production prebuild 仍生成
  development APNs entitlement；config 必须按 `RADAR_APP_VARIANT` 显式选择 production/development。
  production prebuild 已确认 `aps-environment=production` 且声明 `remote-notification` background mode。
- 2026-07-17：Compose 虽已透传 P3 配置，但 release workflow 原先没有把设备、sync、原生 audience 与
  APNs/FCM 配置写入 VPS `.env`。部署入口已补齐同名 Variables/Secrets、清除陈旧 push 私钥，并在 rollout
  开启前校验同步、dispatch 与至少一个完整 provider，默认仍全部关闭。
- 2026-07-17：S6 的 push handler 原先能取出 cursor，但每次仍无条件拉取；前台联网恢复也只依赖
  AppState/人工刷新。现在先比较 durable cursor 去重 stale/duplicate hint，并用 `expo-network` 原生事件
  订阅确认 offline→online 后触发恢复；监听状态不进入 React state，避免 Provider 子树随网络事件重渲染。

## 决策日志

- 2026-07-17：P3 采用 aggregate upsert/tombstone feed，不把现有多 commit 用例包装成虚假的原子领域
  事件；客户端按 aggregate version 幂等覆盖。
- 2026-07-17：第一迁移同时建立 Device/DeviceCredential/PushRegistration/SyncChange 数据边界，但不
  同时开放凭据或推送 API；先验证持久化约束，再增加高风险入口。
- 2026-07-17：cursor 使用数据库生成的全局 `BIGINT IDENTITY`；API 始终 owner-scoped，因而每个 owner
  看到的 cursor 严格递增但可有 gap，避免热点用户计数器锁和并发重复 cursor。
- 2026-07-17：采用 SQLAlchemy `before_flush` 捕获五类 owner 聚合的新建/更新/删除，统一递增
  `aggregate_version` 并在同一事务 append；序列化器只输出移动公开投影，不包含 IM raw payload。并发从
  同一版本分叉由 owner/type/id/version 唯一约束拒绝，不静默覆盖两条同版本 change。
- 2026-07-17：bootstrap 默认设置使用 version 0 表示“尚无持久化 override”，实际 change version 从 1
  开始；分页 watermark 固定在首请求，后续并发写由 watermark 后 change 幂等补齐。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过（2026-07-17，S7） | 53 Markdown files、93 backend Python files、8 harness tests |
| `make backend-check`（PostgreSQL 16） | 通过（2026-07-17） | 182 passed，0 skipped；compileall/ruff 通过，包含全部 repository/owner 隔离/并发测试 |
| PostgreSQL fresh upgrade/downgrade/upgrade | 通过（2026-07-17） | fresh 到 `202607170002`；downgrade 后仅四个 P3 表消失且 users/manual reply 保留；再 upgrade 回 head |
| P3 persistence constraint tests | 通过（2026-07-17） | metadata 4 tests + PostgreSQL 4 tests；覆盖 owner 唯一/复合外键、单 active、hash、生命周期、cursor、重复版本、tombstone 和未知 operation |
| P3 device session PostgreSQL tests | 通过（2026-07-17） | 6 tests；覆盖 hash-only 重注册、轮换/reuse、跨 owner 撤销、设备绑定 access token、并发单次消费、上限与过期 |
| P3 device API/security tests | 通过（2026-07-17） | 7 tests；覆盖 DTO 边界、header 注入、缓存禁止和响应脱敏 |
| `radar-contracts` / `radar-api` | 通过（2026-07-17） | 严格设备 schema；radar-api 10 files / 39 tests |
| RN typecheck / unit tests | 通过（2026-07-17） | 25 files / 74 tests；包含 SecureStore 原子持久化、启动轮换、网络错误保留和注册 metadata |
| 本地 auth fixture 设备协议 smoke | 通过（2026-07-17） | register 201、rotate 200、旧 bearer reuse 401 并撤销设备；仅进程内 fixture |
| S3 capture/feed PostgreSQL tests | 通过（2026-07-17） | 8 tests；覆盖同事务 append/rollback、raw payload 排除、tombstone、并发同版本拒绝、分页/default、owner token、retention reset 与设备 ack |
| S3 API/shared tests | 通过（2026-07-17） | 后端 routes/device-bound gate 10 tests；radar-api 11 files / 44 tests；未知 schema、乱序、重复、泄漏字段和 reset 一致性均 fail closed |
| `make backend-check`（PostgreSQL 16，S3） | 通过（2026-07-17） | 208 passed，1 个上游 TestClient deprecation warning；compileall/ruff 通过 |
| `202607170003` downgrade/upgrade | 通过（2026-07-17） | downgrade 后版本/ack 列消失且 users/messages 保留；upgrade 恢复，owner FK `confdeltype=c` |
| `alembic check` | P3 无漂移；全仓仍失败 | 未检测到四个 P3 表的 upgrade operation；仓库已有 archive FK、三个 Text 类型和 Telegram enabled 索引历史漂移，未在本切片顺手修改 |
| P3 S4 RN SQLite / offline tests | 通过（2026-07-17） | 30 files / 90 tests；真实内存 SQLite 覆盖原子应用、重放/冲突回滚、断点/reset/ack、owner 隔离、筛选/分页、capability 持久化和投影损坏恢复标记 |
| `make rn-check`（S4） | 通过（2026-07-17） | contracts/shared/mobile 检查通过，iOS/Android Expo production export 成功 |
| `make backend-check`（PostgreSQL 16，S4） | 通过（2026-07-17） | 209 passed，0 skipped，1 个上游 TestClient deprecation warning；含设备 capability 契约 |
| `make harness-check`（S4） | 通过（2026-07-17） | 53 Markdown links、88 backend Python files、8 harness tests |
| P3 S5 internal command PostgreSQL tests | 通过（2026-07-17） | 7 tests；覆盖 version precondition、幂等重放/重绑定、并发同键单次应用、稳定 409 与 header/body 配对 |
| P3 S5 RN outbox tests | 通过（2026-07-17） | 31 files / 97 tests；真实 SQLite 覆盖上限、expiry、成功、preflight/服务端冲突、响应不确定重试、reset 保留和用户可见摘要 |
| `make backend-check` / `make rn-check`（S5） | 通过（2026-07-17） | backend 216 passed / 0 skipped；contracts/shared/mobile 与双平台 Expo production export 通过 |
| P3 S6 push route/provider/生命周期 tests | 通过（2026-07-17） | 后端定向 20 tests；覆盖 capability、响应脱敏、hash+加密、轮换、短租约/cursor 推进、APNs/FCM data-only payload、OAuth token 复用和平台无效 token 回收 |
| P3 S6 RN/shared tests | 通过（2026-07-17） | radar-api 11 files / 47 tests；RN 33 files / 103 tests；严格 push 契约、签名 provider/environment 映射、Expo production APNs mode、嵌套 cursor 解码和账号切换全表清理 |
| `202607170005` downgrade/upgrade | 通过（2026-07-17） | push delivery cursor/lease 两列与检查/index 可逆；`alembic check` 未新增 S6 漂移，仍只有既有 archive/Text/Telegram 漂移 |
| `make backend-check`（S6/S7） | 通过（2026-07-17） | PostgreSQL 16 下 224 passed、0 skipped；compileall/ruff 通过，1 条上游 TestClient deprecation warning |
| `make rn-check` + 双平台 production export（S6/S7） | 通过（2026-07-17） | frozen workspace、OpenAPI/shared、Expo compatibility、36 files/117 tests；双平台 production Hermes export 通过 |
| S7 SQLite 规模/重启恢复 | 通过（2026-07-17） | 文件型 Node SQLite 分页写入 10,000 条唯一消息；cursor 1500 后注入断网，关闭/重开同一文件并以 17 页续传余下 8500 条，无 bootstrap；inbox/projection/cursor/ack 一致，293ms 小于 30s 宿主机退化上限，不替代设备性能 |
| S7 push cursor / 网络生命周期 tests | 通过（2026-07-17） | 已覆盖 stale/equal/ahead hint、本地状态缺失/读取失败、初始网络 seed、unknown、offline→online 单次触发及 transport 切换去重 |
| RN production iOS native Release build（S7） | 通过（2026-07-17） | clean production prebuild 后以 iOS Simulator SDK、`CODE_SIGNING_ALLOWED=NO` 完成 Release `xcodebuild`；1995-module Hermes bundle 与 Expo Notifications/Task Manager/Application/SQLite/Network 均进入产物，exit 0；仅有 React Native/Expo 等上游 warning，未代替签名真机运行态 |
| RN production iOS Release 冷启动（S7） | 通过（2026-07-17） | production Release 安装到 iPhone 16e 模拟器并冷启动到登录页，进程持续存活且无 Expo Network 错误；无签名模拟器仍报告预期的 Notifications Keychain `-34018`，因此不作为登录、推送或断网恢复验收 |
| RN production Android native Release build（S7） | 通过（2026-07-17） | clean prebuild 后 `:app:assembleRelease` 完成 2134-module Hermes bundle、四 ABI CMake、lintVital 与 Release APK，905 tasks，`BUILD SUCCESSFUL in 7m 12s`；merged manifest 为 `com.codeiy.im`、cleartext=false，含通知、网络状态权限与 Expo/FCM service；未代替真机运行态 |
| Expo production prebuild entitlement | 通过（2026-07-17） | `RADAR_APP_VARIANT=production` 生成 `aps-environment=production`；iOS `UIBackgroundModes` 包含 `remote-notification` |
| local/prod compose config | 通过（2026-07-17） | 以占位 Cloudflare token 解析；设备/sync/push rollout、APNs/FCM 和 dispatch 参数已透传，默认均 fail closed |
| release workflow config | 通过（2026-07-17） | YAML 可解析；设备/sync/native audience/push Variables 与 Secrets 显式写入 `.env`，push rollout 开启前 fail-closed 校验 provider |

## 回滚与恢复

S1-S3 均为 additive：旧代码不引用新表。回滚应用后可保留新表；确认没有新客户端依赖后才能执行
downgrade 删除它们。credential/推送 API 上线后，紧急回滚先关闭 capability 并撤销 active credential/
registration；绝不恢复已检测复用的 bearer。同步客户端遇到 schema/cursor 不兼容时清 owner 副本并重新
bootstrap，不修改 PostgreSQL 业务权威数据。

## 结果与剩余风险

尚未完成。S1-S6 代码切片已完成；S7 的账号切换全表清理、投影损坏恢复、1 万条宿主机重启续传、
推送 cursor 去重和 offline→online 自动恢复已有自动化覆盖。真机飞行模式、系统 kill/reopen、丢推送、
通知冷启动和设备性能证据尚未完成。
`RN_SYNC_ROLLOUT_ENABLED`、`RN_PUSH_ROLLOUT_ENABLED` 与 provider dispatch 默认均为 false，不能描述为
生产离线或生产推送能力；可见的新商机/AI 回复/每日摘要通知也不在 cursor-only 唤醒切片内。
