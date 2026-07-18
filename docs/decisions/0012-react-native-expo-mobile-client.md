# ADR-0012：移动端迁移到 React Native + Expo 单代码库

> 状态：accepted · 日期：2026-07-17 · supersedes ADR-0006

## 背景

ADR-0006 提议以 SwiftUI 与 Jetpack Compose 维护两个瘦客户端。该提议落地后，仓库已经出现三份
手写 DTO、两份移动网络层和两份移动 UI；推送、离线存储和端上 Agent 尚未实现。新的产品方向要求
把 harness、会话与数据副本放到设备，现有 TypeScript Web/Agent 资产比双份原生业务层更适合复用。

截至本决策日期，Expo 官方稳定矩阵为 SDK 57、React Native 0.86、React 19.2.3；New Architecture
与 Hermes 是默认运行形态，Expo dev client 支持自定义 Swift/Kotlin 模块。版本是实施时的锁定基线，
后续升级必须按 Expo 官方兼容矩阵整体升级，不能独立漂移 React Native 版本。

## 决策

- 新建 `mobile/radar/`，使用 React Native + Expo dev client/prebuild + Hermes + TypeScript strict；
  不使用 Expo Go 作为功能验收环境。
- 生产升级继续使用 iOS bundle id 与 Android application id `com.codeiy.im`；开发和并行 dogfood 使用
  后缀 ID，避免覆盖当前原生 App。
- SwiftUI/Compose App 在 RN 通过功能对齐、登录迁移、订阅、深链和双平台 release 验收前保留；期间只做
  安全与后端契约兼容修复。RN 商店换装后仍保留至少两个 App 版本的服务端兼容窗口。
- 共享 TypeScript 按职责拆到 `packages/radar-contracts`、`radar-core`、`radar-api`、`radar-agent`；平台
  UI 与 native modules 留在 `mobile/radar`，不让共享包依赖 React Native、Next.js 或数据库实现。
- 原生平台能力优先使用审查过的 Expo SDK 模块；自定义能力使用 Expo Modules API。不得新增旧 Bridge
  NativeModule，也不得下载并执行运行时代码。
- RN 必须迁移现有登录状态：iOS 按旧 Keychain service/account 兼容读取；Android 通过一次性原生迁移
  读取旧 EncryptedSharedPreferences。读取失败时要求重新登录，不得记录 token 或密文。
- 客户端推送 API 使用 `expo-notifications`，但后端投递保持可替换的 APNs/FCM adapter；本决策不强制
  使用 Expo Push Service、EAS 云构建或 OTA 更新。
- 根 JS monorepo 使用 pnpm；`backend/pi-agent-runtime` 在单独决策前保留 npm 精确锁和现有 Docker
  构建入口，避免同时迁移应用框架与生产 runner 包管理。

## 备选方案

- **继续 Swift/Kotlin 双端**：原生能力直接，但 DTO、离线存储、Agent 会话和 UI 需要双写，已经触发
  ADR-0006 的复审条件。
- **Kotlin Multiplatform + 双原生 UI**：可共享部分数据层，仍不能复用现有 TS Agent/runtime，且引入
  第三套共享语言和构建链。
- **独立 RN 仓库**：边界清晰，但共享契约、golden fixtures 与后端 runner 更容易漂移，不符合当前
  单仓同步变更纪律。
- **Bare React Native**：可行，但 Expo dev client/prebuild 已覆盖本项目所需的路由、推送、安全存储、
  SQLite 和自定义模块，直接 bare 会增加无用户价值的原生构建维护面。

## 后果

正面影响：移动业务/UI 只实现一份；能直接复用 TS 契约与 Agent 适配层；离线、推送和原生模块仍可
按平台实现。成本：新增 Metro/Expo/EAS 可选工具链；原生换装期间同时维护两代 App；必须解决 token、
RevenueCat、OAuth audience、深链和商店升级兼容。

本决策不证明 pi 能在 Hermes 运行；该能力由 ADR-0013 和 P0 双平台 release spike 单独裁决。即使 pi
spike 失败，RN 客户端迁移仍继续，Agent 暂留服务端。

## 验证与复审

- `make rn-check` 必须包含 frozen install、lint、typecheck、unit/schema drift、Expo doctor 和双平台构建。
- 以 [RN 功能对齐矩阵](../plans/active/2026-07-17-rn-parity-matrix.md)逐行验收；未对齐项不能靠 mock
  或隐藏失败状态标记完成。
- 真实换装前验证旧版 token、RevenueCat entitlement、OAuth、深链、推送 token、账号切换与登出清理。
- 若 Expo SDK 阻塞必须的原生能力，先尝试最小 Expo Module；只有可复现的性能/兼容证据才能复审 bare RN。
