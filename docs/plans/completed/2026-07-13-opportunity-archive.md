# 商机归档功能

> 状态：completed · Owner：Codex · 创建：2026-07-13 · 更新：2026-07-13

## 目标与用户价值

让用户把已处理或暂时不需要展示的商机移出日常看板，同时保留原业务状态、消息、Agent 分析和
回复记录；用户可以查看归档区并恢复，避免用永久删除承担日常整理需求。

## 非目标

- 本期不实现自动归档策略、数据保留期、永久删除或导出。
- 本期不改变现有商机状态机，不停止消息源监听，也不删除消息。

## 背景与当前行为

`OpportunityStatus` 表达待人工、AI 回复、跟进、忽略和关闭等业务生命周期，`CLOSED` 是终态；当前
看板没有独立可见性维度，所有记录始终进入默认列表。归档若复用状态会丢失归档前的业务语义。

## 验收标准

- [x] 列表默认只返回未归档记录，并可查询 active、archived 或 all。
- [x] 当前 owner 可归档、重复归档、恢复自己的商机；其他 owner 仍得到 404。
- [x] 支持一次归档 1–100 条当前 owner 的商机，不部分处理跨 owner ID。
- [x] 归档与恢复不改变状态、不删除消息，并记录非敏感审计事件。
- [x] Web 看板可切换归档视图，执行单条/批量归档和单条恢复，并呈现提交与失败状态。
- [x] migration upgrade/downgrade/upgrade、后端、前端与 harness 检查通过。

## 影响面与风险

- 数据库：新增 nullable 字段、查询索引和事件表；旧数据自然视为 active，无回填锁表。
- API/前端：新增兼容字段与端点；默认列表行为仅排除未来归档记录。
- Worker/IM/订阅：不改变消息源和套餐；SLA 扫描及已排队的 AI 自动回复会跳过归档记录。
- 安全：所有写操作和批量 ID 必须 owner 隔离；理由限制长度，不写入消息正文。

## 实施步骤

- [x] 数据模型、迁移、repository、DTO 与 API 测试。
- [x] 前端类型、数据访问、看板和详情交互。
- [x] 文档、完整验证与计划归档。

## 进度日志

- 2026-07-13：完成现状审计并确定归档为正交可见性维度；下一步实现后端纵向切片。
- 2026-07-13：完成后端和 Web 切片；增加已排队 AI 自动回复的归档执行时保护；下一步完整验证。
- 2026-07-13：首次 CI 证明 migration upgrade/downgrade/upgrade 通过；PostgreSQL 测试夹具因仅设置 UUID
  外键而未建立 ORM relationship，flush 顺序错误，已改为先提交用户再创建商机并等待复验。
- 2026-07-13：复验 CI 全部通过，包括 PostgreSQL 后端测试、前端、Android、iOS、harness 和 pi runtime。

## 发现日志

- 前端将六个内部状态压缩成 pending/replied/ignored，进一步证明归档不能复用状态字段。
- 当前列表最多读取 200 条；本期可用 `archive=all` 保持现有前端 store，服务端分页优化留待列表重构。
- 本机没有 Docker 命令，真实 PostgreSQL 迁移与 repository 测试需由 feature 分支 CI 服务容器验证。

## 决策日志

- 2026-07-13：使用 `archived_at/archived_by_user_id/archive_reason`，恢复时只清空这些字段；拒绝新增
  `ARCHIVED` 状态，因为它会覆盖 replied/closed 等业务事实。
- 2026-07-13：归档审计使用独立事件表，重复归档/恢复不重复写事件。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 通过 | 50 Markdown files、79 backend Python files、8 harness tests |
| `make backend-check` | 通过 | 93 passed，13 个 PostgreSQL 集成测试因本机无 Docker/URL 跳过 |
| `make frontend-check` | 通过 | 7 passed；lint、typecheck、Next production build 通过 |
| `make check` | 通过 | harness、backend、pi runtime 4 tests、frontend 全部通过 |
| GitHub CI 后端 | 通过 | run 29256750303；migration upgrade/downgrade/upgrade，106 tests passed |
| GitHub CI 全平台 | 通过 | run 29256750303；后端、前端、Android、iOS、harness、pi 均完成 |

## 回滚与恢复

应用可先回滚到旧版本，新字段不会影响旧代码。确认没有依赖归档记录后可执行 migration downgrade 删除
事件表和归档字段；该 downgrade 会丢失归档标记，因此生产回滚前应先恢复所有记录或导出标记。

## 结果与剩余风险

已交付 owner 隔离的单条/批量归档、恢复、归档审计、默认列表过滤、归档只读保护，以及 Web 看板和
详情页交互。旧记录自动保持未归档，原状态、消息、Agent 结果和回复历史均被保留。

剩余风险：Web store 当前最多加载 200 条记录，较大数据量需要服务端分页；自动归档、保留期限、按范围
永久删除和导出仍是明确的后续能力，不应复用本期手工归档接口隐式实现。
