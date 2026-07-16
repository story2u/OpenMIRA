# 工作机会发现垂直模式

> 状态：active · Owner：Codex · 创建：2026-07-16 · 更新：2026-07-16

## 目标与用户价值

在现有消息摄取、用量账本和受限 pi Agent Runtime 上增加工作机会发现模式。系统从用户已授权
的 Telegram/企业微信来源中识别真实招聘信息，保存可追溯职位字段，并按用户主动声明的职业
偏好做确定性匹配，在 Web、iOS、Android 提供一致的列表和详情体验。

## 非目标

- 不自动投递、联系招聘方、接受 Offer 或执行其他外部动作。
- 不为雇主筛选候选人，不收集或推断受保护属性。
- 不承诺未经真实生产样本验证的识别准确率。
- 不建立第二套 Agent Runtime、用量账本或链接检查器。

## 背景与当前行为

基线为 `release/v2.0.0@e65c9ca`。`Message` 已保存 owner、来源、作者、平台时间和外部消息
标识，`Opportunity` 仅表达通用商机。摄取入口位于
`backend/app/application/use_cases/ingest_message.py`，pi 后处理位于 `analyze_message.py` 和
`backend/pi-agent-runtime/`。三端已有商机列表、详情、认证和统一 API 边界。

## 验收标准

- [x] 来源职能画像按 owner 隔离、可缓存、可人工覆盖。
- [x] 预筛、招聘分类和结构化提取不阻塞普通消息摄取，且重复消息不重复计费。
- [x] `job_post`/`job_repost` 才创建职位，缺失字段保持 null/unknown，重要字段保留证据。
- [x] 旧商机默认 `business`；职位详情、档案、匹配、反馈均持久化且迁移可回滚。
- [x] 匹配分由确定性规则计算，年龄/性别等限制只产生合规提示。
- [x] Web/iOS/Android 提供工作机会列表、详情和档案入口。
- [x] 虚构 eval 数据和脚本可运行，结果不冒充真实生产准确率。
- [x] `make check` 及可用平台检查通过，功能地图和架构文档与代码一致；iOS 由 macOS CI 验证。

## 影响面与风险

- domain：新增职位、来源、分类和匹配枚举及纯策略。
- infrastructure：新增 SQLModel、Alembic、repositories；扩展受限 Node runner schema。
- application/worker：增加异步职位发现用例，复用当前 message Agent 额度和任务队列。
- API：增加 `/jobs`、`/job-search-profiles` 和来源画像端点，所有资源强制 owner 隔离。
- clients：三端 DTO、导航、列表、详情和档案编辑。
- 安全：就业高影响边界、证据约束、原始消息最小展示、禁止受保护属性参与评分。

## 实施步骤

- [x] 领域枚举、模型、迁移和迁移测试。
- [x] 来源画像、规则预筛、Agent 分类提取和异步任务。
- [x] 去重、确定性匹配、档案与 repositories。
- [x] owner 隔离 API 和 DTO。
- [x] Web、iOS、Android 客户端。
- [x] eval、测试、文档和完整验证。

## 进度日志

- 2026-07-16：完成 release 审计并创建功能分支；下一步建立领域模型和迁移。
- 2026-07-16：完成七张工作机会相关表和 Opportunity 类型字段迁移、来源画像、Agent 结构化提取、去重匹配和 owner 隔离 API。
- 2026-07-16：补充 owner 隔离的过滤消息审计与人工纠正 API；只暴露有限摘要，纠正不绕过模型额度和正式职位提取链路。
- 2026-07-16：完成 Web、SwiftUI、Compose 的列表、详情与档案链路；移动端继续复用各自唯一网络边界。
- 2026-07-16：增加虚构 eval、Runtime token 汇总与 CI 入口；真实群与真实模型金标评估仍待外部验证。

## 发现日志

- 当前消息已有真实 `sent_at`，职位 `posted_at` 可直接使用，不需要模型猜测。
- 当前 Telegram/企微统一消息只有群名，没有稳定群描述字段；画像模型需允许 description 为空。
- 推送通道仍未交付，本次只保存通知偏好和匹配，不声称已推送职位。

## 决策日志

- 2026-07-16：复用现有 message 级 pi Agent 调用和 UsageLedger；避免一次消息重复模型计费。
- 2026-07-16：匹配是纯领域服务；模型只解析职位/偏好并生成解释，不能写分数或决定资格。
- 2026-07-16：第二层去重采用本地稀疏结构化特征余弦相似度，并以公司/城市/资历冲突门禁防止误并；
  它不是神经 embedding，不调用外部 provider。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make backend-check` | 通过 | 192 passed，32 skipped；PostgreSQL 门控集成测试由 CI 运行 |
| `make pi-agent-check` | 通过 | Node 语法检查，8 tests passed |
| `make frontend-check` | 通过 | ESLint、typecheck、18 tests、Next production build |
| `make job-discovery-eval` | 通过 | 虚构基线：画像 5/5、预筛 recall 1.0、匹配 5/5；真实 Agent 提取标记 not_run |
| `make ios-check` | 本地未运行 | 当前主机为 Linux；使用 CI 的 macOS/Xcode job 验证 |
| `ANDROID_HOME=$HOME/.android-sdk make android-check` | 通过 | lintDebug、testDebugUnitTest、assembleDebug |
| Alembic offline upgrade/downgrade | 通过 | head upgrade SQL 与 `202607160001:202607150002` downgrade SQL 均可生成；本机无 Docker/PostgreSQL |
| `make check` | 通过 | harness、backend、pi、eval、frontend 全部通过 |

## 回滚与恢复

先关闭职位发现任务入口，再部署前一版本；执行新迁移 downgrade 会删除职位投影、档案、匹配和
画像表，并移除 `opportunities.opportunity_type`。原始 `messages`、通用商机和 IM 连接不受影响。
若生产已产生用户档案，downgrade 前必须导出相关表，避免不可逆数据丢失。

## 结果与剩余风险

代码路径、三端契约和虚构回归夹具已完成。真实 Telegram/企业微信脱敏金标评估、真实模型提取质量、
移动端真机 E2E 和推送投递仍未验证；当前不得据此宣称生产招聘识别准确率。通知偏好已保存，但仓库
尚无 APNs/FCM 推送通道。
