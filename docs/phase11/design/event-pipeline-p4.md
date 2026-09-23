# 事件管线 P-4:展示单元(P8)+ 成本粗筛(P7)(issue #83 分期 P-4)

> issue **#83**(事件管线与展示)是**分期 epic**,保持 OPEN。本篇 = 分期表 **P-4**,
> 收口「**数据层合了多家、展示层回退到一家**」这个整条 epic 的核心问题(P8),
> 并落地抽取成本粗筛(P7)。**依赖 P-1(剥转载,读路径)、P-2(迁移 `0021` 四列)、P-3(窗口/榜单/代表规则)**。

## 0. 范围与已锁定决定

**已与用户锁定的四个决定:**

1. **一个 PR 交付 P8 + P7**(照 issue §四分期表,收口)。
2. **P8 来源列表走 raw 层全集** —— 实现 `raw_documents.canonical_id` 的分组与读路径,
   **不做**「本期先用事件层」的妥协(详见 §1)。
3. **P8 前端落点 = 新增榜单页 `/board` + 事件详情抽屉改簇合并视图**。
4. **P7 被粗筛掉的 raw 文档落 `status='deferred'` + `task_runs.meta` 记账**,
   **不 delete、不静默丢**。

**产出**:迁移 `0022`(raw 层转载组回填)+ 新命令 `cmd/cluster-raw-link` + 榜单来源改走 raw 层
+ 新榜单页 + 抽屉簇视图 + P7 粗筛。**一个迁移、零新表、零 LLM 增量**。

## 1. P8 展示单元 = 簇

### 1.1 问题

P-2 之前,「同一真实事件被多家报道」在数据层已经合并(P-1 的 `cluster_id` + `cluster_sources`),
**但展示层回退到 canonical 单条**:事件详情抽屉只显示**一条** raw 抽出来的 facts/affected,
来源列表原则上只列「**已被抽取成事件**」的那几家。于是一个簇在库里是「多家」,用户看到的却是「一家」。
**P8 = 把展示单元从「单条事件」改为「簇」。**

### 1.2 展示单元的三个维度

| 维度 | 取数 | 变化 |
|---|---|---|
| **标题** | `event_clusters.title` → `apiEventItem.cluster_title` | 新增字段,前端 `cluster_title ?? title` |
| **内容**(facts / affected) | 簇内**各成员**的并集 → `cluster_facts` / `cluster_affected` | 新增字段;canonical 单条只是子集 |
| **来源列表** | 机构名 → 该机构**全部**链接(见 §2) | `cluster_sources[]` 增 `urls[]` / `canonical` |

`cluster_facts` / `cluster_affected` **仅在簇内成员 > 1 时下发**(单成员簇的并集恒等于它自己,
徒增载荷);同理 `cluster_sources` 维持 issue #48 的「≥2 家机构才挂」,但**额外**在「raw 层
出现了事件层看不见的机构」时也下发 —— 那正是 P8 要补齐的漏源。

⚠️ **并集按字面去重,不做模糊归并**:模糊归并可能把「净利润 3 亿」与「净利润 3.2 亿」合成一条
而**丢掉分歧** —— 那正是 `event_conflicts`(P-1 T3)该暴露的东西,不在这里静默择一。

### 1.3 计数与转载标记**仍走事件层**(关键取舍)

| 概念 | 真源 | 说明 |
|---|---|---|
| `source_count`(机构数) | 事件层 `ListClusterSources` | 客观计数,不变 |
| `independent_count`(独立来源数) | 事件层 P-1 `IndependentCount`(实时指纹) | **与 P-3 响应口径逐字一致** |
| `reprint`(逐来源转载标记) | 事件层 P-1 `ReprintFlags` | 恰好每组留一个不标 |
| **来源列表的机构集 + URL** | **raw 层全集**(P-4 新增) | 只**变全**,不改变上面的计数与标记 |

**为什么计数不走 raw 层**:计数与转载判定是 P-1 的**实时指纹**产物(簇内各机构代表正文跑分组,
阈值 0.85),是**真源**;raw 层是**回填快照**(见 §2.3),两者可能因回填水位漂移。计数走真源、
列表走全集,是「**宁可多列一个机构,不可多算一票**」—— 多列机构是**信息更全**(如实展示「谁报了」),
多算票是**口径错误**(转载被算成印证)。

### 1.4 🔴 一处诚实登记的边界:`canonical_id` 表达不了「非转载多源」

`raw_documents.canonical_id` 的语义是**转载组代表**(「同一篇稿被多家转发」)。它**无法**表达
「一家发了稿 A、另一家发了稿 B」这种**非转载的多源簇**(两家各自独立报道同一件事,正文不同)。
故:raw 层能**拓宽**机构 + URL 列表(补上「报了这篇稿但没被抽成事件」的机构),
但**不能**替代事件层的独立来源判定 —— 那本来就是 P-1 指纹的活。**如实记录,不过度承诺。**

## 2. raw 层转载组:迁移 `0022` + `cmd/cluster-raw-link`

### 2.1 为什么需要(raw 层 ≠ 事件层)

`raw_documents` 是**采集层的全集**;`events` 是**抽取成功后的子集**。P8 之前的来源列表从
`events` 出发,于是「**报了这件事但没被 LLM 抽成事件**」的机构**看不见**。P-4 从 raw 层出发,
把机构集补全。

### 2.2 回填规则(纯函数 `internal/cluster/reprint_raw.go`)

- **判据复用 P-1 正文指纹 `GroupReprints`(阈值 `0.85`)** —— **不另立第二把尺子**。
  `RawGroups(docs)` 对 raw 文档跑同一分组。
- **成组条件 = 组内 ≥2 个不同 `source_id`**(「同机构两条相同」不成组)。
- **代表选取**:冻结代表优先(已非 NULL 者);否则**最早 `retrieved_at`**,同则 `id` 最小。
- **代表冻结**(与 `canonical_event_id` 同纪律):`SetRawCanonicalIDs` 只写
  `WHERE canonical_id IS NULL` —— 防「今天 A 代表、明天 B 代表」。

### 2.3 新命令 `cmd/cluster-raw-link`

`cmd/` 下**第 12 个**管线命令。Flags:`-window-days`(默认 30)/ `-dry-run`。
投影 raw 文档 → `cluster.RawDocEntry` → `cluster.RawGroups` → `SetRawCanonicalIDs`,
`task_runs.meta` 记 `docs/groups/pairs/rows_changed/window_days/dry_run`。

⚠️ **须在采集后跑**(raw 层批量才有意义);接入 `scripts/pipeline.sh`。**两处转载判据可能漂移**
(事件层实时指纹 vs raw 层回填快照)—— 如实登记,并以「raw linker 产出的组也满足 J≥0.85」的
测试断言一致性。

### 2.4 读路径 `store.ListClusterRawSources`

从 `events.raw_document_id` 出发 → `COALESCE(rd.canonical_id, rd.id)` 取**转载组代表** →
join `raw_documents` 取该组**全部**行 → join `sources` 取机构名。
Go 侧按 `(cluster, source)` 分组:同机构多 URL 全列、只计一票(按 `source` 去重天然满足)。

⚠️ **读路径必须 `COALESCE(canonical_id, id)`**:未回填时退化为「自己就是代表」(合法);
且 `rd.origin_kind='pipeline'` 门控(与 worker/reconcile 同一契约)。

## 3. P7 成本粗筛(`internal/extract/gate.go`)

### 3.1 问题与定位

多源后单日入库 ~180 条,而定长抽取**每条都调 LLM**。预算护栏(`ai_daily_token_budget`)是
**总量**闸;粗筛是**优先级**闸 —— 预算不够时先跑谁。**二者正交,粗筛不替代预算护栏。**

### 3.2 判据(源自带信号,落在 `raw_documents.extra`,迁移 `0014`)

排序键(降序优先级):

1. **必送**:`extra.important=1` 或 `extra.confirmed=1`(金十权威标记);
2. **分级**:`extra.level`(财联社 A/B/C → 3/2/1;富途数值 → 其值);
3. **热度**:`extra.reading_num`(财联社);无则 `extra.like_nums`(新浪);
4. **时间**:`retrieved_at` 越新越靠前。

落 `deferred` 的**仅**在显式开启 `DeferLowSignal` 时:排名超 `Max` **且无任何信号**的,
或超 `Window` 且非必送的。

### 3.3 🔴 默认保守

`cmd/worker` 新增 `-coarse-defer`(默认 **false**)/ `-coarse-max`(默认 0 = 不限)/
`-coarse-window`(默认 0 = 不限)。**默认态只排序、不筛任何文档** —— 越激进越可能把重要消息
挡在 LLM 之外,故每个阈值都取到「只有极端积压才触发」。`ai_daily_token_budget` 仍是主护栏,
粗筛只是**附加**。

### 3.4 `deferred` 语义(与 `failed` 的区别)

- `failed` = **异常**(抽取报错),**进对账**(`ReconFailedRaw`);
- `deferred` = **正常的分流**(这轮先不抽),**不进对账**。
  `internal/store/reconcile.go` 的 status 白名单**不含它**(有意)。
- `error` 列复用为**原因**自由文本;`task_runs.meta` 记 `deferred`(条数)+ `defer_reason`(按原因归并)。
- `ListRawPendingStatus`:默认**不取** `deferred`(否则每轮重复取回、死循环);
  `-retry`(=`includeFailed`)隐含放回,**且重加 `retrieved_at > now() - 1 day` 时间窗兜底**。

### 3.5 白名单审计(issue §2.1 #5 教训:硬编码白名单)

新增 raw `status='deferred'` 值后,逐条过所有 status 白名单位置:

| 位置 | 处理 |
|---|---|
| `store/raw_documents.go` 取数分支 | 增 `deferred` 到 `includeFailed` 分支;**不进**默认分支 |
| `store/reconcile.go` 三查 | **不动** —— deferred 不是异常 |
| `store/raw_documents.go` `ListRawDocumentsWithSource`(快讯流) | **不加** —— 见 §6 待确认 1 |
| `ListAnnouncementsWithSource` | 无关(公告 `status='collected'`) |

## 4. 前端

- **新页 `frontend/src/pages/board.tsx`**(`/board`):早/晚档切换 + 日期,**全部写 URL query**
  (`?stage=&date=`,规范 7)。每行 = **一个簇**(标题 `cluster_title ?? title`),
  复用 `EventTable` 渲染。印证度标签由 `independent_count` 派生(`corroborationLabel`)。
  🔴 **不排名** —— 照接口时间序;页面**显式标注「本版无热度排序,只有印证度标签」**。
- **抽屉簇视图**(`EventDetail.tsx` + 新 `EventContent.tsx`):`cluster_title` 存在时用簇标题;
  facts/affected 取并集(拆 `EventContent.tsx` 守「单文件 ≤150 行」)。
- **`EventSources.tsx` 按机构分组**:
  - 每机构一行 → 该机构**全部** URL 逐个列出;无 URL 的源**如实写「该源无外链」**(绝不拼假 URL);
  - 一级源存在(金十):渠道名与一级源名分区,**链接归属一级源**(「原文来自 新华社」);
  - `reprint` 标注每机构一次;底部如实说明**这一栏含「已抽成事件」之外的机构**。
- **导航**:`navItems.ts` 的「发现」组**首位**增「榜单」`/board`(**13 → 14 项**);`App.tsx` 注册 `/board`;
  `e2e_check.mjs` 补路由。

## 5. 决策依据(以代码为准 —— 三处 issue 事实前提更正)

> 项目纪律「以代码为准,勿凭记忆改方案」。issue #83 正文有三处与代码现状不符,**未照字面实现**:

1. **issue 的 🔴「链接取 raw 层全集」与迁移 `0021` 注释直接冲突**:`0021` 逐字写着
   `canonical_id` 的填充是 **P-8 的活**,「在 P-8 落地前**不得**实现基于本列的读路径」。
   ⇒ **本期就是 P-8**,补上回填是分内之事。但 **dev 库实测 `raw_documents` 376 行、
   `processed 且无事件 = 0`** ⇒ 本期做的是**结构上更正确的取法**,**不是**修好了某个
   丢失的源。**不得**写成「修掉了 N 条来源」。
2. **issue P7 的「预算静默降级」缺口已关闭**:`cmd/worker`/`cmd/cluster` 均已写
   `budget_exhausted`/`guard_disabled`(issue #75 / PR #84)。P7 剩余范围**只有粗筛本身**。
3. **issue 的金十两层描述与代码相反**:`internal/collector/jin10.go` 明写「金十快讯**无逐条原文 URL**」,
   `jin10URL()` 返回的是 `data.source_link` = **一级源的链接**。⇒ 把 `rd.url` 挂在「金十」名下是
   **归属错置**;本期按代码更正:**链接归属一级源**。

## 6. 测试

| 测试 | 覆盖 |
|---|---|
| `internal/cluster/reprint_raw_test.go`(单测,新增) | `RawGroups`:≥2 机构成组 / 同机构两支**不**成组 / 代表取最早(同则 id)/ 单成员不成组 / **冻结代表优先且不改写** / 全冻结无成员 |
| `internal/extract/gate_test.go`(单测,新增) | 空 extra 不 panic / 默认不筛 / 必送排最前 / 分级→阅读数排序 / 仅低信号落 deferred(且带原因)/ 时间窗落 deferred / 字符串型信号 |
| `internal/store/pipeline_p2_guard_integration_test.go`(集成,新增) | `origin_kind` 门控:取数两分支 + reconcile 三查**只见 pipeline**;**含反证**(删谓词后 `ListRawPendingStatus` 取回 realtime 行,见 §8) |
| `internal/store/pipeline_p4_integration_test.go`(集成,新增) | `SetRawCanonicalIDs` 幂等 / 代表冻结反证 / **raw 层全集**(「未抽成事件的机构仍在来源里」= P8 硬约束验收)/ 无 URL 源 / 簇标题 / origin_kind 门控 |
| `internal/web/api_v1_cluster_test.go`(单测,更新) | `TestToEventItemRawLayerSources`(raw 机构「丙」出现)/ 多 URL 全列 / 簇标题下发 |
| 前端 | `npx tsc --noEmit`;`vite build`;`/board` 三态 + 筛选写 URL |

## 7. 已知边界 / 风险(如实登记)

- 🔴 **「可审计 ≠ 可见」**:`deferred` 落在库里**不等于**用户看见了。本期处置 = `task_runs.meta`
  记账 + 任务台账可见 + 本文档登记;**若**将来要求逐条可见,需给快讯流加「已延迟抽取」分区(待确认 1)。
- 🔴 **raw 层收益当前不可验证**(见 §5.1):**不得**写成「修掉了 N 条」。
- 🔴 **两处转载判据可能漂移**:事件层实时指纹 vs raw 层回填快照 —— 已登记 + 测试断言一致性。
- **P7 默认保守**:阈值取到只有极端积压才触发;`ai_daily_token_budget` 仍是主护栏。
- **不越界**:三档调度 + flock(P-5)、实时层(P-5)、`human_verdict` 写入口/UI、时间衰减、
  实体/自选权重(本版 =0)、存量簇 `canonical_event_id` 回填(P-3 已定不回填)。
- **另发现的潜在缺陷(不在本期范围)**:`ReconProcessedNoEvent` 把 `r.title` 扫进非指针
  `Detail string`,`NULL` 标题的 raw 行会崩对账 —— 与 P-4 无关,**另开 issue**。

## 8. 验证

1. `go build ./... && go vet ./...` 全绿。
2. `PIKS_TEST_INTEGRATION=1`(本地 5433)对 `store`/`cluster`/`web`/`extract` 全绿。
3. **反证先验注入**:A 步删掉 `ListRawPendingStatus` 的 `origin_kind='pipeline'` 谓词、
   **回读确认落盘**、确认测试变红,再改回(教训:守卫静默通过可能是两层失败叠加)。
4. **迁移 `0022` 幂等**:对 dev 库连跑两次,第二次零行变更;老行 `canonical_id` 不被改写。
5. **`cmd/cluster-raw-link` 实跑**:dev 库 raw 行 → 打印成组数/机构数/回填行数;核对「同机构两支不组」。
6. **接口实跑**:`GET /api/v1/board` 与 `/api/v1/events`:`cluster_sources` 含 raw 层来源、
   同机构多 URL 全列、无 URL 源在、`independent_count` 与 P-3 一致。
7. **前端**:`npx tsc --noEmit` + `vite build` 绿。
8. **不做线上实测**(lab 无多源簇、无 realtime 行;`processed 无事件 = 0` ⇒ raw 层收益零实例)。
9. **粗筛实跑**:`worker -limit 800 -retry` 于 dev,核对 `deferred` 条数 + meta 记账 + 重跑不重复送。
10. ⚠️ **已知 flaky**:`internal/web` 的 `TestTokenSignVerify`(issue #95,~6% 误报)与本期无关。
