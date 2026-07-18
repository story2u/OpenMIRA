# 运行与运维

> 状态：当前事实 · 最后核验：2026-07-18

## 服务拓扑

本地 `backend/docker-compose.yml` 启动 PostgreSQL、Redis、迁移、API、Celery worker、Celery beat
和 Telegram listener。生产 `backend/docker-compose.prod.yml` 另含 frontend 与 cloudflared，并从
GHCR 拉取镜像。`.github/workflows/deploy.yml` 在同一个 release workflow 内构建镜像，并通过 SSH
同步 compose/env 后部署到 VPS。

后端镜像固定复制官方 uv 二进制，使用 `uv sync --locked --no-dev` 从 `pyproject.toml` 与
`uv.lock` 构建 Python 环境；同时从固定 Node 22 镜像保留仓库相对目录，并用
`npm ci --omit=dev --ignore-scripts --install-links` 安装
`backend/pi-agent-runtime/package-lock.json` 中的 pi runtime。runner 通过 `file:../../packages/radar-agent`
消费共享 Agent 契约；`--install-links` 将该包实体化到 runtime，避免最终镜像留下跨 stage 的失效软链接。
后端镜像使用仓库根作为 build context；compose 与部署 workflow 都显式指向 `backend/Dockerfile`。
`.venv` / `node_modules` 不进入 Docker context。CI 分别验证 Python、npm 与根 pnpm 锁。

前端生产镜像同样使用仓库根作为 build context，从根 `pnpm-lock.yaml` 安装 `frontend` 及其
`packages/radar-{api,contracts,agent}` workspace 依赖，再在 `frontend/` 内执行 Next production build。
根 `.dockerignore` 只放行两个镜像需要的后端、前端、共享包、根 workspace manifest 与 pnpm patch，
并再次排除所有嵌套 `node_modules`、`.next`、`.expo`、`dist` 与 coverage 产物。

## 配置分组

以 `backend/.env.example` 和 `backend/app/core/config.py` 为字段真相源。

| 分组 | 关键变量 | 说明 |
| --- | --- | --- |
| 基础 | `APP_ENV`、`DEBUG`、`DATABASE_URL`、`REDIS_URL` | API 与持久化 |
| 队列 | `CELERY_BROKER_URL`、`CELERY_RESULT_BACKEND` | 默认使用 Redis DB 1/2 |
| Web/Auth | `FRONTEND_BASE_URL`、`CORS_ORIGINS`、`JWT_SECRET_KEY`、`ADMIN_API_TOKEN`、`PASSWORD_LOGIN_*`、`DEVICE_REFRESH_TOKEN_DAYS`、`DEVICE_MAX_ACTIVE_PER_USER` | 生产必须使用强随机密钥；密码登录默认每个直连 IP + 邮箱 5 次/300 秒，状态存 Redis DB 0。设备 refresh bearer 默认 30 天、每用户最多 20 台 active 设备，服务端只存 SHA-256 hash；缩短有效期不会自动撤销已签发凭据，紧急处置应调用设备撤销并轮换 JWT secret（影响全部会话） |
| 增量同步 | `SYNC_CHANGE_RETENTION_DAYS`、`SYNC_BOOTSTRAP_TOKEN_MINUTES`、`RN_SYNC_ROLLOUT_ENABLED` | owner change 默认只允许从最近 30 天窗口继续；更旧或超前 cursor 返回显式 reset，客户端必须重新 bootstrap。bootstrap continuation 用 JWT secret 签名、绑定 owner，默认 60 分钟；服务端列表单页硬上限 500。灰度开关默认 false；只有 active `did` 设备同时上报 RN 客户端和 SQLite schema ≥2 才可能获得 `syncAvailable=true`；当前客户端上报 schema 5，push 额外要求 schema ≥3。真机飞行模式/kill-reopen/账号切换证据前不得在生产开启；旧 change row 的定时物理清理仍未实现 |
| 原生推送提示 | `RN_PUSH_ROLLOUT_ENABLED`、`PUSH_DISPATCH_ENABLED`、`PUSH_DISPATCH_{INTERVAL_SECONDS,BATCH_SIZE,LEASE_SECONDS}`、`APNS_{TEAM_ID,KEY_ID,PRIVATE_KEY,TOPIC,SANDBOX_TOPIC}`、`FCM_{PROJECT_ID,CLIENT_EMAIL,PRIVATE_KEY}` | 两个开关默认 false，且依赖同步 rollout。APNs P8/FCM service-account 私钥只进服务端 secrets；换行可用 `\\n`。生产/dev bundle topic 分开；客户端按真实签名 entitlement 上报 `push.environment`，服务端只开放并接受匹配环境。beat 默认 15 秒扫描已提交 SyncChange，单批 100、租约 120 秒；只发送 cursor data/background payload，不发送正文。开启前必须配置相应原生签名/FCM App、用真实设备验证 token 轮换/权限拒绝/失效回收，并监控失败率与 cursor 延迟 |
| OAuth | `GOOGLE_*`、`APPLE_*`、`GOOGLE_NATIVE_CLIENT_IDS`、`APPLE_NATIVE_CLIENT_IDS` | 至少配置一个 provider；Apple secret 可由 key material 生成。原生 audience 用逗号分隔白名单：Google 填移动端请求 ID token 所用的 Web/server client ID；Apple 填精确 bundle ID，生产与 dev 都启用时需分别列出 |
| RN 公共/构建配置 | `EXPO_PUBLIC_API_BASE_URL`、`EXPO_PUBLIC_GOOGLE_*`、`EXPO_PUBLIC_REVENUECAT_*`、`RADAR_GOOGLE_SERVICES_FILE` | API 只填 origin，不含 `/api/v1`；生产 bundle 只接受 HTTPS。Google 登录 ID 和 RevenueCat Public Key 规则不变。Android 直连 FCM 构建还需把对应 application ID 的 `google-services.json` 以受控构建文件路径传给 `RADAR_GOOGLE_SERVICES_FILE`，不得提交真实配置文件；iOS APNs entitlement/凭据由签名与 EAS/Apple 控制台管理。`.dev` bundle 可访问 loopback fixture |
| RN 构建身份 | `RADAR_APP_VARIANT`、`RADAR_APP_VERSION`、`RADAR_BUILD_NUMBER` | 默认 `development` 生成 `.dev` 双平台 ID；`production` 生成既有 `com.codeiy.im` 和 `opportunity-radar` scheme，且必须显式提供三段版本、正整数 build number 与严格 HTTPS `EXPO_PUBLIC_API_BASE_URL`。生产变体关闭 loopback/明文网络例外；实际版本值需高于商店记录 |
| 工作模式 | `DEFAULT_TIMEZONE`、`DEFAULT_WORKDAYS`、`DEFAULT_WORK_START/END`、`PENDING_HUMAN_SLA_MINUTES` | 决定人工/AI 路由与超时 |
| Telegram | `TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`TELEGRAM_BOT_USERNAME`、`TELEGRAM_WEBHOOK_URL`、`TELEGRAM_INTEGRATION_MODE` | 原生 Bot 连接/webhook；首次 release 自动生成并持久化 secret，生产 URL/live/600 秒 TTL 均有 compose 默认值，群额度包含旧 monitor 与新 source |
| 企业微信 | `WECOM_CONNECTION_LIMIT`、`WECOM_WEBHOOK_TOLERANCE_SECONDS`、`WECOM_WEBHOOK_MAX_BODY_BYTES`、`WECOM_ARCHIVE_*` | 自建应用凭据和企业级会话存档凭据均由 `/settings/wecom` 录入并加密存库；存档默认关闭，另需官方 Finance SDK 目录只读挂载和专用 worker；旧 `WECOM_*` 仅保留全局回调兼容 |
| AI/发送 | `AI_ENABLED`、`LITELLM_MODEL`、`OPENAI_API_KEY`、`DEEPSEEK_API_KEY`、`IM_SEND_ENABLED` | AI 与真实发送能力默认开启；部署工作流按 provider 注入服务端密钥。显式设为 `false` 可停用。规则未高置信命中的非空消息会进行语义复核，调用量高于纯关键词模式 |
| AI 自动回复 | `AI_AUTO_REPLY_ENABLED`、`AI_AUTO_REPLY_MIN_CONFIDENCE`、`AI_AUTO_REPLY_COOLDOWN_MINUTES`、`AI_AUTO_REPLY_WINDOW_HOURS`、`AI_AUTO_REPLY_MAX_PER_WINDOW`、`AI_AUTO_REPLY_MAX_CHARS` | 全局能力默认开启；仍需 `IM_SEND_ENABLED`、用户日程和 Business 私聊来源同时授权，后两者默认关闭。策略、投递状态和回滚见自动回复设计 |
| pi Agent | `PI_AGENT_ENABLED`、`PI_AGENT_PROVIDER`、`PI_AGENT_MODEL`、`PI_AGENT_API_KEY`、`PI_AGENT_*TIMEOUT*`、`PI_AGENT_MAX_*` | 默认开启；DeepSeek 可由 GitHub `DEEPSEEK_API_KEY` Secret 映射，OpenAI 可回退使用 `OPENAI_API_KEY` |
| 设备 Agent / 推理网关 | `RN_DEVICE_AGENT_ROLLOUT_{ENABLED,PERCENTAGE}`、`DEVICE_AGENT_{LEASE_SECONDS,RUN_TOKEN_MINUTES,SCHEMA_VERSION,RUNTIME_VERSION,MODEL_ALIAS,POLICY_VERSION}`、`DEVICE_AGENT_{SHADOW_ENABLED,FALLBACK_ENABLED,SHADOW_LOOKBACK_HOURS,EXPIRE_INTERVAL_SECONDS,EXPIRE_BATCH_SIZE}`、`DEVICE_AGENT_ROLLOUT_*`、`DEVICE_AGENT_{RECENT_DEVICE_MINUTES,PRIMARY_CLAIM_WINDOW_SECONDS}`、`DEVICE_AGENT_GATEWAY_*` | rollout/gateway/shadow/fallback 均默认 false。primary 需要稳定百分比 cohort 或最多 100 个 UUID 的白名单，并默认强制当前 runtime/schema/model/policy 的 shadow 样本量、成功率、一致率、P95 readiness；近期设备窗口默认 15 分钟，领取窗口默认 90 秒。网关 key 使用独立 `DEVICE_AGENT_GATEWAY_API_KEY` Secret，不回退到客户端或普通 JWT；base URL 在非本地环境必须 HTTPS，模型 alias 对设备可见，真实 provider model 仅服务端配置。请求/正文/输出 token、超时、每 run 请求数和成本单价均有界；成本单价用“每百万 token 的整数微货币单位”，为 0 时只记 token 不估算成本 |
| 交互式 Agent beta | `INTERACTIVE_AGENT_{BETA_ENABLED,GATEWAY_ENABLED,EXTERNAL_ACTIONS_ENABLED,BETA_MONTHLY_TURN_LIMIT,DEVICE_ALLOWLIST}`、`INTERACTIVE_AGENT_{LEASE_SECONDS,TURN_TOKEN_MINUTES,APPROVAL_TOKEN_SECONDS,MAX_APPROVALS_PER_TURN,SCHEMA_VERSION,RUNTIME_VERSION,MODEL_ALIAS,POLICY_VERSION}`、`INTERACTIVE_AGENT_GATEWAY_*`、`INTERACTIVE_AGENT_EXPIRE_*` | beta/gateway/external-actions 默认 false、独立月 turn 额度默认 0、allowlist 默认空，生产默认工具契约保持 `1:interactive-read-only-v1`。内部动作可显式配置 `2:interactive-internal-v2`；单次批准外发只能配置 `3:interactive-approved-send-v3`，并要求 external-actions 与 `IM_SEND_ENABLED` 同时为 true，部署工作流拒绝其他组合。开启 beta 还强制同步 rollout、正数额度、device UUID allowlist、HTTPS provider gateway 与服务端 secret。approval token 默认 120 秒、每 turn 最多 8 次决定。provider endpoint/key/model 复用 `DEVICE_AGENT_GATEWAY_*`，设备只看到 `radar-interactive-v1` alias。请求默认 128 KB、prompt 64k chars、输出 1 MB、completion 4096 tokens、每 turn 最多 8 个 provider request；turn lease 默认 300 秒，beat 每 30 秒回收失联 reservation。正式套餐映射尚未决定，不得用 0 解释为无限额度 |
| RevenueCat | `REVENUECAT_ENABLED`、`REVENUECAT_SECRET_API_KEY`、`REVENUECAT_PROJECT_ID`、`REVENUECAT_WEBHOOK_*`、`REVENUECAT_RECONCILE_*`、`NEXT_PUBLIC_REVENUECAT_*`、`EXPO_PUBLIC_REVENUECAT_*` | 默认启用；服务端 API/HMAC 来自 GitHub Secrets，Webhook Auth 只保存在 VPS `.env`；Web/RN Public Key 与 Offering ID 来自构建变量，只进入对应客户端构建。RN 未配置 Public Key 时仍显示服务端用量但关闭购买；不得把服务端 Secret 注入任何客户端 |

`JWT_SECRET_KEY` 在首次 VPS 部署缺失或仍为占位值时由 workflow 生成并保留。秘密放 GitHub Secrets，
非敏感配置放 Variables；不得把生产 `.env` 回写仓库。

release workflow 会把设备/同步/原生 audience 与 push 配置显式写入 VPS `.env`。`APNS_PRIVATE_KEY`、
`FCM_PRIVATE_KEY` 必须使用 GitHub Secrets，其余同名项使用 production environment Variables；移除私钥
Secret 后下一次部署会清空 VPS 中的旧值。`RN_SYNC_ROLLOUT_ENABLED`、`RN_PUSH_ROLLOUT_ENABLED`、
`PUSH_DISPATCH_ENABLED` 默认均为 `false`。后两个开关任一启用时，部署会要求同步 rollout、两个 push
开关以及至少一套完整 provider 凭据同时就绪，否则在连接 VPS 前失败。

`RN_DEVICE_AGENT_ROLLOUT_ENABLED`、`DEVICE_AGENT_GATEWAY_ENABLED`、`DEVICE_AGENT_SHADOW_ENABLED` 与
`DEVICE_AGENT_FALLBACK_ENABLED` 均默认 false。打开设备 Agent 前部署会要求同步 rollout、服务端 pi、
gateway、runtime/schema/model alias/policy、独立 provider key、fallback、强制 shadow readiness，以及正数
百分比或设备白名单均就绪。shadow 可在 primary rollout 关闭时单独开启，但仍要求 gateway；fallback 要求
primary rollout 和有效的 server pi provider/model/key。紧急回滚依次关闭 primary rollout、shadow/fallback
和 gateway；现有 Celery runner 保留。关闭开关不会改写已经 consumed 的 usage，active run 由 beat 的
可重复 expire 流程释放 reservation。`analysis_provider_requests` 只用于无正文成本/延迟审计，不是第二套
用户额度。

RN 设备 Agent 只在服务端返回 `deviceAgentAvailable=true` 时领取或恢复未过期 run。客户端上报固定
`agent.runtime=pi-0.80.6`、`agent.schema=1`、`agent.streaming=true`、`agent.submitAnalysis=true`；
本地 SQLite v5 保存有界 input 与
lease/lock/phase/attempt，不保存 run token、prompt transcript、模型输出或 provider 响应。短期 run token
用 device-only SecureStore 保存，登出随 owner run 清理。退后台会取消正在进行的 SSE 并保留恢复记录；
回前台/网络恢复后续租重跑，同一 run 最多三次本地执行，之后显式 fail；本地恢复完成后最多 claim 一条
primary 候选，没有 primary 时最多 claim 一条 shadow 候选。shadow 复用服务端已 consumed 的 ledger，
不写 Message/Opportunity；primary 生产调度只对最近活跃且进入稳定 cohort 的设备开放，并要求当前版本
shadow readiness。合格时自动分析仍预先排入同一 Celery job，但用默认 90 秒领取窗口延迟；worker 看到
active run 时不得接管、标失败、release 或 consume。primary 失败/过期先 release，再用
`device-fallback:{run_id}` 稳定键立即调度 Celery，避免同时存在两次 active allocation。管理员可用
`GET /api/v1/agent/runs/rollout-readiness`（admin token）查看无正文门槛聚合。服务端
开关关闭时客户端不启动新执行，只对已过期记录重试 `expire`。真实 provider、双真机 kill/reopen、结果
一致性和 P95/重复率/quota 漂移阈值完成前不得开启生产灰度。

RN 交互式 Agent 只在 `agentToolsAvailable=true` 时显示入口。客户端上报 `sqlite.schema=5`、
`agent.interactive=true`、`agent.interactiveSchema=3`（最高支持版本），会话/entry 按 owner 本地保留
30 天；turn token 仅
驻留当前运行内存，不写 SQLite、SecureStore 或日志。每次提交先落 user entry，再 claim/heartbeat；pi host
按需加载并按 claim 返回的 v1/v2/v3 精确契约运行。v2 草稿只在本地会话保存且不发送，状态更新只进入既有
幂等 outbox 并显示 queued，认领通过认证在线 API 确认。v3 外发在 `beforeToolCall` 等待用户本次批准，
approval token 只驻留 tool-call closure，并由服务端复核后复用人工回复账本发送。流式文本不逐 token 持久化，最终
entry 批量提交后才 complete。关闭 beta/gateway、schema/policy 错配或移出 allowlist 会阻止新 claim/gateway
request；beat 释放失联 reservation。provider 会看到有界用户输入、所选历史和工具结果，但服务端表/日志
不得保存这些正文。真实 provider、授权 IM 沙箱、双真机和 allowlist smoke 完成前 beta/gateway/external
开关、额度和 allowlist 保持默认关闭，`IM_SEND_ENABLED=false`，schema/policy 保持 v1/read-only。

RN 没有配置 `EXPO_PUBLIC_API_BASE_URL` 时显示可重试的配置错误，不静默回退到 mock 或生产地址。登录
access token、设备 refresh bearer、设备 ID 和 installation ID 只进入 SecureStore；installation ID 只作为
注册输入，服务端用 `JWT_SECRET_KEY` 做 HMAC 后持久化。设备 bearer 是单次轮换凭据：旧 bearer 再次出现会
撤销整个 credential family、设备和关联推送 registration；设备绑定 access token 在设备撤销后立即拒绝。
为兼容 P2 升级，尚未携带 `did` 的旧 access token 只保留到自身过期，不获得 refresh 能力。接受新凭据时
客户端先写入并读回验证 device ID/refresh，再写 access token，失败则回滚三者；启动时仅在明确 401 时清除，
网络错误保留凭据待重试。登出先尽力撤销当前设备，再清 legacy/current/device 凭据，避免旧 token 在下次
启动重新迁移。本地 `radar.db` 只存 owner-scoped 同步/inbox/投影/命令与最后一次 capability 决策；在线
能力查询失败时只允许沿用同 owner 最近一次明确决策，401、422、契约错误和取消请求都 fail closed。看板、
详情、消息和设置保持 online-first，只有已授权 capability 下的网络/5xx 才读 ready 投影；写操作不静默
离线排队，唯一例外是已授权 capability 下的商机内部状态命令。该 outbox 每 owner 最多 100 条 active、
命令 7 天过期、单次 drain 最多 50 条、最多尝试 5 次。`expo-network` 原生监听只在已确认的离线→在线
转换触发恢复，不是后台轮询或可靠消息通道；Android prebuild 会声明 `ACCESS_NETWORK_STATE` 和
`ACCESS_WIFI_STATE`。恢复联网会先同步再校验 base version，409/过期/拒绝保留为用户可见记录。服务端
幂等回执默认 30 天过期并在后续同 owner 命令时清理。`fixture:auth` 是
`.test` 假账号的
本机验证服务；设备注册/轮换/复用撤销及回复/认领/状态等写操作只修改进程内存，不连接数据库或外部 IM，
进程重启会丢失。它只能用于并行 `.dev` 包，不得作为生产回退。

RN Google 原生登录在构建配置阶段要求 Web/server 与 iOS client ID 成对出现，config plugin 会从 iOS
client ID 派生反向 URL scheme；缺任一项即拒绝生成错误配置。Android 使用 Credential Manager 的显式
Google 按钮流，iOS 使用 GoogleSignIn；Apple 只在 iOS 以系统控件开放。客户端不包含 OAuth client secret，
也不持久化 provider ID token；后端 audience 白名单为空时对应原生登录必须 fail closed。发布前应在隔离
测试账号验证成功、取消、错误 audience、首次建号与账号切换，不能用“原生模块可构建”代替 provider E2E。

RN 界面语言不依赖环境变量：运行时按系统首选语言选择 `zh-CN` 或 `en`，其他语言显式回退简体中文。
`expo-localization` config plugin 同时向 iOS 声明 `en`/`zh-Hans`、向 Android 声明 `en`/`zh-CN`，使系统
单 App 语言设置只展示实际支持项。Android 前台期间修改系统语言会触发 locale 更新；iOS 的系统语言
变更按平台生命周期在应用重新打开后生效。当前没有独立的应用内语言偏好，也没有需要同步到后端的字段。

商机语义复核复用 `LITELLM_MODEL`，不需要独立模型服务。每次只传当前 owner 同会话最近 6 条、合计
不超过 4000 字符的规范化历史以及最多 20 条 AI hint；模型输出非法或 provider 失败时回退到规则结果。
DeepSeek 使用 `LITELLM_MODEL=deepseek/deepseek-chat` 和 GitHub Secret `DEEPSEEK_API_KEY`；该 Secret
由部署工作流写入 VPS 环境并透传给 API、Telegram listener 与 Celery 容器。
模型补判不会获得自动发送权限，只创建待人工审核商机。需要控制成本时关闭 `AI_ENABLED` 即可恢复纯
规则路径；后续应在有金标数据后增加本地语义候选层，以降低全量复核调用。

自动回复要求 `AI_AUTO_REPLY_ENABLED` 和 `IM_SEND_ENABLED` 均未被显式关闭，并在用户工作时间页及
Telegram Business 私聊来源逐项授权。全局能力默认开启，但用户和来源默认关闭。先关闭
`AI_AUTO_REPLY_ENABLED` 即可停止创建新投递而保留分析、人工草稿、来源配置和审计账本。详见
[AI Agent 安全自动回复](../integrations/ai-auto-reply.md)。

## 队列

worker 监听 `default,im,ai,agent`。关键任务定义在 `backend/app/worker/tasks.py`：AI 回复、pi 消息
后处理和人工 SLA 超时扫描。自动回复任务本身不做 Celery 自动重试；数据库唯一键吸收重复投递，
`sending` 后结果未知时转人工核对，避免 provider 缺少端到端幂等导致重复联系客户。
`agent.analyze_message` 入队前在 PostgreSQL 原子预留用户额度，成功后
转为 consumed，最终重试失败或入队失败转为 released；最多自动重试 3 次，子进程另有硬超时。
Message 状态、usage ledger 幂等键和 source message 唯一索引共同提供重复任务保护。新增任务时需要
明确 queue、超时、重试、幂等键和可观测字段，并同步 compose/部署配置。

RevenueCat webhook 任务也走 `default` queue；event 行锁和唯一 event ID 防重复处理，每日 beat 按批次
reconcile 活跃/临期/失败用户。关闭 `REVENUECAT_ENABLED` 会关闭新同步和 webhook，不清除已有投影。
完整平台配置、轮换与故障排查见[统一订阅 Runbook](../integrations/revenuecat-paddle-billing.md)。

用户级企业微信 webhook 只做验签、解密、幂等事件预留和入队，消息识别由监听 `im`
queue 的 Celery worker 执行。规范化正文仅在待处理期间加密保存，完成或最终失败后清除。新连接
的企微会话始终进入人工审核，不参与非工作时间 SLA 自动发送。详见
[企业微信集成](../integrations/wecom.md)。

企业微信会话内容存档使用独立 `wecom_archive` queue 和单并发 `wecom_archive_worker`。beat 只为 active
连接排队，worker 从最后成功 `seq` 拉取、用内存中的 RSA 私钥解密并按 active member binding 投影；整批
成功才推进 cursor。生产必须把企业微信官方下载且与 VPS 架构匹配的 SDK 目录挂载到
`/opt/wecom-finance-sdk`，仓库和镜像不包含腾讯二进制。`WECOM_ARCHIVE_ENABLED=false`、SDK 缺失或原生
调用失败均为 fail closed，不会回退到演示数据，也不影响自建应用 webhook。完整边界、配置与回滚见
[会话内容存档设计](../integrations/wecom-conversation-archive.md)。

Telegram listener 每轮加载配置时按有效套餐重算 monitor 配额。降级超额项仅设置
`quota_paused/quota_reason` 并停止对应 listener，不删除群配置；用户通过设置页选择保留项后，下一轮
刷新会停止旧 listener、启动新选择。升级会恢复容量内暂停项，同时保留用户选择优先级。

Telegram 原生连接由 API webhook 处理：先验证 Bot secret，再按 `update_id` 写入幂等事件，随后处理
连接握手或已配置 source 的消息。release 部署在 Telegram token、secret 和 webhook URL 完整时执行
`setWebhook` 注册；具体 GitHub Secrets/Variables 和本地 mock 规则见
[Telegram 原生连接](../integrations/telegram.md)。普通账号 QR 由 `telegram_mtproto_qr_worker` 与
`telegram_mtproto_listener` 两个独立常驻服务处理，生产环境已纳入默认部署与迁移前停机列表。

## 健康与启动顺序

1. PostgreSQL/Redis 通过 healthcheck。
2. 发布时先停止 API、worker、beat、Telegram listener、会话存档 worker 等数据库客户端，避免旧连接耗尽 PostgreSQL
   连接槽；`migrate` 执行 `alembic upgrade head` 并成功退出。
3. API、worker、beat、listener 启动。
4. frontend/cloudflared 对外提供入口。

健康端点：根 `/healthz` 与 API 前缀下 `/api/v1/healthz`、`/api/v1/readyz`。当前 readyz 只返回
静态状态，尚未证明数据库、Redis 或外部 provider 可用；不要把它解释成完整依赖就绪检查。

## 发布与回滚

- 只有向 `release/v<major>.<minor>.<patch>` 分支推送提交才启动自动化；例如
  `release/v1.0.0`。普通分支、`main`、PR、tag 和手动 dispatch 都不会触发这些 workflow。
- 发布链固定为 `CI` 成功后触发生产 `Deploy to VPS`：workflow 内先并行构建两个镜像，两个构建 job
  都成功后才执行部署与健康检查。它直接使用 CI 的 `head_sha` 和 release 分支，避免二级
  `workflow_run` 将运行上下文切回默认分支。
- 镜像使用去掉 `release/` 前缀的版本号（如 `v1.0.0`）和 `sha-<short-sha>` 双标签；部署使用
  版本标签，不使用含 `/` 的分支名或漂移的 `latest`。
- schema 变更上线前评估旧/新应用兼容窗口。不可向后兼容的迁移需要分阶段 expand/migrate/contract。
- 迁移失败时数据库客户端保持停止状态，先检查 `migrate` 日志并修复或回滚，再恢复运行服务；不要让旧
  应用继续访问可能已部分变更的 schema。
- 回滚应用前确认数据库仍兼容旧版本；不能安全降级时以前向修复为主并在计划写明。

## 生产数据维护

手动 workflow `Production Data Maintenance` 提供只读计数和受确认保护的商机数据清理。清理操作会
短暂停止 API、Celery 与 Telegram 摄取服务，在单个事务中解除用量账本的消息引用并删除商机、消息；
可选项还会删除 legacy monitor 与统一 Telegram source，从而释放监听配额，但保留用户、订阅、用量
审计、Telegram connection 及加密 session。操作前后都会输出分表计数，不能通过该 workflow 执行
任意 SQL。执行前仍应确认清理范围；该入口不是归档能力，也不能代替数据库备份。

## 运维缺口

当前仓库没有完整的集中日志/指标/trace、外部告警、备份恢复演练或依赖型 readiness。相关建设记录在
[技术债](../plans/tech-debt.md)，不能仅凭容器 `running` 判断服务健康。
