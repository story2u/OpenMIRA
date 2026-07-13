# 技术债清单

> 状态：维护中 · 最后核验：2026-07-10

这里只记录由代码审计确认、跨越当前小改动的系统缺口。具体执行时创建活动计划并链接回来。

| 优先级 | 缺口 | 当前证据 | 期望完成信号 |
| --- | --- | --- | --- |
| P1 | 前端 API 失败时的错误与本地状态边界仍需继续收敛 | `frontend/lib/app-store.tsx` | 登录态失败显示明确错误；真实写 API 接通；不以本地状态伪造生产结果 |
| P1 | 人工回复、AI 草稿、消息历史、状态更新未从前端接后端 | `frontend/lib/api.ts` 与详情组件 | 关键旅程端到端通过，发送失败不伪造状态 |
| P1 | Celery 发送任务的重试/幂等证据不足 | `backend/app/worker/tasks.py`、回复用例 | 重跑不会重复发消息；失败可恢复且可观察 |
| P1 | repository/API 的用户隔离缺少集成测试 | 当前 `backend/tests/` | 临时 PostgreSQL/API 测试覆盖跨用户拒绝 |
| P2 | 联系方式手工编辑和好友申请执行仍是演示 | `frontend/lib/app-store.tsx` | 接入有审批/审计的真实 API，或删除生产 UI 暗示；pi Agent 当前只提供动作建议 |
| P2 | Alembic metadata 缺少 Telegram enabled 索引 | `alembic check` 报告 `ix_telegram_user_configs_enabled` 未在历史迁移创建 | 用新 migration 修复并让 `alembic check` 无漂移；不得改写已发布迁移 |
| P2 | readiness 只返回静态 ok | `backend/app/api/v1/routes/health.py` | 检查关键依赖并区分 liveness/readiness |
| P2 | 前端缺少组件与 E2E 自动化 | `frontend/` 无测试配置 | 登录后核心只读/写旅程进入 CI |
| P2 | 可观测性与告警未形成闭环 | 只有基础 logging | 结构日志、关键指标/trace、任务/发送告警有仓库配置与演练 |
| P2 | 数据库备份/恢复与迁移回滚未演练 | 部署 workflow | 有可执行 runbook 和最近一次恢复证据 |

维护规则：解决后删除该行并在完成计划保留历史；优先级变化写依据；不要把模糊愿望或当前任务
顺手可修的 TODO 丢到这里。
