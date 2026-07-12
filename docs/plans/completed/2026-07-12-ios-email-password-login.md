# iOS 邮箱密码登录

> 状态：complete · Owner：Codex · 创建：2026-07-12 · 更新：2026-07-12

## 目标与用户价值

iOS 用户可在登录页用已有账户的邮箱和密码换取后端 JWT 并进入应用；登录页与会话层不再包含
粘贴 JWT 的开发调试登录入口。

## 非目标

- 不新增注册、找回密码、修改密码或密码初始化页面。
- 不移除 Sign in with Apple，也不改变 Web OAuth 登录。
- 不为现有 OAuth-only 账户自动生成密码。

## 背景与当前行为

- `mobile/ios/Features/Auth/LoginView.swift` 当前仅提供 Apple 登录，DEBUG 构建额外显示粘贴 JWT。
- `mobile/ios/Core/Auth/SessionStore.swift` 负责 JWT Keychain 持久化和 DEBUG token 恢复。
- `users.password_hash`、PBKDF2 哈希与验证能力已经存在，但 `backend/app/api/v1/routes/auth.py`
  尚无邮箱密码登录端点。
- 认证约束见 [安全基线](../../quality/security.md)，当前能力地图见
  [功能地图](../../product/feature-map.md)。

## 验收标准

- [x] 有密码且启用的账户提交正确邮箱和密码时，API 返回与现有认证一致的 JWT 和用户信息。
- [x] 未知邮箱、错误密码、无密码账户和停用账户均返回相同的 401 文案，且未知邮箱仍执行密码哈希校验。
- [x] 请求字段有长度边界；密码不进入日志、响应或持久化明文。
- [x] iOS 登录页包含带系统语义的邮箱、密码输入和防重复提交按钮，失败可见，成功保存 JWT。
- [x] iOS 代码在 DEBUG 与 Release 中都不再包含粘贴 JWT 的登录入口或会话方法。
- [x] 后端认证测试、iOS 编译和仓库相关检查通过。

## 影响面与风险

- iOS：登录视图、SessionStore、API endpoint 与请求 DTO；Keychain 行为复用，不改存储机制。
- API/application：新增邮箱密码请求 DTO 与公开认证端点。
- infrastructure：复用 `UserRepository.get_by_email` 和已有 `password_hash` 字段；无 schema 变化、无迁移。
- worker/IM/发送：无影响。
- 安全：主要风险为账号枚举、计时差异、暴力猜测、密码泄露。端点使用统一失败响应、固定假哈希和
  Redis 共享滑动窗口；生产边缘仍应提供独立 IP 限流作为纵深防护。

## 实施步骤

- [x] 添加请求契约、邮箱密码认证端点及成功/拒绝路径测试。
- [x] iOS 接入邮箱密码登录并删除 DEBUG token 通道，补请求编码测试。
- [x] 更新本地开发入口与当前事实文档，运行验证并归档计划。

## 进度日志

- 2026-07-12：完成现状核验；下一步先用后端测试固定认证契约。
- 2026-07-12：后端契约、iOS 表单和本地演示账户路径完成；模拟器视觉检查确认布局完整且无调试入口。
- 2026-07-12：补 Redis 多实例共享登录限流与不可用时安全失败；全量验证通过，计划归档。

## 发现日志

- `users.password_hash` 与 `hash_password`/`verify_password` 已由首个认证迁移和安全测试覆盖，
  本需求无需数据库迁移或新增密码学依赖。
- 仓库没有通用认证速率限制器；在本切片临时引入进程内限制会在多实例下失效，因此不伪装成完整防护。
- API 已有共享 Redis DB 0 依赖，适合实现按直连 IP + 规范化邮箱的跨实例限制；邮箱只以 SHA-256
  指纹进入 key，正确登录后清除计数。

## 决策日志

- 2026-07-12：新增 `POST /auth/password/login`，仅认证已有密码账户；保持注册/密码生命周期在本需求之外。
- 2026-07-12：未知账户使用固定 PBKDF2 假哈希验证，并让所有凭据失败返回同一 401；降低账号枚举和
  明显计时差异。
- 2026-07-12：默认 5 次/300 秒滑动窗口；超限返回 429 + `Retry-After`，Redis 不可用时返回 503，
  避免在保护设施失效时开放无限尝试。

## 验证记录

| 命令/场景 | 结果 | 证据或备注 |
| --- | --- | --- |
| 后端认证定向测试 | 通过 | 18 passed |
| `make backend-check` | 通过 | 63 passed，5 skipped；既有 TestClient 弃用 warning 1 条 |
| `make ios-check` | 通过 | xcodegen + iOS Simulator 双架构无签名编译 |
| iOS Simulator 单测 | 通过 | iPhone 17 Pro / iOS 26.3.1，4 passed |
| iOS Simulator 视觉检查 | 通过 | 表单、分隔线、Apple 登录完整显示，无 DEBUG token 入口 |
| `make harness-check` | 通过 | 文档链接、入口大小、分层与脚本测试 |

## 回滚与恢复

回滚新增端点、iOS 表单及文档即可；没有迁移、数据回填或外部副作用。已签发 JWT 与其他登录方式
签发的 JWT 相同，按既有过期时间自然失效。

## 结果与剩余风险

已交付已有账户的 iOS 邮箱密码登录、统一的 Keychain 会话路径、后端 PBKDF2 验证与 Redis 限流，
并移除 DEBUG token 登录。OAuth-only 用户仍没有密码，注册、密码初始化/重置不在本切片；生产边缘
仍应配置独立 IP 限流，以缓解分布式尝试和代理拓扑导致的应用层限流盲区。
