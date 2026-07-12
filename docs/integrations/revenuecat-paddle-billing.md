# RevenueCat + Paddle 统一订阅 Runbook

> 状态：代码已实现，外部 Dashboard 与 Sandbox E2E 待执行 · 最后核验：2026-07-12

本 Runbook 是人工配置和验收清单，不包含任何真实 Secret。后端本地权益投影是最终权限来源；客户端
CustomerInfo 只用于购买 UI。三端必须使用当前登录用户的后端 UUID，不使用 email，也不允许匿名购买。

## 1. 环境与凭据清单

分别准备 RevenueCat Project、Apple App、Google Play App 和 Paddle Web Config。Paddle Sandbox 与
Production 是完全独立的账号/配置，产品、Price、域名、API Key 和联调都要分别完成。RevenueCat
transaction 的 `environment` 区分 Sandbox/Production，但 Customer identity 仍需谨慎隔离测试账号。

GitHub **Secrets**：

- `REVENUECAT_SECRET_API_KEY`：服务端 Secret，供 Customer API 使用。
- `REVENUECAT_WEBHOOK_AUTH_TOKEN`：完整固定 Authorization header 值，建议 `Bearer <random>`。
- `REVENUECAT_WEBHOOK_HMAC_SECRET`：Webhook signing secret，仅在创建/轮换时可见。

GitHub **Variables**：

- `REVENUECAT_ENABLED`、`REVENUECAT_RECONCILE_ENABLED`：完成配置前均保持 `false`。
- `REVENUECAT_PROJECT_ID`。
- `NEXT_PUBLIC_REVENUECAT_WEB_API_KEY`、`NEXT_PUBLIC_REVENUECAT_OFFERING_ID=default`。
- `REVENUECAT_IOS_PUBLIC_API_KEY`、`REVENUECAT_ANDROID_PUBLIC_API_KEY`：进入对应平台构建配置。

Public SDK Key 不是 Secret，但必须使用对应平台的 Key。Paddle API Key、App Store private key 和 Google
service-account JSON 不进入 GitHub/VPS 应用环境；Paddle Key 只粘贴到 RevenueCat Dashboard。

## 2. RevenueCat Project 与目录

1. 创建 Project，添加 Apple App（bundle `com.codeiy.im`）、Google Play App（package
   `com.codeiy.im`）和 Paddle Web Config；记录各平台 Public Key 与服务端 Secret Key。
2. 创建 entitlement：`plus`、`pro`、`max`，不要创建 Free entitlement。
3. Apple 建议产品：`com.codeiy.im.{plus|pro|max}.{monthly|annual}`，六个产品放入同一个
   Subscription Group。每个产品只绑定对应 entitlement。
4. Google Play 创建 subscription `im_plus`、`im_pro`、`im_max`，各自创建 base plan
   `monthly`、`annual`；每个 base plan 对应 RevenueCat product/package。
5. Paddle 创建 Plus/Pro/Max 三个 Product，每个创建 Monthly/Annual Price。Price ID 由 Paddle 生成，
   只通过 RevenueCat 导入，不写入应用代码或数据库默认值。
6. 创建 Offering `default`，添加 custom packages：`plus_monthly`、`plus_annual`、
   `pro_monthly`、`pro_annual`、`max_monthly`、`max_annual`。逐一检查 Apple、Google、Paddle
   产品映射和单一 entitlement 绑定，禁止 Pro 叠加 plus、Max 叠加 plus/pro。

## 3. Paddle Sandbox 与 Production

在两个 Paddle 环境分别操作：

1. Checkout > Website approval 添加 `pay.rev.cat` 与 `im.story2u.xyz`。Sandbox 无需审批；
   Production 必须等待域名审批完成。
2. Checkout settings 未设置 default payment link 时填 `https://pay.rev.cat`；已有其他业务链接可保留。
3. 关闭 abandoned-cart emails（当前 RevenueCat Paddle integration 不支持）。
4. Developer Tools > Authentication 创建专用、**不自动过期**的最小权限 API Key。按 RevenueCat 当前
   Paddle 文档勾选：Addresses/Adjustments/Businesses/Customers/Discounts/Notification settings/
   Notifications/Payment methods/Prices/Products/Subscriptions/Transactions 所需读权限；Client-side
   tokens、Notification settings、Transactions 所需写权限；Customer portal sessions 写权限。平台文档
   变化时以 Dashboard 当前要求为准，禁止扩大到 Reports 或无关管理权限。
5. 保持 Key 弹窗打开，将 Key 只粘贴到 RevenueCat Web > Paddle Config 的 Set secret，然后关闭。
6. 选择 automatic purchase tracking 并 Connect to Paddle；本项目不配置 Paddle webhook，也不接收
   Paddle API Key。导入六个 Price 并连接到 `default` Offering packages。
7. Sandbox 使用 Paddle 测试卡完成成功、取消、续费、失败、退款；Production 上线前做低风险真实交易
   和退款核对税务、收据、管理链接。一个 checkout 只允许一个产品。

## 4. Apple App Store Connect

1. 签署 Paid Applications Agreement，创建同一 Subscription Group 和六个 auto-renewable products，
   配置地区、价格、本地化、审核截图和元数据。
2. 将产品导入 RevenueCat Apple App 并按 package/entitlement 映射；上传 RevenueCat 要求的 App Store
   Connect API key，只放 Dashboard，不放项目。
3. 按 RevenueCat 当前 Apple Server Notifications 文档，将 Production 与 Sandbox App Store Server
   Notification URL 指向 RevenueCat，确认 Dashboard 的 Last received。
4. Xcode/CI 通过 `REVENUECAT_IOS_PUBLIC_API_KEY` 注入 Public Apple Key并启用 In-App Purchase capability。
5. 使用 StoreKit Configuration 做本地 UI 测试；使用 App Store Sandbox/TestFlight 验证真实购买、取消、
   恢复、升级/降级、宽限期和重装恢复。App 内不展示 Paddle checkout 或 Web 购买链接。

## 5. Google Play Console

1. 创建三个 subscriptions 和 monthly/annual base plans，激活测试地区与价格，上传至少一个内部测试构建。
2. 启用 Google Play Android Developer API、Developer Reporting API 和 Pub/Sub；创建 RevenueCat 专用
   service account，按 RevenueCat 文档授予 View app information、View financial data、Manage orders
   and subscriptions 及 Pub/Sub 权限。
3. service-account JSON 只上传 RevenueCat Google App Settings。新凭据可能需要最多约 36 小时生效。
4. 配置 Google Real-Time Developer Notifications 到 RevenueCat 并检查 Last received。
5. 构建时通过 `REVENUECAT_ANDROID_PUBLIC_API_KEY` Gradle property 注入 Public Google Key。
6. 将测试账号加入 License Testers 和目标 test track，验证购买、取消、恢复、续费、付款保留/on-hold、
   退款与重装恢复。Android 不展示 Paddle checkout。

## 6. RevenueCat Webhook

1. RevenueCat Integrations > Webhooks 新建配置。Webhook 当前需要支持该能力的 RevenueCat 套餐（官方
   文档当前标为 Pro integration），购买前确认账号套餐。
2. URL：`https://im.story2u.xyz/api/v1/webhooks/revenuecat`；选择 Sandbox + Production lifecycle events。
3. Authorization Header 填与 GitHub Secret 完全一致的固定值。
4. 开启 HMAC signing，把一次性显示的 secret 存入 GitHub Secret。服务校验
   `X-RevenueCat-Webhook-Signature: t=...,v1=...`、原始 body 和 300 秒容差。
5. 配置 Secrets/Variables 后先部署 `REVENUECAT_ENABLED=true`，确认 `/healthz`；再设置
   `REVENUECAT_RECONCILE_ENABLED=true`。发送 Dashboard 测试事件，确认 API 快速 200、Celery event
   completed、Customer API 全量同步。未知事件也应完成同步，重复 event ID 不重复处理。

## 7. Sandbox 端到端验收

使用同一个真实后端用户 UUID，逐项记录 RevenueCat Customer、`billing_events`、
`billing_subscriptions`、`subscription_accounts` 和 `/subscriptions/me` 结果：

1. H5 Paddle Sandbox 买 Plus Monthly，确认 webhook、后端 Plus、H5/iOS/Android 都显示 Plus。
2. iOS Sandbox 买 Pro，确认后端取最高 Pro，H5/Android 显示来源且出现多渠道重复付费警告。
3. Android License Tester 买 Max，确认最高 Max，三端继续使用同一 UUID。
4. 重放同一个 webhook，确认事件幂等；发送未知 event type，确认仍全量同步且不崩溃。
5. Paddle 取消续费，确认到期前权益保留且 `cancelAtPeriodEnd=true`；到期、refund/revoked 后降级。
6. 购买 annual，跨 UTC 月边界验证 UsageLedger 新自然月；月中 Plus→Pro 不清零已消费/预占。
7. 降级后超额 Telegram Source 仅 `quota_paused`，不删除消息、商机、连接或 Source；升级后容量内恢复。
8. RevenueCat 暂时不可用时 `/sync` 失败但未到期快照不被清空；过期快照不能无限授权。
9. iOS/Android 删除重装并主动恢复购买；用户 A 登出、用户 B 登录，CustomerInfo 和权益不串号。

上述 E2E 尚未实际执行前，功能状态只能是“部分实现，待 Sandbox E2E”。

## 8. 运维、轮换与回滚

- 轮换服务端 Secret：先创建新 Key并更新 GitHub Secret，部署并验证 sync/reconcile，再撤销旧 Key。
- 轮换 Webhook HMAC：RevenueCat 旋转会立即使旧 secret 失效；先暂停 webhook/reconcile 或安排维护窗，
  立即更新 Secret 和部署，再发送测试事件。Authorization token 同理需协调切换。
- 临时关闭新支付：移除三端 Public Key/重新构建客户端，设置 `REVENUECAT_ENABLED=false` 和
  `REVENUECAT_RECONCILE_ENABLED=false`。已有本地未到期权益保留，不删除 billing/usage 数据。
- Provider 故障：不要手工批量降级。恢复后开启 reconcile；对失败 event 使用 RevenueCat Retry。
- 应用回滚：先关闭购买入口和 webhook/reconcile，回滚镜像；数据库迁移仅在确认旧版本兼容且已备份后
  downgrade。`subscription_accounts` 旧 provider 字段保留用于兼容，不能删除支付审计记录。
- 故障定位顺序：Public Key/用户 UUID → Offering/package 是否齐全 → 商店产品状态与地区 → RevenueCat
  Customer → webhook Authorization/HMAC/时间 → Celery worker → Customer API rate limit → 本地投影。
  日志不得打印 Secret、完整 Customer response、付款数据或完整 provider identifier。
- 用户删除账号：阻止新购买/同步，按隐私与财务保留政策处理本地 billing 记录，并通过获授权的
  RevenueCat 管理流程删除/匿名化该 UUID；不得用 email 重新关联。该流程上线前需法务确认保留期限。
