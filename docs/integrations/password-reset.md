# 密码修改与邮箱重置

> 状态：代码已实现，外部 SMTP 待环境配置 · 最后核验：2026-07-15

## 能力边界

- H5、iOS、Android 均可使用邮箱密码登录，并从登录页申请忘记密码重置。
- 已有密码的登录用户可在“账户安全”提供当前密码后修改。
- OAuth 自动创建的无密码账户不能直接在登录态设置密码，必须先通过登录邮箱验证码或邮件链接验证。
- 邮件提供 10 位一次性验证码和高熵 H5 链接；两者对应同一挑战，任一成功后另一种方式立即失效。
- 修改或重置成功会递增 `users.auth_version`，此前签发给所有设备的 JWT 随即失效。

公开 API：

- `POST /api/v1/auth/password/reset/request`
- `POST /api/v1/auth/password/reset/confirm`
- `POST /api/v1/auth/password/change`（需要网站 JWT）

请求重置始终返回相同文案。API 只把规范化邮箱放入 Celery；worker 再查询账户、创建挑战并发送邮件，
避免通过响应内容和主要数据库路径判断邮箱是否存在。

## 安全模型

- 数据库只保存 token/code 的 HMAC-SHA256 摘要，HMAC 密钥复用现有服务端 JWT secret；不保存明文凭据。
- 挑战默认 15 分钟有效、单次使用；创建新挑战会使该用户之前未使用的挑战失效。
- 验证码默认最多错误 5 次；请求和确认分别通过 Redis 按客户端地址及邮箱/凭据指纹限流。
- 密码为 10–128 个字符，继续使用现有 PBKDF2-SHA256 哈希；日志不得记录密码、token、验证码或邮件正文。
- SMTP 未配置、Redis 不可用或任务无法入队时 fail closed，不使用 mock 邮件或在响应中返回验证码。

## GitHub 配置

Repository/Environment Variables：

| 名称 | 示例 | 说明 |
| --- | --- | --- |
| `SMTP_HOST` | `smtp.provider.example` | SMTP 服务地址 |
| `SMTP_PORT` | `587` | STARTTLS 通常为 587，隐式 TLS 通常为 465 |
| `SMTP_USERNAME` | provider 用户名 | 无认证的内部 relay 可留空 |
| `SMTP_FROM_EMAIL` | `no-reply@example.com` | 已在邮件服务验证的发件地址 |
| `SMTP_FROM_NAME` | `商机雷达` | 发件显示名 |
| `SMTP_STARTTLS` | `true` | 与 `SMTP_USE_TLS` 不能同时为 true |
| `SMTP_USE_TLS` | `false` | 465 等隐式 TLS 场景使用 |

Repository/Environment Secret：

| 名称 | 说明 |
| --- | --- |
| `SMTP_PASSWORD` | SMTP 密码或 provider 生成的应用专用密码 |

`PASSWORD_RESET_ENABLED` 默认且固定为 `true`，不需要加入 GitHub vars。`PASSWORD_RESET_TTL_MINUTES`、
失败次数和限流窗口在镜像中有保守默认值，通常也无需加入 GitHub vars；确需调整时应先做安全评审。
部署 workflow 会检查已提供的 SMTP 参数是否完整；完全未配置时仍允许应用部署，但密码重置端点会
fail closed 并返回 503。

### Gmail 配置

个人 Gmail 或 Google Workspace 使用以下 Repository/Environment Variables：

```dotenv
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-account@gmail.com
SMTP_FROM_EMAIL=your-account@gmail.com
SMTP_FROM_NAME=商机雷达
SMTP_STARTTLS=true
SMTP_USE_TLS=false
```

`SMTP_PASSWORD` 必须保存为 GitHub Secret，值为 Google 账户生成的 16 位应用专用密码，不是日常登录
密码。生成应用专用密码前需开启 Google 两步验证；如果账户启用了高级保护、只允许安全密钥，或
Google Workspace 管理员禁用了应用专用密码，该选项可能不可用。`SMTP_FROM_EMAIL` 默认应与认证账户
一致；使用 Workspace 别名时，应先在 Gmail/Workspace 中确认该发件身份已获准。

## 邮件域配置与上线

1. 在邮件服务验证 `SMTP_FROM_EMAIL` 所属域名。
2. 按 provider 指引配置 SPF、DKIM 和 DMARC；代码无法替代这些 DNS 操作。
3. 先在 staging 设置 SMTP vars/secret 并部署。
4. 使用测试邮箱验证连接、HTML/纯文本内容、10 位验证码和 H5 链接。
5. 确认邮件送达和日志脱敏后再部署到生产。
6. 验证不存在邮箱与存在邮箱 HTTP 响应完全一致；不存在邮箱不得产生邮件。
7. 验证错误 5 次、过期、重复使用、重新申请、密码修改后旧 JWT 401。
8. 检查 Celery 日志只包含 challenge ID，不含收件邮箱、token、code 和正文。

## 故障与回滚

- 邮件大量失败：立即把 `PASSWORD_RESET_ENABLED=false` 后重新部署；已有密码登录和 OAuth 不受影响。
- 凭据泄漏：在邮件 provider 撤销并重建密码，更新 `SMTP_PASSWORD` 后部署。
- 投递延迟：检查默认 Celery worker 的 `auth.prepare_password_reset` 任务和 provider 日志；不要人工从数据库
  恢复或回显验证码。
- 应用回滚优先保留 `auth_version` 和挑战表。直接 downgrade 迁移会让旧应用不再检查认证版本，不适合作为
  生产紧急回滚方式。
