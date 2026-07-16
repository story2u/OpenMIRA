# 架构决策记录（ADR）

> 状态：当前规范 · 最后核验：2026-07-11

ADR 记录会长期约束后续开发、替换成本高或存在重要取舍的决定，例如架构边界、数据模型、认证、
外部 provider、任务幂等、部署拓扑。局部实现细节和可轻易撤销的选择不需要 ADR。

## 流程

1. 复制 [ADR 模板](0000-template.md) 为下一个四位编号和短 slug。
2. 提案阶段状态为 `proposed`；合并采用后改为 `accepted`。
3. 决策改变时新增 ADR 并标记旧 ADR `superseded by ADR-xxxx`，不重写历史。
4. 同步更新架构/开发规范中的当前事实；ADR 解释原因，不替代现状文档。

## 索引

- [ADR-0001：使用 uv 管理 Python 与依赖](0001-use-uv-for-python-dependencies.md)（accepted）
- [ADR-0002：以受限子进程集成 pi Agent](0002-integrate-pi-as-constrained-runner.md)（accepted）
- [ADR-0003：默认启用 pi Agent](0003-enable-pi-agent-by-default.md)（accepted）
- [ADR-0004：使用版本化套餐目录与 PostgreSQL 用量账本](0004-versioned-entitlements-and-usage-ledger.md)（accepted）
- [ADR-0005：Telegram 使用统一连接与来源模型](0005-telegram-unified-connection-model.md)（accepted）
- [ADR-0006：移动端作为现有 API 的瘦客户端](0006-mobile-app-thin-client.md)（proposed）
- [ADR-0007：Telegram 普通账号仅使用平台凭据二维码登录](0007-telegram-qr-platform-credentials.md)（accepted）
- [ADR-0008：RevenueCat + Paddle 统一订阅计费](0008-revenuecat-paddle-unified-billing.md)（accepted）
- [ADR-0009：AI 自动回复使用确定性策略门禁和独立投递账本](0009-policy-gated-ai-auto-reply.md)（accepted）
- [ADR-0010：默认启用 AI 分析与受控发送运行能力](0010-enable-ai-reply-pipeline-by-default.md)（accepted）
- [ADR-0011：工作机会发现采用证据约束提取与确定性匹配](0011-job-discovery-evidence-and-deterministic-matching.md)（accepted）

仓库既有分层结构作为现状记录在[架构总览](../architecture/overview.md)；未来对其做持久性改变时
继续递增编号。
