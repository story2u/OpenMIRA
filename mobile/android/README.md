# Android App（商机雷达 / Kotlin + Jetpack Compose）

Kotlin + Compose 原生 app，作为后端 v1 REST API 的瘦客户端，与 `mobile/ios/` 功能对齐。
产品规格与后端契约见[移动端 App 蓝图](../../docs/plans/active/2026-07-11-mobile-app-blueprint.md)，
执行进度见 [P0 计划](../../docs/plans/active/2026-07-11-mobile-app.md)。

> 状态是当前事实，与代码冲突时以代码为准并同步修正本文件。

## 技术栈与约束

- Kotlin 2.1 + Jetpack Compose（Material3）+ Coroutines/StateFlow，轻量 MVVM，无 DI 框架。
- 网络：Retrofit + kotlinx.serialization；`core/network` 是唯一 HTTP 边界。
- DTO 手写镜像后端 `application/dto.py`：枚举属性带 `UNKNOWN` 默认值 + `coerceInputValues`，
  后端新增枚举值时旧版本解码落到 UNKNOWN 而不是整个请求失败（对齐 iOS TolerantEnum）。
- JWT 存 EncryptedSharedPreferences（Keystore 加密），不进明文存储与日志。
- API 地址由 BuildConfig 注入：debug `http://10.0.2.2:8000/api/v1`（模拟器访问宿主机），
  release `https://im.story2u.xyz/api/v1`；cleartext 仅 debug manifest 放行。

## 文件地图

| 路径（`app/src/main/kotlin/com/codeiy/im/`） | 职责 |
| --- | --- |
| `MainActivity.kt` | 入口 + 会话路由 + Navigation Compose（双 Tab：dashboard/settings + 详情/设置子页 + 深链） |
| `core/network/ApiClient.kt` | Retrofit 构造、Bearer 注入、`{"detail"}` 错误映射、401 统一回调清会话 |
| `core/network/RadarApi.kt` | 全部端点定义（含 dashboard/settings/telegram），路径以 `backend/app/api/v1/routes/` 为准 |
| `core/auth/TokenStore.kt` | JWT 加密存取清 |
| `core/auth/SessionStore.kt` | `/auth/me` 会话恢复、邮箱密码 / Google 原生登录、登出 |
| `core/billing/` | RevenueCat 身份、Offering、Google Play 购买与恢复协议边界 |
| `model/Models.kt` | DTO 镜像 + 容错枚举 + `RadarJson` 配置（含看板/设置/Telegram DTO） |
| `ui/theme/` | 语义色/可信度/SOP、DayNight 主题、AppBadge/AppCard/ConfidenceBadge |
| `feature/login/LoginScreen.kt` | 邮箱密码 / Google 原生登录（本地校验 + IME 流转） |
| `feature/dashboard/` | 商机看板：DashboardScreen/ViewModel/OpportunityCard/FilterSheet/Models |
| `feature/settings/` | 设置中心 + 识别规则/工作时间/通知/Telegram 子页 |
| `feature/opportunity/OpportunityDetailScreen.kt` | 详情、消息历史、Agent 发现、回复、状态流转、下拉刷新 |
| `feature/subscription/` | Google Play 套餐页；购买/恢复后由后端同步确认权益 |
| `feature/inbox/InboxScreen.kt` | 旧收件箱（已被看板取代，保留 Badge 供详情页复用） |
| `ui/Time.kt` | ISO8601 相对时间/短时间格式化 |
| `../test/.../ModelsDecodingTest.kt` | 解码契约测试（与 iOS 同一夹具） |
| `../test/.../ViewModelConcurrencyTest.kt` | 收件箱竞态回归 + 模板一次性加载测试 |

## 已实现（P0，Android 侧）

- 导航：Material3 NavigationBar 双 Tab（商机/设置）+ Navigation Compose；商机详情、
  订阅、Telegram、识别规则、工作时间、通知各自独立路由；`opportunity-radar://` 与
  `https://im.story2u.xyz/app/opportunity/{id}` 深链已注册。
- 商机看板：头部待处理数（服务端）、重大商机 banner、状态/平台/排序一级筛选、
  高级筛选 Bottom Sheet（时间范围含用户时区/来源/可信度/阶段/关键词，草稿模式）、
  结果摘要（服务端 total）、富卡片、平板自适应双列 LazyVerticalGrid、
  skeleton/下拉刷新/加载更多/空/失败重试；单一可取消 Job + 写入前筛选校验防竞态。
- 设置中心：分组列表 + 用户头部；套餐（复用真实 API）、Telegram（真实 health+
  connections+sources 读取/启停）、识别规则（关键词+AI 开关，失败回滚）、
  工作时间（一周 7 行 + 时区，失败回滚）、通知（4 开关，推送未开放标注），
  企业微信标注"由管理员配置"不做假连接。
- 登录：邮箱密码 + Google 原生登录；会话 401 统一清理。
- 主题：DayNight 跟随系统深色（对齐 iOS）。
- i18n：`values/strings.xml`（英文默认）+ `values-zh-rCN/strings.xml`（中文）固化跨端共享
  产品文案；现有屏幕内联中文仍待逐屏迁移到 stringResource。

## 待开发

- 推送：后端推送通道（P0 计划步骤 8）落地后接 FCM 注册/接收/深链（步骤 9）。
- P1：Telegram 连接引导、Agent 动作审批、今日概览（见蓝图）。
- CI：`make android-check` 接入 Linux CI。
- 视觉：深色模式主题、App Icon、语义 Badge 组件、本地时区时间显示。
- P1：Telegram 连接引导、Agent 动作审批、订阅只读、今日概览（见蓝图）。

## CI 与 Gradle Wrapper

`.github/workflows/android.yml` 在 Linux CI 运行 `lintDebug testDebugUnitTest assembleDebug`，
Gradle 版本由 workflow 钉死（须与 `gradle-wrapper.properties` 一致）。

wrapper 的 `gradlew` / `gradle-wrapper.jar` 尚未入库（`.gitignore` 已放行）。在任一装有
Gradle 的机器上执行一次并提交，之后 CI 即可改回 `./gradlew`：

```bash
cd mobile/android && gradle wrapper --gradle-version 8.11.1
git add gradlew gradlew.bat gradle/wrapper/
```

## 本地开发

首次需要生成 gradle wrapper（见上节，或直接用 Android Studio 打开本目录）：

```bash
cd mobile/android
gradle wrapper          # 需本机 gradle（brew install gradle）
./gradlew assembleDebug
make android-check      # lint + JVM 单元测试（在仓库根执行）
```

Google 登录需要在 `mobile/android/gradle.properties`（或 `-P` 参数）提供 Web Client ID
（非秘密，可提交）：`GOOGLE_SERVER_CLIENT_ID=xxx.apps.googleusercontent.com`，并确保后端
`GOOGLE_NATIVE_CLIENT_IDS` 包含同一 ID。

Android Studio（Ladybug+，JDK 17）打开 `mobile/android/` 即可运行到模拟器；模拟器内
`10.0.2.2` 指向宿主机的本地 compose 后端。请使用已配置的邮箱密码账户或 OAuth 账户登录。

RevenueCat Public Android API Key 通过 Gradle property `REVENUECAT_ANDROID_PUBLIC_API_KEY` 注入，
例如放在未提交的用户级 `~/.gradle/gradle.properties`。不得填 RevenueCat 服务端 Secret；空值时
App 正常运行但不能购买或恢复。
