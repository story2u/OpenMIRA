# 移动端 App 蓝图（iOS / Android）

> 状态：设计基线（尚无代码） · 隶属计划：[移动端 App P0](2026-07-11-mobile-app.md) · 创建：2026-07-11 · 更新：2026-07-11

本文档是移动端 App 的架构与功能规范，供后续 AI 代理（Claude/Codex）据此开发。它描述目标
设计，不是当前事实；后端与 Web 的当前事实以[架构总览](../../architecture/overview.md)、
[功能地图](../../product/feature-map.md)和代码为准，冲突时先核对代码再修正本文档。
栈选型与边界决策的原因见 [ADR-0006](../../decisions/0006-mobile-app-thin-client.md)。

## 产品定位

移动端是「商机处理端」，不是 Web 的全量移植。核心价值是 Web 端缺失的一环：**推送触达 +
随手处理**（收到商机 → 审核 → 回复 → 跟进，全程在手机完成）。规则管理、模板编辑、企业微信
绑定等重配置场景留在 Web。

## 技术栈与仓库位置

产品决策要求原生双端：iOS 使用 Swift，Android 使用 Kotlin（见 ADR-0006）。双端共享本蓝图
与后端契约，不共享代码；每个功能切片按平台拆分，可由两个 AI 代理并行开发。

| 决策项 | 选择 | 说明 |
| --- | --- | --- |
| 代码位置 | `mobile/ios/`、`mobile/android/`，与 `frontend/`、`backend/` 并列 | 单仓双原生工程，契约同步在同一变更内完成 |
| iOS | Swift + SwiftUI + Swift Concurrency（async/await） | Xcode 工程 |
| Android | Kotlin + Jetpack Compose + Coroutines/Flow | Gradle 工程 |
| 网络层 | iOS：URLSession + Codable；Android：Retrofit + kotlinx.serialization | 每端唯一 HTTP 边界 |
| 架构 | 每端轻量 MVVM（SwiftUI `@Observable` / Android ViewModel + StateFlow） | 不建全局 store；禁止复刻 Web `AppStoreProvider` 的演示态混合模式 |
| 凭据存储 | iOS Keychain；Android Keystore + EncryptedSharedPreferences | JWT 不得进 UserDefaults/SharedPreferences 明文或日志 |
| 推送 | iOS APNs、Android FCM，后端直连投递 | payload 最小化，见 ADR-0006 |

## 与后端的契约

- App 直连 v1 REST API + JWT Bearer，不建 BFF。
- 每端唯一 HTTP 边界：iOS `Core/Network`、Android `core/network`，职责等同
  `frontend/lib/api.ts`。
- 契约同步链：后端 Pydantic DTO → `frontend/lib/types.ts` → iOS Codable（`Models/`）→
  Android data class（`model/`）。后端字段变化必须四端连贯更新
  （[功能地图](../../product/feature-map.md)同步清单的扩展）。
- P0 契约面小，DTO 采用手写镜像；FastAPI 已暴露 `/openapi.json`，漂移成为负担时按
  ADR-0006 复审 OpenAPI 代码生成。
- API 具体查询参数、错误码以 `backend/app/api/v1/routes/` 与 DTO 为准，本文档只锚定端点。

## 目标目录结构

```
mobile/
  ios/                     # Swift + SwiftUI（Xcode 工程）
    App/                   # 入口、导航、深链路由
    Features/              # Inbox / Opportunity / Settings
    Core/                  # Network（唯一 API 边界）、Auth、Push
    Models/                # 后端 DTO 的 Codable 镜像
  android/                 # Kotlin + Compose（Gradle 工程）
    app/src/main/kotlin/<pkg>/
      feature/             # inbox / opportunity / settings
      core/                # network（唯一 API 边界）、auth、push
      model/               # 后端 DTO 的 data class 镜像
```

## 功能模块

双端功能对齐：下表每一行都要求 iOS 与 Android 各自实现并分别满足验收要点。

### P0 — 最小可用闭环

除推送外全部依赖已存在的后端能力；成熟度结论引自[功能地图](../../product/feature-map.md)。

| # | 模块 | 依赖 API | 后端现状 | 验收要点 |
| --- | --- | --- | --- | --- |
| 1 | 登录 | `POST /auth/password/login`、`POST /auth/oauth/{provider}/native`、`GET /auth/me` | 均已实现（原生 id_token 校验 + 邮箱密码登录） | iOS：Sign in with Apple；Android：邮箱密码。token 存 Keychain/加密存储；冷启动经 `/auth/me` 恢复会话。Google 原生登录为双端后续切片 |
| 2 | 商机收件箱 | `GET /opportunities` | 已实现，owner 隔离 | 列表、状态/渠道筛选、分页、下拉刷新；P0 先轮询，推送落地后改为推送触发刷新 |
| 3 | 商机详情 | `GET /opportunities/{id}`、`GET /messages` | API 已实现；Web 未消费这两个端点，不得照抄 Web 本地 store 实现 | 独立请求详情与消息历史；展示检测结果与 Agent 发现（链接核验结论、联系方式、紧急标记） |
| 4 | 回复 | `POST /opportunities/{id}/manual-reply`、`POST /opportunities/{id}/ai-draft`、`GET /templates` | 后端真实发送/生成/落库 | 手动回复真实发送；AI 草稿可编辑后发送；模板只读选用；发送失败可重试且不得伪造已回复状态 |
| 5 | 状态流转 | `PATCH /opportunities/{id}/status`、`POST /opportunities/{id}/claim` | 后端已实现（领域状态机约束） | 认领/跟进/关闭走后端状态机；非法迁移展示后端错误而非本地吞掉 |
| 6 | 推送通知 | 设备注册 API + Celery 发送任务（均待新增） | 后端缺失，唯一新增面 | 新商机、重大商机提醒、AI 自动回复结果、额度耗尽四类事件；点通知深链进详情；`PUSH_ENABLED=false` 时全链路安全关闭 |

### P1 — 差异化能力

| # | 模块 | 依赖 | 说明 |
| --- | --- | --- | --- |
| 7 | Telegram 连接引导 | 统一连接 API（`features/telegram-native-connections` 分支合入后可用） | app 深链拉起 Telegram 完成群授权后回跳；移动端体验优于 Web |
| 8 | Agent 动作审批 | 商机详情中的 Agent 投影；批准后的执行用例后端暂无 | 只读建议列表 + 批准/驳回意向落库；未经独立审批用例不得触发任何外部发送 |
| 9 | 订阅与用量 | `GET /subscriptions/plans`、`GET /subscriptions/me` | 只读展示套餐、AI 额度、TG 限额；升级引导跳 Web（规避 IAP 抽成与审核复杂度） |
| 10 | 今日概览 | `GET /stats/summary` | API 已实现且当前无消费方；做商机数/回复率小面板 |

### P2 — 后置

- 通知偏好与每日摘要：需要新增用户通知偏好 API（Web 端此功能也仅是演示态）。
- 工作时间快捷开关：`/configs/work-mode` 已有，低频操作。
- 离线只读缓存：有真实需求再做。

### 明确不移植

- 好友申请执行按钮（Web 演示态，timer 模拟，无后端）。
- Web 的通知偏好开关（仅页面 state）。
- 规则管理、模板编辑、企业微信绑定（留在 Web）。

## 后端新增面

移动端要求的后端改动只有两块，均需遵守[开发规范](../../development/standards.md)与
[安全基线](../../quality/security.md)：

1. **移动登录端点（P0）**：`POST /auth/oauth/{provider}/native` 校验原生登录取得的
   id_token（复用 `app/core/security.py` 的 JWKS 验签），签发与 Web 相同的 JWT。现有
   callback 面向 Web 重定向，不适配 app。
2. **推送通道（P0）**：DeviceToken 模型（含 platform=ios/android）+ Alembic 迁移、
   注册/注销 API（按用户隔离）、`infrastructure/push/` 下的 APNs（token-based `.p8`）与
   FCM v1（service account）适配器、Celery 发送任务挂接三个既有事件点（摄取用例的审核者
   通知、重大商机 attention 投影、AI 自动回复结果），新增 `PUSH_ENABLED` 环境变量作为与
   `IM_SEND_ENABLED` 同级的安全阀。投递凭据经环境/secret 注入，不入库不入日志。
3. **通知偏好 API（P2）**：与 app 通知设置页同期设计。

## 安全与不变量

承接[安全基线](../../quality/security.md)与架构总览的主要不变量，移动端追加：

- JWT 只存 Keychain（iOS）/ Keystore 加密存储（Android）；日志与崩溃上报不得包含 token、
  消息内容、联系方式。
- 推送 payload 最小化：只含事件类型 + 资源 ID，打开 app 后凭 JWT 拉取详情。
- owner 隔离由后端保证；app 切换账号时必须清空本地缓存与内存态。
- 深链参数先校验（资源 ID 格式、当前登录态）再路由。
- 发送失败不得在 UI 伪造已回复；AI 操作入口展示剩余额度，额度耗尽（后端 fail-closed）
  给出明确提示而非静默失败。

## 验证入口（随 P0 落地）

- `make android-check`：`./gradlew lint test`，Linux CI 可执行。
- `make ios-check`：xcodegen 生成工程 + 不签名编译，需要 macOS（CI 使用 macOS runner）。
- iOS 目录入口与实现状态见 [`mobile/ios/README.md`](../../../mobile/ios/README.md)（含文件地图、
  待办、TestFlight 发布流程）。
- 每个切片的完成定义：接真实 API、处理 loading/error/空态、通过对应平台检查。
- 进度、发现与决策写回[执行计划](2026-07-11-mobile-app.md)，不留在会话里。
