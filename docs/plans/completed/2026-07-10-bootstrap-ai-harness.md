# 初始化 AI harness 工程

> 状态：completed · Owner：Codex · 创建/完成：2026-07-10

## 目标

让 Codex、Claude Code 通过短入口理解项目，并按任务渐进读取版本化知识；同时把依赖边界、文档
连通性和完成验证变成可执行反馈，使 AI 开发不依赖单次会话记忆。

## 验收结果

- [x] `AGENTS.md` / `CLAUDE.md` 是短入口，共享同一规则源。
- [x] `docs/` 覆盖架构、功能地图、开发规范、工作流、测试、安全、运维、计划与 ADR。
- [x] 明确前端真实、部分实现和仅演示能力，避免 mock 被误判为生产功能。
- [x] harness 检查进入根 Makefile 与 CI，验证链接、入口、必需文档和 Python 依赖边界。
- [x] 后端改用 uv 的 `pyproject.toml` + `uv.lock`，本地、CI、Docker 使用 locked sync。
- [x] 前端补齐 ESLint 与独立 TypeScript 门禁，CI 不再依赖跳过类型错误的 build 制造假阳性。

## 关键决策

- 采用“入口地图 + docs 系统记录 + 渐进披露”，不复制一份巨型提示词给每个工具。
- `CLAUDE.md` 通过官方支持的 `@AGENTS.md` 导入维持单一规则源。
- 只机械执行当前稳定边界：domain/core/infrastructure/application 的禁入依赖；不借初始化任务
  强行把现有实用型分层改造成严格 Clean Architecture。
- uv 迁移保留旧 requirements 的直接依赖精确版本，避免把工具迁移变成隐式依赖升级。

## 验证记录

| 命令/场景 | 结果 |
| --- | --- |
| `make harness-check` | 20 份 Markdown 连通，57 个后端 Python 文件通过边界检查 |
| `make check` | Ruff 通过；pytest 12 passed；ESLint、`tsc --noEmit`、Next build 通过 |
| `uv lock --project backend --check` | 122 个包解析，锁文件与项目元数据一致 |
| `docker compose config --quiet` | 通过 |
| `docker build --check -f backend/Dockerfile backend` | 通过，无警告 |
| 后端实际 Docker build | uv `--locked --no-dev` 安装成功，镜像构建成功 |

## 发现与后续

审计发现前端多项 SOP/写操作仍为本地 mock/timer、repository 用户隔离集成测试不足、发送任务
幂等和可观测性仍需建设。它们已进入[技术债清单](../tech-debt.md)，本次不伪装成已解决。
