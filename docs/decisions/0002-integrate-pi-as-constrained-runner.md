# ADR-0002：以受限子进程集成 pi Agent

> 状态：accepted · 日期：2026-07-10

## 背景

系统需要在 IM 消息摄取后继续检查链接、补判商机、提取联系方式，并判断是否应发邮件、加好友、
主动私信或提醒当前系统用户。现有 Python 后端已有 FastAPI、Celery、数据库与 IM 发送边界，AI
能力基于 LiteLLM，但没有可扩展的多步骤 Agent 运行时。

pi 的 `@earendil-works/pi-agent-core` 提供有状态 Agent、结构化工具、工具调用拦截和事件流。pi
运行时要求 Node.js 22.19+，而当前业务后端是 Python；同时消息和网页内容均是不可信输入，不能
让 Agent 获得 shell、文件系统、秘密或不受控外发权限。

## 决策

- 在后端镜像中增加一个轻量 Node.js runner，固定使用
  `@earendil-works/pi-agent-core` / `@earendil-works/pi-ai` 的审查版本。
- Python Celery worker 以一次性子进程调用 runner，通过 stdin/stdout 交换单个 JSON 对象；每次
  消息分析使用无持久会话，设置硬超时和输出大小上限。
- pi Agent 不加载 coding-agent、项目 context、skills 或内置工具，只注册一个
  `submit_analysis` 工具。该工具用 TypeBox 约束最终分析结构，调用后立即结束本次 Agent loop。
- URL 获取在 Python 基础设施适配器中完成。仅允许 HTTP(S) 公网目标，逐跳验证重定向，拒绝
  loopback、私网、link-local、保留地址和 URL 凭据，并限制数量、响应大小和文本长度。
- pi 输出视为不可信：Python 使用 Pydantic 再次验证、截断并执行确定性策略；Agent 只能提出
  邮件、好友申请和私信建议，不能直接执行这些外部动作。
- 重大商机提醒是内部投影，可自动写入并展示；任何外部发送仍受人工批准以及现有
  `IM_SEND_ENABLED` 安全阀约束。
- `PI_AGENT_ENABLED` 默认关闭。模型 provider、model、超时、URL 数量/大小和补判阈值均由环境
  配置，秘密只通过环境传给子进程且不得进入日志或持久化结果。

## 备选方案

- **在 Python 中继续扩展 LiteLLM/LangGraph**：运行时更统一，但没有实际集成 pi，且会形成第二套
  Agent 工具/拦截约定。
- **独立常驻 Node 微服务**：隔离更强、吞吐更高，但增加服务发现、鉴权、部署与健康检查；当前
  流量和一次一消息的任务模型不足以证明这项复杂度。
- **直接启动 pi coding-agent CLI/RPC**：跨语言接入简单，但默认能力面向代码仓库，资源加载和
  工具面过宽；即使关闭工具，也不如专用 runner 的输入输出契约清晰。
- **允许 Agent 直接抓网页和发送动作**：步骤更自主，但会把 SSRF、提示词注入、重复发送和越权
  风险放进模型工具循环，不满足现有安全基线。

## 后果

正面影响：复用 pi 的 Agent loop 和工具契约；业务动作仍留在现有 Python 分层；每次运行隔离、
可超时、可重试；默认不会代表用户联系外部人员。

成本与约束：后端镜像增加 Node 运行时和 npm 锁文件；每条消息启动子进程有固定开销；Node/Python
之间需要维护双重 schema；DNS 预解析不能完全消除 rebinding，生产环境应配合受控 egress/proxy。

## 验证与复审

- Node runner 使用 fake provider 测试只允许结构化提交，不调用真实模型。
- Python 测试覆盖私网 URL、重定向、超限响应、无效 Agent 输出、任务重试与重复消息。
- 迁移需验证 upgrade/head；Compose 和 Docker build 必须证明 Node runner 被生产 worker 包含。
- 当分析吞吐使子进程开销显著、需要长期会话，或要开放自动外部动作时复审；后两项必须新增 ADR。
