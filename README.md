# Go/Next 第一阶段骨架

本目录承载从 `Python/` 旧项目迁移到 Go + Next.js + Tailwind CSS 的新实现。第一阶段只建立可验证骨架，不接管旧业务流量。

## 当前边界

- 保留 `Python/` 作为唯一真实业务实现。
- Go API 只提供 `/`、`/healthz`、`/readyz`、`/metrics` 最小探针。
- `cmd/inventory` 只读扫描旧项目路由、契约、功能文档、Docker 服务、WS 事件、Redis key、DB 表和任务类型，用于后续迁移对账。
- `cmd/inventory-diff` 对比两份 inventory JSON 的数量变化，并可按阈值失败，用于把“清单变化必须解释”变成 CI gate。
- `cmd/route-diff` 对比 Python route inventory 和 Go route metadata，输出阶段覆盖度报告；默认比较当前暴露路由，`-go-routes candidate` 比较全部 Go 候选路由，`-mode openapi-drift` 可在提供 OpenAPI 文件时比对文档级请求/响应 schema、path 参数和 operationId。
- `cmd/golden-http` 读取 `testdata/golden/*.json`，在有 Python/Go base URL 时执行同请求响应对账；CI 先做 fixture 校验。
- `web/` 只提供 Next.js `/` 与 `/admin` 占位入口，不实现业务筛选或旧页面逻辑。

## 本地验证

推荐使用阶段一 gate 统一执行 Go、inventory 和前端构建验证：

```bash
cd go
SKIP_NPM_CI=1 bash scripts/phase1_gate.sh
```

`phase1_gate.sh` 还会额外产出：
- `web-routes.json` / `web-routes.md`：Next.js 路由清单与必须路由检查（`/`、`/admin`、`/login`、`/admin-login`、`/cs-login`、`/version.txt`）
- `web-unit-test.out` / `web-unit-test.json` / `web-unit-test.md`：Next.js 单元测试执行与摘要
- `web-build.out` / `web-build.json` / `web-build.md`：前端构建结果与摘要
- GitHub Actions 会用 `scripts/phase1-summary.mjs` 把 inventory、route、schema、OpenAPI、cutover profile、建议动作和 web 摘要写入 job summary；本地也可执行 `node scripts/phase1-summary.mjs tmp/phase1` 查看同一摘要。
- Cutover profile 的人工执行清单见 `docs/cutover-runbooks.md`；完整机器生成版本可执行 `go run ./cmd/cutover-readiness -all -format runbook`。
- 可通过 `NEXT_REQUIRED_ROUTES` 自定义必需路由，或用 `SKIP_WEB_ROUTE_CHECK=1` 跳过入口检查（仍会产出 route 清单）。

CI 环境不要设置 `SKIP_NPM_CI`，会先执行 `npm ci` 再构建。

GitHub Actions 的 `Go Phase 1 CI` 会上传完整 `go/tmp/phase1/` 为 `phase1-gate-artifacts`，并可通过 repository variables 透传门禁阈值：

- `ROUTE_DIFF_MAX_PYTHON_ONLY`、`ROUTE_DIFF_MAX_GO_ONLY`
- `SCHEMA_DRIFT_MISMATCH_THRESHOLD`
- `PYTHON_OPENAPI_SPEC`、`GO_OPENAPI_SPEC`、`OPENAPI_DRIFT_MISMATCH_THRESHOLD`
- `INVENTORY_BASELINE_JSON`、`INVENTORY_DIFF_PROFILE` 与 `INVENTORY_DIFF_MAX_*`

如果未显式设置 `INVENTORY_BASELINE_JSON`，`phase1_gate.sh` 会在存在 `testdata/inventory/baseline.json` 时自动启用它作为 inventory diff baseline；首次接入或仓库未提交 baseline 时仅产出当前 inventory，不阻塞 gate。更新 baseline 时使用：

```bash
cd go
bash scripts/refresh_inventory_baseline.sh
```

`INVENTORY_DIFF_PROFILE` 默认为 `auto`：GitHub target/ref 为 `main`、`master` 或 `release/*` 时使用 `strict`，所有 inventory 数量阈值默认 0；其它分支使用 `observe`，只产出 diff artifact。任意 `INVENTORY_DIFF_MAX_*` 显式值都会覆盖 profile 默认值。

也可以单独运行：

```bash
cd go
go test ./...
go vet ./...
go run ./cmd/inventory -python-root ../Python -pretty
go run ./cmd/inventory -python-root ../Python -format markdown
go run ./cmd/inventory-diff -baseline tmp/baseline-inventory.json -current tmp/phase1/inventory-report.json -max-routes 0 -format markdown
go run ./cmd/route-diff -python-root ../Python -format markdown
go run ./cmd/route-diff -python-root ../Python -go-routes candidate -format markdown
go run ./cmd/route-diff -python-root ../Python -mode schema-drift -pretty
go run ./cmd/route-diff -python-root ../Python -mode schema-drift -format markdown
go run ./cmd/route-diff -python-root ../Python -mode schema-drift -max-schema-mismatch 0 -format json
go run ./cmd/route-diff -python-root ../Python -mode openapi-drift -format markdown
go run ./cmd/route-diff -python-root ../Python -go-routes candidate -mode openapi-drift -format markdown
go run ./cmd/route-diff -python-root ../Python -mode openapi-drift -python-openapi ../Python/openapi.json -go-openapi tmp/go-openapi.json -max-openapi-mismatch 0 -format json
SCHEMA_DRIFT_MISMATCH_THRESHOLD=1 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh
PYTHON_OPENAPI_SPEC=../Python/openapi.json GO_OPENAPI_SPEC=tmp/go-openapi.json OPENAPI_DRIFT_MISMATCH_THRESHOLD=0 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh
INVENTORY_DIFF_PROFILE=strict SKIP_NPM_CI=1 bash scripts/phase1_gate.sh
go run ./cmd/cutover-readiness -all -format runbook
go run ./cmd/golden-http -cases testdata/golden/phase1-probes.json -validate-only
go run ./cmd/api
```

已有 Python 与 Go 服务同时运行时，可执行首批探针 golden 对账：

```bash
cd go
go run ./cmd/golden-http -cases testdata/golden/phase1-probes.json -python-url http://127.0.0.1:8000 -go-url http://127.0.0.1:9000 -format markdown
```

前端验证：

```bash
cd go/web
npm install
npm run build
```

容器构建：

```bash
cd go
docker build -t wework-go-api --build-arg TARGET_CMD=api .
docker build -t wework-go-outbox-worker --build-arg TARGET_CMD=outbox-worker .
docker build -t wework-go-send-dispatcher --build-arg TARGET_CMD=send-dispatcher .
docker build -t wework-go-archive-media-worker --build-arg TARGET_CMD=archive-media-worker .
docker build -t wework-go-voice-transcription-worker --build-arg TARGET_CMD=voice-transcription-worker .

cd web
docker build -t wework-next-web .
```

## 不可破坏的兼容面

- 旧 API 路径、请求参数和返回结构。
- `/ws/{channel}` 及 Redis Pub/Sub 事件语义。
- `Python/contracts/v1/*.schema.json` 的 task payload 与状态事件契约。
- 数据库 schema、Redis key、投影表和 Docker 运行角色。
- `incoming-worker` 与 `send-dispatcher` 热路径独立运行边界。
