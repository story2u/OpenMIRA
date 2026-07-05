# Next.js Harness Architecture

本文档定义独立 Go + Next.js IM 项目前端的可验证链路。Next.js harness 关注页面、交互、网络状态和可观测证据，不替代后端领域契约。

## 1. 定位与边界

Next.js 前端负责把 Go API、实时事件和用户操作组织成可用界面：

- 消息端工作台：会话列表、消息面板、发送入口、实时状态、客户资料。
- 管理台：账号、分配、配置、内容、诊断、任务和审计。
- 运维入口：健康状态、错误上报、发布验证 smoke。

前端默认不负责：

- 在浏览器中重建权限过滤、列表筛选或统计口径。
- 将投影、发送队列、设备锁等后端副作用实现成页面状态机。
- 绑定某个消息平台或 RPA provider 的专属流程。

## 2. 原则

1. 先约束页面边界，再实现交互行为。
   - 先确认路由、导航、登录态和基本入口可用，再逐步引入发送、搜索、实时回放等交互。

2. 前端只验证展示和交互契约。
   - 业务裁剪、分页口径、状态归类必须在 Go API 或领域服务中验证。

3. 每个验证都有固定产物。
   - 路由清单、e2e 报告、失败摘要、screenshot/trace 必须可落盘，便于阶段回滚对账。

4. 最小可观测。
   - 失败时要求能定位到哪条请求、哪条接口、哪次重连或哪个组件状态。

5. 通道中立。
   - 页面只消费抽象后的 conversation、message、receipt、task、contact 和 automation 状态。
   - 企微、短信、Web chat、内部测试通道等差异应在后端 connector 或 adapter 内处理。

## 3. 分层设计

### L0 入口与导航清单

- 命令：`cd go/web && npm run build`
   - 输出：Next.js build 日志和路由编译结果。
- 覆盖：
  - `/`、`/admin`、`/login`、`/admin-login`、`/cs-login`
  - 页面路由组和动态段是否存在
  - 与 `NEXT_REQUIRED_ROUTES` 配置一致性

### L1 组件与客户端库单元

- 命令：`cd go/web && npm run test`
- 覆盖：
  - session、realtime、workbench、admin、diagnostics 等核心库和纯函数
  - request build、错误映射、状态合并、重试策略、缺参处理
- 产物：Node 测试日志和 JSON/Markdown 摘要。

### L2 Next.js 基础界面 Smoke

- 命令：`cd go/web && npm run test:e2e`
- 覆盖：
  - 路由可达性。
  - 登录入口和主要 shell 是否能首次渲染。
  - token 注入、网络异常、重新连接等基础交互。

推荐执行方式：

```bash
cd go/web
npm run test:e2e
```

### L3 前后端衔接观察

- 目标：用 Go API 和领域契约约束页面请求行为。
- Next.js 侧动作：
  - 验证请求构造是否与 API 端点、参数名、HTTP 方法一致。
  - 验证 realtime hook 能订阅、重连、检测 gap 并触发回源刷新。
  - 不在页面层写复杂业务判断。

### L4 可观测与回归

- 记录：Playwright trace/screenshot、前端错误日志、版本变更事件。
- 标准化指标：
  - 首屏可见失败率。
  - 关键页面加载耗时。
  - token refresh/reconnect 回调是否可复用。
  - API 错误分类是否能被 UI 明确呈现。

## 4. 阶段门禁建议

### 阶段 1：骨架

- `NEXT_REQUIRED_ROUTES` 包含 `/`, `/admin`, `/login`, `/admin-login`, `/cs-login`。
- `npm run test` 与 `npm run build` 必须通过。
- 前端错误日志钩子可用。
- Release gate 产物包含 route、unit、build 摘要。

### 阶段 3-4：工作台和管理台只读

- 关键页面切片可加载并请求成功。
- 管理组和会话组的数据刷新行为必须是“触发请求 + 呈现返回”。
- 页面不重建后台筛选、权限裁剪或统计口径。

### 阶段 5：实时

- `/ws/{channel}` 的 URL 组装、订阅池、重连退避和 gap replay 有覆盖。
- `conversation.message`、`task.status` 等关键事件可被消费者接收并触发回源刷新。

### 阶段 8+：发送与自动化

- 发送入口只创建标准化 outbound task，不绑定具体通道或 provider。
- 自动化动作只消费 provider-neutral capability，不在页面内出现供应商流程。

### 阶段 11：完整产品面

- 登录态恢复。
- 搜索、消息面板、发送消息、任务状态和诊断入口形成可回归 smoke。
- 网络异常提示清晰可见。

## 5. 常见反模式

- 在 React 里写权限裁剪、搜索过滤、统计口径。
- 用快照测试替代接口契约回归。
- 只测 `npm run test`，不验证主路由和关键页面 smoke。
- 在 token 失效时静默重试，不暴露可回归的错误提示。
- 让页面直接依赖某个消息平台、设备协议或 RPA provider。
