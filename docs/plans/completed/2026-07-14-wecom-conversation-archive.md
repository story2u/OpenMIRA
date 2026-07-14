# 企业微信会话内容存档 P1

> 状态：completed（真实企业 E2E 待外部配置） · Owner：Codex · 创建：2026-07-14 · 更新：2026-07-14

## 目标与用户价值

允许已开通企业微信会话内容存档的企业管理员接入真实 Finance SDK，把当前登录用户参与的私聊和群聊
文本消息安全投影到商机识别链路，而不向该用户暴露企业内其他成员消息。

## 非目标

- 不实现媒体下载、历史补录、客户同意管理、代成员发送或普通成员 OAuth 开通。
- 不提交腾讯 SDK 二进制，不伪造真实企业后台已完成配置。
- 不实现企业邀请和成员管理 UI；首版只绑定创建连接的当前用户。

## 背景与当前行为

P0 `wecom_connections` 只能接收成员发给自建应用的消息，无法监听普通会话。P1 依赖企业级会话内容
存档，授权模型、原生 SDK 和只读能力均不同，设计见
[企业微信会话内容存档设计](../../integrations/wecom-conversation-archive.md)。本分支堆叠在尚未合并的
`features/wecom-message-integration`，合并时必须先合 P0 或整体合入。

## 验收标准

- [x] 企业管理员可为当前用户创建、查看、验证、同步和禁用存档连接，秘密不回显。
- [x] 数据库表达企业连接、成员 binding、cursor 和最小事件审计，并提供可往返迁移。
- [x] 真实 SDK provider 可拉取并解密文本消息；生产缺 SDK 时 fail closed。
- [x] 只把包含 active binding 的消息投影给对应 owner，重复消息和重试幂等。
- [x] 私聊/群聊进入现有商机识别，强制人工审核且不能通过存档来源发送。
- [x] 专用 worker、部署配置、UI 配置入口、测试和 Runbook 完整。

## 影响面与风险

- 数据库：新增四张存档表；不修改 P0 表和历史消息。
- 后端：新增 archive provider、同步用例、管理 API、Celery 任务和专用队列。
- 前端：在企微设置页新增企业级存档配置与只读能力说明。
- 部署：新增原生 SDK 只读挂载和单并发 worker；默认关闭。
- 风险：企业级秘密、私聊正文、跨用户数据泄漏、cursor 丢失、原生库崩溃和平台合规。

## 实施步骤

- [x] 设计、枚举、模型、迁移和 repository。
- [x] SDK Protocol/ctypes provider、消息解析与同步用例。
- [x] 管理 API、worker、部署和前端设置页。
- [x] 权限/幂等/失败恢复测试、完整验证和文档归档。

## 进度日志

- 2026-07-14：从 P0 提交 `1df7678` 创建 `features/wecom-conversation-archive`；完成平台边界和数据流设计。
- 2026-07-14：完成四张存档表、Finance SDK provider、owner/member 投影、独立队列、设置 UI 和部署挂载。
- 2026-07-14：审查并修复跨 Telegram/企微 combined quota 绕过，以及软删除后同企业无法重新配置的问题。

## 发现日志

- 会话内容存档是企业级配置，当前仓库没有 Organization 模型；首版以 connection owner 管理连接、member binding 控制消息可见性，避免把 owner 等同于全部消息读者。
- Finance SDK 是原生动态库，不能在 API 请求内调用，也不能把腾讯二进制提交进仓库。
- 真实 SDK 和企业后台由外部管理员提供；仓库只能验证 ABI 生命周期、RSA 向量和缺库失败路径，不能把平台联调结果伪装为已完成。

## 决策日志

- 2026-07-14：使用独立存档表而非扩展 P0 用户连接；两个渠道的凭据、授权和发送能力不同。
- 2026-07-14：使用专用单并发 Celery 队列封装原生 SDK；cursor 只有整批成功才推进。
- 2026-07-14：首版只允许 connection owner 绑定自己，不提供任意 user ID 参数。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 47 Markdown、84 backend Python；8 个 harness 单测通过 |
| `make backend-check` / `make check` | 通过 | 最终 144 passed、29 skipped；1 条既有 Starlette/httpx2 弃用警告 |
| `make frontend-check` | 通过 | ESLint、`tsc --noEmit`、7 个 Vitest、Next.js production build |
| `make pi-agent-check` | 通过 | Node check；4 个 runtime tests |
| `alembic heads` | 通过 | 单 head：`202607140002` |
| SDK 缺失 fail-closed / RSA + ABI 生命周期 | 通过 | provider 单测覆盖缺库、真实 RSA 解密向量、native resource release |
| PostgreSQL archive repository / migration 往返 | 未运行（本地） | 本机没有 Docker/测试 PostgreSQL；门控用例已加入，交由 CI 临时 PostgreSQL 执行 |
| Compose 配置展开 | 未运行（本地） | 本机没有 `docker` 命令；部署 YAML 与 compose 由 CI 验证 |
| 真实企业 E2E | 未运行 | 需要管理员账号、付费存档能力和官方 SDK |

## 回滚与恢复

先设置 `WECOM_ARCHIVE_ENABLED=false` 并停止专用 worker，再禁用存档连接。应用回滚不影响 P0；迁移降级
会删除存档连接、binding、cursor 和事件审计，但不删除已经生成的 Message/Opportunity。

## 结果与剩余风险

代码链路完成，功能地图标记为“部分实现”。上线前仍需企业管理员购买并启用会话内容存档、完成合规
告知、设置公钥和出口 IP、挂载官方 SDK，并验证真实私聊/内部群/外部群的 `tolist` 投影与 key version。
首版只处理文本、只允许 owner 绑定自己、存档来源只读；不支持媒体、企业席位管理或代表成员发送。
