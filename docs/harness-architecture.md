# Go Harness 架构设计

> 本文档定义 Go 重构阶段使用的 harness 架构。
> 它借鉴 `Python/docs/ai/` 的导航、事实源、影响面和回写理念，但不复制 Python 文档体系。
> 目标是把每次迁移从“写完再测”转成“先冻结证据，再实现，再用证据判定是否可接管”。
> 本文档只描述目标架构，不表示这些目录、测试或 CI gate 已全部实现。
> 对齐执行方式的目标提示见 `go/docs/codex-refactor-goal-prompt.md`。

## 1. Harness 定义

这里的 harness 不是单个测试套件，也不是脚本集合，而是一套围绕代码变更的证据系统：

- 它先回答“当前 Python 行为是什么”。
- 再回答“Go 实现是否在同一输入下保持同一契约和边界”。
- 最后回答“本阶段是否有足够证据接管流量，或必须继续留在 shadow / canary”。

`Python/docs/ai/` 的核心价值是导航和事实沉淀：先读索引，再收敛功能域，再核实代码，再评估影响面。Go 侧 harness 要把这个思想变成可执行资产：清单、契约、golden、replay、并发验证、观测报告和 CI gate。

## 2. 设计原则

1. 事实先冻结，再迁移。
   - 每个阶段开始前先由 inventory 产出 route、contract、WS event、Redis key、DB table、task type 和运行角色清单。
   - 清单变化必须能从代码 diff 或人工批准的破坏性变更中解释。

2. 契约优先于实现。
   - Go 新代码不能靠“看起来等价”接管旧流量。
   - HTTP、WebSocket、task payload、Redis key、DB schema、错误结构和权限语义都需要可比较的契约证据。

3. 快速 gate 与深度 gate 分层。
   - 本地默认跑轻量 gate：`go test ./...`、inventory、contract catalog、前端 build。
   - 高风险链路再进入容器集成、replay、race、benchmark、shadow 和 canary。

4. Go 原生优先。
   - 单元测试使用 Go `testing` 包、表驱动子测试、`testdata/`、`t.Helper()`、`t.Cleanup()` 和确定性 fixture。
   - 解析、归一化、路由、权限、状态机等边界逻辑优先补 fuzz seed corpus。
   - 涉及 goroutine、channel、锁、dispatcher、WS fanout 和 Redis consumer 的变更必须考虑 race gate。

5. 环境可复现。
   - 常规 CI 不连接线上云服务，不依赖共享开发库。
   - MySQL、Redis、对象存储 mock、外部 HTTP mock 和 P1/SDK stub 应通过容器或本地 fake 固定版本启动。

6. 可观测即测试输出。
   - harness 产物不仅是 pass / fail，还要包含耗时、队列长度、重试、锁等待、trace id、关键阶段跨度和错误分类。
   - 对收发消息、存档、WS 和媒体链路，日志、metrics、trace 与业务断言同等重要。

7. 前端只验证展示和交互，不补业务事实。
   - Next.js harness 只能验证加载态、参数透传、事件消费和 UI 状态。
   - 权限裁剪、列表筛选、统计口径、状态归类必须在 Go API 或旧 Python API 的契约中验证。

## 3. 与 Python/docs/ai 的关系

Go harness 借鉴三件事：

- 导航：先定位功能域、入口、数据流和禁区，避免全仓猜测。
- 事实源：接口、事件、Redis key、DB 字段、任务状态和展示字段都必须绑定真实代码或运行证据。
- 回写：一旦迁移改变事实、链路、规则或边界，文档和 harness 清单必须一起更新。

Go harness 不复制三件事：

- 不把所有知识都写成长篇 Markdown。稳定契约应进入 schema、golden、inventory 输出和可执行测试。
- 不让文档替代验证。文档负责解释为什么测，测试和报告负责证明测到了什么。
- 不沿用 Python 的运行时假设。Go 需要额外关注 goroutine 生命周期、context deadline、连接池、锁粒度、race 和 backpressure。

Next.js 前端也有独立执行规范：`go/docs/nextjs-harness-architecture.md`。

## 4. 建议目录

```text
go/
  docs/
    harness-architecture.md
    nextjs-harness-architecture.md
    refactor-plan.md
    phased-plan.md
  internal/harness/
    inventory/       # 清单模型、diff、报告格式
    contracts/       # HTTP / WS / task / Redis / schema 契约加载和校验
    golden/          # Python 与 Go 响应对比、语义归一化
    replay/          # WS、Redis Stream、outbox、archive 样本重放
    fixtures/        # 确定性时间、账号、设备、会话、消息 fixture
    env/             # Testcontainers / fake server / local stub 编排
    observe/         # metrics、trace、日志断言和报告收集
  tests/
    contract/        # 跨包契约测试
    integration/     # MySQL / Redis / OSS mock 等容器集成测试
    replay/          # 事件流重放测试
    e2e/             # Next.js 与 Go API 的关键路径 smoke
    shadow/          # Python / Go 双跑对账
  testdata/
    contracts/
    golden/
    replay/
    fixtures/
```

目录只在对应阶段需要时创建。未进入实现阶段前，不为“完整结构”空建大量占位文件。

## 5. 分层架构

### L0 文档导航层

职责：

- 从 `Python/docs/ai/`、`go/docs/refactor-plan.md` 和 `go/docs/phased-plan.md` 收敛功能域、入口和禁区。
- 记录每个阶段的接管范围、暂不接管范围、回滚路径和待确认项。

输出：

- 阶段说明。
- 功能域到 harness gate 的映射。
- 文档与代码冲突清单。

### L1 清单与静态 gate

职责：

- 用 inventory 扫描 Python 和 Go 的 route、schema、Docker role、Redis key、WS event 和 task type。
- 有 baseline 时使用 `inventory-diff` 比较 route、contract、WS event、Redis key、DB table 和 task type 数量变化；`phase1_gate.sh` 默认发现 `testdata/inventory/baseline.json`，也允许 `INVENTORY_BASELINE_JSON` 指向 CI 下载的历史 artifact；`INVENTORY_DIFF_PROFILE=auto` 在主干/发布分支使用 strict 零漂移，其它分支默认 observe，具体阈值仍可用 `INVENTORY_DIFF_MAX_*` 显式覆盖。
- 对 Go 代码执行 `gofmt`、`go test ./...`、`go vet`，前端执行 lint / build。

输出：

- `inventory-report.json`
- `inventory-diff.md`
- `phase1_gate_manifest.json` 中的 inventory baseline、阈值与 artifact 路径
- `scripts/phase1-summary.mjs` 汇总后的 CI job summary，包含 cutover profile ready/pass/fail、失败责任域计数、建议动作和 `cutover-runbooks.md` 链接
- `cmd/cutover-readiness -format runbook` 生成的 profile-specific runbook
- CI 中的基础 pass / fail。

适用阶段：

- 所有阶段。

### L2 单元与领域 gate

职责：

- 覆盖纯函数、状态机、权限判定、分页参数、字段归一化、错误映射和响应序列化。
- 表驱动测试必须用业务场景命名，而不是只写 `case1`、`case2`。

Go 约束：

- 使用 `context.WithTimeout` 控制阻塞路径。
- 使用 fake clock，避免 `time.Sleep`。
- 使用 `testdata/` 保存稳定 fixture。
- 对共享 fixture 做深拷贝，避免并行测试互相污染。

适用阶段：

- 所有新增 Go package。

### L3 Fuzz 与属性 gate

职责：

- 保护输入解析、ID 归一化、消息 payload、WS event、Redis key builder、分页游标和 schema 解码。
- 用旧系统真实样本作为 seed corpus，验证 Go 代码不会 panic、不会生成非法状态、不会越权扩展范围。

适用阶段：

- 认证、任务 payload、消息归一化、联系人身份、会话搜索、外部回调、存档解密后的结构解析。

### L4 契约与 golden gate

职责：

- 同一请求分别打 Python 与 Go，比较状态码、响应字段、错误结构、分页游标和权限裁剪。
- 在 OpenAPI spec 可用时，对比 request/response schema、path 参数和 operationId；未提供 spec 时，仍通过 route metadata 与 JSON Schema catalog 产出 drift 证据，避免“没有文档就没有门禁”。
- OpenAPI drift 必须同时覆盖默认暴露路由和 candidate 路由：默认路由证明当前运行面未漂移，candidate 路由证明后续切流对象的文档契约可审计。
- schema drift 对 required、enum 等集合语义做稳定归一；字段类型、默认值、枚举成员和约束变化应报告为 drift，纯顺序变化不应成为失败原因。
- 对 WS event、task payload、Redis Stream entry 和 outbox event 做 schema 校验。
- golden 更新必须附带解释，不能把 drift 直接接受为新真值。

输出：

- `golden-diff.md`
- `contract-violations.json`
- `route-openapi-drift.md`
- 阶段可接管 / 不可接管结论。

适用阶段：

- 只读 API、低风险写 API、WS gateway、task 创建、主动触达。

### L5 容器集成 gate

职责：

- 用可复现容器验证 MySQL、Redis、对象存储 mock、外部 HTTP mock 和本地 fake SDK。
- 验证事务、锁、Stream pending、Pub/Sub、连接池、迁移脚本和幂等键。

禁止：

- 常规 CI 连接线上 RDS、Redis、OSS、P1 设备或真实企微。
- 通过共享开发库让测试顺序影响结果。

适用阶段：

- 认证基础设施、projection、incoming-worker、send-dispatcher、archive、媒体、统计审计。

### L6 Replay gate

职责：

- 用脱敏事件样本重放 Redis Stream、outbox、WS event、archive callback 和 task status。
- 验证重复投递、乱序、延迟、缺字段、过期任务、断线重连和 gap replay。

输出：

- `replay-report.md`
- 每条样本的输入、输出、幂等键、终态和耗时。

适用阶段：

- 实时推送、消息接收、发送状态、存档、AI/SOP 自动化。

### L7 并发与性能 gate

职责：

- 验证 Go goroutine、channel、锁、worker pool、Redis claim、DB 连接池和 backpressure。
- 使用 race detector 发现数据竞争。
- 使用 benchmark 和负载脚本验证热点路径的尾延迟、分配次数、锁等待和队列积压。

必须覆盖：

- `send-dispatcher` 跨设备并发、同设备串行。
- `incoming-worker` pending reclaim 和 DLQ。
- WS 多连接 fanout、断线重连、replay。
- archive 批量拉取、媒体队列快慢车道。

### L8 可观测 gate

职责：

- 验证每条高风险链路都有 request id / trace id、阶段耗时、错误分类、队列长度和重试次数。
- OpenTelemetry trace、metrics 和结构化日志必须能被 harness 收集并关联到业务断言。

输出：

- `observability-report.md`
- trace 样本。
- metrics 断言结果。

适用阶段：

- 从阶段 2 开始强制启用。

### L9 Shadow 与 canary gate

职责：

- Go 与 Python 双跑同一输入，Go 只读或 dry-run，不写生产事实源。
- 比较输出、状态转换、事件和耗时。
- canary 只接管明确接口、明确租户或明确设备池，且保留快速回滚。

接管条件：

- contract / golden 无未解释 drift。
- replay 通过。
- 高风险链路 race gate 通过。
- 可观测报告能定位失败阶段。
- 回滚路径已验证。

## 6. 功能域映射

| 功能域 | 必备 harness |
| --- | --- |
| 认证与权限 | L1、L2、L3、L4、L8 |
| 客服工作台只读 | L1、L2、L4、L8、L9 |
| 管理后台 | L1、L2、L4、L8 |
| 实时推送 | L1、L4、L5、L6、L7、L8、L9 |
| 消息发送与分发 | L1、L2、L4、L5、L6、L7、L8、L9 |
| 消息接收与 projection | L1、L2、L5、L6、L7、L8、L9 |
| 会话存档与媒体 | L1、L3、L4、L5、L6、L7、L8 |
| 设备网关与 SDK 控制 | L2、L5、L6、L7、L8、L9，真机 gate 单独执行 |
| AI / SOP / 主动触达 | L1、L2、L4、L5、L6、L8、L9 |
| Next.js 前端 | API contract、loading state、事件消费、可访问性 smoke、build gate（详见 `nextjs-harness-architecture.md`） |

## 7. 每次迁移的工作流

1. 读文档。
   - 先读 `Python/docs/ai/00_READ_FIRST.md`、`02_GLOBAL_RULES.md`、`01_SYSTEM_INDEX.md` 和目标功能域文档。
   - 再读 Go 侧计划和本 harness 文档。

2. 生成或核对清单。
   - 运行 inventory。
   - 判断本阶段涉及 HTTP、WS、Redis、DB、task、Docker role、前端页面中的哪些契约。

3. 选择 gate。
   - 按功能域映射选择最低 gate。
   - 高风险链路默认追加 replay、race、observability 和 shadow。

4. 实现最小迁移。
   - 不改变旧契约。
   - 不把前端做成事实源。
   - 不绕过 dispatcher、worker、projection 或 outbox 边界。

5. 运行证据。
   - 本地轻量 gate 必须通过。
   - 阶段要求的深度 gate 必须出报告。

6. 更新事实。
   - 若真实代码改变接口、事件、Redis key、任务语义、运行角色或事实源，必须更新对应 docs 和 harness 清单。
   - 若未改变事实，最终输出必须明确说明已检查，无需回写。

## 8. Gate 命名建议

| Gate | 建议命令 |
| --- | --- |
| 基础 Go 测试 | `go test ./...` |
| Race 测试 | `go test -race ./...` |
| Fuzz smoke | `go test ./... -run=^$ -fuzz=Fuzz -fuzztime=30s` |
| Inventory | `go run ./cmd/inventory -python-root ../Python` |
| Contract | `go test ./tests/contract/...` |
| Integration | `go test ./tests/integration/...` |
| Replay | `go test ./tests/replay/...`、`go run ./cmd/replay-http -cases testdata/replay/<suite>.json` |
| Benchmark | `go test ./... -bench=. -benchmem` |
| Frontend build | `cd web && npm run build` |
| Frontend unit harness | `cd web && npm run test` |
| Frontend E2E harness | `cd web && npm run test:e2e` |
| Frontend E2E（含在 phase1_gate） | `RUN_WEB_E2E=1 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh` |
| Phase-1 全量门禁（本地） | `SKIP_NPM_CI=1 bash scripts/phase1_gate.sh` |
| Phase-1 切流剖分门禁 | `CUTOVER_PROFILE_LIST=workbench-read,admin-accounts SKIP_CUTOVER_AGGREGATE=1 bash scripts/phase1_gate.sh` |
| Phase-1 运行摘要 | `phase1_gate_manifest.json`（记录本次选用 profile、时间戳与门禁开关） |
| Cutover profile runbook | `go run ./cmd/cutover-readiness -all -format runbook` |

命令只是约定名称，实际落地时应按模块和 CI 成本拆分。重 gate 不应默认阻塞每次保存，但必须阻塞高风险阶段接管。

## 9. 反模式

- 只断言 `err == nil`，不验证状态、字段、事件和副作用。
- 直接改 golden 让测试通过，却没有解释 drift 来源。
- 用大 mock 把真实 contract 差异藏掉。
- CI 连接真实云服务或真实企微设备。
- 用 `time.Sleep` 等待异步结果，而不是可观测信号、fake clock 或明确 deadline。
- 把 Redis / projection 缺失时的全量扫描当成兼容路径。
- 在 Next.js 里补权限过滤、业务筛选或状态归类。
- 新旧实现长期并存却没有 shadow 差异报告和下线条件。

## 10. 外部理念参考

- Go `testing` 包提供测试、子测试、benchmark、fuzz 和测试资源管理能力，是 Go harness 的默认底座：https://pkg.go.dev/testing
- Go fuzzing 适合用 seed corpus 扩展输入空间，保护解析器、协议边界和状态机：https://go.dev/doc/security/fuzz/
- Go race detector 应用于 goroutine、channel、锁和共享状态的迁移验证：https://go.dev/doc/articles/race_detector
- Testcontainers for Go 支持用容器编排依赖服务，适合替代共享开发库做集成测试：https://golang.testcontainers.org/
- Pact 的 consumer-driven contract 思路适合 API 消费方和提供方分阶段迁移：https://docs.pact.io/
- OpenAPI / JSON Schema 适合把 HTTP 与 JSON payload 从文档描述提升为可校验契约：https://spec.openapis.org/oas/latest.html 和 https://json-schema.org/
- OpenTelemetry Go 适合把 trace、metrics、logs 纳入验证产物，而不仅是上线后排障工具：https://opentelemetry.io/docs/languages/go/
- SWE-bench 的任务级评测思想说明，现代 harness 越来越强调“真实仓库任务 + 可执行判定器”，这适合指导本项目的阶段接管判断：https://www.swebench.com/

## 11. 现代趋势借鉴（Go 实现）

- 方向一：契约优先迁移。将旧 Python 接口定义收敛为 `schema` / `route` / `event` 三类可验证源，先冻结，再改造，再逐批对账。
- 方向二：可观测先于覆盖率。测试结果附带关键指标（耗时、重试率、队列长度）能让“为什么通过/为什么失败”有实据。
- 方向三：双实现对照。以 `schema-drift`、`openapi-drift`、`golden-http`、`replay-http` 与 shadow/canary 组成“多视角对账”，相比单点断言更能防漏。
- 方向四：失败可解释。每次 gate 输出都需带理由桶（例如 schema mismatch reason、event miss reason），避免“通过但未知原因”。
- 方向五：渐进开关与可回滚。Go 仍沿用 `GO_ENABLE_*` 机制，在没有充分 evidence 前不改变默认流量路径。
- 方向六：前端只测展现和交互。Next.js 层先做加载态、参数透传、状态渲染、事件消费 smoke，不在前端复原原有业务事实推断。
