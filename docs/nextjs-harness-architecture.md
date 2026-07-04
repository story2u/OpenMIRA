# Next.js Harness 架构设计

> 本文档定义 Go 迁移阶段中 Next.js 前端的可验证链路。  
> 对齐 `Python/docs/ai/` 的“先导航、先清单、先验证”理念，但不把 Python 文档体系逐字搬到前端。

## 1. 定位与边界

前端迁移不能“推断业务事实”，只能验证并承接 Go/Python 的兼容契约。  
Next.js harness 负责两件事：

- 验证“页面级行为”是否仍可用、可恢复、可继续操作；
- 形成可回放的证据（路由、交互、网络态）供切流决策。

本架构默认不负责：

- 验证 DB/Redis/任务状态的一致性（后端 harness 承担）；
- 在前端重写权限过滤、列表过滤、统计口径归一化；
- 将投影、发送队列、设备锁等后端副作用迁移成前端逻辑。

## 2. 原则

1. **先约束页面边界，再迁移交互行为**  
   先确认路由、导航、登录态、版本、基本入口可用；再逐步引入发送、搜索、实时回放等交互。

2. **前端只验证“展示/交互契约”**  
   业务裁剪、分页口径、状态归类必须在 Go API 或旧 Python API 契约里验证。

3. **每个验证都有固定产物**  
   路由清单、e2e 报告、失败摘要必须可落盘，便于阶段回滚对账。

4. **最小可观测**  
   失败时要求有可定位到“哪条请求/哪条接口/哪次重连”的证据，不靠肉眼猜测。

5. **高频变更留白口径**  
   `node --test` 与 Playwright 的“低成本断言”先于真实 Python/Go 全量联调。

## 3. 分层设计（Go 迁移专用）

### L0 入口与导航清单

- 命令：`cd go && node scripts/next-routes.mjs web/app --check --markdown`
- 输出：`go/tmp/phase1/web-routes.json`、`go/tmp/phase1/web-routes.md`
- 覆盖：
  - `/`、`/admin`、`/login`、`/admin-login`、`/cs-login`、`/version.txt`
  - 页面路由组/动态段是否存在（如 `/realtime/...`）
  - 与 `NEXT_REQUIRED_ROUTES` 配置一致性

### L1 组件与客户端库单元

- 命令：`cd go/web && npm run test`
- 覆盖：
  - `sessionToken`、`realtime`、`sessionLogin`、`workbenchSend`、`admin*` 等核心库/纯函数
  - 关注 request build、错误映射、状态合并、重试策略、缺参处理
- 产物：通过 Node 测试日志落盘（建议统一由 `go/scripts/phase1_gate.sh` 管理）

### L2 Next.js 基础界面 smoke

- 命令：`cd go/web && npm run test:e2e`
- 覆盖：
  - 路由可达性（主要入口、登录页、版本页）
  - 关键 shell 文案/导航可见性（避免首次渲染错误）
  - `/version.txt` 有内容返回
- 扩展：可以加入 token 注入场景（CS/管理员）与异常网络时文案。

  推荐执行方式：`RUN_WEB_E2E=1 SKIP_NPM_CI=1 bash scripts/phase1_gate.sh`（在 Go 项目根目录）。

### L3 前后端衔接观察

- 目标：用后端 harness 产物（golden / route-diff / replay）约束 API 行为
- Next.js 侧动作：
  - 仅验证请求构造是否与 API 端点、参数名、HTTP 方法一致；
  - 不在页面层写复杂业务判断。

### L4 可观测与回归

- 记录：Playwright trace/screenshot（如启用）、前端错误日志、版本变更事件
- 标准化指标：
  - 首屏可见失败率（页面关键文案缺失）
  - 关键页面加载耗时异常（dev 环境可加可选阈值）
  - token refresh/reconnect 回调是否可复用（通过单元测试）

## 4. 关键门禁建议（按阶段）

### 阶段 1（骨架）
- `NEXT_REQUIRED_ROUTES` 包含 `/`, `/admin`, `/login`, `/admin-login`, `/cs-login`, `/version.txt`
- `npm run test` 与 `npm run test:e2e` 必须通过
- 前端错误日志钩子（`ClientTelemetry`）必须可用，不要求上报链路成熟
- `phase1_gate` 产物建议包含：
  - `web-e2e.out`
  - `web-e2e.json`
  - `web-e2e.md`

### 阶段 3~4（工作台/管理台只读）
- 关键页面切片必须可加载并请求成功
- 管理组和会话组的数据刷新行为必须是“触发请求 + 呈现返回”，不在组件层重建后台筛选
- 实时 hook 仅做事件订阅、gap 检测、gap replay 路径触发，不做业务状态推导

### 阶段 5+（实时）
- 连接 `/ws/{channel}` 的 URL 组装、订阅池、重连退避、gap replay 回退流程应有覆盖
- `conversation.message`、`task.status` 等关键事件可被消费者接收并触发回源刷新

### 阶段 11（前端接管）
- 需要在前端层提供完整 smoke，可回归：
  - 登录态恢复（含 tab-scoped token）
  - 关键业务流转（搜索、消息面板、发送消息）
  - 版本切换、降级提示、网络异常提示

## 5. 与现有文档的映射

- 与 `go/docs/harness-architecture.md`：共享 L0/L4/L8 视角
- 与 `go/docs/phased-plan.md`：对应阶段 1、3、4、5、11 的验收入口
- 与 `go/docs/refactor-plan.md`：与“Next.js 前端分层”约束保持一致，不允许前端承担事实裁剪

## 6. 常见反模式（需避免）

- 在 React 里写“权限裁剪 + 搜索过滤 + 统计口径”，把后端职责提前移入前端；
- 用快照测试替代接口契约回归；
- 只测 `npm run test`，不验证主路由和关键页面 smoke；
- 在 token 失效时静默重试，不暴露可回归的错误提示；
- 让 Playwright 连接真实生产通道却不记录可复现 trace。

## 7. 对外接口级兼容提醒

前端迁移成功仅在“页面成功渲染 + 契约请求可执行”成立时成立。  
`/api/v1/**`、`/ws/{channel}`、`/version.txt`、`/healthz`、`/readyz`、`/metrics` 的契约一致性仍由后端 harness 判定；
前端侧只负责展示与交互不变形。
