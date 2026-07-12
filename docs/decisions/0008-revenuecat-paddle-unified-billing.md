# ADR-0008：RevenueCat + Paddle 统一订阅计费

> 状态：accepted · 日期：2026-07-12

## 背景

产品在 Web、iOS 和 Android 销售同一组 Free/Plus/Pro/Max 权益。Apple、Google Play 和 Web
支付有不同生命周期、税务和管理入口，而 Telegram/pi Agent 配额必须由后端一致执行。

## 决策

- RevenueCat 聚合 App Store、Google Play 和 Paddle 状态；Paddle 是 Web Merchant of Record。
- Web 只用 RevenueCat Web SDK 启动 Paddle Checkout；移动端只使用各自商店，不跳 H5 支付。
- 三端在登录后使用后端 `users.id` UUID 作为 App User ID，禁止匿名或 email identity。
- webhook 只作为同步信号。后端验证固定 Authorization 与 raw-body HMAC 后异步重新查询 Customer，
  不直接把单个事件解释为最终权益，也不直接消费 Paddle webhook/Apple receipt/Google token。
- `billing_subscriptions` 保存渠道事实；`subscription_accounts` 是有效权益投影，并作为受限 API 的
  唯一授权来源。多个有效渠道取最高套餐并显式告警，不自动取消、迁移或退款。
- Billing Period 只描述支付生命周期；pi Agent Usage Period 永远是 UTC 自然月，年度用户仍按月重置。

## 取舍

RevenueCat 降低三套商店生命周期实现成本，但引入第三方可用性、费用和 Dashboard 配置依赖。保留本地
投影使业务检查低延迟、可审计且不会信任客户端；同步失败时保留尚未过期快照，过期权益不能无限延长。
Paddle 处理 Web 税务、收据和续费，项目不持有 Paddle API Key。Webhook 功能依赖 RevenueCat 当前
支持该集成的套餐；无法满足时定时 reconcile 不能被描述成实时同步替代品。

## 后果

- 上线前必须按 Runbook 配置四个平台并完成 Sandbox E2E；仅合并代码不代表支付可用。
- 账户删除要先停止新同步，再按隐私策略删除/匿名化本地 billing 记录并通过 RevenueCat Dashboard/API
  处理对应 UUID；财务审计记录按法定期限最小化保留。客户端登出必须清理 RevenueCat identity。
- 关闭新支付时关闭客户端 Public Key/购买入口和 webhook/reconcile，但保留未到期本地权益投影；恢复
  provider 后再全量 reconcile，不能用 Free 覆盖网络失败用户。
