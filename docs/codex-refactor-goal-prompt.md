# Codex 长任务提示（可直接放入 goal）

目标：按 `go/docs/refactor-plan.md`、`go/docs/phased-plan.md`、`go/docs/harness-architecture.md` 推进重构，把 Python/FastAPI + React/Vite 迁移为 Go + Next.js + TailwindCSS，并保持兼容性边界不变。

## 目标硬约束（必读）

- 不改变对外接口：`/api/v1/**`、`/ws/{channel}`、`/healthz`、`/readyz`、`/metrics` 结构保持兼容。
- 不改 Redis key 命名、DB schema、WS 事件名与 payload 语义。
- 不改 Docker 运行角色边界（api / incoming-worker / send-dispatcher / ws-gateway / archive-sync-worker / archive-media-worker / maintenance-worker / automation-worker / ...）。
- 每个阶段按顺序推进：先补 harness/验证，再实现迁移，再跑测试和对账。

## 执行规则

- 只在有明确“证据闭环”时推进下一阶段：
  - `go test ./...`
  - `go run ./cmd/inventory -python-root ../Python -pretty`
  - `go run ./cmd/inventory-diff -baseline <old.json> -current <new.json>` 清单数量漂移检查（有 baseline 时）
  - `go run ./cmd/golden-http -python ...` 对应阶段 fixture 对账
  - `go run ./cmd/route-diff` 清单一致性检查
  - `go run ./cmd/route-diff -mode openapi-drift` OpenAPI 文档级契约对账（配置 spec 时比对 request/response schema、path 参数和 operationId）
  - Next.js `npm run build`（需要前端变更时）
- `SKIP_NPM_CI=1 bash scripts/phase1_gate.sh`（本地门禁；CI 运行请按仓库 CI 配置）
- `SKIP_NPM_CI=1 NEXT_REQUIRED_ROUTES="/,/admin,/login,/admin-login,/cs-login,/version.txt" bash scripts/phase1_gate.sh`（自定义 Next.js 必需入口）
- `SKIP_WEB_ROUTE_CHECK=1 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh`（如仅需产物，暂不校验必需前端入口）
- `RUN_WEB_E2E=1 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh`（执行前端 Playwright smoke，并产出 `web-e2e.{out,json,md}`）
- `RUN_LIVE_GOLDEN=1 PYTHON_BASE_URL=http://localhost:8000 GO_BASE_URL=http://localhost:8080 bash scripts/phase1_gate.sh`（可选：运行 `/readyz`、`/metrics` 运行态对账）；
- `RUN_LIVE_GOLDEN=1 RUN_PHASE2_AUTH_GOLDEN_LIVE=1 PYTHON_BASE_URL=http://localhost:8000 GO_BASE_URL=http://localhost:8080 bash scripts/phase1_gate.sh`（可选：再包含阶段2会话认证只读链路对账）。
- `RUN_REPLAY_GATING=1 bash scripts/phase1_gate.sh`（可选：运行 `go/testdata/replay/phase5-realtime-read-replay.json` 的 validate-only replay fixture 校验）；
- `RUN_REPLAY_GATING=1 RUN_REPLAY_COMPARE=1 bash scripts/phase1_gate.sh`（可选：除校验外还会运行 replay diff）。
- `PYTHON_OPENAPI_SPEC=<path> GO_OPENAPI_SPEC=<path> OPENAPI_DRIFT_MISMATCH_THRESHOLD=0 bash scripts/phase1_gate.sh`（可选：启用 OpenAPI 文档级 drift 阈值；未配置 spec 时仍产出 `route-openapi-drift*.{json,md}` 并记录 `not_configured`）。
- `INVENTORY_DIFF_PROFILE=strict INVENTORY_BASELINE_JSON=<path> bash scripts/phase1_gate.sh`（可选：启用 inventory 数量 drift gate；`auto` 会在主干/发布分支 strict、其它分支 observe，也可按 `INVENTORY_DIFF_MAX_*` 对 route、contract、WS event、Redis key、DB table、task type 等设置阈值）。
- `CUTOVER_PROFILE_LIST=<profiles>` 可限定切流 profile；`SKIP_CUTOVER_AGGREGATE=1` 可跳过 aggregate 总结，适合快速迭代
  - `phase1_gate` 每次执行会生成 `go/tmp/phase1/phase1_gate_manifest.json`，记录 profile 清单与关键参数，便于门禁复盘
- `go run ./cmd/cutover-readiness -all -format runbook` 可输出当前 Go profile 定义驱动的 cutover runbook；`go/docs/cutover-runbooks.md` 是人工阅读入口。
- 未满足门禁不得默认开启 `GO_ENABLE_*` 候选开关的生产级行为。
- 所有差异必须先写在 `go/docs/phased-plan.md` 当前进展，再提交。

## 按阶段执行（每阶段都要有可回滚提交）

- 阶段 0～2：只做 inventory、契约、只读配置与 session 基础设施。
- 阶段 3～4：客服工作台/管理后台只读与低风险写入。
- 阶段 5：WS 连接与事件回放兼容。
- 阶段 6～7：发送链路与 send-dispatcher 任务执行边界。
- 阶段 8～10：入站、存档、AI/SOP/KB 与自动化。
- 阶段 11：Next.js 完整接管前端展示与交互（不做业务推断）。
- 阶段 12：灰度、canary、回滚验证。

## 任务上下文提交格式

每次提交请在返回内容中输出：

1. 本阶段目标与完成项（对应哪个阶段/模块）。
2. 影响的接口、契约、Redis key、DB table、task type 与 WS event。
3. 通过的 harness 门禁（列出命令与摘要）。
4. 下一阶段待办和阻塞项（如有）。
5. 若某处未迁移，说明是“有意保留”或“暂不支持（需显式确认）”。

## 风险边界（默认不做）

- 不做主观业务重构（例如把权限过滤、状态归类挪到前端）。
- 不做新 schema 变更，不做 Redis key 重命名。
- 不做高风险真机执行前置假设，除非当前阶段已有 replay / shadow / canary 证据。
