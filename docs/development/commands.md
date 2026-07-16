# 开发命令

> 状态：当前事实 · 最后核验：2026-07-12

命令默认从仓库根目录执行。根 `Makefile` 是 AI 和 CI 的稳定入口；子项目原生命令用于缩小反馈。
后端统一由 uv 管理 Python、`.venv`、依赖声明和锁文件；不要使用 `pip install -r` 或手工修改环境。

## Harness

```bash
make harness-check
```

只依赖 Python 标准库，检查必需入口、入口大小、Markdown 本地链接、文档索引、CI 接入、后端
Python/domain 外部依赖边界、Alembic 单链单 head 和持久化 datetime timezone，并运行检查器回归测试。

## 后端

首次本地安装（uv 会按 `requires-python` 获取合适的 Python，并从 `uv.lock` 创建 `.venv`）：

```bash
cd backend && uv sync --locked --dev
```

常用检查：

```bash
make backend-check
cd backend && uv run --locked pytest -q tests/test_detection_policy.py
cd backend && uv run --locked ruff check app tests scripts alembic --select E,F,ASYNC --ignore E501
cd backend && uv run --locked python -m compileall -q app tests scripts alembic
```

依赖变更使用 `uv add <package>`、`uv add --dev <package>` 或 `uv remove <package>`，同时提交
`pyproject.toml` 和 `uv.lock`。升级单包使用 `uv lock --upgrade-package <package>`；CI 和 Docker
始终使用 `--locked`，禁止隐式改锁。

本地直接启动 API 需要有效 `.env`：

```bash
cd backend
cp .env.example .env
make dev
```

## pi Agent runtime

运行时要求 Node.js 22.19+，依赖由独立 npm 锁文件管理：

```bash
make pi-agent-check
cd backend/pi-agent-runtime && npm ci --ignore-scripts
cd backend/pi-agent-runtime && npm test
```

`pi-agent-check` 做 locked install、Node 语法检查和 faux-provider 测试，不需要真实模型 key。更新 pi
必须同时修改 `package.json` / `package-lock.json`，使用精确版本并审查 transitive diff；禁止运行未知
lifecycle script。

工作机会发现的虚构样本确定性评估：

```bash
make job-discovery-eval
```

该命令不调用真实模型，不得把输出当作生产招聘识别准确率。使用经人工复核的 Agent 输出评分方法见
[`evals/job-discovery/README.md`](../../evals/job-discovery/README.md)。

## 前端

首次安装与常用检查：

```bash
cd frontend && pnpm install --frozen-lockfile
make frontend-check
cd frontend && pnpm dev
```

`frontend-check` 运行 ESLint、独立 `tsc --noEmit`、Vitest 和 production build。独立 typecheck 是必须项，
因为当前 `next.config.mjs` 的 build 兼容设置会跳过 TypeScript 错误。

## iOS App

需要 macOS + Xcode 和 xcodegen（`brew install xcodegen`）；`.xcodeproj` 是生成产物不入库：

```bash
make ios-check
cd mobile/ios && xcodegen generate && open OpportunityRadar.xcodeproj
```

`ios-check` 用 xcodegen 生成工程并在 iPhone 16 Simulator 上执行 build + XCTest（不签名）。
`make check` 不包含 `ios-check`，因为 Linux 本地环境无法执行；CI 使用独立 macOS job。

## Android App

需要 JDK 17 + Android SDK（装 Android Studio 即含）；仓库已提交固定 Gradle wrapper：

```bash
make android-check                    # ./gradlew lintDebug testDebugUnitTest assembleDebug
```

`android-check` 跑 Android Lint、JVM 单元测试和 debug assemble。CI 使用独立 Linux/JDK 17 job；
`make check` 暂不包含它，避免无 Android SDK 的环境阻塞。

## 完整本地检查

```bash
make check
```

依次运行 harness、uv locked sync、后端语法/lint/test、pi runtime locked install/test、工作机会
确定性评估、前端 lint/typecheck/test/build。需要安装 uv、Node/npm 与 pnpm。

## Docker 集成环境

```bash
cd backend
cp .env.example .env
docker compose build
docker compose up -d postgres redis
docker compose run --rm migrate
docker compose up api celery_worker celery_beat telegram_listener
```

- API 文档：`http://localhost:8000/docs`
- 根健康检查：`http://localhost:8000/healthz`
- API 健康检查：`http://localhost:8000/api/v1/healthz`

## 数据库迁移

```bash
cd backend
alembic current
alembic upgrade head
alembic downgrade -1
```

自动生成迁移只能作为草稿；必须人工/代理审查约束、索引、默认值、回填、upgrade 和 downgrade。

## 与 CI 的对应关系

- `.github/workflows/ci.yml` 的 harness job：`python scripts/harness_check.py`。
- backend job：固定 uv 版本、Python 3.12、`uv sync --locked`、migration upgrade/downgrade/upgrade、compileall、Ruff、pytest。
- pi-agent job：Node 22、`npm ci --ignore-scripts`、语法检查和 faux-provider 测试。
- frontend job：Node 22、pnpm 10、frozen install、lint、独立 typecheck、Vitest、build。
- ios/android jobs：xcodegen + XCTest；Gradle lint + unit test + debug assemble。

若本地命令与 CI 漂移，优先统一根 Makefile和本文件，不在入口提示词复制更多命令。
