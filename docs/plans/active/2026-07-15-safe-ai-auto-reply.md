# 安全 AI 自动回复

> 状态：active · Owner：Codex · 创建：2026-07-15 · 更新：2026-07-15

## 目标与用户价值

把现有“非工作时间立即排队发送”的原型改造成显式用户/来源授权、分析后决策且可审计的安全接待
链路。全局运行能力默认开启；用户可以为 Telegram Business 私聊单独授权，系统只在低风险场景自动确认需求；其他消息稳定
进入人工审核。

## 非目标

- 不开放 Telegram 普通账号 MTProto 自动发送，不为群组或频道自动回复。
- 不开放企业微信会话存档自动发送，不实现跨平台自主销售。
- 不让 pi Agent 获得 IM 发送工具，不实现长期 Agent 会话或自治工具循环。
- 不在本次实现通知服务、自动报价、合同、付款、退款或法律答复。
- 不自动开启任何用户日程或 Telegram 来源授权，不在本任务内执行真实生产发送。

## 背景与当前行为

`IngestMessageUseCase` 当前先调用 `enqueue_ai_reply`，再调度 pi 分析；用户日程中的
`auto_reply_outside_hours` 未参与决策。`AIAutoReplyUseCase` 对 adapter 的 dry-run 回执仍写 outgoing
Message 并标记 `REPLIED`，来源也没有自动回复授权。完整设计见
[AI Agent 安全自动回复](../../integrations/ai-auto-reply.md)。

## 验收标准

- [x] 新迁移保留既有数据，所有 Telegram 来源默认不允许自动回复，并新增幂等投递账本。
- [x] 只有显式授权且具有 `can_reply` 权限的 Telegram Business 私聊、用户总开关、非工作时间和双重服务端安全阀同时满足时才可发送。
- [x] pi Agent 完成后才评估发送；失败、额度不足、未配置或风险门禁拒绝均转人工。
- [x] 自动发送使用数据库幂等预留和 at-most-once 恢复；重复任务不会重复联系客户。
- [x] dry-run、provider 失败和发送结果不确定都不会创建 outgoing Message 或标记 `REPLIED`。
- [x] 自动草稿经过长度、URL、金额和承诺词检查；失败不二次自动改写。
- [x] 工作时间页真实保存总开关；Telegram 设置页可为合格来源单独授权。
- [x] Web AI 草稿按钮调用后端真实 API，不再生成本地固定文案。
- [x] API、迁移、领域策略、任务幂等、前端关键状态有测试。
- [x] 架构、功能地图、运维配置和安全文档与代码一致。

## 影响面与风险

- domain：新增自动回复投递状态、决策原因和纯策略；不得依赖框架。
- application：摄取只建立候选，Agent 完成后调度；自动回复用例实施最终门禁。
- infrastructure：SQLModel/Repository、Telegram 来源授权、adapter 回执语义。
- migration：新增来源字段与投递表；默认 false，避免升级后意外发送。
- worker：任务编排顺序、幂等和失败恢复；禁止发送任务自动重试。
- API/frontend：来源 PATCH、日程总开关、真实 AI 草稿。
- deployment：四个全局运行开关默认开启并可由 GitHub Variable 显式关闭；用户/来源授权仍默认关闭。
- security：提示词注入、重复发送、人工竞态、dry-run 假成功、用户越权和日志泄密。

## 实施步骤

- [x] 审计当前消息摄取、Agent、状态机、adapter、Telegram Source、前端和部署行为。
- [x] 写设计、威胁模型、验收标准和 ADR。
- [x] 新增领域策略、投递账本、来源授权与 Alembic migration。
- [x] 重排摄取/Agent/自动回复任务，修正 dry-run 和幂等语义。
- [x] 扩展 API 与前端设置，接通真实 AI 草稿。
- [x] 增加后端、前端和迁移测试。
- [x] 更新架构、功能地图、运维和文档索引，完成本地验证。
- [ ] 在 CI 完成 PostgreSQL 迁移往返、iOS 和 Android 平台构建后归档计划。

## 进度日志

- 2026-07-15：从 `release/v2.0.0` 的 `be02b2e` 创建 `features/safe-ai-auto-reply`；工作区初始干净。
- 2026-07-15：完成现状审计并写入可执行设计；下一步实现领域策略和迁移。
- 2026-07-15：完成后端安全门禁、投递账本、Telegram Business 来源授权、三端工作时间开关和 Web
  真实 AI 草稿；`make check` 通过。等待 CI 平台检查与真实 Telegram Business 隔离 E2E。

## 发现日志

- `IngestMessageUseCase` 的自动回复任务早于 pi Agent，存在真实竞态。
- `UserWorkSchedule.auto_reply_outside_hours` 已持久化但摄取路由未使用；Web 保存时固定写 true。
- Telegram Bot adapter 在 `IM_SEND_ENABLED=false` 返回 dry-run receipt，但自动回复用例仍记录发送成功。
- 新版 Telegram `TelegramSource` 能表达连接类型和私聊来源，适合承载最小来源授权；MTProto 能力是只读。
- `notify_reviewers` 当前只写日志，不是可用通知通道，本次不把它包装成已交付通知。

## 决策日志

- 2026-07-15：选择三段式边界“Agent 分析、确定性策略决策、adapter 执行”；不把发送工具授予 Agent。
- 2026-07-15：第一版只允许 Telegram Business 私聊，来源和用户均显式 opt-in；初始设计将全局开关
  设为 false，随后由 ADR-0010 按产品决定调整为默认 true，来源/用户授权边界不变。
- 2026-07-15：发送采用 at-most-once；provider 成功后进程崩溃的未知结果不自动重发，交人工恢复。
- 2026-07-15：独立投递账本只保存摘要和安全状态，不保存第二份完整消息或 provider raw response。
- 2026-07-15：发送前在同一事务锁定投递与商机，并写入 `ai:auto_reply` 系统占用；provider 结果不确定时
  保持 `sending` 且禁止人工重发，必须先核对 provider，避免重复联系。
- 2026-07-15：来源授权和最终发送都检查 Telegram Business `can_reply`，权限撤销后 fail closed。
- 2026-07-15：按产品决定将 `AI_ENABLED`、`PI_AGENT_ENABLED`、`AI_AUTO_REPLY_ENABLED` 和
  `IM_SEND_ENABLED` 默认设为 true；ADR-0010 记录风险与回滚，用户/来源授权保持默认 false。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 53 个 Markdown 链接、89 个后端 Python 文件、8 个 harness 单测 |
| `make backend-check` | 通过 | 171 passed、32 skipped；跳过项需要 `SUBSCRIPTION_TEST_DATABASE_URL` |
| Pi runtime | 通过 | 语法检查及 4 个 Node 测试 |
| `make frontend-check` | 通过 | lint、typecheck、16 个 Vitest 测试和 Next.js production build |
| Android `make android-check` | 未运行 | 当前机器没有 Java/JDK；命令明确失败，等待 GitHub CI |
| iOS `make ios-check` | 未运行 | 当前 Linux 环境没有 macOS/Xcode；等待 GitHub CI |
| Alembic upgrade/downgrade/upgrade | 未运行 | 当前机器没有 Docker/PostgreSQL；CI PostgreSQL 16 会执行 |

## 回滚与恢复

先将 `AI_AUTO_REPLY_ENABLED=false`，确认 worker 不再创建新的发送账本，再回滚应用镜像。来源授权和
投递审计可以保留；生产通常不 downgrade。若必须 downgrade，先导出 `auto_reply_deliveries` 审计并
确认没有新版本 worker 运行，再执行迁移回退。

## 结果与剩余风险

代码实现和本地可执行检查已完成，计划在平台 CI 通过后归档。生产外发不会在本计划内开启；真实
Telegram Business 发送、权限撤销、provider 超时不确定状态和人工核对恢复仍需隔离账号验收。
