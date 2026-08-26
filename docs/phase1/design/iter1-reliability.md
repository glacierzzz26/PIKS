# 迭代 1 — 可靠性 设计文档

> 阶段:phase1(地基与可靠性)。上一迭代 `iter0-min-loop.md`(已冻结)。
> 状态:**已定稿冻结**(D8~D11 均按推荐确认)。按此契约实现,改动需走变更。

## 1. 目标与范围

### 目标
迭代 0 跑通了「采→抽→发」最薄闭环;迭代 1 把**数据可信度**补起来,让系统可以长时间无人值守运行而不污染知识库:

1. **事件语义去重聚类**:同一真实事件被多条快讯反复报道时,聚为一簇,只发布一条规范卡片。
2. **raw↔event 对账**:定期核对「采集了多少、成功抽了多少、失败多少、孤儿多少」,产出如实报告。
3. **任务重试幂等**:失败的采集/抽取可安全重跑,不产生重复。
4. **增量发布**:事件被补证/复核后,对应卡片增量重写,不再「生成一次就定型」。
5. **G1 东财快讯真实接入**(探针验证 + 适配器骨架)。

### 边界(明确不做)
- 不做新增事件类型 / 不做实体抽取(迭代 3)。
- 不做行情、情绪、每日复盘(迭代 2)。
- 不做个人回写 harvest(迭代 4)。
- 不做语义聚类的新增存储后端(继续 PostgreSQL,无向量库)。

## 2. 数据模型变更(迁移 0002)

```sql
-- 2.1 事件簇:同一真实事件的报道集合
CREATE TABLE event_clusters (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title      TEXT NOT NULL,                  -- 簇规范标题(LLM 生成或取 canonical 事件标题)
  status     TEXT NOT NULL DEFAULT 'active', -- active/merged/archived
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2.2 events 增列
ALTER TABLE events ADD COLUMN cluster_id   UUID REFERENCES event_clusters(id);
ALTER TABLE events ADD COLUMN published_at TIMESTAMPTZ;  -- 增量发布锚点
CREATE INDEX idx_events_cluster_id ON events(cluster_id);
CREATE INDEX idx_events_published_at ON events(published_at);
```

说明:
- `cluster_id` 为 `NULL` 表示未聚类(默认);被并入簇的非规范成员,`status` 置 `merged`(不发布)。
- `published_at`:首次发布写入;增量发布比较 `updated_at > published_at` 决定是否重写卡片。
- 迁移 0002 追加式应用,不破坏 0001(迭代 0 已冻结)。

## 3. 命令与组件

### 3.1 `cmd/cluster` — 事件语义去重聚类

**候选生成(纯规则,零成本)**:对未聚类(`cluster_id IS NULL`)且 `status IN ('extracted','verified')` 的事件:
- **高置信自动合并**(不调 LLM):规范化标题完全相同(去空白/全半角/标点归一化) 且 `event_type` 相同 → 直接并入同一簇。
- **中等置信候选**(调 LLM 确认):规范化标题词集 Jaccard ≥ 0.7;或(受影实体交集 ≥1 且 `occurred_at` 相差 ≤3 天)。候选对批量送便宜档模型判断「是否同一事件」。

**LLM 批量确认(省钱核心)**:把候选对聚成 ≤N 对一组(默认 20),一条提示词让模型对每对输出 `{is_same: bool, canonical_title: string}`。只对中等置信候选调用,高置信全规则。

**应用**:确认同一 → 建簇(标题取 LLM canonical 或最早事件标题)→ 成员全部 `cluster_id` 指向该簇;`canonical` = 簇内最早且置信度最高的事件,其余 `status='merged'`(未发布的不发;已发布的在发布器删除旧卡片)。

**护栏**:每轮最多处理 K 个候选(默认 100,防爆量);`task_runs` 记录 `{candidates, llm_batches, merged, tokens}`。

### 3.2 `cmd/reconcile` — raw↔event 对账

只读检查,产出报告写 `task_runs.meta` + 发布 `00-System/recon-{date}.md`(Generated 仓库):

| 检查项 | 口径 | 异常处理 |
|---|---|---|
| 孤儿 raw | `status='raw'` 且 `retrieved_at` 超 7 天 | 报告,不自动改(等人工) |
| 抽取失败 | `raw.status='failed'` | 报告;`worker -retry` 重跑 |
| 抽取空结果 | `raw.status='processed'` 但无 `events` 关联 | 报告(可能是源内容无事件) |
| 孤儿 event | `events.raw_document_id` 指向不存在/被删 raw | 报告(理论不该发生) |
| 缺证据 | `events.status IN (extracted,verified)` 且无 `evidences` 行 | 报告 |
| 静默失败 | 某 active 源近 24h 采集 0 条 | 报告(源健康监控联动) |

报告结构:`## 检查项 / ## 异常清单(每项:实体 id + 简述)/ ## 结论(通过|需关注)`。数据诚实:异常如实列,不掩盖。

### 3.3 任务重试幂等

- 采集器已幂等(content_hash 去重);抽取器已幂等(重复抽取会再建事件?**不会**——同一 raw 只 `status IN ('raw','failed')` 才会被 pick)。
- 新增 `worker -retry`:pick 范围扩为 `status IN ('raw','failed')`(默认仍只 `raw`),重跑失败文档。
- 发布器已幂等(文件存在跳过 + `published_at`)。

### 3.4 增量发布

发布器 `publisher` 选择范围扩为:
- `status IN ('extracted','verified')` 且 `published_at IS NULL`(新发布);
- 或 `updated_at > published_at`(已发布但被补证/复核,增量重写)。

重写时**内容 hash 比对**:卡片 md5 与磁盘一致则跳过(避免 git 噪音)。删除被 `merged` 且已发布的旧卡片(`git rm`)。仅 canonical 事件发布卡片。

### 3.5 G1 东财快讯真实接入

- **探针脚本** `cmd/probe/dongcai.go`:请求东财快讯接口(如 `push2.eastmoney.com` 快讯列表),打印真实响应字段,用于**逐字段对照真实 DTO**(数据诚实:不凭想象写字段)。需要用户侧网络可达。
- 探针验证后:实现 `internal/collector/dongcai.go` 真实驱动(分页、时间游标、归一化 RawNews),替换 stub。
- 若用户环境访问受限,标记 G1 延后到迭代 1 验收后,先保证本地闭环。

## 4. AI 成本策略(延续 §5 分层)

| 环节 | 模型档 | 频率 | 成本控制 |
|---|---|---|---|
| 事件抽取 | `deepseek-chat` | 每快讯 | 已含预算护栏 |
| 去重确认 | `deepseek-chat` | 仅中等置信候选 | 批量提示词(N 对/条)+ 每轮候选上限 K |
| 深度复盘/周报 | `deepseek-reasoner` | 迭代 4 | — |

去重确认默认不新增日预算档位:复用 `PIKS_AI_DAILY_TOKEN_BUDGET`(与抽取同池),保证总成本有上限。

## 5. 验收标准(smoke test)

1. `0002` 迁移后 `events` 有 `cluster_id`/`published_at`,`event_clusters` 建表成功。
2. **去重**:注入 2 条「同一事件、不同措辞」的新闻 → `cmd/cluster` 后 1 簇,仅 canonical 卡片发布,重复卡片删除。
3. **对账**:制造 1 个 `raw.status='failed'` 与 1 个孤儿 event → `cmd/reconcile` 报告如实列出,`task_runs.meta` 有异常计数。
4. **重试**:`worker -retry` 重跑 failed raw → 成功,不产生重复事件。
5. **增量发布**:补证一条已发布事件(`updated_at` 变化)→ publisher 重写该卡片;内容无变化时 git 零提交。
6. **幂等回归**:迭代 0 全部验收项(去重采集、抽取、发布幂等)重跑仍通过。

## 6. 定稿拍板点(需用户确认)

| # | 决策 | 拍板 |
|---|---|---|
| **D8** | 事件去重的存储形态 | ✅ 新增 `event_clusters` 表 + `events.cluster_id` |
| **D9** | 去重确认的 AI 策略 | ✅ 高置信纯规则直合 + 中等置信便宜档批量确认 |
| **D10** | 簇内发布规则 | ✅ 仅 canonical 发布,非 canonical 已发布旧卡片删除 |
| **D11** | G1 东财接入节奏 | ✅ 迭代 1 内做探针验证 + 适配器骨架;受阻则标记延后 |

> 2026-08-26 用户定稿门确认(D8~D11 全按推荐),设计冻结。
