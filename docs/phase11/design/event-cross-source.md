# 跨源同事件判定与簇内来源可见

> 阶段 P11。对应 issue **#48**(epic issue #43 任务卡 **T2**)。状态:**已实现**(2026-09-20,dev-only,未部署)。
> 前置:**T1 已交付**(6 个独立机构源,PR #46 / `50ddaf3`,`raw_documents.extra` 由迁移 `0014` 引入)。
> 关联红线:「**扩候选池不降门槛**」(`docs/phase2/design/cluster-quality.md`)。
> **后续 T3(漏报 / 冲突检测,issue #49)见 §8~§10**(本篇同一文档续写)。

## 1. 问题:多源采集了,但同一件事没被认成一件事

T1 之后快讯有 6 个独立机构源(东财/金十/财联社/新浪/同花顺/富途),同一件真实事件
会被**多家分别报道,措辞各异**。这带来两个缺口:

1. **同一事件多源各占一条 `events` 卡** —— 事件流被同一件事刷屏,「多源印证」的语义
   完全没有兑现(多源采集白做);
2. **端上看不出「几家报道了同一件事」** —— 即便某簇已合并,用户也看不到簇内是哪些机构、
   各自的原文链接在哪。

⚠️ **关键结构事实**(决定本任务的性质):cluster 跑在 **`events`(LLM 抽取后)** 上,
**不是** `raw_documents` 上。多源链路 = `raw_documents`(6 源)→ `worker` 抽取 → `events` → `cluster`。
即多源**不直接**在原始层合并,**先在抽取层各自成 event,再靠 cluster 合并**。
→ 本任务实质是**校准 cluster 在跨源场景下的表现 + 打开簇内来源的读路径**,不是新写一套合并。

## 2. 根因:跨源标题写法差异让相似度坍塌(不是阈值太紧)

各家对同一件事的标题写法分两类:

| 写法 | 机构 | 形态 |
|---|---|---|
| **【标题】+ 正文** | 东方财富 / 新浪财经 | 规范标题放在【】里,后接公告/播报正文 |
| **裸标题** | 财联社 / 金十数据 / 同花顺 / 富途资讯 | 只有标题本身 |

全文 Jaccard 会让**同一事件**的相似度被正文段稀释到阈值以下。**实测**(2026-09-20,
dev 库 163 条 6 源真实标题):

```
东财：「【天山生物：持股5%以上股东拟减持不超3%股份】天山生物(300313.SZ)公告称，持股5%以上
       股东新疆畜牧业集团有限公司计划自公告披露之日起15个交易日后的三个月内…」
财联社：「天山生物：持股5%以上股东拟减持不超3%股份」
                                              ↓ 同一事件，全文 Jaccard = 0.165
```

0.165 << 0.7 → **漏合并**。这是结构性的,**不是阈值偏紧** —— 故修法必须回到归一化
(把因正文稀释而坍塌的相似度**还原**),而**不是**下调 0.7。

## 3. 修法一:归一化加固(抽取【】内嵌标题)

`internal/cluster/cluster.go` 新增 `stripBracketWrap`,在 `NormalizeTitle` 最前置一步:

| 输入形态 | 输出 | 例 |
|---|---|---|
| `【标题】正文` | 取【】内 | 东财/新浪:规范标题在前 |
| `标题【正文】` | 取【】外 | 少数源:标题在前 |
| 无【】/ 未闭合 / 【】内 **<6 字** | 原样返回(不猜测) | 栏目标签 |

**长度门槛(≥6 字)的依据**:实测两处栏目标签行(`【电报解读】` 4 字、`【风口研报·公司】` 7 字)
在加固后与其余 161 行的最高相似度分别只有 **0.040 / 0.059** —— 抽取与否都不影响任何判定。
取「长度」而非「括号内外是否同源」这类跨段推断,是因为前者只依赖本行内容、无需猜测署名与标题的关系。

## 4. 阈值校准(实测,非凭感觉)

**基线 = 未加固的归一化(log-normalize),对比 = 加固后**,同一批 **163 条 6 源真实标题**
(dev 库 `raw_documents.title`,排除历史 `news-flash` 源)。相似度 = 归一化后字符二元组 Jaccard。

| 阈值 | 加固前(对) | 加固后(对) | 新增 |
|---|---|---|---|
| 0.60 | 51 | 80 | +29 |
| **0.70(采用)** | **36** | **65** | **+29** |
| 0.80 | 29 | 57 | +28 |

**0.7 下新增的 29 对,逐条人工核对,全部为同一真实事件**(涉及 13 个真实事件):
天山生物减持 / 新华制药多奈哌齐注册证书 / 立讯精密转债到期 / 良品铺子增资 / 盛和资源澄清 /
四方光电减持 / 埃夫特减持 / 永信至诚减持 / 深水海纳减持 / 日本奄美大岛地震 / 西班牙养老院火灾 /
8 月并购市场报告 / 等。**无一对是不同事件被并**(误合并为 0)。

### 4.1 为何不降门槛(对「扩候选池不降门槛」红线的实测兑现)

两条独立证据:

1. **降门槛不增加专属召回** —— 同一批标题在 0.6 阈值下也只是 51 → 80:加固带来的增量
   (29 对)在 0.7 与 0.6 下**基本相同**,即 0.6 并没有捞出「只有降门槛才能拿到」的重复;
   它只是把候选对整体从 65 推到 80(多 15 对送 LLM 确认 = 白花 token)。
2. **判别边界留有余量** —— 同股但**不同批次**的药品注册证书(新华制药左卡尼汀 / 硫酸镁 /
   多奈哌齐 = 三个**不同事件**)在 0.6/0.7/0.8 各阈值下实测 Jaccard **最高仅 0.515**,
   距 0.7 有 0.185 余量。若降到 0.6 仍不入候选,但余量被吃掉近半;降到 0.5 即触线。
   → 保持 0.7,让判别边界远离误合并。

> ⚠️ **Jaccard 只是候选的两个 OR 分支之一**:另一分支「实体交集 ≥1 且 occurred_at ≤3 天」
> 不受本阈值影响。故「保持 0.7」不等于收紧整体召回,红线也仅约束「不要为了召回而降门槛」。

## 5. 修法二:簇内来源可见(≥2 家才下发)

「簇内可见各源来源」需要读**被并入的成员**(`status='merged'`),因为它们才是跨源证据的来源。

- **`internal/store/events.go`**
  - `EventForAPI` 增 `ClusterID *string`;`ListEventsForAPI` / `ListEventsByIDs` 增选 `e.cluster_id`。
  - 新增 `ListClusterSources(ctx, clusterIDs) map[string][]ClusterSource`,SQL 用
    `DISTINCT ON (e.cluster_id, s.name)` 按**机构**去重(同机构发多条时优先保留**带 url** 的那条),
    `Origin` 取 `raw_documents.extra->>'source'`(金十自带的**上游一级源**,如「新华社」)。
  - ⚠️ 这是**唯一需要读 `merged` 事件的读路径**(其余列表查询一律排除 merged 以免重复卡)。
- **`internal/web/api_v1.go`** —— `apiEventItem` 增 `cluster_sources`(`omitempty`);
  handler 先过滤出 `filtered` 并收集 `cluster_id`,**一次批量**取来源(不逐事件往返),
  再在 `toEventItem` 中**仅当簇内确有 ≥2 家机构时**才挂 `cluster_sources` ——
  单源簇/未聚类事件**不下发**,不谎报「多源印证」。
- **前端**(`frontend/src/`)—— `EventDetail` 的来源区在 ≥2 家时列出「N 家媒体报道了同一件事」
  + 每家机构(各带原文链接,走 `SourceLink` 防死链)+ 「转自 {origin}」+「(无原文链接)」兜底;
  `EventTable` 在标题行加 `· N 家印证` 徽标。文案白话,不出现 cluster/聚类等实现黑话(P6-2 纪律)。

**零 schema**:全部复用既有 `events.cluster_id` + `raw_documents.source_id/url/extra`,**无迁移**。

## 6. 验证

| 项 | 方式 | 结果 |
|---|---|---|
| 阈值校准 | 离线对 163 条真实标题跑 Jaccard(见 §4) | ✅ 36 → 65 @0.7;新增 29 对均为同一事件 |
| 归一化规则 | `TestStripBracketWrap`(8 案例,含栏目标签/未闭合/空串) | ✅ |
| 相似度还原 | `TestCrossSourceSimilarityRestored`(东财 vs 财联社,须 ≥0.7) | ✅ |
| **不误合并** | `TestNoFalseMergeSameStockDifferentAnnouncements`(同股不同批次证书不得成候选) | ✅ |
| 端到端 | `TestCrossSourcePairEndToEnd`(归一化后两侧全同 → 走高置信直合,零 token) | ✅ |
| 跨公司不误并 | `TestCrossSourceNoWrongCompany`(同类型同结构、不同公司不得成候选) | ✅ |
| 簇内来源(真库) | `TestListClusterSources`(隔离 `T2TEST` 数据;merged 成员须出现、同机构去重优先留带 url、origin 带出) | ✅ |
| API 投影 | `TestToEventItemClusterSources`(跨源下发 / 单源省略 / 未聚类省略 / nil 不 panic) | ✅ |
| 静态检查 | `go build ./...`、`go vet ./...`、全 `go test ./...`、前端 `tsc --noEmit` | ✅ |

### ⚠️ 未验证项(如实登记)

- **LLM 确认分支未跑通真实网关** —— 校准期间 AI 网关返回
  `CreditsError: Insufficient balance`(OpenCode Zen),故 0.7 候选对**送 LLM 的实际
  确认结果**未取到真值。本任务所有结论均**不依赖**该分支:归一化后跨源对多数直接落到
  高置信直合(零 token),阈值校准用的是纯规则的 Jaccard。
  建议在网关恢复后跑一次 `cmd/cluster` 实测(dev 库,观察 llm_pairs / merged 计数)。
- 本任务为 **dev-only**,未部署 lab(改动随未来镜像重建生效)。

## 7. 影响面与边界

- **只影响跨源/内嵌标题的归一化**,不改 Auto 与 LLM 的判定逻辑、不改 union-find、不改重审视 Pass;
  `GenCandidates` 仅拆成 `autoGroups` / `llmPairs` 两个纯函数(便于离线取证),行为等价。
- **单源事件行为不变** —— 无 `cluster_id` 或簇内只 1 家机构时,前端与 API 均与改动前一致。
- **不改阈值**(0.7 保持),符合红线。
- 相关后续:**issue #45**(事件状态机制「多源印证分级」直接依赖本任务下发的 `cluster_sources`);
  **issue #49**(T3 漏报/冲突检测)可复用同一来源读路径 —— 本任务已兑现,见 §8~§10。

---

# T3:漏报 / 冲突检测(issue #49)

> 状态:**已实现**(2026-09-20,dev-only,未部署)。**零 schema**。前置 = 本篇 T2(同一来源读路径)。

## 8. 问题:多源之后还剩两个盲区

T2 解决了「同事件认出来」,但两端仍看不见:

1. **单源不可见** —— 只有 1 家机构报的事件,端上完全看不出。它可能是**独家的正常报道**
   (财经快讯里独家极常见),也可能是别家漏了。要把它**标出来让人自己判断**,
   **不是替人下结论**。
2. **冲突不可见** —— 多家报同一事件时,数字若有出入,现在被**静默合并**:canonical 只留一条,
   另一条进 `status='merged'` 就看不见。**这违反 Fact ≠ Inference —— 模型不能悄悄替人挑一个版本。**

### 8.1 红线(issue #49 原文,实现必须逐条兑现)

| 红线 | 兑现方式 |
|---|---|
| 单源判定基准 = **机构**级,不是 row 数 | `ListClusterSources` 已按 `sources.name`(机构名)去重;`source_count` 即去重后机构数 |
| 同机构不同端点**不算**独立源 | 同上去重口径 |
| **「单源」≠「漏报」** | 标签措辞取中性「**单一来源**」;文案明写「独家报道很常见,不代表消息不实」;**绝不丢弃任何事件** |
| 冲突必须**留双方原文**、显性展示,**禁止静默择一** | `Conflict.SentenceA/B` 带两侧原句;前端 `EventConflicts` 两条都渲染,不给「结论版」 |
| 绝不改写 `events` 既有字段 | 标题/facts/status/canonical 一律不动;冲突是**读路径新增字段** |

## 9. `source_count`:`cluster_sources` 反推不出单源

**关键陷阱**:`cluster_sources` 是 `omitempty`,**单源簇与未聚类事件都不下发**。
客户端光看「有没有 `cluster_sources`」**分不清「就 1 家」和「还没聚类」** —— 会把
「还没聚类」误读成「单一来源」。故必须加**显式计数**:

```go
SourceCount int `json:"source_count"` // 簇内**机构**数(去重);未聚类 = 1
```

- `source_count == 1` → 前端标「单一来源」;
- `>= 2` → 保留 T2 的「N 家印证」(`cluster_sources` 同时下发);
- **未聚类按 1 算** —— 确实只有自己那一家。

⚠️ 这是**客观计数**,不是可信度判断。`source_count` 与 `cluster_sources` 的
「有/无」不是同一件事,前端**必须读 `source_count`** 判单源。

## 10. 冲突判定:确定性数值比对(不用 LLM)

**为什么不用 LLM**:判定必须**可复现、可离线复算**。且实现期 AI 网关不可用
(dev OpenCode Zen `CreditsError`,见 §6 未验证项),把判定压在 LLM 上没有必要。

**落点**:`internal/cluster/conflicts.go`(新文件)—— 该包已拥有「给一组事件、导出结构」的
纯函数(`Bigrams`/`Jaccard`/`NormalizeTitle`),`internal/web` 新增 import `internal/cluster`
**无环**(cluster 只 import ai/model/store)。

**规则(逐条)**

1. **数值提取**:`(\d+(?:\.\d+)?)\s*(个百分点|百分点|万亿元|亿元|万元|万股|万手|倍|家|只|元|%|％)`,
   `%`/`％` 归一为 `%`。**单位必须白名单锚定** —— 裸数字会把日期/年份卷进来。
2. **骨架归一**:把「数值+单位」整体删掉(正则 `ReplaceAllString` 成空)。
   ⚠️ **只删数字不删汉字** —— 连汉字一起删会让「回购」vs「增持」骨架全同,
   实测骨架 Jaccard 从 0.40 冲上 1.00 变**误报**。
3. **配对**:A 的每句取 B 中骨架 Jaccard 最高的一句。
4. **门控**:`Jaccard >= 0.6` 才继续(见下方标定);低于此不算「同一句」。
5. **冲突判定**:两侧该单位各**恰好 1 个不同**的值 → 报冲突。任一侧多值(口径不同)
   → **不报警**(宁可漏报不可误报)。
6. 输出带双方**原文**句子(`SentenceA`/`SentenceB`),按归一化单位**去重**(`%`/`％` 只报一次)。

### 10.1 门控 0.6 的标定(生产真实语料,实测算出)

生产库 `piks-postgres`(669 events / **1410 条事实句** / 6580 快讯,镜像 `v0.0.0-616f31b`)。

**随机句对误报率**(1410 句池,20 万次抽样):

| 门控 | 随机句对误报率 |
|---|---|
| 0.0(无门控) | 5.6535% |
| 0.5 | 0.1305% |
| **0.6(采用)** | **0.0750%** |
| 0.7 | 0.0555% |
| 0.8 | 0.0320% |

**同事件不误报**:生产自带 8 组同标题重复事件(极可能同一真实事件的多条报道)共
**39 个句对 → 检出 0 条差异**。

**真阳性(构造用例)**:

| 门控 | 真阳性 | 对抗集真阴性 |
|---|---|---|
| 0.5 | 5/5 | 2/4 |
| **0.6** | **3/5** | **0/4** |
| 0.7 | 3/5 | 0/4 |
| 0.8 | 3/5 | 0/4 |

取 **0.6**:对抗集误报首次归零(0/4),同时保住常见真阳性(跨源转载多为近同一句,
骨架 J≈1.0);再往上(0.7/0.8)真阳性不增、只让改写余量更紧,故取 0.6。

### 10.2 ⚠️ 已知召回边界(如实登记,勿「修」)

「远改写」真阳性中 2 例(骨架 J≈0.471 / 0.500)落在**误报带内**(对抗集 J=0.400~0.500),
**规则不可分**。这是纯 Jaccard 门控的固有边界,靠调门控解决不了 —— 需要 LLM 语义判定,
**留给 issue #45**。`TestDetectFactConflictsKnownRecallBoundary` **钉住**该已知漏报,
防止有人为凑召回擅自下调门控(那会立刻引入对抗集误报)。

### 10.3 「双源 Evidence」= **读路径派生,不落库**

`evidences` 表可写(`internal/store/evidences.go:CreateEvidence`),但它是**抽取时**产物。
冲突是**聚类后跨事件派生**结论 —— 写进去需可重复执行的清理,且把「派生结论」混进
extraction 审计轨迹。故与 T2 的 `cluster_sources` 同范式:**读路径每次派生**。
「留双方原文」由 `Conflict.SentenceA/B` + 各机构 `url` 兑现。

### 10.4 改动清单(全部零 schema)

| 文件 | 改动 |
|---|---|
| `internal/cluster/conflicts.go` **新增** | `Conflict` / `DetectFactConflicts` / 单位白名单 / 骨架归一 / `conflictGate=0.6` |
| `internal/store/events.go` | 新增 `ListClusterMembersWithFacts(ctx, clusterIDs)` —— `ListClusterSources` 不返回 facts,**故意不按机构去重**(冲突要逐成员 facts) |
| `internal/web/api_v1.go` | `apiEventItem` 加 `source_count`(非 omitempty)+ `event_conflicts,omitempty`;`toEventItem` 变 4 参;`handleAPIEvents` 批量取成员算冲突,**默认附带** |
| `frontend/src/lib/types.ts` | `EventItem` 加 `source_count?` / `event_conflicts?` |
| `frontend/src/components/events/EventDetail.tsx` | chip 行加「单一来源」(`.st-dim`,**不复用 `.st-amber`** —— 那是抽取态的色,issue #80 后 `.st-amber` = 已抽取);来源区单源分支加白话说明 |
| `frontend/src/components/events/EventSources.tsx` **新增** | 来源区(单源一行 / ≥2 家列各源),为守 150 行硬规则拆出 |
| `frontend/src/components/events/EventConflicts.tsx` **新增** | 冲突分区:逐条列「对不上的数字」+ 双方原文 |
| `frontend/src/components/events/EventTable.tsx` | 标题行在 `· N 家印证` 旁对称加 `· 单一来源` / `· 说法不一致` |
| `frontend/src/lib/constants.ts` | `SINGLE_SOURCE_LABEL`(不动 `EVENT_STATUS`) |
| `frontend/src/lib/glossary.ts` | 「单一来源」「来源说法不一致」词条 |
| `scripts/deploy.sh` | **同步 lab 侧 `scripts/`**(修编排漂移根因,见 §11) |

**不做**:不改 `events.status`(8 处正向白名单会静默吞事件:`events.go:41,77,131,151,179,203,240`、`search.go:41`、`event_clusters.go:48`);
不加迁移;不改 canonical 选取;**不改写 facts/标题**;不实现 issue #45 的三级分级。
`cmd/cluster` 默认 `-limit 100` 会静默漏聚类(进而把「没跑到」显成「单一来源」)—— **另开 issue #53**(2026-09-21 已修:`-limit` 默认改 0=不限,`truncated` 记入 `task_runs.meta`;见 §13)。

## 11. `scripts/` 编排漂移(实现期发现的根因,随本 PR 修)

**现象**:仓库 `scripts/pipeline.sh` 早已是 `collector -driver all`(6 机构源)+ `worker -limit 300`
(T1 的 `dd05f96`),但 **lab 上的 `/home/rguo/piks/scripts/pipeline.sh` 从未同步**,
仍是 `-driver dongcai` + `worker`(默认 limit 50) → **6 源一条没采**。

**根因**:`scripts/deploy.sh` 只 `scp` `configs/docker-compose.prod.yml`,**从不同步 `scripts/`**。
故编排脚本与代码版本脱钩。

**修法**:deploy.sh 加一段 `scp`,把 **lab 上运行的**脚本(`pipeline.sh`/`backup.sh`/
`health.sh`/`setup.sh`)同步到 `$C/scripts/` 并 `chmod +x`(dev 侧工具 `check-*`/`fix-*` 不上 lab)。
**这是根因修复,不是一次性补同步** —— 此后脚本随部署同步,杜绝再次漂移。

> ⚠️ **本次不执行 lab 侧动作**(用户 2026-09-20 指示「先不升级生产环境,等 issue 处理完」)。
> 本 PR 只落 deploy.sh 的同步逻辑;lab 的多源采集待 issue 合入后再跑。

## 12. T3 验证

| 项 | 方式 | 结果 |
|---|---|---|
| 冲突真阳性 | `internal/cluster` 8 项单测(真阳性/对抗集/改写/单侧多值/边界/空输入/单位最长匹配) | ✅ 8/8 |
| 已知召回边界 | `TestDetectFactConflictsKnownRecallBoundary` **钉住**故意漏报 | ✅ |
| `source_count` 投影 | `TestToEventItemSourceCount`(未聚类=1 / 单源=1 / 跨源=3 / 竞态回退=1) | ✅ |
| 冲突投影 | `TestToEventItemEventConflicts`(有冲突带双方原句;同数改写不报;单源不报) | ✅ |
| 成员 facts(真库) | `TestListClusterMembersWithFacts`(`T3TEST%` 隔离;canonical 与 `merged` 各自独立;空输入→nil) | ✅ 测后零残留 |
| 静态检查 | `go build ./...`、`go vet ./...`、`go test ./... -count=1`、前端 `tsc --noEmit` | ✅ |

### ⚠️ 未验证项(如实登记)

- **生产无跨机构簇** —— 6 源未在 lab 生效(见 §11),故**拿不到「多源冲突」的生产统计**。
  §10.1 的误报率来自生产**真实事实句池**(衡量句子层误报,有效),同事件验证来自生产同标题重复组;
  **「真实 6 机构跨源冲突」的标定需待 lab 多源采集落地后回填。**
- **产品后果**:按 T3 规则,生产 **663/669 = 99.1%** 的事件会被标「单一来源」(生产只 1 个源)。
  标签按设计**正确**,但 99% 命中率等于**噪音而非信号** —— **T3 的 UI 价值取决于 lab 多源采集落地**。
- 本任务 **dev-only**,未部署 lab。

## 13. `-limit` 静默截断修复(issue #53,2026-09-21)

**根因**:`cmd/cluster` 默认 `-limit 100`,`ListUnclusteredEvents` 按 `created_at ASC` 只取最旧
100 条;超出部分**不报错、不告警、不进日志**,`task_runs` 记「成功」。生产实测坐实:
**首 100 条候选 autoGroups=0 / llmPairs=20(几乎全是实体分支噪音),而全量 190 条才出
autoGroups=6 / llmPairs=199** —— 旧窗口**一条真重复都碰不到**。

**修法**(零 schema):
- `cmd/cluster -limit` 默认 **0 = 不限**(取全部未聚类事件);`-limit>0` 仍保留给调试。
- `store.UnclusteredEventsTruncated(limit)`:limit>0 时探测「未聚类事件是否多于 limit」。
- 命中截断时写 `task_runs.meta` 的 `limit`/`truncated`,并往 stderr 打 **WARN**(不再静默)。

**生产量化**(2026-09-21,`190` 条未聚类事件,真实 LLM 确认 `deepseek/deepseek-v4.1-flash`):

| 口径 | 簇数 | 被合并事件 | 仍单条 |
|---|---|---|---|
| 当前生产库(已跑过两次 `-limit 100`) | 6 | 13 | 196 |
| 去掉 limit(全量 190,确定性 `autoGroups`) | 9 | 9 | 172 |
| 去掉 limit(全量 190,**确定性 + LLM 确认** `is_same`) | **13** | **14** | 163 |

⇒ 修复后同一批数据多合出 **7 簇 / +14 条事件**(相较旧的 **+9 簇 / +15 条** 全量口径的确定性部分)。
⚠️ 未修复时被截断的那部分恰是「后 90 条」,其中含金十/富途/新浪对同一事件的多家报道
(如「卡什卡利」系列 6 对完全同题在库中只剩单条)。
