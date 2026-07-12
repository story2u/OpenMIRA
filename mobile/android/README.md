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
| `MainActivity.kt` | 入口 + 会话路由（恢复中/恢复失败/登录/收件箱） |
| `core/network/ApiClient.kt` | Retrofit 构造、Bearer 注入、`{"detail"}` 错误映射（`ApiError`） |
| `core/network/RadarApi.kt` | 全部端点定义，路径以 `backend/app/api/v1/routes/` 为准 |
| `core/auth/TokenStore.kt` | JWT 加密存取清 |
| `core/auth/SessionStore.kt` | `/auth/me` 会话恢复、邮箱密码登录、登出 |
| `model/Models.kt` | DTO 镜像 + 容错枚举 + `RadarJson` 配置 |
| `feature/login/LoginScreen.kt` | 邮箱密码登录 |
| `feature/inbox/InboxScreen.kt` | 收件箱：筛选、分页、下拉刷新、30s 轮询 |
| `feature/opportunity/OpportunityDetailScreen.kt` | 详情、消息历史、Agent 发现、回复、状态流转 |
| `ui/Time.kt` | ISO8601 相对时间/短时间格式化 |
| `../test/.../ModelsDecodingTest.kt` | 解码契约测试（与 iOS 同一夹具） |

## 已实现（P0，Android 侧）

- 登录：邮箱密码（`POST /auth/password/login`）；`/auth/me` 冷启动恢复；登出清加密存储。
- 收件箱：状态/渠道筛选、分页、下拉刷新、30s 轮询。
- 详情：概要、检测依据、Agent 发现（链接核验/联系方式/动作建议/attention）、消息历史。
- 回复：手动回复、AI 草稿（可编辑）、模板选用；失败报错不伪造已回复；额度耗尽展示后端提示。
- 状态流转：认领/跟进/忽略/关闭，非法迁移展示后端 409 错误。

## 待开发

- Google 原生登录（Credential Manager + `POST /auth/oauth/google/native`，后端已就绪）。
- 推送：后端推送通道（P0 计划步骤 8）落地后接 FCM 注册/接收/深链（步骤 9）。
- P1：Telegram 连接引导、Agent 动作审批、订阅只读、今日概览（见蓝图）。
- CI：`make android-check` 接入 Linux CI。

## 本地开发

首次需要生成 gradle wrapper（仓库只提交了 `gradle-wrapper.properties`，不提交二进制 jar）：

```bash
cd mobile/android
gradle wrapper          # 需本机 gradle（brew install gradle），或直接用 Android Studio 打开本目录
./gradlew assembleDebug
make android-check      # lint + JVM 单元测试（在仓库根执行）
```

Android Studio（Ladybug+，JDK 17）打开 `mobile/android/` 即可运行到模拟器；模拟器内
`10.0.2.2` 指向宿主机的本地 compose 后端。本地联调账号：
`docker compose run --rm api python scripts/dev_login.py` 会创建 `demo@local.dev`、认领无主
商机并打印一次性密码，直接用邮箱 + 该密码登录。
