# 事件管线 P-2:数据面地基(迁移补列)(issue #83 分期 P-2)

> 状态:**已实现(dev-only)** · 日期:2026-09-23 · 落地:issue **#83** 分期 **P-2**
> 前置:P-1(剥转载,读路径派生)已合入 dev(`reprint-stripping.md`)。本篇是**数据面地基**,
> 与 P-1 的读路径是**不同层面** —— 本篇只补**非冗余**列,并**打通已有消费端**。

## 0. 范围:8 项 → 1 项已落地 + 3 项刻意不做 + 4 项本迁移新增

issue 正文 §三 P4 表列了「迁移 8 项」。**勿照字面全做** —— P-1 的读路径范式已经把其中几项
变成**冗余**。逐项判定:

| issue P4 原列 | 判定 | 理由 |
|---|---|---|
| `raw_documents.extra` | ✅ 已落地 | 迁移 `0014`(issue #43 T1),上游原始字段原样留存 |
| `raw_documents.origin` | 🔴 **不做** | issue §六 #7 已自证可从 `extra->>'source'` 读到,建列是**第二真源** |
| `raw_documents.is_reprint` | 🔴 **不做** | P-1 已**读路径派生**(`internal/cluster/reprint.go`)。更根本:转载是**簇相对**属性(需 canonicalIdx 才能定「谁转载谁」),**列级布尔本就不良定义** |
| `events.corroboration` | 🔴 **不做** | P-1 已派生 `independent_count`。**持久化会与 `reexamine` 的 `MergeClusters` 漂移** —— 簇成员数一变,印证度就得重算 ⇒ 两处真源 |
| `event_clusters.canonical_event_id` | ✅ **本迁移新增** | `ApplyClusters` 早已算出 canonical 却**丢弃**,此处持久化 |
| `raw_documents.origin_kind` | ✅ **本迁移新增** | 给 P-5 实时层立**结构性隔离契约** |
| `raw_documents.canonical_id` | ✅ **本迁移新增**(只建列) | raw 层转载组代表,**填充属 P-8**,本期无消费端 |
| `events.human_verdict` | ✅ **本迁移新增**(只建列) | 人工标记,**引擎永不碰**,本期无 UI |

## 1. 迁移 `0021_event_pipeline_p2.sql`

4 条 `ADD COLUMN IF NOT EXISTS`(幂等)+ 逐列 `COMMENT ON COLUMN`(照 0016/0019 写明文纪律)。
**全部无 FK** —— `canonical_event_id`/`canonical_id` 指向未来可能消失的行,与既有决策边
`relationships` 同一纪律(无 FK、接错静默悬空,故回填与写入逻辑要保守)。

| 列 | 表 | 类型 | 语义 | 消费端 |
|---|---|---|---|---|
| `origin_kind` | `raw_documents` | `TEXT NOT NULL DEFAULT 'pipeline'` | `pipeline` / `realtime` 分道闸 | worker / reconcile **按 `='pipeline'` 过滤**(P-5 前置契约) |
| `canonical_event_id` | `event_clusters` | `UUID` | 簇的**详情入口事件** | `ApplyClusters` 写入;存量回填 |
| `canonical_id` | `raw_documents` | `UUID` | raw 层转载组代表(NULL=自己是代表) | **只建列**,分组填充属 P-8 |
| `human_verdict` | `events` | `TEXT` | 人工标记 | **只建列**(本期无 UI),引擎永不碰 |

### 1.1 `origin_kind` 的契约(本迁移只为把门装上,今日无实时行)

- `NOT NULL DEFAULT 'pipeline'` 字面量 ⇒ **PG 11+ 仅改元数据,不重写表**;老行自动补齐。
- 加 `CHECK (origin_kind IN ('pipeline','realtime'))`(约束先 `DROP IF EXISTS` 再建,保证幂等)。
- worker(`ListRawPendingStatus`)与 reconcile 的**全部 raw 层查询**一律只认 `origin_kind='pipeline'`。
  今日库中**无 `realtime` 行**,过滤是 `no-op` —— 但它**是契约不是优化**:未来 P-5 的实时层落
  `'realtime'` ⇒ 结构上**无法**被抽进 `events`(worker 只取 pipeline),也无法进对账。集成测试
  钉死该门控(`TestP2OriginKindGate`)。
- ⚠️ **去重键(迁移 `0016` 的两条 partial unique index)不含 `origin_kind`** —— 这是 issue 的
  **有意设计**(跨管线统一去重)。副作用:P-5 必须补一步「`realtime` → `pipeline` 提升」,
  否则先落的 realtime 行会把正式行的 `ON CONFLICT DO NOTHING` 吃掉(该条永远抽不出事件)。
  **本迁移不实现提升** —— 那是 P-5 的活。

### 1.2 `canonical_event_id` 的回填(比较器逐字对齐 `ApplyClusters`)

```sql
UPDATE event_clusters c
SET canonical_event_id = m.event_id
FROM (
  SELECT DISTINCT ON (cluster_id) cluster_id, id AS event_id
  FROM events
  WHERE cluster_id IS NOT NULL
    AND status <> 'merged'
  ORDER BY cluster_id, created_at ASC, confidence DESC
) m
WHERE c.id = m.cluster_id
  AND c.canonical_event_id IS DISTINCT FROM m.event_id;
```

- **比较器与 `internal/cluster/cluster.go` 的 `ApplyClusters`(及抽出的纯函数 `canonicalIndex`)
  逐字一致**:`created_at ASC`,同则 `confidence DESC`。
- 用 `status <> 'merged'` 而非白名单 —— 与 `countActiveMembers` 同口径,新增 status 值时**不漏**
  (issue §2.1 #5 教训)。
- **无在产成员的簇**(已被 `MergeClusters` 吸走)**保持 NULL** —— **合法状态,非漏回填**;
  消费方必须处理 NULL,不得回退猜首个事件。
- 幂等(`IS DISTINCT FROM` 守卫),重跑不改行。

### 1.3 「代表冻结」纪律(与 `canonical_id` 同)

`canonical_event_id` 一经选定即**冻结**:`reexamine` 把更老的簇并入 survivor 时**不重选**代表
(故运行期的 `canonical_event_id` 可能 ≠「最早非 merged 成员」—— 这是**允许的**)。回填只在
迁移时对**存量**算一次。

> 🔄 **P-3 更新(2026-09-23)**:本处原写「P-3 若改 P6 选取规则,**须自行补迁移重算**」。
> **P-3 的实际决定是:不补迁移、不回填,存量冻结。** 只有**新簇**用 P6 新选取规则(见
> [event-pipeline-p3.md](./event-pipeline-p3.md) §4.3)。理由:回填会移动既有代表 ⇒ 与「代表冻结」
> 纪律冲突,且 `based_on` 决策边可能随重算漂移,收益低于风险。
> **后果(必须显式记录,不得静默)**:`canonicalIndex`(新簇)**不再**与本节回填 SQL(存量)逐字一致
> —— 这是**批准的设计决定**,非漂移。存量簇的 `canonical_event_id` 保持本迁移冻结时的旧口径。

这与 P-1 的「代表/簇标题选定后冻结(防漂移)」是同一纪律。

### 1.4 `human_verdict` 的纪律

人工标记,**引擎永不写**:幂等重跑(`MergeClusters` 的 `SET status=CASE…`、`SetEventCluster`
的 `status='merged'`)**都不得触碰本列**。取值域**本版不定**(待 P-3/P-4),故**故意不加 CHECK**
—— 加了会挡住将来的人工取值。集成测试 `TestP2HumanVerdictSurvivesRecluster` 钉死该回归。

## 2. 消费端改动(不只建列)

### 2.1 `internal/model/model.go`(+4 字段)

`RawDocument` 增 `OriginKind`/`CanonicalID`;`Event` 增 `HumanVerdict`;`EventCluster` 增
`CanonicalEventID`。

> ⚠️ `RowToStructByName` 是**严格**模式(pgx v5.10 已核源码:struct 字段无对应列即
> `cannot find field X in returned row`)⇒ **每个 SELECT 的列清单必须同步**,否则运行期报错。
> 故本次把两个共享列常量(`eventCols`/`rawDocCols`)一并改到位。

### 2.2 `internal/store` 消费端

| 文件 | 改动 |
|---|---|
| `raw_documents.go` | `rawDocCols` 末尾加 `origin_kind,canonical_id`;`InsertRawDocument` 增这两列(`OriginKind` 空值经 `defaultStr` 落 `'pipeline'`);`ListRawPendingStatus` **两处 WHERE 加 `AND origin_kind='pipeline'`** |
| `events.go` | `eventCols` 末尾加 `human_verdict`(一处 const,13 个 SELECT 全自动跟上);`CreateEvent` **不传** `human_verdict`(引擎不碰,走 NULL) |
| `event_clusters.go` | `CreateEventCluster` 增 `canonical_event_id` 列与 `$3` 参数;`GetEventClusterByID` SELECT 增该列 |
| `reconcile.go` | raw 层**三处**检查加 `origin_kind='pipeline'`(`ReconStaleRaw`/`ReconFailedRaw`/`ReconProcessedNoEvent`)+ `ListReconDaily` 三处子查询 —— 实时层行**不进对账** |

> ⚠️ `ReconOrphanEvent`/`ReconMissingEvidence` 是**事件层**检查,不动(事件层今日无实时行)。

### 2.3 `internal/cluster/cluster.go`

`ApplyClusters` 把已算出的 canonical **持久化**:

```go
canonical := canonicalIndex(events, comp)          // 抽出的纯函数(可单测)
cid, err := s.CreateEventCluster(ctx, &model.EventCluster{
    Title: title, CanonicalEventID: &events[canonical].ID,
})
```

合并循环改 `for _, idx := range sorted { if idx == canonical { continue } … }`。
代表选择规则**不变**(最早创建、同则更高置信);`canonicalIndex` 与迁移回填 SQL 的比较器
**逐字一致**(单测 `TestCanonicalIndex` + 集成测 `TestP2CanonicalEventIDBackfill` 双重锁定)。

> 🔄 **P-3 更新**:上述「规则不变 / 逐字一致」**只描述 P-2 当时的形态**。P-3 已把**新簇**的代表
> 规则改成 `有链接 > 非转载 > 最早 > 高置信`(`canonicalIndex` 现带 `meta` 参数),且**回填 SQL 不动**
> ⇒ 二者**有意分叉**(存量冻结 vs 新簇新规),详见 §1.3 与 [event-pipeline-p3.md](./event-pipeline-p3.md) §4。

## 3. 测试

| 测试 | 覆盖 |
|---|---|
| `internal/cluster/canonical_test.go`(单测) | `canonicalIndex` 5 例:最早当选 / 同时间取高置信 / 同时间同置信取首个(确定性)/ 单成员 / 最早压过高置信 |
| `internal/cluster/canonical_integration_test.go` | `ApplyClusters` 后读回 `canonical_event_id` = 最早成员,其余 `merged` |
| `internal/store/pipeline_p2_integration_test.go` | ① `origin_kind` 门控(worker 只见 `pipeline`、`realtime` 不可见;reconcile 门控;`rawDocCols` round-trip);② `0021` 回填(最早胜/平局取高置信/全员 merged→NULL/空→NULL/幂等重跑);③ `human_verdict` 跨 `SetEventCluster`+`MergeClusters` 存活 |

集成测试沿用既有双开关(`PIKS_TEST_INTEGRATION=1` + `PIKS_DATABASE_URL`),临时库自建 + `migrate`,
不污染开发/生产数据。

## 4. 不做(登记给后续分期)

- **三档调度 + flock**(P-5 纠缠) —— 不在本篇。
- **簇规范标题投票**(P6) —— **P-3 已落地**:改为**无 LLM 规则**(旧实现是 LLM 重写标题、
  不同 pair 措辞会发散),见 [event-pipeline-p3.md](./event-pipeline-p3.md) §5。
- **`canonical_event_id` 的存量回填重算**(P-3):**有意不做**,见 §1.3 与
  [event-pipeline-p3.md](./event-pipeline-p3.md) §4.3。
- **`canonical_id` 的分组与填充**(P-8):在 P-8 落地前**不得**实现基于本列的读路径
  (会显示 0 个来源,列全为 NULL)。
- **`human_verdict` 的写入口 / UI**(P-3/P-4):本版**只建列**,无消费端,**不得**写成已上屏。
- **`realtime` → `pipeline` 提升**:P-5 的活(见 §1.1)。

## 5. 与 P-1 的边界(为什么 P-1 的技能不是「没做完」)

| | P-1 剥转载 | P-2 本篇 |
|---|---|---|
| 层面 | **读路径派生**(零 schema) | **数据面地基**(迁移补列) |
| 产出 | `independent_count` / `reprint`(派生) | 4 个**写一次**的列 |
| 为何不落 `is_reprint`/`corroboration` | 派生即真源,无漂移 | 落库会与 `reexamine` 的簇合并**并发漂移** ⇒ 两处真源 |

P-1 立红线「代表/簇标题选定后冻结」,P-2 的 `canonical_event_id` 是同一纪律的**持久化落地**。
