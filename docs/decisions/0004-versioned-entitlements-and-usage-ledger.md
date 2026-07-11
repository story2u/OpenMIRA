# ADR-0004：使用版本化套餐目录与 PostgreSQL 用量账本

> 状态：accepted · 日期：2026-07-11

## 背景

pi Agent 默认开启后，需要在入队前限制每个用户的模型成本，同时让 Free、Plus、Pro、Max 套餐
共享稳定、可审计的群数量和月度 AI 分析额度。额度检查会并发发生，不能依赖 Redis 自增值或只在
前端隐藏入口；支付渠道尚未确定，也不适合把套餐规则散落在 provider webhook 中。

## 决策

- 首版套餐目录作为版本化领域代码维护，plan code 固定为 `free`、`plus`、`pro`、`max`。
- Free 允许 1 个 Telegram 群、1 个企微群和每月 100 次 AI 分析；Plus/Pro/Max 的 TG + 企微合计
  上限为 10/50/100，每月 AI 分析额度为 1,000/5,000/10,000。
- 用户可见额度按一次顶层 pi 分析任务计一次，不按内部模型轮次计数；provider 请求、token 和金额
  作为独立成本指标观测。
- `subscription_accounts` 保存外部订阅映射和有效周期；只有 active/trialing 且当前时间处于周期内
  的付费套餐生效，其他情况 fail closed 到 Free。
- `usage_ledger` 是额度真相源。enqueue 前通过用户行锁创建 reserved 记录，成功转 consumed，入队
  失败或最终重试失败转 released；user + feature + idempotency key 唯一。
- Free 使用 UTC 自然月，付费用户使用当前 billing period。套餐升级不改写历史账本。
- 套餐降级不删除群配置。平台 monitor 分离用户启用状态与 `quota_paused`，用户选择保留项并保存稳定
  优先级；升级恢复容量后仍保留该优先级，供未来再次降级使用。
- Redis 可以缓存读模型，但不得成为余额判定或对账的唯一来源。

## 后果

额度判断可以从 PostgreSQL 重建、审计，并能在并发请求下保持上限。自动分析和手工重跑共享同一
预留路径；无 owner 消息不运行 Agent。代价是每次分析入队前增加数据库事务和用户级行锁，高流量
租户后续可能需要 rollup 或分片，但不能牺牲账本可重建性。

付费群上限当前按 Telegram 与企微合计，这是本阶段的产品假设；若改为各渠道独立额度，只更新新
版本 plan catalog 和迁移策略，不在 API/worker 中新增套餐分支。

## 后续

接入支付 webhook 前补 provider 事件验签、幂等和乱序保护；实现用户级企微群配置后接入同一合计
额度；补齐降级时的超额群选择、客服调整审计、provider 成本指标与预算告警。
