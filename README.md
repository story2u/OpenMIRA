# Standalone Go + Next.js IM

本目录是一个独立运行的 Go + Next.js + Tailwind CSS IM 项目。后续开发以本仓库的产品目标、架构契约和发布证据为准，不再把其他运行时或历史实现作为功能边界。

## 战略方向

- 后端以 Go 为核心运行时，承担会话、消息、任务、实时网关、投影、审计、管理和后台 worker。
- 前端以 Next.js 为主入口，提供消息端工作台、管理台、诊断和运维界面。
- 消息收发使用通道连接器模型，企微只作为可选连接器之一，不进入 IM core 的强约束。
- RPA 使用 provider 模型，魔云腾/MytRpc 只作为可选 provider 之一，不作为自动化能力的默认前提。
- 所有新增能力必须能在本仓库内构建、测试和部署；任何临时桥接都要有明确下线条件。

## 当前状态

Phase 1 skeleton 已具备 Go API、incoming worker、Next.js、Docker compose、inventory、route metadata 和 readiness profile 等基础资产。仓库中仍保留部分过渡期候选开关和命名，它们只服务于阶段性验证，不定义长期架构。

新的开发方向见：

- [docs/product-roadmap.md](docs/product-roadmap.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/milestones.md](docs/milestones.md)
- [docs/standalone-cleanup.md](docs/standalone-cleanup.md)
- [docs/capability-inventory.md](docs/capability-inventory.md)
- [docs/harness-architecture.md](docs/harness-architecture.md)
- [docs/release-readiness.md](docs/release-readiness.md)
- [docs/nextjs-harness-architecture.md](docs/nextjs-harness-architecture.md)

## 本地验证

基础验证：

```bash
cd go
go test ./...
go vet ./...
```

发布就绪报告用于生成当前发布证据：

```bash
cd go
go run ./cmd/release-readiness -all -format markdown
```

前端验证：

```bash
cd go/web
npm install
npm run test
npm run build
```

容器构建：

```bash
cd go
docker build -t im-go-api --build-arg TARGET_CMD=api .
docker build -t im-go-incoming-worker --build-arg TARGET_CMD=incoming-worker .

cd web
docker build -t im-next-web .
```

## 长期工程边界

- IM core 不直接依赖具体消息平台、RPA 供应商或设备控制协议。
- API、事件、任务和投影需要有本仓库维护的契约定义、测试和可观测证据。
- 写路径默认队列化、幂等化、可重试，关键 worker 与 API 进程独立部署。
- WebSocket、Redis Stream、outbox、DB transaction、缓存和对象存储都必须有可复现的本地或 CI 验证方式。
- 高风险能力先通过内部连接器、fake provider、shadow 运行和发布就绪 profile 证明，再进入生产流量。

## 需要逐步清理的过渡资产

清理原则和执行顺序见 [docs/standalone-cleanup.md](docs/standalone-cleanup.md)，现有能力归属见 [docs/capability-inventory.md](docs/capability-inventory.md)。

- 仍以 `phase1`、`candidate` 命名的 artifact 和开关。
- 与单一供应商绑定的路由、env、compose 服务和 worker 装配。
- 以通道专名或 RPA 供应商专名定义核心领域模型的代码。
- 无法在本仓库独立验证的外部桥接路径。
