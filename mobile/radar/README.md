# 商机雷达 React Native 开发包

这个 Expo SDK 57 / React Native 0.86 工程承载 P0 兼容性验证、P1 应用骨架和 P2 业务纵向切片。
它使用 Expo Router、共享契约/API/Agent 包和独立开发 bundle id；认证与在线商机看板可显式连接 API，
但当前不替代商店中的 iOS/Android 客户端。

当前固定检查：

- 在真实 `@earendil-works/pi-agent-core` 上执行注入式 stream 与结构化 tool call；Metro 会把该包的
  React Native 入口收窄到 `Agent`，并用本地最小兼容层替换其 Node-only `pi-ai/compat` barrel；
- 写入 10,000 条 SQLite change log，关闭并重开数据库后重放中断的 projection；
- 通过本地 fixture 与 `expo/fetch` 按字节分片读取 UTF-8 SSE，并验证 AbortController 取消；
- 从旧安全存储迁移 token，并保证新存储回读成功后才删除旧值。
- 真实登录先清 legacy 凭据再写入并回读 current token；登出按 legacy → current 顺序清理，防止残留
  token 在后续启动重新迁移。
- 用 fail-closed capability、脱敏日志、错误边界和 session 恢复作为后续业务切片的基础。
- 版本化迁移 `radar.db`，按 owner 隔离 change inbox、projection、command outbox 与 sync cursor；
  token/password 永不进入该数据库。
- 在线看板只读取服务端 dashboard，覆盖状态/平台/来源/时间/trust/SOP/关键词筛选、四种排序、分页、
  重大提示、空态与重试；详情深链独立并行读取商机与首批消息，展示 Agent 发现、既有回复和按 20 条
  分页的双向消息历史。P3 前不从 SQLite projection 提供离线结果。
- 设置中心通过共享严格契约读取 owner 级设置，支持关键词/AI 语义、任意多段工作时间、IANA 时区与
  通知偏好；写失败保持或恢复服务端旧值。Telegram 移动端只读取健康、连接和来源并可启停现有连接，
  新建握手继续由 Web 完成，provider 错误详情不进入移动日志或 UI。
- 套餐页并行读取服务端目录、用量和当前客户端管理入口；官方 RevenueCat SDK 只使用平台 Public Key
  和认证用户 UUID，支持 Offering、Free 用户购买、用户触发恢复及购买后服务端 sync。未配置 key、
  Expo Go、缺 Offering 或 provider 失败均显式关闭购买，不回退本地价格或假权益。
- 登录页在 iOS 使用 Apple 系统控件，并在双端使用 Google 官方按钮资产。Google Android 桥基于
  Credential Manager，iOS 桥基于 GoogleSignIn 9.x；两条路径只把 ID token 交给后端换取站内 JWT，
  取消不报错，provider/native 错误详情和 token 不进入 UI、日志、SecureStore 或 SQLite。
- 生产界面使用类型化简体中文/英文 catalog，并按系统首选语言自动选择；iOS 向系统声明
  `en`/`zh-Hans`，Android 声明 `en`/`zh-CN`，不支持的语言显式回退 `zh-CN`。当前不提供应用内语言
  开关，用户应使用系统或系统的单 App 语言设置；catalog key、插值参数和生产 TSX 硬编码中文由测试审计。

开发包使用 `com.codeiy.im.dev`，无法读取生产包 `com.codeiy.im` 的应用沙箱。旧 token 的真实覆盖升级
验证必须在备份设备上用同包名 release candidate 完成；单元测试只验证迁移事务语义。

商店候选包必须显式使用生产变体；它沿用既有双平台 ID `com.codeiy.im` 和 Android 已发布的
`opportunity-radar` deep-link scheme，同时移除 `.dev` 包的本机 HTTP/ATS 例外。版本与 build number
必须高于商店现有记录，配置缺失时构建会直接失败：

```bash
RADAR_APP_VARIANT=production \
RADAR_APP_VERSION=1.0.0 \
RADAR_BUILD_NUMBER=2 \
EXPO_PUBLIC_API_BASE_URL=https://api.example.com \
corepack pnpm@10.25.0 prebuild --clean
```

以上示例版本和 API 域名只展示格式，不能替代发布前查询 App Store Connect / Play Console 的真实版本
序列与生产 origin；production 缺少严格 HTTPS origin 会在 Expo config 阶段失败。同 ID 无签名模拟器
构建也不能证明商店签名覆盖升级或旧安全存储可读；无签名 iOS 模拟器可能直接走“迁移失败、重新登录”
路径，只有使用商店同签名的备份真机候选包才能验收 token 覆盖升级。

```bash
# 在仓库根目录安装唯一 lockfile
corepack pnpm@10.25.0 install --frozen-lockfile
make rn-check

# 以下命令在 mobile/radar 目录运行
corepack pnpm@10.25.0 typecheck
corepack pnpm@10.25.0 test
corepack pnpm@10.25.0 exec expo install --check
corepack pnpm@10.25.0 fixture:sse
corepack pnpm@10.25.0 prebuild --clean
```

认证 API 必须显式设置 origin（不含 `/api/v1`）：

```bash
EXPO_PUBLIC_API_BASE_URL=https://api.example.com corepack pnpm@10.25.0 start
```

生产 bundle 只接受 HTTPS。独立 `.dev` bundle 可用本地 fixture 验证完整登录链：

```bash
corepack pnpm@10.25.0 fixture:auth
EXPO_PUBLIC_API_BASE_URL=http://127.0.0.1:8788 corepack pnpm@10.25.0 ios:release
```

真实购买构建只允许使用对应 RevenueCat Public Key；iOS/Android 分开配置。不得把
`REVENUECAT_SECRET_API_KEY` 放进 Expo 环境变量：

```bash
EXPO_PUBLIC_REVENUECAT_IOS_API_KEY=appl_public_key \
EXPO_PUBLIC_REVENUECAT_ANDROID_API_KEY=goog_public_key \
EXPO_PUBLIC_REVENUECAT_OFFERING_ID=default \
corepack pnpm@10.25.0 prebuild --clean
```

Google 原生登录构建需要成对配置 Web/server client ID 与 iOS client ID。前者也是 Android/iOS 请求的
ID-token audience，必须同步加入后端 `GOOGLE_NATIVE_CLIENT_IDS`；iOS ID 用于 SDK 配置和反向 URL
callback。Apple 后端 `APPLE_NATIVE_CLIENT_IDS` 必须包含当前构建的精确 bundle ID（例如 dev 与生产
并行时分别列出）。这些 client ID 是公开标识，但任何 client secret 都不得进入 App：

```bash
EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID=web-client.apps.googleusercontent.com \
EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID=ios-client.apps.googleusercontent.com \
corepack pnpm@10.25.0 prebuild --clean
```

Apple/Google 与 RevenueCat 都依赖原生模块，Expo Go 不提供这些生产路径。发布前必须在隔离 provider
测试账号上验证成功、取消、错误 audience、首次建号和账号切换；仅使用假 client ID 的 Release 构建
不会触发真实授权，也不能替代该发布闸门。

原生推送同样只支持 development/production build，不以 Expo Go 作为证据。Android 直连 FCM 构建需为
当前 `com.codeiy.im(.dev)` Firebase App 提供 `google-services.json`，文件不得提交仓库：

```bash
RADAR_GOOGLE_SERVICES_FILE=/secure/path/google-services.json \
corepack pnpm@10.25.0 prebuild --clean
```

iOS 由签名 entitlement 决定 APNs sandbox/production，客户端运行时读取该值，不用 bundle 名猜测。服务端需
分别配置 `APNS_TOPIC=com.codeiy.im` 与 `APNS_SANDBOX_TOPIC=com.codeiy.im.dev`，或仅开放实际发布的环境。
客户端只有在 capability 开启且用户主动点击“启用同步唤醒”后才请求权限和注册原生 token；payload 不含
消息正文，推送丢失仍由前台恢复/手动增量同步补偿。

固定假账号为 `developer@example.test` / `fixture-password`。fixture 同时提供确定性的三条 dashboard 数据、
独立详情、Agent 发现和 55/0/1 条消息场景，以及只存在于进程内存的模板、AI 草稿、认领、状态、幂等
人工回复、用户设置、Telegram 健康/连接/套餐暂停来源，以及 Free 订阅目录/用量/管理快照；可验证分页、
空态和写操作 UI。fixture 不提供 RevenueCat Offering，订阅 sync 固定 503，绝不模拟交易成功。它不连接数据库/
外部 IM、不记录请求正文，重启即清空，且不得作为生产回退。Android Emulator 使用
`http://10.0.2.2:8788`。

兼容性实验室只在开发模式路由 `/lab` 可见，并访问本机 `8787` 端口；运行 SSE 实验前保持
`fixture:sse` 进程开启。仅独立开发包允许本地明文流量，生产配置不得复制该例外。

`expo-modules-jsi@57.0.3` 在 Xcode 26.3 / Swift 6 C++ interop 下会把无命名空间的 `abs` 判为
歧义；`patches/` 中的一行补丁显式使用 `Swift.abs`。升级该依赖时必须先移除此补丁并重跑 iOS
release build，确认上游已修复。
