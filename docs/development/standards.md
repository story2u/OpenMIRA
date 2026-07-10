# 开发规范

> 状态：当前规范 · 最后核验：2026-07-10

## 通用原则

- 先沿用相邻代码的命名、错误处理和测试模式；不要为单个功能引入第二套架构。
- 函数和模块保持单一职责，优先显式数据流，避免隐藏全局状态和难以追踪的副作用。
- 注释解释“为什么”和约束，不复述代码。演示、兼容或临时逻辑必须写清退出条件。
- 处理不可信输入时 fail closed；用户可恢复的错误给出明确原因，不吞异常。
- 日志使用结构化字段，包含可定位实体的 ID，禁止记录凭据、token 或完整外部 payload。

## Python / 后端

- 目标运行时 Python 3.11+；使用类型标注、`async` I/O 和项目现有 Pydantic/SQLModel 模式。
- Python 与依赖统一由 uv 管理：声明在 `backend/pyproject.toml`，精确解析在 `backend/uv.lock`。
  修改依赖只使用 uv 命令并同时提交两者；不得维护第二份手写 requirements 真相源。
- Ruff 行宽 100；格式与 import 顺序以 `backend/pyproject.toml` 为准。
- API route 负责 HTTP 解析、依赖注入和响应映射；跨步骤业务流程进入 application use case；
  可独立验证的业务判断进入 domain service。
- 领域代码不得导入 FastAPI、SQLAlchemy/SQLModel、Celery 或具体 provider。
- 访问数据库通过 repository；事务提交行为沿用现有 repository/session 约定，不在 mapper 中 I/O。
- 外部平台 payload 先在 adapter 中规范化为 `InboundMessage`；平台差异不得泄露进检测策略。
- 时间使用 timezone-aware datetime。工作时间判断统一走 `WorkTimeService`。
- 状态变化先调用 `ensure_transition_allowed`；不得在 route/worker 中任意赋值绕过状态机。
- 对外错误使用稳定、可操作的 detail；内部异常保留上下文并避免暴露秘密。

## API 契约

- Pydantic DTO 是 HTTP 契约；SQLModel 实体不得作为未审查的公共响应直接泄露。
- 现有前端契约使用 camelCase；新增字段保持同一资源内一致，不做隐式“双命名”。
- 列表接口必须有明确上限和用户隔离；详情/消息/状态操作必须验证资源 owner。
- 写接口要定义幂等性或重复提交行为。外部消息摄取继续使用
  `(channel, external_message_id)` 唯一约束。
- breaking change 优先新增兼容字段/端点并记录迁移；确需破坏时写 ADR 与发布计划。

## 数据库与迁移

- 每次模型结构变化新增 `backend/alembic/versions/` 迁移，命名说明业务意图。
- 升级和降级路径都应明确；大表回填、锁表或数据丢失风险必须写入执行计划。
- 已进入共享分支的迁移视为不可变；用新迁移修正，不修改历史。
- JSONB 只用于边界不稳定或天然文档型数据；需要查询、约束或关联的字段应结构化。
- 新唯一约束或索引要有并发/查询理由，并补重复数据或竞态测试。

## TypeScript / 前端

- Next.js App Router 页面放 `frontend/app/`；可复用业务组件放 `components/`；基础控件放
  `components/ui/`；数据访问集中在 `lib/api.ts`，共享契约放 `lib/types.ts`。
- 默认保持严格类型，不使用 `any` 掩盖 API 漂移；外部响应在边界映射并补默认值的业务理由。
- 只有需要 hooks、事件或浏览器 API 的组件使用 `'use client'`；不要无故扩大客户端边界。
- 异步 UI 必须有 loading、error、empty 和重复提交保护；真实写操作不得只乐观改本地 store。
- 无障碍是完成条件：语义元素、label、键盘操作、焦点、`aria-live` 与对比度按现有模式维护。
- 响应式从窄屏开始；复用现有 design token 和 shadcn/Base UI 组件，不硬编码第二套视觉系统。
- mock 只能用于明确的 demo/fixture。登录后的生产请求失败不得静默展示看似真实的 mock 数据。

## AI 与 IM 集成

- 提示词集中在 `backend/app/infrastructure/ai/prompts.py` 或清晰的 AI adapter；提示词变更要用
  固定输入测试行为边界，不只检查字符串。
- 模型输出始终视为不可信：限制长度、验证结构，并在发送前执行确定性策略检查。
- `AI_ENABLED` 控制生成，`IM_SEND_ENABLED` 控制真实发送；测试和本地默认必须安全关闭。
- 发送逻辑需先考虑重试与幂等，避免任务重跑造成重复消息。
- 新 provider 实现现有端口并在组合根注册，领域/application 不直接分支判断 provider。

## 文档与决策

- 当前架构/行为更新权威文档；未来工作写活动计划；持久且难逆的取舍写 ADR。
- 文档链接使用仓库相对路径，命令可复制执行，状态描述必须能指向代码或测试证据。
- 规则应可验证；若一条规范反复被违反，将其升级为 lint/测试/CI，并提供修复型错误信息。
