# 事件管线 P-3:早/晚档窗口 + 簇内代表选取(issue #83 分期 P-3)

> 状态:**已实现(dev-only)** · 日期:2026-09-23 · 落地:issue **#83** 分期 **P-3**
> 前置:P-1(剥转载,读路径派生 `independent_count`,PR #92)、P-2(数据面地基,迁移 `0021`,
> PR #93)均已合入 dev。本篇推进 issue §四 **P-3 = P1 窗口/打分 + P6 簇内代表选取 → 产出榜单**。
> **零 schema、零迁移、零 LLM 增量。**

## 0. 范围与已锁定决定

issue §四:`| **P-3** | 早/晚档窗口(P1)+ 簇内代表选取(P6)| P-2 | 产出榜单 |`。

与用户锁定的四个决定:

| # | 决定 | 理由 |
|---|---|---|
| 1 | **交付面 = 仅后端**(窗口 + 榜单 API + 代表规则);前端展示留 **P-4** | P-4 才是「展示单元(簇合并视图)」;本期收口数据面 |
| 2 | **P6 新代表规则只对「新簇」生效,不回填存量簇** | 保 P-2「代表冻结」纪律与 `based_on` 决策边不漂移 |
| 3 | **榜单按时间序 + 印证度计数,不排名** | issue P1 红线:「排序信号 = 跨渠道独立报道数,作**印证度标签**,**不排名**」 |
| 4 | **`canonicalTitle` 改无 LLM 规则**(issue 规则 3 升为主规则) | 零 token、确定性;标题取**成员原文**(Fact)而非 LLM 改写(Inference);消除「LLM 标题发散」待验证问题;契合「只做归因」与 #86 减量 |

**产出**:窗口/打分纯函数 + `GET /api/v1/board` + 新代表选取规则 + 标题规则改造 + 测试。

---

## 1. P1 早/晚档窗口

### 1.1 窗口口径(阅读节奏,非热度切口)

| 档 | 窗口 | 时长 |
|---|---|---|
| **late 晚盘** | 当日 09:15 → 当日 18:30 | 9.25h |
| **early 早盘** | 前一日 18:30 → 当日 09:15 | 14.75h |

- **衔接无重叠无缝隙**:`early(d).end == late(d).start == d 09:15`、`late(d).end == early(d+1).start == d 18:30`(单测 `TestBoardWindow` 钉死)。
- **窗口理由 = 阅读节奏**(早上看隔夜+盘前 / 晚上看全天),issue 明确「**非热度切口**」。
- 默认档:请求未给 `stage` 时按**当前北京时刻**选 —— `>=18:30` → `late`,否则 `early`。

### 1.2 窗口锚 `events.created_at`(入库/抽取时刻),不是 `occurred_at`

窗口理由是阅读节奏,锚「**我们何时拿到它**」才对得上读者的时间轴。`occurred_at` 是事件**声称**发生的时间(常缺失、且跨源口径不一),不适合当阅读窗口边界。

> ⚠️ **已知边界(如实登记)**:抽取滞后会把事件推进比原始到达更晚的窗口。因管线按 `pipeline.sh` 收盘后批量跑,**当日采集的事件在 `created_at` 上落在晚间窗口** —— 对早/晚档是合理的(读者次日晨读),但若将来改实时落库需重新评估锚点。
>
> 🔴 **P-5 修订(2026-09-23,已落地)**:上条「重新评估」已发生 —— 实践证明锚 `created_at` 让**早榜结构性恒空**(全链收盘后一次跑 ⇒ `created_at` 全落晚窗;09:15 早档跑出的 `created_at` 又在晚窗内)。P-5 依本条授权**改锚「原始到达时刻」** `COALESCE(rd.published_at, rd.retrieved_at, e.created_at)`,`e.created_at` 仅作 `raw_document_id IS NULL` 兜底。**后果**:早/晚榜成员集合与本文档验收时**不同**。详见 [event-pipeline-p5.md](./event-pipeline-p5.md) §2。

### 1.3 窗口查询:`store.ListEventsInWindow`

- 半开区间 `[start, end)`:`created_at >= $1 AND created_at < $2`。
- **排除 `status='merged'`**(榜单不展示已被并入的重复报道)。
- **不 LIMIT** —— issue P1「窗口内**全部**合并事件(**不截断**)」;日量约百条量级,可控。
- 返回 `EventForAPI`(带来源名/链接/`cluster_id`),与事件流同一投影,供前端与 `toEventItem` 复用。

---

## 2. P1 打分接口(归一化加权,本版不排名)

```go
score = Σ wᵢ · normᵢ(信号ᵢ),   normᵢ = sigᵢ / maxᵢ(窗口内最大值)
```

- 本版权重 `{CrossChannel:1, Entity:0, Watch:0}`(issue P1)。`Entity`/`Watch` 信号本版**恒 0**,接口先立,将来调权重只改 `boardWeights`,**响应形状不变**。
- `maxᵢ = 0` 时该项贡献 0(防除零)。
- 🔴 **本版 `score` 照算并随行下发,但\*\*不用于排序\*\*** —— 榜单按 `created_at` 倒序(issue P1「不排名」)。它是接口预留:引入实体/自选信号后即为排序依据。

---

## 3. 榜单接口 `GET /api/v1/board`

- 参数:`?stage=early|late`(缺省按北京时刻)、`?date=YYYY-MM-DD`(缺省北京今日)。
- 鉴权:`requireAuth`(与全部 `/api/v1` 一致,白名单式)。
- 响应 `apiBoard`:`stage` / `date` / `window_start` / `window_end`(RFC3339)/ `count` / `items[]`。
- 每行 `apiBoardRow` = **内嵌 `apiEventItem`**(与 `/api/v1/events` 单条**结构一致**,前端复用同一渲染)+ 榜单特有 `score`。
- 印证度**标签**(单一来源/多家印证/广泛报道)仍由**前端**从 `independent_count` 派生 —— 后端只给客观计数,不新增中文标签(P-1 既有约定)。

> **与 `/api/v1/hot-topics` 的区别**:热榜是**刻意隔离**的另一张表/另一个进程/另一套接口(红线:热度可被操纵、只作展示、不与印证度合并)。榜单**不碰**热榜数据,只呈现事件层的客观报道计数。

---

## 4. P6 簇内代表选取(新规则,仅对新簇)

### 4.1 代表选取规则

```
有直接链接 > 无链接 → 来源独立性强(非转载) > 弱 → 首发时间最早(同则更高置信)
```

实现:`canonicalIndex(events, comp, meta)` + `betterCanonical`,比较次序即上式。`meta` 为与事件下标平行的 `memberMeta{hasURL, isReprint}`:

- `hasURL`:来自 `raw_documents.url`(`store.ListEventDocMeta`)。
- `isReprint`:由分量内各成员**正文**经 P-1 指纹分组派生(`ReprintFlags(contents, -1)`,原发取组内最小下标,确定性)。

🔴 **不用「渠道数」选代表** —— 那是**簇级**属性,不是成员级(P6 红线)。

### 4.2 建簇期取数:`store.ListEventDocMeta`

`ApplyClusters` 拿到的 `[]model.Event` **只有 `RawDocumentID` 外键**,没有 url/content。故新增按 event-id 集合取 `(url, content)` 的查询(不要求 `cluster_id` 非空,故可用于**建簇前**)。**只对多成员分量取数**(单成员分量代表恒为自己),控制正文取数面。

### 4.3 🔴 存量 / 新簇口径**有意分叉**(不回填)

- **P-2** 曾立硬约束:迁移 `0021` 的回填 SQL 与 `canonicalIndex` **逐字一致**,并预告「P-3 若改 P6 规则,**须自行补迁移重算**」。
- **P-3 决定:不补迁移、不回填**。存量簇的 `canonical_event_id` 保持 P-2 冻结时的旧口径(「最早非 merged」),**只有新簇**用 P6 新规则。
- **理由**:回填会移动既有代表 ⇒ 与「代表选定后**冻结**」纪律冲突,且 `based_on` 决策边(`to_id = research_runs.id` 等)可能随重算漂移。收益(存量簇换个代表)低于风险。
- **后果(必须显式记录,不得静默)**:`canonicalIndex`(新簇)与迁移 `0021` 回填 SQL(存量)**不再逐字一致**。这是**批准的设计决定**,不是漂移。
- **边界**:`reexamine.go` 的 `pickSurvivorIndex`(**簇级** survivor 选举,含 #75 多成员簇护栏)**不改** —— P6 只动簇内**成员级**代表选取,`reexamine` 遵守「survivor 恒为既有簇、代表冻结」(D-Q3)。

---

## 5. `canonicalTitle` 改无 LLM 规则

### 5.1 旧实现的问题

旧实现取「**第一个命中** IsSame pair 的 LLM `canonical_title`」。issue P6 指出:

1. 不满足 prompt 要求的「覆盖面最广」;
2. `canonical_title` 是 LLM **重写**的(Inference),不同 pair 措辞可能发散 ⇒ 投票会退化成「全 1 票」。

### 5.2 新规则(issue 规则 3 升为主规则)

对分量内每个成员 i,算 `score_i = Σ_{j≠i} Jaccard(Bigrams(NormalizeTitle(t_i)), Bigrams(NormalizeTitle(t_j)))`,取 **score 最大**者的**成员原文标题**;平票 → `CreatedAt` 最早;再平 → 分量内先到者(确定性)。重合度全 0 时退化为「最早一条」。

- **零 token、确定性、可复现**;标题是**成员自己的话(Fact)**,不是模型改写。
- 复用既有 `NormalizeTitle`/`Bigrams`/`Jaccard`(与聚类同一把尺子)。

### 5.3 移除死字段 `PairVerdict.CanonicalTitle`

该字段旧用途仅为承载 LLM 重写标题;无 LLM 规则后**无人消费**,一并从结构体、`ConfirmPairs` 的 prompt 输出 JSON、`mock.go` 删除 —— prompt 少要一个字段,省一点输出 token,并避免误导后来者。

---

## 6. 测试

| 测试 | 覆盖 |
|---|---|
| `internal/cluster/canonical_test.go`(单测,重写) | `canonicalIndex` 新规则:有链接胜无链接(压过最早)/ 非转载胜转载 / 链接优先于独立性 / 同级取最早 / 同时间取高置信 / 同时间同置信取首个(确定性)/ 单成员。另 `TestCanonicalTitleOverlap`:最高重合成员胜 / 单成员 / 无共性退化最早 |
| `internal/cluster/canonical_integration_test.go`(集成,更新 + 新增) | `ApplyClusters` 落 `canonical_event_id`(无元数据 ⇒ 退化最早);**新增** `TestApplyClustersRepresentativePrefersURL`:**有链接的更晚成员胜出**,更早无链接者被标 merged |
| `internal/store/pipeline_p3_integration_test.go`(集成,新增) | `ListEventsInWindow` 半开边界(start 含 / end 不含)/ 排除 merged / 按 `created_at` 倒序 / 投影字段齐备;`ListEventDocMeta` 的 url/content 取回与零值分支、空入参 |
| `internal/web/board_test.go`(单测,新增) | `boardWindow` 早/晚边界 + **无缝无叠**;`boardStageNow` 默认档;`boardScore` 归一化 / w=0 项 / max=0 防除零 |

---

## 7. 不做(登记给后续分期)

- ~~**三档调度 + flock + crontab**(P-5)~~:**P-5 已落地** —— `pipeline.sh` 加 `STAGE=early|late`
  (按档闸门/记账/`flock`)+ crontab 4 条,见 [event-pipeline-p5.md](./event-pipeline-p5.md) §4。
- ~~**实时层 `origin_kind='realtime'` + 提升**(P-5)~~:**P-5 已定形** —— **只做 raw 层滚动读**
  (`/api/v1/flashes?hours=N`),**不启用** realtime 写入方,见 [event-pipeline-p5.md](./event-pipeline-p5.md) §1。
- **展示单元 / 簇合并视图 UI**(P-4):**P-4 已落地** —— 新榜单页 `/board` + 详情抽屉簇视图
  (`cluster_title` / `cluster_facts` / `cluster_affected`)+ 来源按机构分组,见
  [event-pipeline-p4.md](./event-pipeline-p4.md) §1/§4。
- **`canonical_id` 的 raw 层分组填充**(P-8):**P-4 已落地** —— `cmd/cluster-raw-link` +
  `store.ListClusterRawSources`,见 [event-pipeline-p4.md](./event-pipeline-p4.md) §2。
- **`human_verdict` 写入口 / UI**(P-3/P-4 尚未定义取值域)。
- **时间衰减**(早盘 14.75h 窗口稀释):issue 待决项,归一化接口**已留位置**。
- **实体 / 自选信号权重**:本版 `w=0`,接口先立。
- **存量簇回填重算**:见 §4.3,有意不做。
- **簇级 LLM 标题调用(phase 2)**:改用无 LLM 规则后不再需要(除非将来要求「更规范的标题」)。
- 🔴 **窗口锚「原始到达时刻」**(P-5 改,P-3 已授权):见 §1.2 修订注;**早/晚榜成员集合因此变化**。

## 8. 验证

- `go build ./... && go vet ./...` 全绿。
- `PIKS_TEST_INTEGRATION=1`(本地 5433)对 `store`/`cluster`/`web` 三包实跑全 `ok`。
- **接口实跑**:本地起 `cmd/web` + 登录 → `GET /api/v1/board?stage=early&date=…` 返回窗口内事件、窗口边界正确、merged 被排除、`independent_count`/`source_count`/`score` 齐备、按 `created_at` 倒序。
- **未做线上实测**(dev 无新簇、无 realtime 行);迁移 `0021` 不受影响(P-3 无迁移),存量 `canonical_event_id` 不被改动。
- ⚠️ **既有 flaky 测试**:`internal/web` 的 `TestTokenSignVerify`(issue #78 / PR #91)有 ~6% 概率误报 —— 篡改 base64 末位有时解码出**相同字节**(43 字符 RawURLEncoding 末位仅 4 bit 有效)。**与 P-3 无关**(`auth.go`/`auth_test.go` 与 dev 逐字相同),应另行开 issue 修。
