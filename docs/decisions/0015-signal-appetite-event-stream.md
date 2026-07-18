# ADR-0015：信息胃口采用端侧事件日志与独立同步流

> 状态：accepted · 日期：2026-07-18

## 背景

ADR-0014 规定现有 Message、Opportunity 和设置继续以 PostgreSQL 为权威，RN SQLite 是可重建投影。
信息胃口是新领域：教学必须离线、可撤销、多设备可合并，直接覆盖一份偏好 JSON 会丢失并发教学、
版本因果和审计历史；把所有既有业务立即迁移到事件溯源又超出本功能范围。

## 决策

- 只对信息胃口新增 append-only `attention_events`；客户端在同一排他事务中追加事件并更新物化投影。
- capture 与 apply 分离。样本、原因、候选、试跑、Shadow、临时关注和 schedule 都是版本化事件；只有
  明确批准的 `PreferenceApplied` 改变 active preference，回滚追加 `PreferenceReverted`。
- 服务端增加独立 `signal_appetite_events` 流和 cursor API，不复用 projection `SyncChange`。事件按 owner
  与已认证 device 绑定、禁止正文键、限制 64 KiB；event ID 内容相同才幂等，冲突返回 409。
- SQLite 是该新增领域的离线执行真相；服务端保存多设备交换与审计副本。既有消息/商机权威不改变，
  决策投影只引用本地同步消息 ID，不把正文写入偏好事件。
- capability 与 `SIGNAL_APPETITE_SYNC_ENABLED` 独立且默认关闭。旧客户端忽略未知能力；关闭同步不影响
  单设备教学、L0/L1 过滤和回滚。

## 备选方案

- **单一 preference JSON last-write-wins**：简单，但会覆盖并发样本，无法证明 apply/rollback 因果。
- **复用 SyncChange projection feed**：适合服务端权威读模型，不适合客户端先产生的领域意图和双向 append。
- **全面切换事件溯源**：长期可能统一，但会推翻 ADR-0014 并扩大消息、商机和发送路径的迁移风险。
- **通用 CRDT**：可合并任意字段，但复杂度高，且不能天然表达必须审批的版本状态机。

## 后果

得到离线教学、审计、幂等、多设备和显式回滚；代价是维护独立 cursor、事件 schema/fold 与投影修复。
服务端没有完整消息正文，不能独立重放过滤决定；这是本地优先与内容最小化的有意边界。

## 验证与复审

纯 fold、真实文件 SQLite、owner 清理、重复/冲突 event、多设备 push/pull、cursor 续传、API owner/device
隔离和 Alembic migration 均有自动化覆盖。真机跨设备、长时间离线并发与存储增长完成前保持 capability
关闭。若事件 schema 演进频繁、冲突率或投影修复率超阈值，复审版本仲裁与 compaction；不得改成静默覆盖。
