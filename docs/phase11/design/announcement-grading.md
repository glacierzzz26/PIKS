# A 层公告分级(标题规则 + 标签落库 + 前端折叠)✅ 已实现(dev-only)

> 阶段:数据源分层(issue **#68** S1;承 issue #45 收口 + epic **#43** T4)。
> 2026-09-21。**上游设计**:[source-tiering.md](./source-tiering.md) §3(A 层策略)、§9(待决 1/3 由本篇收口)。
> **前置**:[announcement-source.md](./announcement-source.md)(T4 公告链路;`status='collected'`)。

## 1. 目标与结论

**目标**:公告实测 ~1200 条/交易日(2026-09-15 实测 1800 条),消息页「公告」tab**全量平铺**,
用户「看不过来」。给公告按**重要性分档**,让**必读+重要**先出来、其余可一键展开。

**结论**:新增 `internal/announce` **纯规则标题分级**(must/important/routine/noise),
**只落标签**、**不进 LLM**(公告现状成本本就是 0),前端**默认折叠**为必读+重要。

| 决策点 | 定夺 | 依据 |
|---|---|---|
| 分级依据 | **标题关键词规则**(非 LLM、非类型码) | 巨潮 `announcementType` 不可解(`announcementTypeName` 逐条 null);规则=确定性,同 P7 纪律 |
| 是否进 LLM | **否** | 公告 `status='collected'` 本就不进 worker(§3.1);分级**不产生成本收益**,价值纯在展示层 |
| 落点 | **新增 `raw_documents.grade` 列**(迁移 `0017`) | 可查询/排序/建索引;塞 `extra` 虽零 schema 但检索难(source-tiering §9 待决 1) |
| 折叠语义 | **只折叠、不隐藏** | 红线(source-tiering §3.3):规则误判一条 = 用户永远看不到,故必留「查看全部」入口 |
| 未分级行 | **按「常规」显示** | 历史行 / 非公告源 `grade=NULL`;默认归常规,任一视图都不消失 |

## 2. 全链

```
cmd/collector -driver cninfo-announce
  → announce.Grade(title)                       ← internal/announce/grade.go
  → raw_documents (status='collected', grade=must|important|routine|noise)
  → GET /api/v1/announcements[?grade=...]       ← internal/web/api_v1.go gradeMatch
  → 消息页「公告」tab: 默认只显必读+重要 + 「查看全部（另有 N 条）」   ← announcements.tsx
```

落点:

| 环节 | 文件 |
|---|---|
| 规则引擎 | `internal/announce/grade.go`(判定次序 + 6 张关键词表)、`grade_test.go` |
| 迁移 | `migrations/0017_raw_document_grade.sql` |
| 采集 | `cmd/collector/main.go`(`isAnnounce` → `status='collected'` + `grade`) |
| 存储 | `internal/store/raw_documents.go`(`rawDocCols`、INSERT 10 参、`RawDocAnnouncement.Grade`、`ListAnnouncementsWithSource` 选 `rd.grade`) |
| 模型 | `internal/model/model.go`(`RawDocument.Grade *string`) |
| API | `internal/web/api_v1.go`(`apiAnnouncement.Grade`、`grade` 查询参、`gradeMatch`)、`api_v1_announce_grade_test.go` |
| 前端 | `lib/constants.ts`(`ANNOUNCE_GRADES`/`ANNOUNCE_GRADE_FALLBACK`/`announceGradeLabel`)、`components/events/{AnnounceRow,GradeFilterBar,AnnounceSearch}.tsx`、`pages/announcements.tsx` |

## 3. 分级规则(`internal/announce/grade.go`)

### 3.1 判定次序(**不可调换**,顺序错了会误分级)

```
否定式(neg) → 中介机构衍生文件(intermediary) → 必读(must) → 重要(important)
→ 噪音(noise) → 常规(routine) → 默认常规
```

**为什么 neg / intermediary 必须前置**(2026-09-18 单日实测踩出的两个坑):

| 坑 | 实例 | 若不前置会怎样 |
|---|---|---|
| **否定式样板** | 「最近五年**未被**证券监管部门和交易所采取监管措施或处罚情况的公告」 | 含「处罚」→ 误判**必读**(单日 3 条) |
| **中介机构衍生文件** | 「…关于**重大资产重组**部分限售股份解除限售上市流通的**核查意见**」 | 含「重大资产重组」→ 误判**必读**;居然智家一家就有 **4 份**不同券商核查意见占满必读位 |

> 「法律意见」而非「法律意见书」 —— 实测标题多作「…之法律意见」,**用「法律意见」做前缀子串**才盖得住。

### 3.2 四档与关键词

| 级别 | 含义 | 关键词(节选) |
|---|---|---|
| **必读** `must` | 监管动作 / 退市风险 / 重大重组 / 控制权变更 | 立案告知·退市·风险警示·监管函·警示函·处罚·纪律处分·重大资产重组·控制权变更·要约收购·破产·重整·资金占用·违规担保·市场禁入 …(32 词) |
| **重要** `important` | 股权激励 / 回购 / 增减持 / 重大合同 | 股权激励·限制性股票·回购·增持·减持·重大合同·中标·框架协议·战略合作·向特定对象发行 …(14 词) |
| **常规** `routine` | 定期报告 / 三会决议 / 权益分派 | 年度报告·半年度报告·季度报告·董事会决议·股东大会·权益分派·分红·业绩快报 …(15 词) |
| **噪音** `noise` | 工商变更 / 独董述职 / 中介衍生件 | 工商变更·独立董事·述职·名称变更·经营范围·迁址·H股公告 …(9 词) |

⚠️ **改关键词表必须重跑校准**(`grade_test.go` 的分布用例会拦占比漂移)。

### 3.3 校准实测(2026-09-18,巨潮单日 **1196** 条真实标题)

| 级别 | 条数 | 占比 |
|---|---|---|
| 必读 | 14 | 1.2% |
| 重要 | 226 | 18.9% |
| 常规 | 678 | 56.7% |
| 噪音 | 278 | 23.2% |

→ **必读+重要 = 240 条(20.1%)**,可折叠 **956 条(79.9%)**。
**跨 5 个交易日**实测可丢弃率 **79.9%~87.5%**(source-tiering §3.2 的「噪音≈80%」方向**成立且已量化**)。

> ⚠️ **source-tiering §3.2 的两处口径警告已收口**:
> ① 「半年度 111」的**污染已修** —— 规则用「半年度报告」全称 + `intermediary` 前置把「摘要/意见书/鉴证」挡掉;
> ② 「精确比例未验证」**已量化**(上表)。**占比随样本日波动**,故单测断言取**区间 75~90%** 而非定值。
>
> ⚠️ **Go 规则与 Python 校准脚本逐条比对结果完全一致**(14/226/678/278),不存在两套实现漂移。

## 4. 落库(迁移 `0017`)

```sql
ALTER TABLE raw_documents ADD COLUMN IF NOT EXISTS grade TEXT;
ALTER TABLE raw_documents ADD CONSTRAINT raw_documents_grade_check
  CHECK (grade IS NULL OR grade IN ('must','important','routine','noise'));
CREATE INDEX idx_raw_docs_grade ON raw_documents(grade) WHERE grade IS NOT NULL;
```

- **`grade IS NULL` = 未分级**:非公告源(快讯各源)**不带**该列;公告源迁移前的历史行亦为 NULL。
- 采集侧:仅 `SourceType=='announcement'` 时赋 `grade`(`cmd/collector/main.go`),快讯源留空。
- ✅ **`status='collected'` 逐字不变** —— 公告仍不进 worker、不报对账异常(T4 的隔离未被分级触碰)。

## 5. API 与前端

### 5.1 `GET /api/v1/announcements`

- 响应 `apiAnnouncement` 新增 **`grade`**(`omitempty`;空=未分级)。
- 新增查询参 **`grade=must|important|routine|noise`**;`all`/空 = 不过滤。
- ⚠️ **`gradeMatch` 把 `NULL` 当「常规」** —— `grade=routine` 查询**同时放行未分级行**。
  理由:历史行不能因为「没分级」就从「常规」视图里消失(**宁可多显示,不可误隐藏**,红线 §3.3)。

### 5.2 前端(`pages/announcements.tsx`)

- **默认只显必读+重要**;`GradeFilterBar` 给「查看全部（另有 N 条）」一键放开(`hidden` 计数显式告知折叠了多少条)。
- `AnnounceRow` **只在 must/important 上挂级别 chip** —— 常规/噪音是默认档,逐行挂标签只会制造噪音;低级别靠分组标题体现。`gradeOf()` 与后端 `gradeMatch` **同口径**(空/未知 → 常规)。
- **诚实标注**:筛选条写明「级别按标题规则自动判断,**非官方认定**;标「噪音」的**只收起、未删除**」。
- 折叠是**客户端**行为(后端一次性全量下发),沿用本项目 `usePagedQuery` 客户端切片的既有约定。

## 6. 验证记录

| 层 | 验证 | 结果 |
|---|---|---|
| 规则 | `go test ./internal/announce` | ✅ 逐条钉住四档 + 三处易错点 + 分布区间 |
| API | `go test ./internal/web`(`gradeMatch`) | ✅ 含「未分级不隐藏」反向用例 |
| 真库往返 | `store_test` 集成测(`PIKS_TEST_INTEGRATION` 门控) | ✅ 分级行读回 `must`、未分级行读回 NULL |
| 迁移 | scratch 库全量套用(17 个迁移) | ✅ CHECK 约束拒绝非法值 |
| **E2E**(真驱动 + 真接口) | `cmd/collector -driver cninfo-announce` 落 scratch 库 | ✅ `new=151`;`routine 107 / noise 24 / important 18 / must 2`(**必读+重要 13.2%**);`status='collected'` 151 条**全保持**;必读桶(广和通重大资产重组 / *ST发展预重整)与噪音桶(工商变更/独董离任/保荐代表人变更/律所法律意见)语义**逐条抽查正确** |
| **E2E**(折叠契约) | 插一条 `grade=NULL` 历史行后查 API | ✅ `grade=routine` 由 107 → **108**(历史行不消失);默认视图(must+important)**仍 20**(历史行不混入) |
| 前端 | `tsc --noEmit` + `vite build` | ✅ 零错,无循环依赖(已抽 `AnnounceSearch` 破环) |

## 7. 红线

- ⚠️ **只折叠、不隐藏** —— 必留「查看全部」入口;分级失败/未命中**默认落「常规」**(不落噪音)。
- ⚠️ **分级是机器判定(Inference)不是事实** —— UI 必须如实标注「按标题规则自动判断,非官方认定」(同 P7 量价形态)。
- ⚠️ **不得因「降成本」给公告分级** —— 公告现在零 token 成本(source-tiering §1.1),该论据不成立。
- ⚠️ **不得凭 `announcementType` 臆断类别** —— 类型码未解,只用标题规则。
- ⚠️ **`status='collected'` 不得被分级改动** —— 将来若要「必读+重要进抽取」是**独立改判**,须同步改 worker 取值口径 + reconcile 口径(不在本期)。

## 8. 已知边界(如实登记)

- **分级只覆盖个股公告**:非个股公告(基金/债券/港股 761 条/日)本就未采(source-tiering §9 待决 2)。
- **规则是标题级、有误判**:否定式与中介衍生件是两个已修的大坑,但长尾误判不可完全避免 —— 这也是「只折叠不隐藏」红线存在的原因。
- **占比随样本日波动**(79.9%~87.5%),故不承诺固定比例。
- **未上生产**:本篇为 dev-only;lab 侧需重跑 `collector -driver cninfo-announce` 才会给存量公告补 `grade`(历史行 NULL 按常规显示,不阻塞)。

## 9. 依据

- 上游设计:[source-tiering.md](./source-tiering.md) §3(A 层)、§9(待决 1/3 收口)
- 公告链路:[announcement-source.md](./announcement-source.md)(T4);`docs/数据源总览.md` §2.1.2
- 规则/落点:`internal/announce/grade.go`、`migrations/0017`、`cmd/collector/main.go`、`internal/web/api_v1.go`
- 现状清册:`docs/数据源总览.md`;进度:`docs/进度总表.md`
