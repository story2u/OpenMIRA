# 商机语义识别增强

> 状态：completed · Owner：Codex · 创建：2026-07-12 · 更新：2026-07-12

## 目标与用户价值

在不扩大真实 IM 发送权限、不引入新的模型服务或数据库迁移的前提下，提升 Telegram 与企业微信
消息中隐性商机的召回能力：启用 AI 时，不再只分析命中关键词灰区的消息，而是结合最近对话、来源
信息和管理员配置的 AI hint 进行语义复核；纯 AI 新发现的商机始终进入人工审核。

本计划交付开源方案调研、可回归验证的首个纵向切片，以及后续积累标签后切换到轻量开源分类器的
演进边界。

## 非目标

- 本次不训练或随应用镜像分发 SetFit、GLiClass、Sentence Transformer 模型。
- 本次不部署 Argilla、Cleanlab、Snorkel 或独立向量数据库。
- 本次不新增商机标注、转化归因、租户业务画像 UI；这些需要独立的数据模型与产品设计。
- 不改变 pi Agent 的链接核验、联系方式提取和套餐额度逻辑。
- 不允许模型新发现的商机直接触发非工作时间自动回复。

## 背景与当前行为

- 主检测入口为 `backend/app/domain/services/detection_policy.py`。规则分数达到 `0.75` 时直接判定；
  分数低于 `0.35` 时不会调用 AI，因此没有已有关键词的语义商机可能漏判。
- `backend/app/application/use_cases/ingest_message.py` 只把当前消息文本传给 detector；数据库已经提供
  `MessageRepository.list_by_conversation`，但检测链路未使用它。
- `backend/app/infrastructure/ai/litellm_client.py` 的分类提示词只有通用 B2B 定义，没有来源、对话、
  正反边界或 AI hint 示例。
- pi Agent 能高置信补判，但它是异步、按套餐计量的后处理，并且不应成为首轮识别的唯一召回层。

## 开源方案调研

| 项目 | 可借鉴能力 | 适配判断 |
| --- | --- | --- |
| [Semantic Router](https://github.com/aurelio-labs/semantic-router) | 用 utterance 示例、向量相似度和可优化阈值做低延迟语义路由；MIT | 架构最贴近当前需求；本次借鉴“候选召回而非最终裁决”，待有标注样本和 embedding 运行条件后再评估引入 |
| [SetFit](https://github.com/huggingface/setfit) | 基于 Sentence Transformer 的少样本分类，多语言、推理成本低；Apache-2.0 | 适合作为积累高质量正负样本后的主分类器，不适合在零金标阶段直接上线 |
| [GLiClass](https://github.com/Knowledgator/GLiClass) | 本地零样本、多标签轻量分类，单次前向支持动态标签；Apache-2.0 | 可用于未来无外部 API 的候选生成；当前需新增模型托管、下载和容量基线，先不进入生产依赖 |
| [Snorkel](https://github.com/snorkel-team/snorkel) | 融合关键词、正则、模型等弱监督 labeling functions；Apache-2.0 | 适合离线生成训练标签，不适合作为当前在线请求链路依赖；上游开源版最新 release 为 2024，维护重心已转移 |
| [Argilla](https://github.com/argilla-io/argilla) | 人工标注、语义搜索和持续评估；Apache-2.0 | 产品能力匹配反馈闭环，但官方已声明进入稳定维护、停止新增功能；本次先复用现有审核界面，不部署新服务 |
| [Cleanlab](https://github.com/cleanlab/cleanlab) | 标签错误、离群样本和主动学习样本选择；AGPL-3.0 | 适合离线数据质量任务；许可证和独立 ML 工作流需要单独审查，不进入应用运行时 |
| [BERTopic](https://github.com/MaartenGr/BERTopic) | 对未识别消息聚类，发现新商机表达和未知意图；MIT | 适合周期性离线漏判分析，不负责在线二分类 |

### 选型结论

采用渐进式混合架构：高精度规则保留；启用 AI 时对规则未确定的所有非空消息进行语义复核；复核输入
包含有限的同会话历史、来源元数据和启用的 AI hint；AI 新发现只进入人工审核。该切片不增加重型
ML 依赖，可复用现有 LiteLLM 配置，并为后续 Semantic Router/SetFit 适配保留领域端口。

后续拥有分层金标数据后，按以下顺序演进：

1. 从人工确认、误报原因和抽样漏判构建验证集，按来源统计 precision/recall，而非只看 accuracy。
2. 用 embedding/Semantic Router 风格的语义层做高召回候选生成，LLM 只复核候选和不确定样本。
3. 标签达到可训练规模后对比 SetFit、GLiClass 与现有 LLM，按召回、人工审核量、延迟和成本选型。
4. 用 Snorkel/Cleanlab 思路离线融合弱标签和发现标签问题，不让训练工具进入在线事务路径。

## 验收标准

- [x] AI 关闭时保持现有规则检测行为，不产生外部模型调用或静默 mock。
- [x] AI 开启时，规则分低于 `0.35` 的非空消息也会进入语义分类器。
- [x] 分类器输入包含当前消息、有限的同会话历史、来源类型、群名和启用的 AI hint。
- [x] 历史消息有数量和字符上限，不传 raw webhook、token、session 或其他秘密。
- [x] AI 输出非法、越界或 provider 失败时 fail closed 到现有规则结果，不因分类失败中断消息落库。
- [x] 纯 AI 新发现的商机一律为 `PENDING_HUMAN`，不得进入 `AI_AUTO_REPLY`。
- [x] 高分规则直通、AI 补判、AI 拒绝、AI 关闭和上下文截断均有自动化测试。
- [x] 架构总览、功能地图、运行配置与实际代码一致。

## 影响面与风险

- **domain**：扩展 AI 分类端口的请求上下文；规则仍是纯逻辑，不依赖 provider。
- **application**：摄取时读取有限会话历史；AI 新发现强制人工审核。
- **infrastructure**：集中分类提示词并验证模型 JSON；provider 错误回退规则结果。
- **API/worker**：沿用现有 detector 组合根；无新端点、队列或权限。
- **数据库/migration**：无结构变化。
- **前端**：无契约和 UI 变化。
- **部署**：无新依赖；新增可调的对话条数/字符上限时同步环境变量文档。
- **安全**：消息文本仍是不可信数据；只传规范化、截断字段；模型判断不能扩大外部动作权限。
- **成本**：`AI_ENABLED=true` 时分类调用覆盖面会增大；通过上下文上限、规则高分直通和现有 AI 总开关
  控制，后续以语义候选层降低调用量。

## 实施步骤

- [x] 增加检测上下文 DTO、AI 分类请求和纯 AI 结果的人工审核标记。
- [x] 摄取用例加载并截断最近对话，排除当前消息重复内容。
- [x] 重写分类提示词与严格结果校验，把 AI hint 作为领域示例传入。
- [x] 调整 detector：高分规则直通，其余非空消息均可 AI 复核，失败回退规则结果。
- [x] 增加领域、分类适配器和摄取编排测试。
- [x] 更新架构、功能、运行文档并完成相关检查。

## 进度日志

- 2026-07-12：从 `release/v2.0.0` 创建 `features/opportunity-semantic-detection`；完成开源方案初筛；下一步实现检测契约与测试。
- 2026-07-12：完成领域端口、上下文摄取、严格模型输出、安全路由和文档；项目级检查通过，计划归档。

## 发现日志

- 现有 `AI_HINT` 在 detector 中与关键词完全同义，没有向模型提供 hint 的语义描述。
- `MessageRepository.list_by_conversation` 已提供按时间返回的有限历史，可复用，无需新增查询接口。
- 当前 `AI_ENABLED=false` 时 classifier 已做规则回退，因此扩大全量语义复核不会改变默认安全配置。
- 原会话历史查询未按 owner 过滤；为避免同一 Telegram chat 被多个 owner 监听时交叉取数，本次将
  检测和回复历史查询统一收紧为显式 `owner_user_id` 条件。

## 决策日志

- 2026-07-12：首个切片不增加开源 ML 运行时依赖；原因是缺少金标集、模型制品管理和部署容量证据；替代方案为复用现有 LiteLLM 并借鉴 Semantic Router 的分层职责。
- 2026-07-12：纯 AI 补判强制人工审核；原因是扩大召回后不能同时扩大自动发送风险。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| 目标测试 `test_detection_policy.py`、`test_opportunity_classifier.py`、`test_ingest_semantic_detection.py` | 通过 | 12 passed |
| `make harness-check` | 通过 | 38 个 Markdown 链接、71 个后端 Python 文件；7 个 harness 单测通过 |
| `make backend-check` | 通过 | 77 passed、7 skipped；1 个 FastAPI/Starlette 依赖弃用告警 |
| `make pi-agent-check` | 通过 | Node syntax check 与 4 个 runtime 测试通过；npm audit 0 vulnerabilities |
| `make frontend-check` | 通过 | ESLint、TypeScript、Next.js production build 通过 |
| 辅助目标 mypy | 未通过（非项目 gate） | 触发 `db/models.py` 与 `repositories.py` 既有 SQLModel/SQLAlchemy 类型基线，共 39 项；本次 CI 规则、compile 和测试均通过 |

## 回滚与恢复

本次无迁移和外部写操作。回滚代码即可恢复原检测门槛与单消息输入；关闭 `AI_ENABLED` 可立即停止
模型分类调用并保留规则识别。无须回填或删除业务数据。

## 结果与剩余风险

已交付上下文感知的混合商机检测：保留高置信规则直通，AI 开启时语义复核其余非空消息，使用 owner
隔离、数量和字符均受限的最近会话与 AI hint；模型输出严格校验，失败回退规则；模型补判只进人工
审核。没有新增依赖、迁移、API 或真实发送权限。

剩余风险：尚无生产金标集，无法在本分支声称 precision/recall 已提升；`AI_ENABLED=true` 的调用量
会增加。下一阶段应先增加明确的商机确认/误报原因和非商机抽样标注，再离线比较 Semantic Router
风格 embedding 召回、SetFit 与 GLiClass，达到成本和质量门槛后替换全量 LLM 复核。
