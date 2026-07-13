# 企业微信消息收发 P0

> 状态：active · Owner：Codex · 创建：2026-07-14 · 更新：2026-07-14

## 目标与用户价值

交付用户级企业自建应用连接，使登录用户可以安全接收成员发给应用的文本消息、识别商机，并在人工
确认后向原成员真实回复；同时为会话内容存档群监控制定不误导的平台边界。

## 非目标

- 本期不实现普通企微群监听、会话内容存档 SDK、微信客服或群机器人摄取。
- 本期不开放企微 AI 自动发送，不实现企业内多操作员共享连接。
- 本期不下载图片、语音、视频或文件。

## 背景与当前行为

当前 `WeComAdapter` 只使用全局环境变量，同步处理 webhook，未绑定 owner、未做事件级幂等，并把完整
解密 payload 写入 Message。发送仅适用于成员 userid，但 UI 没有用户级配置且详情页回复仍是本地状态。
设计真相见 [企业微信消息收发设计](../../integrations/wecom.md)。

## 验收标准

- [x] 用户可创建、查看、验证和删除自己的一套自建应用连接，API 永不回显秘密。
- [x] 连接专属 webhook 完成安全 XML、验签、解密、timestamp 和 receive_id 校验。
- [x] webhook 事件幂等、快速 ACK，并由 Celery 异步进入 owner-scoped 摄取链路。
- [x] 重复事件只产生一条 Message/Opportunity；不同连接的 provider ID 不冲突。
- [x] active 来源可经人工 API 发送；未验证、停用、无能力和 provider 失败均 fail closed。
- [x] 发送成功才记录 outgoing Message 和更新状态；相同 idempotency key 不重复发送。
- [x] `/settings/wecom` 具备真实 loading/error/pending/active/empty 状态和配置说明。
- [ ] 迁移往返、后端、前端、harness 和现有跨端检查通过。

## 影响面与风险

- 数据库：新增连接、来源、webhook event 和 outbound delivery 表；旧全局配置不迁移。
- API/worker：新增用户管理路由、连接 webhook 和 Celery 任务；旧 webhook 保持兼容。
- 前端：设置中心新增企微页面；详情页人工回复从本地写入改为真实 API。
- 部署：新增 webhook 容差和 body limit 非秘密配置；用户秘密只进数据库密文。
- 高风险：真实 IM 发送、外部 XML、重放、重复发送和 owner 串数据。

## 实施步骤

- [x] 数据模型、迁移、DTO、repository、密码学与设计测试。
- [x] 连接管理 API、异步 webhook、worker 幂等处理。
- [x] 动态发送 adapter、人工回复幂等和失败一致性。
- [x] Web 设置页面与真实回复调用。
- [ ] 文档、完整验证与计划归档。

## 进度日志

- 2026-07-14：从最新 `release/v2.0.0` 创建 `features/wecom-message-integration`；完成现状审计和设计。
- 2026-07-14：完成 P0 数据、API、加密 webhook、Celery、人工发送、Web 配置页和文档；本地全量后端/前端/harness 验证通过。

## 发现日志

- 现有 webhook 未设置 owner，因此即使摄取成功也不会进入用户看板或用户级 Agent/工作时间策略。
- 当前 Redis access token key 是全局常量，多连接会发生凭据串用。
- 当前前端人工回复是本地 store 更新，不能作为任何 IM 平台真实发送证据。
- 异步事件仅依赖唯一约束不足以防止并发 worker；新增 10 分钟 processing 租约，崩溃后可恢复。
- 本机没有可用 PostgreSQL/Docker daemon，两项仓储集成测试按项目门控跳过；必须以 GitHub CI 迁移往返和 Postgres 结果收尾。

## 决策日志

- 2026-07-14：P0 只实现企业自建应用成员单聊；普通群监控必须等待会话内容存档 P1。
- 2026-07-14：事件待处理正文使用服务端加密短期保存，完成后清除；不把正文放入 Celery 参数。
- 2026-07-14：P0 每用户限制一条连接，但 schema 支持未来多连接，不使用 owner 唯一约束。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 45 个 Markdown 链接、81 个后端 Python 文件、8 项 harness 回归 |
| Python compile + CI Ruff 规则 | 通过 | `E,F,ASYNC --ignore E501` |
| 后端 `pytest -q` | 通过 | 133 passed, 27 skipped；跳过项为未提供 PostgreSQL URL 的集成测试 |
| 前端 test/typecheck/lint/build | 通过 | 7 tests；Next.js 生产构建包含 `/settings/wecom` |
| Mypy | 未通过（既存基线） | 101 项剩余错误为 SQLModel/RevenueCat/路由既存类型问题；本次引入的 Telegram send 协议不匹配已修复 |
| PostgreSQL migration downgrade/upgrade | 待 CI | 本机无可用 PostgreSQL/Docker daemon |

## 回滚与恢复

先关闭连接管理入口和连接专属 webhook，再回滚应用。新增表与旧路径正交；确认没有待处理事件后可执行
migration downgrade。删除表会丢失连接密文、来源和事件审计，但不删除既有 Message/Opportunity。

## 结果与剩余风险

代码 P0 已实现，待 GitHub CI 和真实企业后台联调。会话内容存档、普通群聊和微信客服仍不在已交付范围。
