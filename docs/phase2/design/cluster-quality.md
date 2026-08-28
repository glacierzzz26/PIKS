# 聚类质量修复 — 跨簇重复盲区 + 重审视 Pass 设计文档

> 状态:**已定稿冻结(2026-08-28,用户确认)**。范围:`cmd/cluster` 事件聚类去重的质量修复,兑现进度总表「已知遗留 → 质量技术债」的**跨簇重复盲区**项(进度总表 line 99)。
> **约束:只做 dev 验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次不部署、prod DB 无迁移、生产管线行为不变。
> 前置:迭代 1 聚类已上线(规则直合 + LLM 批量确认)。契约依据:`internal/cluster/cluster.go`、`internal/store/events.go`、`cmd/cluster/main.go`。

---

## 1. 背景与现状(dev 实测证据)

dev 库(9 事件)存在 **同一真实事件拆成两个活跃簇** 的漏聚:

| 簇 | 簇标题 | canonical(活跃成员) | merged 成员 |
|---|---|---|---|
| `ac352063` | 央行宣布下调存款准备金率0.25个百分点 | `f34bccb7`(conf 0.90) | `840e4c2b` |
| `f3af76f5` | 央行宣布下调金融机构存款准备金率0.25个百分点 | `4cb21ee3`(conf 1.00) | `35a9d3f5` |

- 两条 canonical **同一真实事件**:`event_type=policy`、`occurred_at` 同为 `2026-08-26 01:30`、受影响实体都含「银行」(预筛条件 `Jaccard≥0.7` 与「实体交集+3 天内」均命中)。
- Web 事件页(`ListEventsForPublishWithSource` 过滤 `status IN ('extracted','verified','published')`)**
  把这两张卡当两条事件展示** → 用户看到重复卡,知识库可信度受损。
- 参考:生产 2026-08-27 运行 `llm_pairs=9 merged=0`(进度总表 line 98)同样疑似漏聚,同一结构性原因。

## 2. 根因分析

`ListUnclusteredEvents`(events.go line 94)只返回 `cluster_id IS NULL` 的事件:

```sql
WHERE cluster_id IS NULL AND status IN ('extracted','verified','published')
```

因此聚类只比较「未聚类事件 ↔ 未聚类事件」,**已聚类 canonical 永不进入候选池**:

1. **跨簇重复**:两个 canonical 各属一个簇,后续聚类运行彼此不可见 → 永久性跨簇重复。
2. **新事件 ↔ 既有簇**:一条报道到达时,若其唯一重复对象已聚类,则该新事件永远留在未聚类池(`cluster_id IS NULL` 恒真),既不会并入既有簇,每轮还被反复送检(浪费)。

两条裂缝同根:候选池缺少「既有 canonical」。

## 3. 方案:重审视 Pass(既有 canonical 重新互检)

在 `cmd/cluster` 正常聚类 pass **之后**追加一个重审视阶段,把候选池扩为 **全部活跃簇 canonical + 剩余未聚类事件**,复用现有 预筛→LLM 确认→并查集 机制,把确认的同簇重复**并入既有簇**(不新建簇)。

```
正常 pass(现状):未聚类事件两两互检 → 建簇/合并成员
        ↓
重审视 pass(新增):池 = {每个活跃簇的 canonical} ∪ {剩余未聚类事件}
        ├─ GenCandidates(同类型 + Jaccard≥0.7 或 实体交集+3天)
        ├─ ConfirmPairs(LLM 批量确认,同一 budget 护栏)
        └─ 应用:同分量内 → 并入既有簇(absorbed 簇整体迁入 survivor 簇)
```

### 3.1 候选池与「代表事件」

- **活跃簇**:`event_clusters.status='active'`。
- **代表事件(canonical)**:簇内 `status IN ('extracted','verified','published')` 的最早创建成员(同则高置信),即簇的代表卡。
- 池 = 所有活跃簇的代表 ∪ `ListUnclusteredEvents`(未聚类事件)。

### 3.2 分量应用规则(防过度合并的关键)

对 `BuildComponents` 输出的每个分量(≥2 成员):

1. **survivor 簇** = 分量内**属于既有簇**的代表中「最早创建,同则高置信」者;若分量内无任何簇成员(理论上不发生,护栏跳过)。
2. 其余成员:
   - 代表另一簇 → **整体并簇**(`MergeClusters`):该簇全部成员 `cluster_id` 改指 survivor,原代表 `status→merged`,簇行 `status='merged'`。
   - 未聚类事件 → 并入 survivor 簇(`SetEventCluster(...,'merged')`)。
3. **保留 survivor 簇标题不动**(vault 卡面稳定,不重写)。

**为什么不过度合并**:判定仍是「同类型 + 预筛命中 + **LLM 确认**」三关,与迭代 1 同阈值;重审视只修「结构性看不到」,不降低判定门槛。

### 3.3 预算与幂等

- 复用迭代 1 的 `PIKS_AI_DAILY_TOKEN_BUDGET` 护栏:正常 pass 消耗后剩余 token 供重审视;剩余 ≤0 则跳过重审视(如实记 meta)。
- **幂等**:确认过的对并入同一簇后,该簇代表只剩一个,下一轮重审视不再生成该对 → 不重复消耗。

## 4. 契约

### 4.1 store(`internal/store/events.go`、`event_clusters.go`)

```go
// events.go
// ClusterRepresentative 活跃簇的代表事件(clusterID + 事件本体)。
type ClusterRepresentative struct {
    ClusterID string
    Event     model.Event
}

// ListActiveClusterRepresentatives 每个活跃簇返回一个代表事件:
// 簇内 status IN ('extracted','verified','published') 的最早创建成员(同则高置信)。
// DISTINCT ON (e.cluster_id) ORDER BY e.cluster_id, e.created_at, e.confidence DESC
func (s *Store) ListActiveClusterRepresentatives(ctx context.Context) ([]ClusterRepresentative, error)

// event_clusters.go
// MergeClusters 把 absorbID 簇整体并入 survivorID 簇:
//   - absorb 簇全部事件 cluster_id → survivor;
//   - 其中非 merged 成员(原代表)status → 'merged'(bump updated_at,触发增量发布删卡);
//   - absorb 簇行 status → 'merged'。
// absorbID 必须 ≠ survivorID;事务内两步。
func (s *Store) MergeClusters(ctx context.Context, absorbID, survivorID string) error
```

### 4.2 cluster(`internal/cluster/cluster.go`)

```go
// ReexamineClusters 重审视既有簇 canonical 与未聚类事件(§3)。返回:并入簇的事件数(新 merged)、消耗 token。
// 复用 GenCandidates + ConfirmPairs + BuildComponents;分量应用规则见 §3.2。
func ReexamineClusters(ctx context.Context, s *store.Store, p ai.Provider, batch int, maxTokens int64) (merged int, tokens int64, err error)

// 纯函数,可单测:
// pickSurvivorIndex 从分量挑 survivor 下标:属于既有簇者按「最早创建,同则高置信」;无簇成员返回 -1。
// clusterOf 与 pool 平行:pool[i] 所属簇 id,未聚类为 ""。
func pickSurvivorIndex(pool []model.Event, clusterOf []string, comp []int) int
```

### 4.3 cmd/cluster(`cmd/cluster/main.go`)

- 新增 flag `-reexamine`(默认 `true`)。
- 正常 pass 结束后调用 `ReexamineClusters`;meta 增补 `reexam_pairs / reexam_merged / reexam_tokens / reexam_skipped_budget`。

## 5. 验收清单(dev-only)

- [ ] 跨簇重复修复:dev 跑 `cmd/cluster` 后,「央行降准 0.25」两簇合并为一簇。survivor 按「最早创建」比较器 = `f34bccb7`(CreatedAt 14:40:32,早于 `4cb21ee3` 的 15:02:47)→ 簇 `ac352063` 存活、`f3af76f5` 整簇并入并标 merged;
- [ ] Web 事件页不再出现两张重复降准卡(被并 canonical 变 merged 被过滤);
- [ ] 无过度合并:星河固态电池簇(正常)、宁德时代(单事件)、异常甲/乙 保持不动;
- [ ] 幂等:重跑一遍 `cmd/cluster`,无新增 merged、无新增 LLM 消耗(或仅零对);
- [ ] 预算护栏:设极低 budget 使重审视跳过 → 如实记 `reexam_skipped_budget`,不报错;
- [ ] 生产未动:未部署 lab、prod 无新迁移/无新代码、生产管线行为不变(代码仅随未来镜像重建生效);
- [ ] 数据诚实:meta 如实反映实际执行(合并数、token、是否跳过)。

## 6. 涉及文件

- 新增:`internal/cluster/reexamine.go`(`ReexamineClusters` + `pickSurvivorIndex`)。
- 修改:`internal/cluster/cluster.go`(无——复用现有函数)、`internal/store/events.go`(`ListActiveClusterRepresentatives`)、`internal/store/event_clusters.go`(`MergeClusters`)、`cmd/cluster/main.go`(接入 + meta)。
- 测试:`internal/cluster/cluster_test.go` 增 `pickSurvivorIndex` 纯函数用例 + 跨簇标题预筛用例(降准双标题生成 LLM 对)。
- 零迁移、零新表、零新 app_config 键(复用 extract 档 + 既有 budget)。

## 7. 边界(明确不做)

- ❌ 不做阈值放宽(维持 Jaccard≥0.7 / 实体交集+3天 / 同类型 + LLM 确认三关);LLM 偏保守问题(llm_pairs=9 merged=0)不靠降门槛解决,靠「结构性可见」解决。
- ❌ 不做 canonical 跨簇重选(如「新事件更早创建则翻盘成 canonical」):本轮 survivor 恒为既有簇,保持最小 churn;列为后续可选。
- ❌ 不做 `merged` 事件纳入检索 grounding(G8 归档已知权衡项,独立质量话题,不在本迭代)。
- ❌ 不做生产部署:不重建 lab 镜像、不触发 prod 管线。

## 8. 决策留痕

| # | 决策 | 理由 |
|---|---|---|
| D-Q1 | 重审视并入 `cmd/cluster`(而非独立命令) | 复用既有调度/budget/task_runs 记账;一次运行同时修「跨簇重复 + 新事件↔既有簇」;零新入口 |
| D-Q2 | 复用迭代 1 预筛阈值 + LLM 确认,不降门槛 | 根因是结构性盲区(候选池缺既有 canonical),不是阈值太紧;降门槛会引入过度合并风险 |
| D-Q3 | survivor 恒为既有簇,标题不动 | 已建簇已有 canonical 与卡面;重选/改名引发卡片重写与 git 噪音,收益低(后续可选) |
| D-Q4 | absorbed 簇行标 `merged` 而非删除 | 保留审计痕迹;`event_clusters.status` 本就含 merged 语义 |
