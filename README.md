<div align="center">
  <img src="frontend/public/logo-512.png" width="72" alt="商机雷达 Logo">
  <h1>商机雷达 · Opportunity Radar</h1>
  <p>多 IM 渠道商机识别与 AI 辅助跟进工具</p>
  <p>
    <a href="https://im.story2u.xyz">在线产品</a> ·
    <a href="https://img.story2u.xyz/story2u/message-flow-intro-1080p.mp4">45 秒介绍视频</a> ·
    <a href="docs/product/feature-map.md">功能成熟度</a>
  </p>
</div>

> 在线首页无需账号；进入真实商机看板需要 Google 或 Apple 登录。

<p align="center">
  <a href="https://img.story2u.xyz/story2u/message-flow-intro-1080p.mp4">
    <img src="assets/01-landing-hero.png" width="100%" alt="商机雷达产品首页与商机看板预览">
  </a>
</p>

<p align="center">
  点击上方画面观看 <a href="https://img.story2u.xyz/story2u/message-flow-intro-1080p.mp4">1080p 产品介绍视频</a>
</p>

## 核心能力

- 连接 Telegram Bot、普通账号只读来源与企业微信 Webhook。
- 使用确定性规则与 LiteLLM 语义复核识别群聊和私聊商机。
- Pi Agent 整理上下文、核验公开链接、提取联系人和建议下一步动作。
- 白天人工审核；非工作时间按安全策略处理，AI 新发现的商机仍强制人工审核。
- Web、iOS、Android 共享后端用户和订阅权益。
- RevenueCat 聚合 Paddle、App Store 和 Google Play；外部 Sandbox 仍需人工配置验证。

## 工作流程

```text
消息渠道 → 幂等摄取 → 规则/语义识别 → Pi Agent 结构化分析 → 人工审核与跟进
```

后端是受限业务 API 的最终权限来源。客户端 CustomerInfo 和模型输出都不能直接提升权益
或执行外部动作。

<p align="center">
  <img src="assets/02-workflow.png" width="100%" alt="从消息渠道接入到人工跟进的四步工作流程">
</p>

## 产品界面

<table>
  <tr>
    <td width="50%">
      <img src="assets/07-opportunity-dashboard.png" alt="商机看板">
      <br><strong>商机看板</strong><br>
      聚合 Telegram 与企业微信消息，按状态、平台、风险和相关度进行筛选与排序。
    </td>
    <td width="50%">
      <img src="assets/08-opportunity-detail.png" alt="商机详情与处理流程">
      <br><strong>商机详情</strong><br>
      查看链接核验、联系方式提取、跟进建议及需要人工确认的后续动作。
    </td>
  </tr>
  <tr>
    <td width="50%">
      <img src="assets/09-settings.png" alt="商机识别与工作时间设置">
      <br><strong>用户级设置</strong><br>
      配置关键词、AI 语义识别和工作时间，设置按当前用户隔离并由后端持久化。
    </td>
    <td width="50%">
      <img src="assets/10-telegram-connection.png" alt="Telegram 连接中心">
      <br><strong>Telegram 连接中心</strong><br>
      支持 Bot 群组/频道、Business 私聊和普通账号 QR 等不同接入路径。
    </td>
  </tr>
</table>

更多页面截图保存在 [`assets/`](assets/) 目录。

## 系统架构

- Web：Next.js 16、React 19、Tailwind、shadcn/ui
- API：FastAPI、SQLModel、PostgreSQL、Redis
- Worker：Celery、LiteLLM、LangChain、受限 pi Agent runner
- Mobile：SwiftUI / Swift Concurrency、Kotlin / Jetpack Compose
- Billing：RevenueCat + Paddle/App Store/Google Play，本地权益投影最终裁决

详见 [架构总览](docs/architecture/overview.md)。

## 本地启动

```bash
cd backend
cp .env.example .env
docker compose up -d postgres redis migrate api celery_worker celery_beat

cd ../frontend
corepack pnpm@10.25.0 install --frozen-lockfile
corepack pnpm@10.25.0 dev
```

## 安全边界

- 用户资源按后端 `users.id` 隔离；Telegram Session 仅以服务端密文持久化。
- Webhook 执行验签与幂等；模型输入不包含 Telegram Token、API Hash 或 Session。
- AI 草稿与 Agent 外部动作建议必须经人工确认。

<p align="center">
  <img src="assets/05-security-and-cta.png" width="100%" alt="商机雷达安全与权限边界">
</p>

## 当前成熟度

Telegram Bot 摄取、商机识别、基础看板和后端回复链路已实现；部分 Web 编辑操作、通知偏好、移动端
分发与真实支付 Sandbox E2E 仍为 Beta 或待外部验证。权威状态见[功能地图](docs/product/feature-map.md)。

## Roadmap

- 完成 RevenueCat/Paddle/App Store/Play Sandbox E2E。
- 将前端仍为本地状态的设置与回复动作迁移到真实 API。
- 完成 iOS TestFlight 与 Android 测试渠道分发。
- 在真实但脱敏的金标数据上校准商机识别阈值。

## License

本项目采用 [MIT License](LICENSE) 开源。
