# ADR-0010：默认启用 AI 分析与受控发送运行能力

> 状态：accepted · 日期：2026-07-15

## 背景

ADR-0009 建立了服务端、用户日程、来源授权、确定性策略和投递账本等多层门禁。产品决定让完整 AI
链路在标准部署后即可运行，不再要求运维额外打开四个全局开关。此前全局默认关闭会让已完成来源授权
的用户仍无法使用功能，也容易造成环境之间配置漂移。

## 决策

- `AI_ENABLED`、`PI_AGENT_ENABLED`、`AI_AUTO_REPLY_ENABLED`、`IM_SEND_ENABLED` 默认均为 `true`。
- Pydantic Settings、`.env.example`、本地/生产 Compose 和部署工作流保持同一默认值。
- GitHub Variable 或 VPS `.env` 显式配置 `false` 仍可覆盖默认值，作为成本控制和紧急停用手段。
- 自动回复的有效授权不依赖默认值本身：用户日程开关和 Telegram Business 私聊来源开关继续默认
  `false`，且还需 `can_reply`、非工作时间、pi Agent 完成、确定性风险门禁和幂等投递全部通过。
- 迁移继续把既有用户日程自动回复设置重置为 `false`，不会因服务端默认变化批量开启历史用户外发。
- `IM_SEND_ENABLED=true` 允许用户主动确认的人工发送调用真实 provider；adapter 仍要求有效连接和凭据。

## 后果

标准部署无需额外 Variable 即可执行 AI 分类和 pi Agent 分析。用户显式完成双层授权后，合格的
Telegram Business 私聊可以自动回复。

风险是错误配置的 provider 会更早暴露为调用失败，人工发送也不再默认 dry-run。自动外发仍由数据库
授权、Business 权限和确定性策略 fail closed；运维必须保留将任一全局开关设为 `false` 的回滚能力。

## 验证与回滚

配置单测验证四个默认值。部署 workflow 必须把 fallback `true` 写入 VPS `.env`，同时尊重显式
`false`。需要立即停止外发时先设置 `AI_AUTO_REPLY_ENABLED=false` 或 `IM_SEND_ENABLED=false` 并重新部署；
只停止模型调用时设置 `AI_ENABLED=false` 和 `PI_AGENT_ENABLED=false`。
