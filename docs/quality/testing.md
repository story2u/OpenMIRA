# 测试策略

> 状态：当前策略 · 最后核验：2026-07-17

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
| 移动端 | DTO、Billing fake、ViewModel、Hermes/平台编译 | Vitest/Expo export；XCTest/xcodebuild；JUnit/Gradle lint + test + assemble |
| 共享契约 | FastAPI OpenAPI snapshot、生成 TS、core/API/Agent 跨 runtime 规则 | `make contracts-check` + `make shared-check` |
| UI/端到端 | 登录后关键旅程、真实 API 写操作 | iOS Release + 本地确定性 fixture 人工验收；自动化 UI E2E 仍是技术债 |

## 按风险选择验证

- 纯文档：harness check，必要时人工确认图表/命令。
- 领域或 mapper：目标 pytest + backend check。
- API/repository/auth：单测 + HTTP/DB 集成测试 + backend check。
- schema/migration：upgrade、数据断言、downgrade（若安全）、再 upgrade；至少在临时数据库验证。
- 前端组件/页面：frontend check；交互改动做键盘、窄屏、loading/error/empty 人工或浏览器测试。
- RN 界面文案：typecheck + Vitest；catalog key 和插值参数必须一致，生产 `.tsx` 不得新增绕过 catalog 的
  硬编码中文。新增语言还需验证原生语言声明、双平台 Release 和至少一台真机的系统语言切换。
- RN 同步：用真实文件型 SQLite 覆盖分页、断点、关闭/重开、cursor 续传、投影计数和 ack；至少保留
  1 万条 change 的宿主机时间上限回归。该上限只用于发现算法/事务退化，不能替代 iOS/Android 真机的
  飞行模式、系统 kill/reopen、冷启动、内存与耗时证据。
- RN 生命周期：对推送 cursor 的落后/相等/领先与本地状态读取失败做确定性测试；网络恢复只允许明确的
  offline→online 转换触发一次，初始在线、未知状态和 transport 切换不得制造重复同步。
- IM/AI/队列：单元 fake + 任务重复执行/失败路径；真实发送只在授权的隔离环境冒烟。
- 人工回复：除 adapter fake 外，必须覆盖持久化幂等键的同键重放/异文冲突、并发发送、provider
  结果不确定、投影恢复、归档/状态竞态和明确禁用；数据库约束与行锁使用临时 PostgreSQL 验证。
- pi Agent：faux provider，不使用真实 key/付费模型；URL 使用 MockTransport + 固定 DNS；同时验证
  Docker 镜像内 Node/model registry 和 Alembic upgrade/downgrade。
- 设备 Agent：本地 SSE fake 必须覆盖 UTF-8 分块、强制 alias/system prompt/单工具、usage、非 2xx、
  取消和 provider 身份不泄露；RN 用真实 pi `Agent` 执行共享 tool schema。SQLite 覆盖 run 恢复、续租、
  前后台中断、token 缺失、过期、重试上限和恢复后单次 shadow claim。PostgreSQL 必须覆盖并发 shadow
  唯一、shadow 不改投影/不新增 ledger、expire 保留 server truth，以及 primary release 后只建立一条 fallback
  reservation；还必须覆盖稳定 cohort、同版本 readiness、claim-next 复用既有 reservation、不同幂等键的
  同消息 reservation 唯一、领取窗口后的 worker 不接管 active run，以及陈旧/禁用/最终失败 worker 不标错
  或 release。fake/export 不能替代真实 provider 与双真机 kill/reopen。
- 交互式 Agent：真实文件 SQLite 覆盖 session/entry 顺序、owner、容量、保留、删除和损坏；真实 pi
  `Agent` + faux SSE 分别覆盖 v1 三项只读工具、v2 精确六工具与 v3 仅增加 `send_reply`，以及工具循环、
  上下文截断、流式取消和 provider 身份脱敏。v1 必须拒绝内部写，v2 必须拒绝外发/未知工具；草稿需证明只落本地且未调用在线 draft API，
  status 需覆盖 outbox 幂等/版本/过期/冲突并诚实返回 queued，claim 需覆盖认证、owner/归档拒绝和取消传播。
  v3 需覆盖审批正文编辑、拒绝不执行、purpose token 分离、SQLite 拒绝 token material、服务端确认后才显示
  sent，以及重放/参数篡改/过期/device revoke/version drift/provider uncertain/并发最多一次调用。
  PostgreSQL 覆盖独立 quota、claim 幂等、并发 reservation/provider stream 唯一、device revoke、lease/lock、
  consume/release 与内部命令 receipt；OpenAPI 生成必须证明内联消息联合无悬空 `$defs`。双平台 Hermes export
  仍不能替代真实 provider、系统 kill/reopen、键盘/窄屏/无障碍和 allowlist 灰度。
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
