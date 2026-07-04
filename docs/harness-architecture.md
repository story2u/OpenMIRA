# Standalone IM Harness Architecture

本文档定义独立 Go + Next.js IM 项目的验证体系。Harness 不再服务于运行时对照，而是服务于本仓库自己的产品契约、可靠性目标和发布决策。

## 1. Harness 定义

Harness 是围绕代码变更建立的一套证据系统：

- 它回答“当前 IM 产品契约是什么”。
- 它回答“这次变更是否保持 API、事件、任务、实时推送和数据投影可用”。
- 它回答“本阶段是否具备独立发布、回滚和观测能力”。

文档说明为什么测；schema、fixture、测试、report 和 CI gate 负责证明测到了什么。

## 2. 设计原则

1. 契约先于实现。
   - HTTP、WebSocket、task payload、outbox event、Redis key、DB schema、错误结构和权限语义都要有可执行证据。

2. 通道和 provider 可替换。
   - 消息通道、RPA provider、外部平台代理和媒体服务必须通过接口进入 core。
   - 企微、魔云腾/MytRpc、LiveKit、Coze 等只能出现在 adapter、provider 或 integration package 内。

3. 快速 gate 与深度 gate 分层。
   - 本地默认跑 `go test ./...`、`go vet ./...`、前端单元测试和构建。
   - 高风险链路进入容器集成、replay、race、benchmark、shadow 和发布就绪 profile。

4. 环境可复现。
   - 常规 CI 不连接线上云服务，不依赖共享开发库。
   - MySQL、Redis、对象存储 mock、外部 HTTP mock、消息通道 fake 和 RPA fake 应能固定版本启动。

5. 可观测即测试输出。
   - Harness 产物需要包含耗时、队列长度、重试、锁等待、trace id、关键阶段跨度和错误分类。
   - 收发消息、归档、WS、媒体和自动化链路的日志、metrics、trace 与业务断言同等重要。

6. 前端只验证展示和交互。
   - Next.js harness 验证页面加载、参数透传、事件消费和 UI 状态。
   - 权限裁剪、列表筛选、统计口径、状态归类必须在 Go API 或领域服务中验证。

## 3. 建议目录

```text
go/
  docs/
    architecture.md
    product-roadmap.md
    milestones.md
    harness-architecture.md
    nextjs-harness-architecture.md
    release-readiness.md
  internal/harness/
    inventory/       # 项目清单、diff、报告格式
    contracts/       # HTTP / WS / task / Redis / schema 契约加载和校验
    golden/          # 产品级 golden fixture、语义归一化
    replay/          # WS、Redis Stream、outbox、archive 样本重放
    fixtures/        # 确定性时间、账号、设备、会话、消息 fixture
    env/             # Testcontainers / fake server / local stub 编排
    observe/         # metrics、trace、日志断言和报告收集
  tests/
    contract/
    integration/
    replay/
    e2e/
    shadow/
  testdata/
    contracts/
    golden/
    replay/
    fixtures/
```

目录只在对应阶段需要时创建，不为“完整结构”空建大量占位文件。

## 4. 分层架构

### L0 文档与范围 Gate

职责：

- 从 `docs/product-roadmap.md`、`docs/architecture.md` 和 `docs/milestones.md` 收敛当前阶段目标、非目标和下线清单。
- 记录每个阶段的发布范围、暂不发布范围、回滚路径和待确认项。
- 对照“通道中立”和“provider 中立”原则，阻止新功能把供应商细节写入 core。

输出：

- 阶段说明。
- 功能域到 harness gate 的映射。
- 文档与代码冲突清单。

### L1 清单与静态 Gate

职责：

- 用 inventory 扫描 Go 项目的 route、schema、Docker role、Redis key、WS event 和 task type。
- 对 Go 代码执行 `gofmt`、`go test ./...`、`go vet ./...`。
- 对前端执行 unit test、route check 和 build。
- 在显式配置 `RUN_REFERENCE_GATES=1` 时产出 external reference inventory、route diff 和 schema/OpenAPI drift；默认 standalone gate 不需要外部 reference root。
- 在过渡期继续产出现有 golden/readiness artifact，但不把这些 artifact 作为长期架构命名。

输出：

- `inventory-report.json`
- `inventory-report.md`
- `reference-gates.md`
- `phase1_gate_manifest.json`
- CI job summary
- 基础 pass / fail

适用阶段：所有阶段。

### L2 单元与领域 Gate

职责：

- 覆盖纯函数、状态机、权限判定、分页参数、字段归一化、错误映射和响应序列化。
- 表驱动测试必须用业务场景命名，而不是只写 `case1`、`case2`。

Go 约束：

- 使用 `context.WithTimeout` 控制阻塞路径。
- 使用 fake clock，避免 `time.Sleep`。
- 使用 `testdata/` 保存稳定 fixture。
- 对共享 fixture 做深拷贝，避免并行测试互相污染。

适用阶段：所有新增 Go package。

### L3 Fuzz 与属性 Gate

职责：

- 保护输入解析、ID 归一化、消息 payload、WS event、Redis key builder、分页游标和 schema 解码。
- 用真实但脱敏的样本作为 seed corpus，验证 Go 代码不会 panic、不会生成非法状态、不会越权扩展范围。

适用阶段：认证、任务 payload、消息归一化、联系人身份、会话搜索、外部回调、归档结构解析。

### L4 契约与 Golden Gate

职责：

- 验证 API 状态码、响应字段、错误结构、分页游标和权限裁剪。
- 对 WS event、task payload、Redis Stream entry 和 outbox event 做 schema 校验。
- Golden 更新必须附带解释，不能把未解释的 drift 直接接受为新真值。

输出：

- `golden-diff.md`
- `contract-violations.json`
- 阶段可发布 / 不可发布结论。

适用阶段：只读 API、低风险写 API、WS gateway、task 创建、主动触达。

### L5 容器集成 Gate

职责：

- 用可复现容器验证 MySQL、Redis、对象存储 mock、外部 HTTP mock、消息通道 fake 和 RPA fake。
- 验证事务、锁、Stream pending、Pub/Sub、连接池、schema 变更脚本和幂等键。

禁止：

- 常规 CI 连接线上 RDS、Redis、OSS、真实消息平台或真实设备。
- 通过共享开发库让测试顺序影响结果。

适用阶段：认证基础设施、projection、incoming-worker、send-dispatcher、archive、媒体、统计审计。

### L6 Replay Gate

职责：

- 用脱敏事件样本重放 Redis Stream、outbox、WS event、archive callback 和 task status。
- 验证重复投递、乱序、延迟、缺字段、过期任务、断线重连和 gap replay。

输出：

- `replay-report.md`
- 每条样本的输入、输出、幂等键、终态和耗时。

适用阶段：实时推送、消息接收、发送状态、归档、AI/SOP 自动化。

### L7 并发与性能 Gate

职责：

- 对锁、连接池、worker pool、WS fanout、Redis consumer、outbox relay 和发送调度做 race 与 benchmark。
- 定义 p50/p95/p99、队列堆积、水位、超时、失败率和重试预算。

输出：

- `benchstat` 或稳定 benchmark 输出。
- `race` gate 结果。
- 性能预算报告。

适用阶段：写路径、实时路径、worker、媒体处理、批量同步。

### L8 发布就绪 Gate

职责：

- 验证目标运行面所需 route、feature flag、env、compose service、secret、fixture 和回滚路径是否齐备。
- 输出按 profile 聚合的 pass / fail，让发布讨论基于证据而不是口头检查。

输出：

- `release-readiness-*.json`
- `release-readiness-*.md`
- 发布建议、阻塞项和回滚检查点。

现有脚本生成 `release-readiness-*.json` / `release-readiness-*.md`；旧 `cutover-*` artifact 只作为历史 fallback 被 summary 脚本识别。

## 5. 推荐命令

| Gate | 命令 |
| --- | --- |
| Go unit | `go test ./...` |
| Go vet | `go vet ./...` |
| Frontend unit | `cd web && npm run test` |
| Frontend build | `cd web && npm run build` |
| Phase gate | `SKIP_NPM_CI=1 bash scripts/phase1_gate.sh` |
| API local run | `go run ./cmd/api` |
| Readiness profile | `go run ./cmd/release-readiness -all -format markdown` |

## 6. 反模式

- 把消息通道、RPA provider 或外部平台的字段直接写入 IM core。
- 用手工验证替代可复现 fixture。
- 只看 HTTP 成功，不验证 outbox、投影、WS 和任务终态。
- 在 React 组件中重写权限、筛选、统计口径。
- 让单个外部服务失败导致 API 写路径不可恢复。
- 继续扩展临时桥接能力，却没有下线条件和替代 provider。
