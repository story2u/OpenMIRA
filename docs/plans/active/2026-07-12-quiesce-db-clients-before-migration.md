# 发布迁移前收敛数据库客户端

> 状态：active · Owner：Codex · 创建：2026-07-12 · 更新：2026-07-12

## 目标与用户价值

生产 release 部署在迁移前停止长期运行的数据库客户端，避免 PostgreSQL 连接耗尽导致迁移失败。

## 非目标

- 不调整 PostgreSQL `max_connections`、连接池大小或业务查询逻辑。
- 不改变应用服务、迁移镜像或数据库 schema。

## 背景与当前行为

`release/v2.0.0` 的 Deploy to VPS 运行 `29186803632` 在 `migrate` 容器连接 PostgreSQL 时收到
`TooManyConnectionsError`。现有 workflow 在迁移前保留 API、Celery 与 Telegram listener，旧连接会与
迁移竞争连接槽。

## 验收标准

- [ ] 迁移前停止已配置的 API、Celery worker/beat、Telegram listener。
- [ ] 迁移成功后按原有流程重建运行服务。
- [ ] Harness 拒绝缺少或错误排序的数据库客户端停止步骤。
- [ ] `release/v2.0.0` 的重新发布通过迁移和健康检查。

## 影响面与风险

部署 workflow、Harness、运维文档受影响；无 API、domain、数据库迁移或前端改动。迁移期间相关服务会
短暂不可用；迁移失败时客户端保持停止，优先保证 schema 与应用版本不会混跑。

## 实施步骤

- [x] 从失败运行日志确认 PostgreSQL 连接耗尽位置。
- [ ] 在部署脚本中收敛数据库客户端后再执行迁移。
- [ ] 添加 Harness 回归检查并更新运行文档。
- [ ] 验证、同步默认分支 workflow、重新发布 release。

## 进度日志

- 2026-07-12：开始；确认 release workflow 由默认分支 `main` 定义，修复需同步两个分支。

## 发现日志

- 2026-07-12：构建、SSH 与环境同步均成功；失败仅发生在 Alembic 创建 asyncpg 连接时。

## 决策日志

- 2026-07-12：停止已知数据库客户端而非所有 runtime 服务；保持 frontend 与 cloudflared 可运行，并在
  迁移失败时避免旧 API 继续访问不确定 schema。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make harness-check` | 待运行 | |
| `make backend-check` | 待运行 | |
| release 部署 | 待运行 | |

## 回滚与恢复

回滚此 workflow commit 并重新部署可恢复旧顺序，但不建议在连接耗尽未解决时执行。迁移失败时先检查
`docker compose logs migrate`，修复连接压力或数据库配置后重新发布；确认 schema 状态后再启动客户端。

## 结果与剩余风险

完成时补充实际发布证据。若停止客户端后仍连接耗尽，需要在 VPS 上审计 `pg_stat_activity` 与
`max_connections`，作为独立数据库容量问题处理。
