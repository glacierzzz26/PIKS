# 聚类质量修复 — 重审视 Pass 归档

> 日期:2026-08-28。范围:**仅 dev 环境验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产管线行为不变。
> 设计:`docs/phase2/design/cluster-quality.md`(已定稿冻结,2026-08-28 用户确认)。前置:迭代 1 聚类已上线。

## 交付内容

| 文件 | 改动 |
|---|---|
| `internal/cluster/reexamine.go` | 新增 `ReexamineClusters`(重审视 Pass:既有 canonical ∪ 未聚类事件 → GenCandidates → ConfirmPairs → 并查集 → 并入既有簇)+ 纯函数 `pickSurvivorIndex` |
| `internal/store/events.go` | 新增 `ClusterRepresentative` + `ListActiveClusterRepresentatives`(每活跃簇一个代表事件,DISTINCT ON 最早创建/高置信) |
| `internal/store/event_clusters.go` | 新增 `MergeClusters`(absorb 簇全体成员迁入 survivor + 原代表标 merged + 簇行标 merged,事务内) |
| `cmd/cluster/main.go` | 新增 `-reexamine` flag(默认 true);正常 pass 后调用重审视,沿用日预算剩余额度;meta 增 `reexam_pairs/merged/tokens/skipped_budget` |
| `internal/cluster/cluster_test.go` | 新增 `TestCrossClusterCandidate`(降准双标题生成 LLM 对)+ `TestPickSurvivorIndex`(比较器) |
| `docs/phase2/design/cluster-quality.md` | 设计定稿(根因、方案、契约、护栏、验收) |

零迁移、零新表、零新 app_config 键;复用 `GenCandidates`/`ConfirmPairs`/`BuildComponents`/`SetEventCluster`。

## 根因(简述)

`ListUnclusteredEvents` 只返回 `cluster_id IS NULL` 的事件 → 已聚类 canonical 永不进入候选池:
1. **跨簇重复**:两条 canonical 各属一簇后彼此不可见(dev 实测:央行降准 0.25 拆成 `ac352063`/`f3af76f5` 两簇,Web 显示两张重复卡);
2. **新事件 ↔ 既有簇**:重复对象已聚类的新报道永远滞留未聚类池。

## 验收证据(dev,真实数据)

修复前 dev:9 事件 / 3 活跃簇(降准两簇重复)。

运行 `go run ./cmd/cluster`(正常 pass + 重审视):

```
cluster: events=3 auto_groups=0 llm_pairs=0 clusters=0 merged=1 tokens=357
```

`merged=1` 来自重审视(正常 pass 无候选对;重审视 1 对 LLM 确认,357 tokens)。结果:

- **降准两簇合一**:`f3af76f5`(canonical `4cb21ee3`)整簇并入 `ac352063`(canonical `f34bccb7`,CreatedAt 更早 → survivor);`4cb21ee3` 标 merged、`35a9d3f5` 的 cluster_id 改指、簇行 `f3af76f5` status='merged'。
- **Web 事件页一张降准卡**:`ListEventsForPublishWithSource`(status 过滤)返回降准类事件 1 张(原 2 张)。
- **无过度合并**:星河簇(`a49347d3` active)原样;宁德时代/异常甲/异常乙 保持未聚类。

重跑一遍:`events=3 llm_pairs=0 merged=0 tokens=0` —— 幂等、零新增 LLM 消耗。

预算护栏(临时注入一对测试事件使正常 pass 消耗 token,设 budget=10400 < 已用):

```
DEBUG reexam: budget=10400 maxTokens=37 tokens=374
cluster: reexamine skipped (daily token budget exhausted)
```

task_runs meta:`"reexam_skipped_budget": true`。测试注入全部清理(临时事件/簇/task_runs 行已删,budget 回 0),dev 复原为 9 事件,仅保留预期合并。

## 验收清单

- [x] 跨簇重复修复:降准两簇合一,`f3af76f5` 标 merged(验收见上)
- [x] Web 事件页不再重复(降准卡 2 → 1)
- [x] 无过度合并:星河簇原样、宁德/异常未聚类保持
- [x] 幂等:重跑 0 新增 merged、0 新增 LLM 消耗
- [x] 预算护栏:正常 pass 耗尽预算 → 重审视跳过 + 如实记 `reexam_skipped_budget`,不报错
- [x] 生产未动:未部署 lab、prod 无新迁移/无新代码、生产管线行为不变
- [x] 数据诚实:meta 如实反映实际执行(reexam_pairs/merged/tokens/skipped_budget)
- [x] 单测:`go test ./...` 全过(新增 2 用例);`go vet ./...` 干净

## 已知权衡与说明(数据诚实)

1. **survivor = 既有簇,不跨簇重选 canonical**:被并 canonical(如高置信 `4cb21ee3`)成为 merged,其卡面内容并入 survivor 簇标题;更优标题暂不迁移(避免卡面重写/git 噪音),列为后续可选。
2. **阈值不降**:仍为「同类型 + Jaccard≥0.7 或 实体交集+3 天 + LLM 确认」三关;重审视修的是结构性盲区,不因 LLM 偏保守而降低门槛。
3. **预算护栏场景**:dev 正常 pass 常为 0 对,跳过分支需注入消耗才能触发;生产重消耗日(如 `llm_pairs=9`)剩余不足时自动顺延,属保护行为。

## 遗留/后续(不做)

- canonical 跨簇重选/标题迁移(见上,可选增强)。
- LLM 判定偏保守的阈值标定(进度总表 line 98 原话:需真实重复样本统计,独立质量话题)。
- `merged` 事件纳入检索 grounding(G8 归档已知权衡项,独立质量话题)。
