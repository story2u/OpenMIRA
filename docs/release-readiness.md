# Release Readiness

本文档定义独立 IM 项目的发布就绪检查。现有命令和 artifact 中仍有部分过渡期命名，后续会统一重命名；发布判断以 profile 的检查结果、回滚路径和观测数据为准。Reference comparison 是可选证据，默认不作为独立发布前提。

## 使用方式

```bash
cd go
go run ./cmd/release-readiness -all -format markdown
go run ./cmd/release-readiness -profile <profile> -format markdown
go run ./cmd/release-readiness -profile <profile> -strict
```

## 检查维度

- Route：目标 API 是否挂载。
- Flag：运行开关是否显式配置。
- Env：secret、DSN、token、URL、目录和安全参数是否完整。
- Service：compose 或部署单元是否包含必要 API、Web、worker、Redis、cache 和 provider。
- Fixture：contract、golden、replay 或 smoke fixture 是否覆盖目标链路。
- Observe：metrics、logs、traces、错误分类和告警是否可用。
- Rollback：关闭开关、停止 worker、恢复队列和重放 outbox 的路径是否明确。

## Profile 说明

### session-access

范围：登录、刷新、退出、当前用户、管理员代发 token。

发布前确认：

- `SESSION_JWT_SECRET`、管理员账号和登录限流配置存在。
- passwordless 登录默认关闭，除非产品明确需要。
- 登录失败、token 过期和刷新失败有可观测错误。

### incoming-ingest

范围：通道事件进入 API、队列、worker、消息表、投影和 realtime。

发布前确认：

- API fast-ack 不阻塞外部平台。
- 入站事件有幂等键。
- Connector adapter 失败不影响 core replay。

### task-status

范围：通用任务创建、列表、详情、状态、重试。

发布前确认：

- 任务状态机、幂等、重试和错误分类有测试。
- `task.status` realtime event 可被页面消费。

### workbench-read

范围：客服工作台 bootstrap、summary、会话列表、搜索、消息历史、客户资料。

发布前确认：

- 页面只展示 Go API 返回，不在前端重算筛选口径。
- 查询性能、分页和权限裁剪有观测。

### workbench-actions

范围：已读、AI 开关、补发、撤回、转接、通话和预占释放等写操作。

发布前确认：

- 所有动作写入标准任务或领域事件。
- 外部 provider 失败时有清晰终态。

### realtime-workbench

范围：WebSocket、stream channel、snapshot、gap replay。

发布前确认：

- 断线重连、重复事件、乱序和 gap 检测有测试。
- Redis Pub/Sub 或 Stream 故障可告警。

### admin-observability

范围：审计、系统日志、统计、客户端错误和观测 dashboard。

发布前确认：

- 高风险操作有审计。
- 指标能定位 API、worker、connector、provider 和页面错误。

### admin-accounts

范围：账号、客服用户、联系人缓存和分配关系。

发布前确认：

- 账号变更有权限、审计和事件。
- 联系人缓存刷新不阻塞消息核心链路。

### admin-assignments

范围：分配规则、工作量、claim/release、清理和自动分配。

发布前确认：

- 分配锁、幂等和并发竞争有测试。
- 规则变更可回滚。

### admin-config-content

范围：敏感词、快捷话术、AI 配置、知识库和企业绑定。

发布前确认：

- 配置变更有权限和审计。
- 文件上传目录和对象存储安全可控。

### contact-sync

范围：联系人同步 API 和调度 worker。

发布前确认：

- 同步失败不影响消息收发。
- 外部联系人字段通过 connector adapter 进入本地身份模型。

### connector-events

范围：消息 connector 的外部事件回调。当前 golden fixture 覆盖企微 connector 的好友添加和客户联系回调。

发布前确认：

- 该 profile 只验证 connector adapter，不定义 core 消息模型。
- 回调验签、去重、重放和错误分类完整。

### send-dispatch

范围：发送调度 worker、outbound connector 调用、失败分类和状态回写。

发布前确认：

- Dispatcher 不直接绑定单一 outbound connector。
- 文本、图片、文件、语音等 payload 通过统一 outbound contract。

### platform-proxy

范围：外部业务平台代理读写和 sidebar task。

发布前确认：

- 外部业务平台是 integration provider，可关闭、可降级。
- 平台失败不阻断 IM core。

### ai-outreach

范围：主动触达上下文读取和发送任务创建。

发布前确认：

- 创建的是标准 outbound task 或 automation run。
- AI/provider 失败可重试、可审计。

### archive-pipeline

范围：归档状态、cursor、媒体任务、callback、pull、media run 和事件通知。

发布前确认：

- 归档链路可重放、可补偿。
- 媒体对象引用稳定，下载和上传失败可降级。

### archive-voice-transcription

范围：语音转写重试和 worker。

发布前确认：

- 转写供应商作为 media provider 接入。
- 凭证、限流、失败分类和重试预算明确。

### archive-cold-storage

范围：归档冷存储维护 worker。

发布前确认：

- 冷存储根目录、对象存储配置和恢复路径明确。
- worker-only profile 有独立告警。

### device-ops

范围：设备发现、RTC、控制、屏幕、音频桥接和相关 provider。

发布前确认：

- 设备能力作为 automation provider，不进入 IM core。
- 默认部署可以不启动设备 provider。
- 任何输入控制都需要租户、权限、租约和审计。

### sop-operations

范围：SOP 流程、策略、analytics、dispatch task、媒体上传和测试。

发布前确认：

- SOP 发送走标准 outbound task。
- 媒体和外部平台能力通过 provider 接入。

### admin-diagnostics

范围：设备/账号映射、异常会话、脏联系人、归档状态、outbox 修复和维护 dry-run。

发布前确认：

- 诊断入口默认只读。
- 写修复操作需要权限、审计、dry-run 和回滚说明。
