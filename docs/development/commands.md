# 开发命令

> 状态：当前事实 · 最后核验：2026-07-18

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
pnpm install --frozen-lockfile
make frontend-check
cd frontend && pnpm dev
```

`frontend-check` 运行 ESLint、独立 `tsc --noEmit`、Vitest 和 production build。独立 typecheck 是必须项，
因为当前 `next.config.mjs` 的 build 兼容设置会跳过 TypeScript 错误。

## React Native 与共享 TypeScript

Web、`mobile/radar` 和 `packages/*` 使用根 `pnpm-lock.yaml`；`backend/pi-agent-runtime` 继续使用独立
`package-lock.json`。首次安装与 P1 检查：

```bash
pnpm install --frozen-lockfile
make contracts-check
make shared-check
make rn-check
```

`contracts-check` 先确认 FastAPI OpenAPI snapshot 未漂移，再检查生成的 TypeScript；`shared-check` 覆盖
core/API/Agent 共享包；`rn-check` 追加 RN typecheck、Vitest、Expo 依赖矩阵和双平台 Hermes export。
RN Vitest 还会校验中英文 catalog key/插值参数一致，并扫描生产 `.tsx`，阻止把新的硬编码中文界面文案
绕过 catalog 提交；交互式 Agent 测试另覆盖真实文件 SQLite、真实 pi loop + faux SSE、本地三工具与
content-free turn 协调。后端对应 PostgreSQL 用例需要显式传入测试数据库 URL，不能把 skipped 当通过。
原生 Release 构建由 CI 的 `rn-ios` / `rn-android` job 执行，本地需要 Xcode 或 Android SDK 时可按
`mobile/radar/README.md` 运行。开发包的本地明文 SSE 例外不得复制到生产 bundle 配置。

RN 认证、在线 dashboard、详情/消息、回复/状态与设置/Telegram shell 需要显式 origin：

```bash
EXPO_PUBLIC_API_BASE_URL=https://api.example.com pnpm --dir mobile/radar start
pnpm --dir mobile/radar fixture:auth
```

本地 fixture 只接受 README 中的固定 `.test` 假账号，并返回确定性看板、商机详情、Agent 发现、分页/
空消息数据，以及进程内模板/AI 草稿/认领/状态/幂等回复、用户设置、Telegram 连接/套餐暂停来源和
只读订阅目录/用量/管理快照；订阅 sync 固定返回未配置，不模拟 Offering 或交易成功；
不连接外部 IM 或数据库。Release 生产 bundle 仍拒绝明文 HTTP。

原生 Google 登录的 prebuild/release 构建必须成对传入 `EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID` 与
`EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID`；前者同步加入后端 `GOOGLE_NATIVE_CLIENT_IDS`，后者用于 iOS
SDK 与回调 scheme。Apple 构建的精确 bundle ID 必须加入 `APPLE_NATIVE_CLIENT_IDS`。本地可用格式
合法的假 ID 验证编译/config plugin；CI 的 RN 双平台 Release job 也只用非敏感假 ID 覆盖原生依赖链，
真实登录、错误 audience 和首次建号只能在隔离 provider 环境验收。

商店换装构建必须显式设置 `RADAR_APP_VARIANT=production`，并提供新的 `RADAR_APP_VERSION`（严格
`major.minor.patch`）与正整数 `RADAR_BUILD_NUMBER`。该变体生成既有 `com.codeiy.im` 双平台身份和
`opportunity-radar` deep-link scheme，并关闭 dev 包的 loopback/明文网络例外；缺版本信息会在 Expo
config 阶段 fail closed。production 还必须在构建期提供不含路径/凭据/查询的 HTTPS
`EXPO_PUBLIC_API_BASE_URL`，防止产出只能显示配置错误的商店包。CI 使用 `.test` 占位 origin 和占位
版本验证无签名 Release 编译，实际发版值仍由商店与生产部署记录决定。

RN 语言声明来自 `mobile/radar/app.json` 的 `expo-localization` config plugin。`prebuild --clean` 后，iOS
产物应在 `Info.plist` 含 `en`/`zh-Hans`，Android 应生成含 `en`/`zh-CN` 的
`res/xml/locales_config.xml` 并在 manifest 引用；修改支持语言时必须同时复跑 `rn-check` 和双平台
production Release 构建。

## iOS App

需要 macOS + Xcode 和 xcodegen（`brew install xcodegen`）；`.xcodeproj` 是生成产物不入库：

```bash
make ios-check
cd mobile/ios && xcodegen generate && open OpportunityRadar.xcodeproj
```

`ios-check` 用 xcodegen 生成工程并在 iPhone 16 Simulator 上执行 build + XCTest（不签名）。
`make check` 不包含 `ios-check`，因为 Linux 本地环境无法执行；CI 使用 Xcode 16.4 + iOS 18.5 的
独立 macOS job，runner 缺少对应 runtime 时会先通过 `xcodebuild` 安装。

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

VPS release 使用仓库根作为两个镜像的 build context。修改 Dockerfile、根 workspace、共享包或
`.dockerignore` 后，应在推送 release 分支前额外运行：

```bash
docker build -f backend/Dockerfile -t im-backend:verify .
docker run --rm -e DATABASE_URL=postgresql+asyncpg://user:pass@localhost:5432/im \
  -e ADMIN_API_TOKEN=test-admin-token \
  -e JWT_SECRET_KEY=test-jwt-secret-key-that-is-long-enough \
  --entrypoint sh im-backend:verify \
  -c 'test -f /app/pi-agent-runtime/node_modules/@story2u/radar-agent/src/analysis.mjs && node --check /app/pi-agent-runtime/src/index.mjs && python -c "import app.main"'
docker build -f frontend/Dockerfile -t im-frontend:verify .
```

后端命令验证 `file:` 共享 Agent 包已实体化进最终镜像，而不是只验证依赖 stage；前端命令必须从根
context 执行，因为 Dockerfile 消费根 pnpm lock/workspace 与共享包。镜像启动 smoke 可将前端映射到
本地端口后请求 `/`，预期返回 HTTP 200。

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
- backend job：固定 uv 版本、Python 3.12，并安装 `@story2u/radar-agent` 的最小 Node 依赖后执行
  `uv sync --locked`、migration upgrade/downgrade/upgrade、compileall、Ruff、pytest；Node 依赖用于校验
  Python gateway 与共享 TypeScript Agent 契约一致。
- pi-agent job：Node 22、`npm ci --ignore-scripts`、语法检查和 faux-provider 测试。
- frontend job：Node 22、pnpm 10、根 frozen install、共享包/RN JS、Web lint/typecheck/Vitest/build。
- ios/android jobs：原生旧 App 的 xcodegen/XCTest 与 Gradle 检查；`rn-ios` / `rn-android` 另做 Expo
  prebuild 和 RN Release 构建。旧 iOS App 固定 Xcode 16.4 + iOS 18.5；RN iOS 因
  `expo-modules-jsi@57.0.3` 的 Swift tools 6.2 要求固定使用 Xcode 26.3。

若本地命令与 CI 漂移，优先统一根 Makefile和本文件，不在入口提示词复制更多命令。
