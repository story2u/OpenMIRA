# 集成 pi 消息后处理 Agent

> 状态：completed · Owner：Codex · 创建/完成：2026-07-10

## 目标与用户价值

每条新 IM 消息完成快速摄取后，可异步进入 pi Agent 后处理：安全获取公开链接、补判商机、提取
联系方式、生成后续行动建议，并在重大商机出现时提示资源所有者。模型失败或链接不安全不得阻塞
webhook，也不得触发未经批准的外部发送。

## 非目标

- 本切片不实现 SMTP/邮件 provider，也不使用个人 Telegram 账号自动加好友。
- 邮件、好友申请、主动私信只形成带理由和草稿的可审计建议，不默认自动执行。
- 不给 pi shell、文件系统、项目源码、数据库或任意 HTTP 工具。
- 不引入跨消息长期 Agent 会话或让聊天内容修改系统提示词/工具权限。

## 背景与原有行为

`IngestMessageUseCase` 原先只用规则和可选分类器同步判断商机；非商机消息直接结束。链接安全、联系
方式和好友申请在前端是 timer/mock，后端 DTO 返回固定空值。Celery 只处理 AI 回复和人工 SLA。
pi 本地源码 `/Users/bruce/git/xyz/pi` 的 `@earendil-works/pi-agent-core` 0.80.6 提供自定义工具、
`beforeToolCall` 和终止型工具结果，可用于建立窄能力 Agent。

## 验收结果

- [x] `PI_AGENT_ENABLED=false` 时摄取行为与原有行为一致，不启动 Node 或网络请求。
- [x] 新消息只入队一次；worker 重试/重复执行不会创建重复分析或重复商机。
- [x] URL 检查拒绝非 HTTP(S)、URL 凭据、私网/本机/保留地址，限制重定向、数量和正文大小。
- [x] pi 只能调用 `submit_analysis`，输出经 TypeBox 与 Pydantic 双重验证；超时/崩溃安全失败并可观测。
- [x] pi 可把规则漏判的高置信消息升级为商机，并把链接结论、联系方式、行动建议投影到商机。
- [x] 重大商机在 dashboard 显示明确提醒；资源仍按 `owner_user_id` 隔离。
- [x] 重新分析 API 只允许资源 owner，且只入队，不在 HTTP 请求中调用模型。
- [x] 外部邮件/好友/私信建议不会自动发送；现有 `IM_SEND_ENABLED` 约束不被绕过。
- [x] migration、后端、Node runner、前端、Compose、Docker 和 harness 验证通过。

## 影响面与风险

- domain/application：新增 Agent 分析契约、动作枚举、后处理用例和任务队列端口。
- infrastructure：安全链接读取、pi 子进程适配器、消息/商机投影持久化。
- migration：Message 分析审计字段和 Opportunity Agent 投影字段。
- worker/deploy：新增 `agent` 队列、Node 22 runner 和 pi npm 依赖。
- API/frontend：重跑端点、Agent 状态/建议字段和重大商机提醒。
- 安全：SSRF、提示词注入、PII 发送给模型、模型越权、重复动作、秘密泄露和任务风暴。

## 实施记录

- [x] 定义双端 schema、领域策略和 faux-provider 测试。
- [x] 实现安全链接检查与受限 pi runner/子进程适配器。
- [x] 增加数据库迁移、repository、后处理用例和 Celery 任务。
- [x] 接入摄取、重跑 API、DTO 和 dashboard 提醒。
- [x] 更新 Docker/Compose/env、架构/功能/安全/运维文档。
- [x] 完整验证并归档计划。

## 进度与发现日志

- 2026-07-10：从最新 `main` 创建 `features/pi-agent-integration`，核对 IM 与 pi 运行时。
- 2026-07-10：采用受限 Node 子进程与 Python 策略执行边界；完成
  message → agent queue → URL inspector → pi → projection 纵向切片。
- 2026-07-10：Docker 镜像构建成功，镜像内 Node 22.22.0、pi model registry 和 Celery task 可导入。
- pi 本地 `main` 仅落后远端一个 Bun 版本提交，Agent API 与 0.80.6 package 未变化。
- pi faux provider 真实执行工具 schema 时发现 nullable string 联合的分支顺序会把 `null` 转为空串；
  已把 TypeBox 的 `Null` 分支前置并用测试固定。
- 首次真实 Alembic upgrade 发现 JSON server default 中 `:null` 被 SQLAlchemy 当成 bind 参数；改用
  `jsonb_build_object` 后 upgrade/downgrade/upgrade 通过。
- `alembic check` 仅报告既有 `ix_telegram_user_configs_enabled` 缺失，pi 新增 metadata 无额外漂移；
  已记录到技术债，不在本功能分支混入无关索引迁移。

## 决策日志

- 2026-07-10：选一次性受限子进程，不选 coding-agent CLI 或常驻微服务；理由见 ADR-0002。
- 2026-07-10：内部重大商机提醒可自动投影，所有外部动作只建议、不执行；自动外发另行决策。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 24 Markdown、62 backend Python、5 harness tests |
| `make check` | 通过 | 26 backend tests；前端 lint/typecheck/build 通过 |
| Node runner tests/check | 4 tests 通过 | 使用 pi faux provider，不调用真实模型 |
| 目标 backend tests | 14 tests 通过 | 开关、policy、SSRF、超时、子进程契约、补判用例 |
| Alembic upgrade/head | 通过 | 临时 PostgreSQL：upgrade → downgrade 002 → upgrade；head 003 |
| Compose/Docker build | 通过 | local/prod config、Docker check/full build、镜像内 runtime 冒烟 |
| workflow YAML / `git diff --check` | 通过 | 3 个 workflow 可解析，无空白错误 |

## 回滚与恢复

关闭 `PI_AGENT_ENABLED` 可立即停止新分析；关闭时队列适配器无副作用退出。应用回滚前确认新增列
保持向后兼容；迁移 downgrade 只删除新增列。由于本切片不执行外部动作，不需要撤回外部消息。

## 结果与剩余风险

已交付默认关闭、可重试、可审计的 pi 消息后处理链路，以及真实链接核验、联系方式投影、行动建议
和看板重大商机提醒。没有使用真实 provider key 或生产消息做付费模型冒烟；启用前仍需在隔离环境
配置 provider 并观察成本、延迟与误判。应用级 DNS 检查不能完全消除 rebinding，生产需配置受控
egress。邮件、加好友和私信仍是建议；真实执行必须另建审批/审计能力与 ADR。既有 Telegram enabled
索引漂移保留在技术债。
