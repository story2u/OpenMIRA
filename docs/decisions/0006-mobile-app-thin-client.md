# ADR-0006：移动端作为现有 API 的瘦客户端（Swift/Kotlin 原生双端）

> 状态：proposed · 日期：2026-07-11

## 背景

商机时效性强，Web 端依赖 30 秒轮询和用户守在桌面前，非工作时间与移动场景缺少触达和处理
能力。后端 v1 REST API 已按 owner 隔离，覆盖商机列表、详情、消息历史、回复、状态流转全流程。
需要同时交付 iOS 与 Android；产品决策明确要求 iOS 使用 Swift、Android 使用 Kotlin 原生开发，
不采用跨端框架。

## 决策

- 新建 `mobile/ios/`（Swift + SwiftUI + Swift Concurrency）与 `mobile/android/`
  （Kotlin + Jetpack Compose + Coroutines/Flow）两个原生工程；双端共享契约与蓝图，不共享代码。
- App 直连现有 FastAPI v1 REST API + JWT，不引入 BFF/GraphQL 聚合层。
- 每端保持唯一网络边界（iOS `Core/Network`、Android `core/network`），后端 DTO 以手写镜像
  维护（Swift Codable / Kotlin `@Serializable` data class）；Web 中的演示态能力一律不移植。
- 推送是唯一新增后端能力：DeviceToken（含 platform）模型 + 注册 API + Celery 发送任务 +
  `PUSH_ENABLED` 安全阀；后端直连 APNs（token-based `.p8`）与 FCM v1（service account）投递，
  payload 只携带事件类型与 ID，不携带消息内容。
- 移动端 OAuth 采用原生登录（Sign in with Apple / Google Sign-In）+ 后端 id_token 校验端点
  （复用既有 JWKS 验签），不复用面向 Web 的 callback 重定向。

## 备选方案

- React Native + Expo：单代码库、可镜像现有 TS 类型、交付成本约一半；被产品决策否决
  （要求原生），且原生方案在推送、深链与平台能力上没有桥接层。未选择。
- Flutter：单代码库；同样被原生要求排除，且 Dart 与现有 TS 契约割裂。未选择。
- Kotlin Multiplatform 共享业务/网络层 + 双端原生 UI：可减少 DTO 双写；P0 契约面小，
  引入共享层的构建复杂度不划算。未选择，DTO 漂移成为负担时复审。
- PWA：零上架成本；iOS 推送可靠性与留存差，而推送是移动端核心价值。未选择。
- BFF/GraphQL：聚合灵活；当前无聚合需求。未选择。

## 后果

- 双份实现与测试，功能交付成本约 2×；每个功能切片按平台拆分，可由两个代理并行开发。
- 契约同步链扩为四端：后端 Pydantic DTO → Web types → iOS Codable → Android data class；
  手写镜像起步，FastAPI 已暴露 `/openapi.json`，漂移频发时复审 OpenAPI 代码生成。
- CI 需要 macOS runner 执行 iOS 检查；Android 检查可留在 Linux。
- APNs `.p8` 与 FCM service account 凭据进入后端 secrets 管理，遵守安全基线。
- 换来的收益：无桥接层的原生体验，推送、深链、系统集成直接使用平台能力。
- App Store 上架要求 Apple 登录（后端 provider 已具备）；app 内订阅升级受 IAP 规则约束，
  P1 只做只读展示并引导去 Web。

## 验证与复审

P0 验收（见[移动端 App P0 计划](../plans/active/2026-07-11-mobile-app.md)）在双端端到端跑通
即视为决策有效。复审信号：双端并行交付节奏不可持续或 DTO 漂移频发时，复审 Kotlin
Multiplatform 共享层或 OpenAPI 代码生成；出现第二类客户端聚合需求时复审 BFF。
