# ADR-0014：先用服务端变更日志同步，不立即切换事件溯源权威

> 状态：accepted · 日期：2026-07-17

## 背景

目标架构希望设备持有可离线使用的数据，并最终把正文真相移到端上。当前系统的事实来源是 PostgreSQL
业务表，repository 多处自行 commit；消息摄取、商机创建、Agent 投影和回复经常跨多个提交完成。若直接
把这些操作宣称为一条原子领域事件流，会产生半完成事件、双权威冲突和难以回滚的数据迁移。

当前最紧迫的用户价值是推送触发的增量同步与离线读取，不要求先重写全部领域模型。

## 决策

- P0-P5 期间 PostgreSQL 的 Message、Opportunity、用户设置、订阅和审计模型保持业务权威；设备
  SQLite 是可删除、可从服务端 bootstrap 重建的副本。
- 服务端新增按用户隔离的 append-only `SyncChange`：包含单调 cursor、event id、aggregate type/id/
  version、operation、schema version、createdAt，以及有界 payload 或 tombstone。
- 第一版 change 表达 aggregate projection 的 upsert/delete，而不是完整领域意图。对现有跨多 commit
  用例允许客户端观察到可恢复的中间态，最终以较高 aggregate version 幂等覆盖。
- 业务写与对应 change 必须在同一个数据库事务内提交。若某个 repository 无法满足，先为该纵向切片
  收敛事务或暂不开放同步；不得 post-commit 尽力补写并声称可靠。
- 同步 API 提供有界 bootstrap、游标分页、gap/过期 cursor reset、schema 兼容错误和 owner 隔离；
  推送只携带资源 ID/游标提示，丢推送后仍可补拉。
- 客户端在一个 SQLite 事务中保存 change inbox、更新物化表并推进 cursor；重复、乱序、kill/reopen
  必须安全。数据库损坏或 schema 不兼容时清除业务副本并重新 bootstrap。
- 设备离线写通过 command outbox 返回服务端。命令携带 idempotency key、base aggregate version 和
  expiry；409 冲突进入用户可见处理，不使用 last-write-wins 静默覆盖。外部发送默认不做离线自动重放。
- 只有 P3-P5 的同步、冲突、恢复和多设备指标稳定后，才能新写 ADR 决定是否把端上事件日志提升为权威。
  E2EE 信封、设备密钥和服务端正文删除也必须独立决策。

## 备选方案

- **立即全面事件溯源**：最终形态统一，但需同时重写事务、领域聚合、API、移动存储与迁移，失败面过大。
- **继续 REST 全量轮询**：实现简单，不能支持可靠推送、离线副本或端上任务协调。
- **通用 CRDT**：能合并任意并发写，却会绕过现有商机状态机和外发审批，不适合本项目的受控命令模型。
- **数据库 CDC 直接暴露客户端**：可捕获所有行变化，但泄露物理 schema，难以做 owner/契约版本和
  业务 tombstone，不作为公共同步协议。

## 后果

第一阶段同步是可回滚的读模型复制，不要求立即完成不可逆的数据权威迁移；旧客户端和 Web 继续访问
现有 API。代价是服务端在一段时间内同时维护业务表与 change feed，必须严防漏写、游标竞态、保留期
和大 payload，并为同步发出的 repository 写增加事务测试。

## 验证与复审

- PostgreSQL 集成测试覆盖并发 cursor、owner 隔离、分页、重放、乱序、tombstone、gap/reset 和事务回滚。
- RN 测试覆盖 bootstrap、幂等 apply、kill/reopen、账号切换清库、数据库重建与 outbox 冲突。
- 当 change 漏写率、同步 gap、恢复失败或存储增长超过发布阈值时关闭 `syncAvailable`，客户端回到 REST。
- 只有至少两个 App 版本稳定使用同步，并完成跨平台设备/密钥设计，才复审端上事件权威与 E2EE。
