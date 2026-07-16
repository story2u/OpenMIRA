# 工作机会发现评估集

本目录只包含完全虚构、使用 `example.com` 的离线样本：来源画像、消息分类、职位提取和确定性匹配。
它用于防止策略与结构化契约回归，不代表 Telegram/企业微信生产数据上的准确率。

运行确定性基线：

```bash
make job-discovery-eval
```

输出中的 `prefilter_baseline` 只衡量进入 Agent 的候选路由，不是最终招聘分类精度。默认不会调用真实
模型，因此 `pi_agent_extraction.status` 为 `not_run`。经人工审阅的 Agent JSONL 可另行评分：

```bash
cd backend
uv run --locked python ../evals/job-discovery/evaluate.py \
  --predictions ../path/to/reviewed-predictions.jsonl
```

预测文件每行需包含夹具 `id`、structured output 的 `job`、`field_evidence`、`compliance_flags`，以及
可选 `duplicate_key`。不得将真实群消息、用户信息、Telegram Session 或 API 凭据加入评估集。

