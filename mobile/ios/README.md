# iOS App（商机雷达 / OpportunityRadar）

Swift 6 + SwiftUI 原生 app，作为现有后端 v1 REST API 的瘦客户端。本文件是 iOS 目录的
入口与迭代状态，供后续 AI（Claude/Codex）跟进。完整产品规格、模块优先级与后端契约见
[移动端 App 蓝图](../../docs/plans/active/2026-07-11-mobile-app-blueprint.md)，执行进度与
决策见 [P0 计划](../../docs/plans/active/2026-07-11-mobile-app.md)，栈决策见
[ADR-0006](../../docs/decisions/0006-mobile-app-thin-client.md)。

> 状态是当前事实，与代码冲突时以代码为准并同步修正本文件。

## 技术栈与约束

- Swift 6 严格并发；SwiftUI + `@Observable` 轻量 MVVM，不引全局 store。
- 工程由 xcodegen 从 `project.yml` 生成：`.xcodeproj` 是产物、不入库，改工程配置改
  `project.yml`。`Info.plist` / `entitlements` 是手写源文件（xcodegen 只用 `INFOPLIST_FILE`
  引用，不再生成，避免每次 generate 覆盖）。
- `Core/Network` 是唯一 HTTP 边界；JWT 只存 Keychain，不进 UserDefaults/日志。
- DTO 手写镜像后端 `application/dto.py`，枚举容错（未知值落 `unknown`，防后端新增值导致
  旧版本整列表解码失败）。

## 文件地图

| 路径 | 职责 |
| --- | --- |
| `App/OpportunityRadarApp.swift` | 入口 + 会话路由（恢复中/登录/收件箱） |
| `Core/AppConfig.swift` | API 根地址：Release 读 Info.plist，DEBUG 走本地 + `RADAR_API_URL` 覆盖 |
| `Core/Network/APIClient.swift` | 唯一 HTTP 边界：JWT 注入、ISO8601（含小数秒）解码、错误映射 |
| `Core/Network/Endpoints.swift` | 各资源方法，路径参数以 `backend/app/api/v1/routes/` 为准 |
| `Core/Auth/Keychain.swift` | JWT 的 Keychain 读写清 |
| `Core/Auth/SessionStore.swift` | `/auth/me` 会话恢复、邮箱密码/Apple 登录、登出 |
| `Core/Billing/` | RevenueCat 身份、Offering、购买与恢复协议边界；只使用 Public iOS Key |
| `Features/Auth/RadarLogoView.swift` | 应用内品牌 Logo；不依赖系统管理的 AppIcon 运行时读取 |
| `Features/Auth/LoginView.swift` | 邮箱密码登录 + Sign in with Apple |
| `Features/Inbox/InboxView.swift` | 收件箱：筛选、分页、30s 轮询 |
| `Features/Opportunity/OpportunityDetailView.swift` | 详情、消息历史、Agent 发现、回复、状态流转 |
| `Features/Subscription/` | App Store 套餐页；购买/恢复后由后端同步确认权益 |
| `Models/Models.swift` | DTO 镜像 + 容错枚举 + JSONValue |
| `Tests/ModelsDecodingTests.swift` | 解码/编码契约测试 |
| `Assets.xcassets/` | App 图标（1024 无 alpha） |

## 已实现（P0，iOS 侧）

- **登录**：已有密码账户通过 `POST /auth/password/login` 登录；Sign in with Apple 通过
  `POST /auth/oauth/apple/native` 换 JWT；`/auth/me` 冷启动恢复会话；登出清 Keychain。
  DEBUG 与 Release 均不提供粘贴 JWT 的旁路入口。
- **收件箱**：`GET /opportunities`，状态/渠道筛选、分页、下拉刷新、30s 轮询。
- **详情**：`GET /opportunities/{id}` + `GET /messages`，展示概要、检测依据、Agent 发现
  （链接核验、联系方式、动作建议、attention 标记）、消息历史。
- **回复**：手动回复真实发送、AI 草稿（可编辑后发送）、模板只读选用；发送失败报错且不
  伪造已回复状态；AI 额度耗尽展示后端提示。
- **状态流转**：认领 / 跟进 / 忽略 / 关闭，走后端状态机，非法迁移展示后端错误。
- **订阅代码链路**：登录后用后端用户 UUID 绑定 RevenueCat，加载 `default` Offering，支持
  App Store 月付/年付购买、用户主动恢复购买、后端权益同步和同渠道管理入口。未配置 Public Key
  时保持只读且关闭购买；真实 App Store Sandbox E2E 仍待外部 Dashboard 配置后验证。

## 待开发

按 [P0 计划](../../docs/plans/active/2026-07-11-mobile-app.md) 的步骤编号：

- **登录余项**：Google Sign-In（需接入官方 SPM 依赖，后端 `POST /auth/oauth/google/native`
  已就绪，只差前端）。
- **步骤 8-9 推送**：后端 DeviceToken + 注册 API + APNs 适配器 + Celery 任务（步骤 8）落地后，
  iOS 做推送注册/接收/深链，并把收件箱从轮询改为推送触发刷新。
- **P1**：Telegram 连接引导、Agent 动作审批、今日概览（见蓝图 P1 表）。
- **CI**：`make ios-check` 尚未接入 CI；需要 macOS runner，且单元测试（`xcodebuild test`）
  未纳入 `ios-check`，接 CI 时一并补。
- **Android**：iOS 真机联调通过后按同一蓝图与契约开工（`mobile/android/`）。

## 本地开发

```bash
cd mobile/ios && xcodegen generate && open OpportunityRadar.xcodeproj   # ⌘R 跑模拟器
make ios-check                                                          # 生成工程 + 不签名编译（需 macOS + xcodegen）
```

DEBUG 默认连本地后端 `http://127.0.0.1:8000/api/v1`；要连线上联调，在 scheme 里设环境变量
`RADAR_API_URL=https://im.story2u.xyz/api/v1`。本地创建演示账户密码：起后端后运行
`docker compose run --rm api python scripts/dev_login.py`，再用脚本输出的邮箱和临时密码登录（详见
[开发命令](../../docs/development/commands.md)）。

RevenueCat Public iOS API Key 通过构建设置 `REVENUECAT_IOS_PUBLIC_API_KEY` 注入。该值是平台
Public SDK Key，不得填 RevenueCat 服务端 Secret；未配置时 App 正常运行但不能购买或恢复。

## 发布到 TestFlight / App Store

Release 构建已就绪：生产 API 地址烧进 Info.plist（`RadarAPIBaseURL`）、含 1024 无 alpha
图标、声明 `ITSAppUsesNonExemptEncryption=false`（免出口合规问答）。上传需 Apple Developer
账号与 App Store Connect 中 bundle id 为 `com.codeiy.im` 的 app 记录。

1. `cd mobile/ios && xcodegen generate && open OpportunityRadar.xcodeproj`
2. target → Signing & Capabilities → 勾自动签名、选 Team（Sign in with Apple capability 已在
   entitlements 内，自动带上）。
3. 设备选 “Any iOS Device” → Product → Archive → Organizer → Distribute App → App Store
   Connect → Upload。
4. App Store Connect → TestFlight → 加内部测试员 → 手机装 TestFlight 安装。

线上后端 `APPLE_NATIVE_CLIENT_IDS` 默认已是 `com.codeiy.im`（与 bundle id 一致），真机
Apple 登录直连线上后端；若报 401，检查 Apple 开发者后台该 App ID 是否开启 Sign in with
Apple。邮箱密码登录只适用于已有 `password_hash` 的账户。

版本号在 `project.yml` 的 `MARKETING_VERSION` / `CURRENT_PROJECT_VERSION`，每次上传按需 bump。
