# H5 / iOS / Android 密码管理

> 状态：completed · Owner：Codex · 创建：2026-07-15 · 完成：2026-07-15

## 目标与用户价值

为所有已验证邮箱用户提供跨 Web、iOS、Android 一致的密码能力：已有密码用户可在登录态修改密码；
忘记密码或 OAuth 无密码用户可通过邮箱一次性验证码/链接设置新密码。成功后立即吊销旧 JWT，三端以后端
认证状态为准。

## 非目标

- 不新增公开邮箱注册、管理员代改密码、短信验证码或多因素认证。
- 不更换现有 PBKDF2 密码哈希算法和 JWT 认证体系。
- 不在测试或生产中提供 mock 邮件投递，也不自动配置外部 SMTP 服务。

## 背景与当前行为

- 后端只有 `POST /api/v1/auth/password/login`，`users.password_hash` 可为空。
- Web 登录页仅提供 OAuth；iOS/Android 已支持已有密码账户登录。
- 三端均没有修改密码、忘记密码或重置页面，后端没有邮件适配器和重置挑战表。
- 认证入口位于 `backend/app/api/v1/routes/auth.py`，JWT 校验位于 `backend/app/api/deps.py`。

## 验收标准

- [x] 登录用户提供正确旧密码后可修改密码；OAuth 无密码用户必须先完成邮箱验证。
- [x] 忘记密码请求对存在/不存在邮箱返回相同响应，不暴露账户状态。
- [x] 一次性重置链接或验证码短时有效、只可使用一次、错误尝试有上限且数据库只保存摘要。
- [x] 修改或重置成功后旧 JWT 失效，新密码可登录，历史密码不可登录。
- [x] 邮件未配置时功能明确关闭，生产不回退 mock，也不记录令牌、验证码或密码。
- [x] H5、iOS、Android 均有忘记/重置和登录态密码管理入口及 loading/error/success 状态。
- [x] API DTO、三端模型、迁移、部署配置、测试和功能地图保持一致。

## 影响面与风险

- 数据库：`users.auth_version` 与一次性 `password_reset_challenges`；迁移必须可回滚且保留用户。
- API/安全：新公开重置端点、登录态修改端点、限流、邮箱枚举、重放与会话吊销。
- Worker/部署：新增 SMTP 邮件任务和配置；邮件密钥只从服务端 Secret 注入。
- 三端：登录页、账户安全页、API 契约和认证失效后的回登录行为。
- 主要误用路径：暴力猜码、重复使用、旧 JWT 继续操作、OAuth 账户被未验证设置密码、日志泄密。

## 实施步骤

- [x] 完成模型、迁移、认证版本与重置领域/持久化测试。
- [x] 完成 SMTP 适配器、Celery 投递和公开/登录态 API。
- [x] 完成 H5 邮箱登录、忘记/重置和账户安全页面。
- [x] 完成 iOS 原生表单、API 模型、设置入口与测试。
- [x] 完成 Android Compose 表单、API 模型、导航入口与测试。
- [x] 更新配置/部署/文档，运行完整检查并归档计划。

## 进度日志

- 2026-07-15：完成仓库与认证边界审计，从 `release/v2.0.0` 创建
  `features/password-management`；下一步实现后端安全闭环。
- 2026-07-15：后端、三端 UI、SMTP/Celery、迁移、部署配置与文档已实现；本地 `make check`
  通过。等待 CI 执行 PostgreSQL 迁移往返、iOS 和 Android 平台构建。
- 2026-07-15：CI run `29374072798` 六个 job 全部通过；计划归档。

## 发现日志

- Web `/login` 当前仅 OAuth，而移动端已有邮箱密码登录；本任务需先补齐 Web 登录契约。
- OAuth 自动创建用户的 `password_hash` 为空；首次设置密码不能绕过邮箱所有权验证。
- 现有 JWT 没有服务端会话版本，单纯更新 hash 无法吊销既有 token。

## 决策日志

- 2026-07-15：新增用户级 `auth_version` 并写入 JWT；密码变更后递增，兼容已有无版本 token 为版本 0。
- 2026-07-15：重置邮件同时提供高熵链接 token 和便于移动端粘贴的短码；两者只存带服务端密钥的摘要，
  任一路径成功都会消费同一挑战并使其他挑战失效。
- 2026-07-15：使用通用 SMTP + Celery，不绑定单一邮件 SaaS；未配置时 fail closed。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| 后端认证目标测试 | 通过 | `150 passed, 30 skipped`；无本地 PostgreSQL 时仓储集成测试按约定跳过 |
| Alembic upgrade/downgrade/upgrade | 通过 | CI PostgreSQL 16，run `29374072798` |
| `make harness-check` | 通过 | 49 个 Markdown 链接、88 个后端 Python 文件、8 个 harness 单测 |
| `make check` | 通过 | backend、Pi Agent、frontend lint/typecheck/14 tests/build |
| `make ios-check` | 通过 | GitHub macOS 15 / Xcode 16.4 CI，生成工程并完成模拟器测试 |
| `make android-check` | 通过 | GitHub CI，完成 lintDebug、testDebugUnitTest、assembleDebug |

## 回滚与恢复

先关闭 `PASSWORD_RESET_ENABLED` 停止新挑战和邮件；回滚客户端入口不影响既有登录。迁移 downgrade 删除
挑战表和 `auth_version`，但会失去已发生密码变更的旧 JWT 吊销信息，因此生产回滚优先保留 schema、
仅回滚应用镜像。SMTP 凭据可独立撤销或轮换。

## 结果与剩余风险

已交付后端安全闭环、数据库迁移、Celery SMTP 邮件、H5/iOS/Android 忘记与修改密码入口，以及部署和
运维文档。外部 SMTP 尚未配置，因此真实邮件送达率、SPF/DKIM/DMARC 和邮箱端到端仍需按集成文档在
目标环境验证。`PASSWORD_RESET_ENABLED` 后续调整为默认开启，但 SMTP 未配齐时仍 fail closed；已有 OAuth
和密码登录不受影响。
