# ADR-0003：默认启用 pi Agent

> 状态：accepted · 日期：2026-07-11

## 背景

ADR-0002 为降低首次上线风险，将 `PI_AGENT_ENABLED` 设计为默认关闭。产品现在决定把消息后处理
作为标准能力启用，并通过 GitHub Variables/Secrets 配置 DeepSeek。当前订阅、免费额度和原子计量
尚未实现，因此默认启用会在 provider key 有效时处理所有可归属的新消息。

## 决策

- Python Settings、`.env.example`、本地 Compose 和生产 Compose 的 `PI_AGENT_ENABLED` 默认值统一
  改为 `true`；显式设为 `false` 仍是即时停用开关。
- provider/model 继续使用 GitHub Variables。当 provider 为 DeepSeek 时，部署优先读取
  `DEEPSEEK_API_KEY` Secret，再回退通用 `PI_AGENT_API_KEY`，并在运行环境中映射为通用 key。
- 当前固定 pi 0.80.6 时，DeepSeek provider 使用 `deepseek`，默认建议模型为
  `deepseek-v4-flash`；需要更强推理时可配置 `deepseek-v4-pro`。
- 缺少 key 时不入队并记录非敏感配置告警；无效 key 的任务安全失败。不允许静默切换 provider、
  使用匿名服务或 mock。
- 免费用户额度和订阅 entitlement 在独立执行计划中实现；在完成前，默认开启不等于已经具备用量
  限制或付费隔离。

## 后果

正面影响：部署只需提供 provider 配置即可自动处理新消息；本地、生产和文档默认值一致；仍保留
全局 kill switch。

风险与成本：订阅计量上线前，有效 key 会允许所有可归属消息消耗模型额度；provider 故障会形成
失败/重试流量。生产负责人应设置 provider 预算告警，并在成本异常时把 GitHub Variable
`PI_AGENT_ENABLED` 设为 `false` 后通过正常 release 流程部署。

## 后续

实施 `docs/plans/active/2026-07-11-subscriptions-and-agent-quotas.md`，在 enqueue 之前增加原子额度预留、
订阅 entitlement、usage ledger、升级入口和账单 webhook。完成前不得声称非订阅用户已经受限。
