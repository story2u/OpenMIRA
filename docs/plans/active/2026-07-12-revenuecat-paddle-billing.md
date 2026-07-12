# RevenueCat + Paddle 统一订阅计费

> 状态：active · Owner：Codex · 创建：2026-07-12 · 更新：2026-07-12

## 目标与用户价值

以 RevenueCat 聚合 Paddle、App Store 和 Google Play 订阅，后端维护最终权益投影；Web、iOS、Android
使用同一用户 UUID 购买和恢复，并由本地快照统一执行套餐额度。

## 非目标

- 不自动配置或宣称完成 RevenueCat、Paddle、App Store Connect、Google Play Console。
- 不实现周订阅、Lifetime、次数包、Add-on、Seat、优惠码、试用、企业合同或跨渠道自动迁移/退款。
- 不直接消费 Paddle webhook，也不由后端直接验证 Apple receipt 或 Google purchase token。

## 基线审计

- 基线：`release/v2.0.0`，SHA `bc0d830eaccd7172a451b5856c0f32341120a4ba`；创建分支时工作区干净。
- 后端：FastAPI 0.139、SQLModel 0.0.39、Alembic 1.18.5、Celery 5.6.3、Redis 8.0.1，PostgreSQL。
- 现状：`SubscriptionAccount` 同时保存 provider 与权益；`SubscriptionSnapshot.period` 对付费用户使用支付周期，
  会让 annual 用户一年只得到一次月额度。
- API：已有 `GET /subscriptions/plans`、`GET /subscriptions/me`；H5 套餐页只读展示本地套餐和用量。
- iOS：SwiftUI/Swift 6、xcodegen、Keychain JWT，`Core/Network` 为 HTTP 边界。
- Android：基线已包含 Kotlin/Compose 工程、SessionStore 和唯一 API client，不再是 README-only。
- quota：Telegram monitor/source 降级设置 `quota_paused` 并保留数据；升级按 retention/创建顺序恢复。
- CI/部署：release push 跑 PostgreSQL migration/tests、前端、pi、iOS、Android；默认分支 workflow_run SSH 部署。

## 验收标准

- [ ] Billing domain、支付记录、事件和权益投影支持多个渠道并可迁移回滚。
- [ ] Billing period 与 UTC 自然月 usage period 分离，annual 与月中升级测试通过。
- [ ] RevenueCat Customer 全量同步、webhook 双重校验、幂等 Celery 和每日 reconcile 完成。
- [ ] `/catalog`、`/me`、`/sync`、`/management` 保持旧契约兼容并以后端快照为权威。
- [ ] H5、iOS、Android 使用官方 RevenueCat SDK、登录用户 UUID、真实 offering 和后端确认链路。
- [ ] 未配置支付时 fail closed 且应用可用；生产不回退 mock。
- [ ] Runbook、ADR、feature map、部署注入和平台检查完整；Sandbox E2E 未运行时明确标记待验证。

## 影响面与风险

涉及 billing schema、月额度、RevenueCat 外部 API/webhook、Celery、三端购买身份、商店合规和生产秘密。
网络同步失败不得清空仍有效权益；未知 provider 字段必须安全降级；重复 webhook 不得重复处理。

## 实施步骤

- [x] Commit 1：Billing domain、迁移、Usage Period。
- [x] Commit 2：RevenueCat client、稳定模型、同步用例。
- [x] Commit 3：Webhook、Celery、reconcile。
- [x] Commit 4：订阅 API 与 DTO。
- [ ] Commit 5：H5 RevenueCat Web/Paddle。
- [ ] Commit 6：iOS RevenueCat/App Store。
- [ ] Commit 7：Android RevenueCat/Google Play。
- [ ] Commit 8：部署、CI、ADR、Runbook、完整验证。

## 进度日志

- 2026-07-12：完成基线审计并创建分支；开始 billing domain 与 migration。
- 2026-07-12：新增 billing products/subscriptions/events、权益投影字段和可回滚迁移；usage period 固定为 UTC 自然月。
- 2026-07-12：按官方 v1 CustomerInfo endpoint 实现有限重试 adapter、稳定模型、mock provider 和全量权益投影；management URL 加密保存。
- 2026-07-12：实现 Authorization + raw-body HMAC webhook、幂等事件仓储、Celery 全量同步和每日批量 reconcile；未知事件保持兼容。
- 2026-07-12：新增 catalog/sync/management，扩展 `/me` billing/usage 周期与来源字段；sync 无客户端升级 payload 且按用户 Redis 限流。

## 发现日志

- Android 已由 PR #9 脚手架完成，实施改为在现有唯一 network boundary 上增加套餐最小真实链路。
- 现有 `SubscriptionSnapshot.period` 是支付周期和用量周期混用的根因。

## 决策日志

- 2026-07-12：保留 `SubscriptionStatus` 作为兼容投影状态；底层支付使用更完整的
  `BillingSubscriptionStatus`，避免破坏现有客户端。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make check` | 待运行 | |
| RevenueCat/Paddle/App Store/Play Sandbox E2E | 未运行 | 需要外部 Dashboard 与测试账号配置 |

## 回滚与恢复

关闭 RevenueCat 开关可停止新购买和同步，保留最近未过期本地快照。数据库按 migration downgrade 回滚；
客户端 SDK 配置缺失时隐藏购买动作。不得通过删除支付记录或 UsageLedger 回滚。

## 结果与剩余风险

待完成。
