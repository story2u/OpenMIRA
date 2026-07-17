# 信息胃口（Signal Appetite）

> 状态：已实现、默认灰度关闭 · 最后核验：2026-07-18 · 适用端：`mobile/radar`

信息胃口回答“此刻，什么信息值得占用我的注意力”。用户通过真实本地消息教学，Pi 形成候选偏好、
用历史消息试跑，只有在明确确认后才更新稳定版本。普通界面不暴露关键词规则、阈值或模型参数。

## 体验与信息架构

主导航为“首页 / 机会 / Pi / 设置”。首页是注意力控制台，包含当前胃口、意图地图、四类投递统计、
教学和安静区入口；原商机看板保留在“机会”。Pi 可从地图节点、安静区纠正和商机详情接收带概念或
本地资源 ID 的自然语言草稿，路由不携带消息正文。

教学手势采用固定物理语义，RTL 不镜像：

| 动作 | 结果 | 视觉/触觉 | 等价操作 |
| --- | --- | --- | --- |
| 左滑超过宽度 40%，或超过 8% 后以 850 px/s fling | positive，有效信息 | 青绿色语义层、星形、阈值 haptic，卡片旋转离场 | “有效信息”按钮、读屏 action |
| 右滑超过相同阈值 | negative，无效信息 | 暖灰红语义层、空心圆、阈值 haptic，卡片旋转离场 | “无效信息”按钮、读屏 action |
| 未达阈值、纵向意图、屏幕边缘起手或多点触控 | 不提交并回弹 | 不触发 haptic | 无 |

卡片变换在 Reanimated UI thread 上运行，下一张卡同时预载并轻微上浮；Reduce Motion 下缩短位移并以
淡入淡出为主。单次样本只追加事件，原因可以随后补充；最近 10 次操作可连续撤销。

## 意图地图、时间线与过滤成果

地图以“现在的我”为中心，确定性预布局核心、上下文、降低关注和临时节点；实线表示用户确认，虚线
表示 Pi 推测，节点半径和描边表达权重/置信度。SVG 节点最多 30 个，支持平移、缩放和节点解释，同时
提供线性语义列表给 VoiceOver/TalkBack。地图底部的一天轨道不是独立日程配置器；选择 06/09/12/18/
21/24 时，会突出该窗口的 active intent，并显示默认投递节奏。

四类结果统一为立即提醒、稍后处理、摘要和安静收起。安静区默认保留本地可抽查副本，每条展示原因摘要、
可核验证据、处理位置、置信度与 deterministic/on-device/cloud evaluator，不保存或展示模型私有推理。
已有稳定版本时，候选可影子观察 24 小时；当前过滤不变，地图切换当前/候选节点和模拟统计。

## Agent、数据与执行边界

Pi schema v4 在 v1-v3 之上增加 15 个信息胃口工具：inspect、start/capture/summarize teaching、propose、
simulate、apply、start shadow、explain/list/correct decision、temporary focus、schedule、undo 和 compare。
capture/propose/simulate 不改变稳定偏好；`apply_appetite_change` 在 `beforeToolCall` 暂停，必须由本次调用
的一次性确认卡解锁。外部发送继续使用独立 Approval Gate，两种批准不能互换。

SQLite schema v7 保存 append-only `attention_events`，并折叠为 preference、intent、example、session、
decision、shadow、temporary focus 与一次性 UI state 投影。L0/L1 离线 evaluator 在边界不确定时只能进入
inbox/digest，云端不可用不得 suppress。服务端 `signal_appetite_events` 只同步 content-free、owner/device
绑定事件；同 event ID 仅在内容完全一致时幂等，否则 409。能力由
`SIGNAL_APPETITE_SYNC_ENABLED` 和设备 `signalAppetiteSyncAvailable` 单独门控，默认关闭。

## 验证、性能与无障碍

- `make check`：通过；后端 229 passed / 86 skipped，Harness、Pi runtime、Web、共享包、RN 检查与双平台
  Hermes export 均通过。
- RN：53 files / 174 tests；包含左右语义、fling/回弹/纵向/边缘/多点、连续撤销、100 次连续教学状态、
  30 节点上限、时间窗、Shadow、离线决策、SQLite fold/sync、Agent faux provider 与日志正文脱敏。
- 结构性能证据：手势变换在 UI thread；React 只在方向/阈值变化时更新语义状态，不接收逐帧位移；布局进入页面前确定性计算；节点
  ≤30；下一张卡预载。P90 55fps、热机、内存压力和低端 Android 仍需真机 profiler，当前不得视为验收。
- 无障碍代码路径：按钮与滑动等价、读屏 actions、地图线性列表、selected/busy/live-region 语义、
  Reduce Motion、类型化中英文。VoiceOver/TalkBack、外接键盘、大字体、小屏和平板视觉仍需真机 QA。

## Golden / Screenshot 清单

仓库目前没有 RN screenshot harness，且本次未连接授权真机，因此没有把设计稿或合成图冒充运行截图。
以下是发版前必须用确定性 fixture 在 iOS 与 Android 补齐的清单：

| 场景 | 自动化结构证据 | 运行截图状态 |
| --- | --- | --- |
| 教学首屏、左/右 25%、左右提交态 | 手势状态机 + UI-thread deck | 未采集 |
| 原因 Sheet、教学总结、候选试跑 | repository/service + 页面分支 | 未采集 |
| 默认地图、时间线切换、过滤成果 | 确定性 30 节点模型 + 时间窗测试 | 未采集 |
| Shadow 当前/候选、安静区解释 | 投影读取 + 页面分支 | 未采集 |
| 深色、大字体、小屏、平板 | 深色 token；其余需设备 | 未采集 |

## 已知限制与回滚

- 真实 haptic、55fps、VoiceOver/TalkBack、系统 RTL 和双真机 kill/reopen 未验收；rollout 必须保持关闭。
- 严格隐私“立即删除”模式未交付；默认安静区不会删除消息，避免不可逆误删。
- 工作机会尚无独立 RN 列表，本版只让相关语义参与教学与地图，机会 Tab 仍以现有商机看板为主。
- 偏好同步关闭时，本地事件和全部 L0/L1 能力继续可用；关闭 capability 会隐藏在线同步入口但保留事件
  供恢复。稳定偏好通过 `PreferenceReverted` 回滚，不删除审计历史；服务端表/API/migration 均为 additive。
