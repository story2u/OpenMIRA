# Harness 原则

> 状态：已采用 · 最后核验：2026-07-10

本项目把“harness”定义为围绕编码模型的工程系统：它负责提供可发现上下文、限定动作边界、
保存任务状态、给出快速反馈并证明变更完成。它不等同于一份长提示词。

## 采用的范式

1. **入口是地图，不是百科。** `AGENTS.md` 保持短小，`CLAUDE.md` 导入同一入口，具体知识
   进入结构化 `docs/`。代理只按任务逐步加载必要上下文。
2. **仓库是可见的系统记录。** 架构、功能状态、决策、执行计划和运维约束都版本化；只存在于
   聊天或个人记忆中的关键结论应落回仓库。
3. **规则优先变成可执行反馈。** 能由类型、测试、迁移、lint 或 CI 检查的约束，不只写成
   “请注意”。`scripts/harness_check.py` 首先覆盖文档连通性与稳定的分层边界。
4. **执行计划是一等工件。** 复杂工作把验收、进度、发现、决策和验证结果写入活动计划，完成
   后归档，使长会话或换代理不依赖隐藏上下文。
5. **强边界、局部自治。** 中央约束依赖方向、数据契约、安全与完成证据；边界内允许代理选择
   最简单的可维护实现。
6. **验证闭环，而非一次生成。** 代理从最小相关检查开始，逐级扩展到完整验证；失败信息应能
   指向修复动作。
7. **harness 保持最小。** 每个新增规则都应对应实际失败模式。模型、代码和团队变化后，删除
   已无价值的脚手架，避免规则冲突和上下文腐化。

## 反馈升级路径

同类问题第一次出现时修复代码和测试；重复出现时更新相关文档或模板；仍反复出现时将规则
升级为静态检查、结构测试或 CI gate。反过来，机械规则若持续误报，应降级或移除。

## 当前组成

| 责任 | 仓库实现 |
| --- | --- |
| 上下文选择 | `AGENTS.md`、`CLAUDE.md`、`docs/README.md` 任务路由 |
| 项目记忆 | `docs/architecture/`、`docs/product/`、ADR、归档计划 |
| 任务状态 | `docs/plans/active/` 与计划模板 |
| 工具入口 | 根 `Makefile` 与 `docs/development/commands.md` |
| 结构约束 | `scripts/harness_check.py` 的导入边界检查 |
| 完成证据 | 后端测试/lint、前端 lint/build、CI |
| 风险与权限 | `docs/quality/security.md` 与入口文件全局约束 |
| 漂移治理 | Markdown 链接、必需文档和入口大小检查 |

## 维护触发器

- AI 第二次犯同类错误：把经验落到最近的权威文档。
- 文档规则需要强制执行：优先加清晰报错的检查，而不是继续加粗提示词。
- 新增子系统：更新架构图、功能地图、命令和必要的局部入口。
- 活动计划超过一个会话：保持进度、决策与验证日志可恢复。
- 每季度或发生大版本升级：审查入口长度、死链接、过时命令与无效约束。

## 调研依据

- OpenAI 的 [Harness engineering](https://openai.com/index/harness-engineering/) 提出短
  `AGENTS.md` 作为目录、结构化仓库知识库、渐进披露、执行计划与机械化约束。
- OpenAI 的 [Codex 介绍](https://openai.com/index/introducing-codex/) 明确 `AGENTS.md`
  应提供导航、测试命令和仓库实践。
- Anthropic 的 [Claude Code 项目记忆](https://code.claude.com/docs/zh-CN/memory)
  建议保持 `CLAUDE.md` 简洁，并在已有 `AGENTS.md` 时通过导入共享单一规则源。
- Anthropic 的 [长时应用开发 harness 设计](https://www.anthropic.com/engineering/harness-design-long-running-apps)
  强调从简单方案开始、用独立验证反馈迭代，并逐项检验脚手架是否仍有价值。
