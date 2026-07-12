# 测试策略

> 状态：当前策略 · 最后核验：2026-07-10

测试的目标是给代理可操作反馈并证明用户行为，而不只是提高覆盖率数字。

## 测试金字塔

| 层级 | 关注点 | 当前工具/位置 |
| --- | --- | --- |
| Harness/结构 | 文档可达、依赖边界、迁移图、datetime timezone | harness script + stdlib unittest |
| 领域单测 | 检测策略、状态迁移、时间窗、纯映射 | `backend/tests/test_*.py` + `uv run pytest` |
| 适配器/安全单测 | webhook 解析、签名/加密、Telegram user client | backend tests + fakes/monkeypatch |
| pi Agent 契约 | TypeBox 工具、faux provider、子进程 JSON、SSRF/重定向与策略投影 | `backend/pi-agent-runtime/test` + backend tests |
| 应用/API 集成 | repository、owner 隔离、用例编排、HTTP 契约 | 当前覆盖不足，新增功能应优先补齐 |
| 前端静态/单元 | 类型、billing 映射、lint、Next.js 构建 | Vitest + ESLint + `pnpm typecheck` + `pnpm build` |
| 移动端 | DTO、Billing fake、ViewModel、平台编译 | XCTest/xcodebuild；JUnit/Gradle lint + test + assemble |
| UI/端到端 | 登录后关键旅程、真实 API 写操作 | 当前未建立，是已知技术债 |

## 按风险选择验证

- 纯文档：harness check，必要时人工确认图表/命令。
- 领域或 mapper：目标 pytest + backend check。
- API/repository/auth：单测 + HTTP/DB 集成测试 + backend check。
- schema/migration：upgrade、数据断言、downgrade（若安全）、再 upgrade；至少在临时数据库验证。
- 前端组件/页面：frontend check；交互改动做键盘、窄屏、loading/error/empty 人工或浏览器测试。
- IM/AI/队列：单元 fake + 任务重复执行/失败路径；真实发送只在授权的隔离环境冒烟。
- pi Agent：faux provider，不使用真实 key/付费模型；URL 使用 MockTransport + 固定 DNS；同时验证
  Docker 镜像内 Node/model registry 和 Alembic upgrade/downgrade。
- 部署：完整 check + `docker compose config` + 健康/ready 检查和回滚思路。

## 测试编写规则

- 测试名描述行为和条件，不描述内部方法。
- 每个回归测试应在修复前失败；断言可观察结果和关键副作用。
- 时间、随机、外部网络和 provider 响应用显式 fake 固定；不要依赖真实账号或当前时间。
- 覆盖权限拒绝、重复消息、非法状态迁移、外部超时/错误和空数据。
- 不通过降低断言、扩大 sleep、吞异常或把生产逻辑切到 mock 修复 flaky test。
- 新测试放在离责任最近的层；不要让 UI E2E 承担本可由领域单测快速发现的规则。

## 当前基线与缺口

已有测试主要覆盖检测、时间窗、安全工具、mapper、Telegram adapters/user client 与入口 import。
薄弱区包括：真实数据库 repository、FastAPI 权限/契约、Celery 幂等与重试、OAuth 回调集成、前端
组件和端到端旅程。新增相关功能时应就地补测试，不等待集中“补覆盖率”项目。

## 完成证据

交付时记录实际命令、退出状态和关键数量（例如 `N passed`）。未能运行的检查要说明原因、替代证据
与剩余风险。CI 绿色不能替代对高风险外部集成的针对性验证。
