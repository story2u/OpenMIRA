# 工作机会发现架构

> 状态：当前实现 · 最后核验：2026-07-16

工作机会发现是通用商机链路上的垂直投影，不是第二套消息摄取或 AI Runtime。它只处理当前用户
已经授权的 Telegram/企业微信来源，以原始 `Message` 为证据，模型结果不能直接执行投递、联系
招聘方或改变用户权益。

## 处理流水线

```text
授权消息
  -> 来源职能画像（7 天缓存，人工覆盖优先）
  -> 确定性预筛（关键词、来源先验、风险和重复信号）
  -> 现有 Celery + UsageLedger 幂等预留
  -> 受限 pi Agent 分类与证据约束提取
  -> Pydantic 校验和字段证据裁剪
  -> 精确指纹 + 结构化特征相似度去重
  -> 确定性档案匹配
  -> JobOpportunityDetail/Source/Match 投影
```

普通消息摄取不等待 Agent。只有预筛候选进入已有 `agent.analyze_message` 任务，同一消息沿用现有
用量幂等键，不重复消耗月额度。Agent 失败不阻塞消息保存，也不会写入不完整的正式职位。

## 来源画像

`SourceFunctionalProfile` 以 `owner_user_id + channel + external_source_id` 唯一，输入限制为来源名称、
可选描述/username 和最多 20 条脱敏、截断样本。规则画像提供低成本初值；显式重算通过
`profileSourceFunction` 工具调用现有 pi Agent。生成结果保存先验、噪音、可靠性、置信度与短证据，
默认 7 天过期。`manual_override` 始终优先，模型重算不会覆盖它。

群名只是一个信号。没有描述或样本时允许生成低置信画像；Telegram/企微适配器未提供描述时保持
`null`，禁止模型补写。来源画像只能调整进入 Agent 的阈值，不能直接把消息判为招聘。

## 分类与提取边界

正式职位仅接受 `job_post` 和 `job_repost`。候选人自荐、求职请求、课程/培训广告、普通讨论、
spam 和 scam 只写最小分类审计，不生成职位。Agent 只能调用固定 structured-output 工具，输出经
Node TypeBox 和 Python Pydantic 双重校验。

重要字段必须有当前消息或有限授权上下文中的短证据；无证据值降为 `null`/`unknown`。`posted_at`
固定来自平台 `Message.sent_at`。`source_message_url` 与 `application_url` 分开保存，私有来源没有稳定
链接时前者保持空值。

显式年龄、性别等限制只保存原文透明度和 compliance flag。系统不采集用户受保护属性，也不把这些
限制加入档案、资格判断或排序。

## 去重与匹配

第一层使用申请链接、规范化公司/岗位/地点和内容指纹。第二层使用本地、确定性的稀疏结构化特征
向量计算余弦相似度，并结合 45 天窗口、公司、城市和 seniority 冲突门禁；这不是外部神经 embedding，
不会增加模型调用或泄漏消息。相似来源保留为 `JobOpportunitySource`，主记录不删除，冲突字段显式
标记。后续接入外部 embedding provider 必须另行做隐私、成本与回退评审。

`JobSearchProfile` 只接收用户主动声明的职业因素。匹配权重为角色 25、技能 25、资历 10、地点/
工作模式/时区 15、薪资 10、雇佣类型 5、语言 5、签证 5。硬约束可以产生 `not_eligible`；缺失字段
进入 `unknown_constraints`，不能当作不匹配。Agent 可解释文本，但不能写入或覆盖最终分数。

## API 与客户端

- 档案：`/api/v1/job-search-profiles` CRUD 与 `/parse` 预览，解析结果必须由用户确认后保存。
- 职位：`GET /api/v1/jobs`、`GET /jobs/{id}`、`POST /jobs/{id}/feedback`。
- 误判审计：`GET /api/v1/job-message-audits`、
  `PATCH /job-message-audits/{id}/correction`。接口只返回 owner 自有消息的有限摘要；人工纠正
  进入评估审计，不绕过 UsageLedger 自动调用模型或创建职位。
- 来源画像：`GET/PATCH .../functional-profile` 与 `POST .../recompute`。
- Web：`/jobs`、`/jobs/[id]`、`/settings/job-search`。
- iOS/Android：工作机会一级入口、列表、详情、档案和筛选；都通过各自唯一 API boundary。

所有查询从 JWT 当前用户派生 owner，不接受客户端提交任意 user ID。推送通道当前尚未交付；
`notification_enabled` 仅保存偏好，不能描述为已发送职位通知。

## 观测、评估与限制

每次 Runtime 调用只记录 task、provider、model、prompt version、延迟、token 汇总和结果状态，
不记录 prompt、完整输出、凭据或 Telegram Session。离线虚构数据位于 `evals/job-discovery/`；
`make job-discovery-eval` 只验证规则路由、确定性匹配和评估 harness。真实 Pi Agent 提取、真实群噪音、
多语言表现与生产阈值必须用经授权脱敏的金标样本另行验证。
