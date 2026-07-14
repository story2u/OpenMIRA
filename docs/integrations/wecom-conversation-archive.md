# 企业微信会话内容存档设计

> 状态：P1 代码已实现，待真实企业 E2E · 最后核验：2026-07-14 · 适用分支：`features/wecom-conversation-archive`

## 能力边界

企业微信会话内容存档是企业级付费能力，不是普通成员 OAuth。企业管理员必须在管理后台购买并开启
功能、选择存档成员、配置调用 IP 和消息加密公钥；成员只能确认告知，不能独立授予企业存档 Secret。
官方使用说明见 [会话内容存档](https://developer.work.weixin.qq.com/document/path/91360)，数据拉取协议见
[获取会话内容](https://developer.work.weixin.qq.com/document/path/91774)。

本期支持：

- 已被企业纳入存档范围的当前成员与其他成员之间的文本私聊。
- 当前成员参与且由存档接口返回的内部群、外部群文本消息。
- 当前成员与已满足平台同意要求的外部联系人文本消息。
- 同一条企业消息按本地用户投影，进入现有 Message、Opportunity 和 Pi Agent 链路。

本期不支持：

- 读取启用存档之前的本地历史记录，或读取两个都不在存档范围内的成员会话。
- 通过存档 API 代表成员回复；来源始终为 `manual_only`。
- 图片、语音、视频、文件和媒体下载。
- 普通成员绕过企业管理员自行开启存档。
- 自动把企业内全部存档消息暴露给安装管理员或任意本地用户。

## 所有权与访问控制

存档连接由完成企业管理员配置的本地用户拥有，但连接本身代表一个企业，不代表该用户个人账号。
`WeComArchiveMemberBinding` 显式把本地 `users.id` 映射到企业微信 `userid`。消息只会投影给消息参与者中
存在的 active binding；连接 owner 不会因此自动看到其他成员消息。

P1 首版创建连接时只允许绑定当前登录用户。数据库允许一个连接绑定多个用户，为企业邀请和席位功能
保留结构，但 API 不接受任意 `user_id`。同一企业可以被不同账号重复配置，因此所有查询仍以 connection
和 binding 为安全边界，不能只按 `corp_id` 授权。

## 数据模型

### `wecom_archive_connections`

- `id`、`owner_user_id`、`display_name`、`corp_id`
- `secret_encrypted`：会话存档 Secret 密文
- `private_key_encrypted`：RSA 私钥密文
- `public_key_version`：企业微信后台显示的公钥版本
- `status`、`enabled`、`last_polled_at`、`last_error`
- 唯一约束 `(owner_user_id, corp_id)`

### `wecom_archive_member_bindings`

- `connection_id`、`user_id`、`wecom_user_id`、`display_name`
- `enabled`、`last_matched_at`
- 唯一约束 `(connection_id, user_id)` 和 `(connection_id, wecom_user_id)`

### `wecom_archive_cursors`

- 每个 connection 一行，保存最后成功处理的 `last_seq`；官方 SDK 返回该序号之后的消息。
- cursor 只在当前批次的所有用户投影均成功或幂等完成后推进。
- 使用处理租约避免同一连接并发拉取。

### `wecom_archive_events`

- `(connection_id, provider_message_id)` 唯一，长期保存 hash、消息类型、seq、状态和错误类别。
- 不保存完整 provider JSON、Secret、私钥、解密随机密钥或媒体正文。
- 解密后的文本直接进入现有 owner-scoped Message；事件表不复制正文。

现有 `wecom_sources` 继续承担用户看板来源。存档私聊和群聊分别写入 `private`、`internal_group` 或
`external_group`，接收能力为 `message_archive`，发送能力为 `manual_only`。

## 原生 SDK 边界

官方 Finance SDK 是原生动态库。应用通过 `WeComArchiveProvider` Protocol 隔离 SDK：

```text
Celery beat
  -> 为 active connection 排队
  -> wecom_archive 单并发 worker
  -> CtypesWeComFinanceProvider
  -> libWeWorkFinanceSdk_C.so
  -> GetChatData / DecryptData
  -> 规范化 ArchiveMessage
  -> 按成员 binding 投影
  -> IngestMessageUseCase
```

生产只允许真实 SDK provider；动态库不存在、SDK 返回非零或凭据错误时连接进入 error/保持 cursor，不得
回退到 mock。测试通过 fake Protocol 实现固定向量，不把 mock 注册到生产容器。

动态库通过只读挂载提供，默认路径 `/opt/wecom-finance-sdk/libWeWorkFinanceSdk_C.so`。仓库不提交腾讯
二进制。私钥仅在 worker 内存中解密，用于解密每条消息的 `encrypt_random_key`，不写日志和临时文件。

## 拉取、幂等与失败恢复

1. beat 周期性扫描 active connection，将 connection ID 投递到 `wecom_archive` 队列。
2. worker 对 cursor 获得短租约；正在处理的连接不会重复执行。
3. SDK 从 `last_seq` 之后拉取最多配置条数，逐条解密。
4. 未知消息类型只写安全审计并推进；文本类型转换为内部稳定模型。
5. 从 `from`、`tolist` 提取参与者；只匹配 active member binding。
6. 每个匹配用户生成独立 external message ID：`wecom-archive:{connection}:{user}:{msgid}`。
7. 建立 owner-scoped source，强制人工审核，再调用现有摄取用例。
8. 整批成功后把 cursor 设置为批次最大 `seq`；失败保留原 cursor，重试依赖 Message/Event 幂等。

## API

```text
GET    /api/v1/integrations/wecom/archive-connections
POST   /api/v1/integrations/wecom/archive-connections
POST   /api/v1/integrations/wecom/archive-connections/{id}/verify
POST   /api/v1/integrations/wecom/archive-connections/{id}/sync
DELETE /api/v1/integrations/wecom/archive-connections/{id}
```

所有接口需要网站 JWT，仅 owner 可管理。创建接口接受当前用户的 `wecomUserId`，不接受本地 user ID。
读取接口不回显 Secret、私钥、密文或任何密钥摘要。手动 sync 只入队并限流，不在 API 请求中执行原生 SDK。

## 部署与人工配置

1. 企业管理员购买并启用会话内容存档，将目标成员加入存档范围。
2. 生成 RSA 密钥对，把公钥填入企微后台并记录版本；私钥只输入商机雷达。
3. 将 VPS 出口 IP 加入企业微信调用 IP 白名单。
4. 从企业微信官方页面下载与 Linux/CPU 匹配的 Finance SDK，把下载包中的动态库及其同目录依赖完整放到
   VPS 的 `WECOM_ARCHIVE_SDK_DIR`，只读挂载到 API 和专用 worker；不要只复制一个 `.so`。
5. 在 GitHub Actions Variables 设置 `WECOM_ARCHIVE_ENABLED=true`（部署工作流会写入 VPS `.env`），
   并在 VPS `.env` 设置 SDK 宿主目录 `WECOM_ARCHIVE_SDK_DIR`，然后重建专用 worker。
6. 在 `/settings/wecom` 创建存档连接，绑定当前成员 userid，验证后执行首次同步。

VPS 上建议先把官方 SDK 文件放入仅 root/部署用户可写目录，再重建服务：

```bash
cd /home/ubuntu/wework
install -d -m 0755 /home/ubuntu/wework/wecom-finance-sdk
# 由管理员把官方 SDK 包中的动态库及其同目录依赖放入上面的目录。
printf '\nWECOM_ARCHIVE_SDK_DIR=/home/ubuntu/wework/wecom-finance-sdk\n' >> .env
docker compose --env-file .env -f docker-compose.prod.yml up -d --force-recreate \
  api celery_beat wecom_archive_worker
docker compose --env-file .env -f docker-compose.prod.yml logs --tail=100 wecom_archive_worker
```

在 GitHub Variable 尚未打开开关前，上述容器会保持 fail closed。不要把私钥写入 VPS `.env`；它由
owner 在页面提交后使用现有应用加密密钥存入数据库。

配置关闭或 SDK 未挂载时，P0 和其他功能必须正常运行；存档连接显示“服务端尚未配置”，不生成 mock 消息。

## 安全与合规

- 企业必须完成员工告知和外部联系人同意，不以技术实现替代法律依据。
- Message/Opportunity 继续按 owner 隔离；管理连接不授予查看全企业消息的权限。
- 日志不记录正文、参与者列表、Secret、私钥、random key 或完整 SDK 响应。
- 默认只处理文本，正文沿用现有消息保留和归档规则；事件审计不复制正文。
- AI 只接收规范化消息文本，不接收企业微信凭据或 SDK 加密材料。
- 删除连接会先禁用并清除凭据，保留已经生成且属于用户的商机审计；不声称删除企业微信源数据。

## 验收重点

- 无 binding 的消息绝不生成本地 Message。
- owner 不能看到只属于其他 binding 的消息。
- 私聊和群聊分别形成稳定来源，重复批次不重复生成消息或商机。
- 处理中失败不推进 cursor；重试后可恢复。
- SDK 缺失和未知消息类型 fail closed，不影响 API、P0 webhook 和其他 worker。
- 存档来源无法通过人工回复或 AI 自动回复 API 发送。
