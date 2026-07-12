# Telegram 普通账号 QR 连接

> 状态：active · Owner：Codex · 创建：2026-07-12 · 更新：2026-07-12

## 目标与用户价值

让登录用户通过 Telegram QR 登录自己的普通账号，并选择自己已加入的群组/频道开始只读监听。网页只显示
QR 码、连接和来源状态，不采集或展示用户的 API Hash、手机号、验证码或 Session。

## 非目标

- 不使用普通账号向外发送消息、加好友或发私信。
- 不迁移、解密或删除旧 `TelegramUserConfig` / `TelegramMonitor` 的兼容数据。
- 不把平台 `TELEGRAM_MTPROTO_API_HASH` 返回给 API、前端、日志或任务异常。

## 背景与当前行为

P2 卡片和 `/connect/mtproto-qr` 当前安全拒绝；即使环境开关为 true，能力状态也被固定为 false，因为尚未
存在 QR 生命周期 worker。系统已有 Telethon 依赖与旧 MTProto listener，但新连接表的凭据槽尚未用于 QR。

## 验收标准

- [x] 开启平台全局凭据和 worker 后，已登录用户可创建 QR 登录尝试并轮询二维码/状态；未开启时明确说明缺失能力。
- [x] QR 成功后仅以应用加密格式保存 session；所有响应、日志、页面和错误均不含 API Hash、手机号、验证码或 session。
- [x] 独立常驻 worker 完成 QR 生命周期和普通账号消息监听，消息按 owner/来源/订阅额度摄取且幂等。
- [x] 用户只能查看、取消或删除自己的 QR 尝试、连接和群组来源；过期/失败路径可恢复。
- [x] 新前端使用真实 API 显示二维码、加载/失败/过期状态及可监听来源，不存在旧凭据表单或相关提示。
- [x] 同一用户最多保留一个 pending QR 尝试；重复或并发请求复用该尝试，不会无界创建 Telethon 连接。
- [x] legacy 配置 API 在数据迁移和下线方案完成前保持兼容可用。

## 影响面与风险

涉及加密 session 的 schema/迁移、Telethon 外部 API、API/worker/Redis、Next.js 交互、Docker/生产配置与
权限隔离。普通账号 session 属于高价值秘密：worker 只能解密使用，API 不得返回；登录与监听需要限速、TTL、
断线重试和审计。Telegram QR 登录可能被用户取消或需要迁移 DC，必须 fail closed 并可重试。

## 实施步骤

- [x] 审计旧 Telethon listener、加密字段和新 Telegram connection/attempt 模型；确定 QR session/来源数据契约。
- [x] 新增 QR 尝试持久化字段、加密 session 仓储与 Alembic 迁移；owner/过期/秘密泄露场景待生产隔离验证。
- [x] 实现 Telethon QR adapter、应用用例、API 轮询与取消接口；接入专用 worker。
- [x] 实现普通账号来源发现/选择与只读监听，复用既有摄取/套餐额度逻辑。
- [x] 重写 P2 前端为真实 QR 与连接状态，删除任何用户凭据采集/提示。
- [x] 更新 Compose、部署、文档/ADR/功能地图；生产隔离环境冒烟待执行。

## 进度日志

- 2026-07-12：开始。确认 P2 当前被 API 硬编码关闭，不能由环境开关单独启用。
- 2026-07-12：实现平台凭据 QR worker、只读 listener 与用户来源选择；删除旧用户凭据采集路由和前端客户端。
- 2026-07-12：review 修复恢复 legacy API；新增 pending QR 数据库唯一约束、API 复用与部署迁移停机保护。
- 2026-07-12：CI 发现 ORM enum 按成员名存储；新增 `202607120003` 修复 partial index 条件，不改写已推送迁移。

## 发现日志

- Telethon QR URL 是短期登录 bearer grant；它不能进入未加密 JSON metadata，也不能交给第三方二维码服务生成图片。

## 决策日志

- 2026-07-12：使用平台统一 API ID/API Hash 和二维码登录，而非用户手填凭据；减少秘密暴露面并使 worker 生命周期可控。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| `make check` | 通过 | harness、后端 46 passed / 5 skipped、pi runtime、前端 lint/typecheck/build 通过 |
| `make harness-check && make backend-check`（review 修复后） | 通过 | harness 7 tests；后端 59 passed / 6 PostgreSQL tests skipped |
| `make pi-agent-check` 与前端 lint/typecheck/build | 通过 | pi runtime 4 tests；Next.js production build 成功 |
| Alembic head | 通过 | `202607120002 (head)`；本机无 Docker，未实际连接 PostgreSQL 执行迁移 |
| Compose config（本地/生产） | 通过 | 两个新增 worker 服务均可渲染 |
| QR 尝试/API/worker 真实 Telegram 冒烟 | 未运行 | 需要平台 API 凭据与隔离 Telegram 普通账号 |
| QR 回归测试 | 通过 | 覆盖重复请求复用、worker 未启用拒绝、QR grant/session 加密持久化与 owner-scoped repository |

## 回滚与恢复

可关闭 QR worker 开关并停止其服务，现有连接不再刷新或摄取；加密 session 保留以便恢复。若凭据泄漏，删除
对应连接/session 并要求用户重新扫码，不在日志或响应中暴露其内容。

## 结果与剩余风险

本地实现与完整静态/测试检查已完成；保留 active 状态，直到隔离 Telegram 账号完成 QR、选群、监听、
重启恢复和套餐限额的真实验证。
