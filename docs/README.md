# 项目知识库导航

> 状态：维护中 · 最后核验：2026-07-10 · 适用范围：整个仓库

这里是项目的版本化知识库和 AI 渐进式上下文入口。`AGENTS.md` / `CLAUDE.md` 只给出
全局约束与路线；具体事实、规范和计划放在这里。运行时代码、数据库迁移、测试与部署配置
是行为的最终证据，文档与它们冲突时应先确认代码，再在同一变更中消除漂移。

## 30 秒项目画像

商机雷达从 Telegram 与企业微信接收消息，用规则和可选 AI 分类器识别商机。工作时间内
进入人工审核，非工作时间可进入 AI 自动回复队列；可选 pi Agent 会异步检查公开链接、补判商机、
提取联系方式并给出需审批的后续动作建议。后端是 FastAPI + SQLModel/PostgreSQL + Celery/Redis，
pi 使用镜像内受限 Node runner；前端是 Next.js 16 + React 19。当前前端仍包含明确的演示态好友
申请等 SOP 流程，不能把所有可见交互都视作后端已交付能力。

## 任务路由

| 任务 | 必读 | 需要时再读 |
| --- | --- | --- |
| 新功能/跨层改动 | [架构总览](architecture/overview.md)、[功能地图](product/feature-map.md)、[开发工作流](development/workflow.md) | [执行计划](plans/README.md)、[ADR](decisions/README.md) |
| 后端/API/数据 | [开发规范](development/standards.md)、[开发命令](development/commands.md) | [安全基线](quality/security.md)、[运行与运维](operations/runtime.md) |
| 前端/交互 | [功能地图](product/feature-map.md)、[开发规范](development/standards.md) | [测试策略](quality/testing.md) |
| IM、认证、AI 自动回复 | [安全基线](quality/security.md)、[架构总览](architecture/overview.md) | [运行与运维](operations/runtime.md) |
| 缺陷修复 | [开发工作流](development/workflow.md)、[测试策略](quality/testing.md) | 受影响模块对应文档 |
| CI、文档或 harness | [Harness 原则](harness/principles.md)、[开发命令](development/commands.md) | [技术债](plans/tech-debt.md) |

## 权威文档

- [架构总览](architecture/overview.md)：系统上下文、模块职责、依赖方向与关键数据流。
- [功能地图](product/feature-map.md)：产品能力、入口、成熟度和真实/演示边界。
- [开发规范](development/standards.md)：后端、前端、契约、数据库和可观测性约束。
- [开发工作流](development/workflow.md)：AI 执行功能、修 bug、更新知识的标准闭环。
- [开发命令](development/commands.md)：可复制的本地与 CI 验证命令。
- [测试策略](quality/testing.md)：风险分层、测试放置和完成标准。
- [安全基线](quality/security.md)：认证、秘密、外部输入、发送动作和 AI 风险控制。
- [运行与运维](operations/runtime.md)：服务拓扑、环境变量、队列与部署事实。
- [ADR 索引](decisions/README.md)：重要、持久、难逆的设计决策。
- [执行计划](plans/README.md)：复杂工作的活文档、完成归档和技术债。
- [Harness 原则](harness/principles.md)：本工程采用的近期 harness 范式与维护规则。

## 文档维护协议

- 文档描述当前事实，计划描述未来动作；两者不要混写。
- 新能力只有在代码、测试和运行路径具备后才能标为“已实现”。
- 改动架构边界、功能成熟度、公共 API、环境变量或验证命令时必须更新对应文档。
- 已失效内容直接修正或删除；需要保留历史原因时写 ADR 或归档计划。
- 运行 `make harness-check` 验证入口大小、链接、必需文档和 Python 分层边界。
