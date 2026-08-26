# phase1 定稿设计索引

> 阶段:地基与可靠性(迭代 0 + 迭代 1)。已定稿设计在此登记,此后按它执行,改动需走变更。

## 迭代 0 — 最小闭环

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [iter0-min-loop.md](./iter0-min-loop.md) | ✅ 已定稿冻结 | 2026-08-26 | 6 领域表 + task_runs;采集(file 保底 + dongcai 待验证);AI OpenAI 兼容 + 模型分层;Markdown 发布。归档见 `../stages/iter0.md` |
| [iter1-reliability.md](./iter1-reliability.md) | ✅ 已定稿冻结 | 2026-08-26 | 可靠性:事件去重聚类(event_clusters)、raw↔event 对账、重试幂等、增量发布、G1 探针 |

## 待办

- [x] 迭代 0 定稿门确认(D1~D7,用户批准计划即定稿)
- [x] 迭代 1 定稿门确认(D8~D11,按推荐)
- [ ] 迭代 1 实现与验收
