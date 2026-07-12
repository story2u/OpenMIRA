# Android App（Kotlin + Jetpack Compose）

尚未脚手架。按[移动端 App 蓝图](../../docs/plans/active/2026-07-11-mobile-app-blueprint.md)与
[P0 计划](../../docs/plans/active/2026-07-11-mobile-app.md)步骤 1 的 Android 子任务创建
Gradle 工程：`app/src/main/kotlin/<pkg>/{feature,core,model}`，验证入口 `make android-check`
（`./gradlew lint test`）。iOS 端已先行实现，可对照 `mobile/ios/` 的模块划分与 API 契约。
