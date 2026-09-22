# D 层 热榜:独立表 + 独立页(从零新建)

> 阶段:事件类多源交叉验证(epic **#43**)的续篇 issue **#68** 的 **D 层**。
> 前置:[source-tiering.md](./source-tiering.md) §1.4(现状核查)、§6(D 层定性)、§10(红线)。
> 姊妹实现:[flash-cadence.md](./flash-cadence.md)(C 层,常驻采集 + per-host 三护栏)、
> [announcement-grading.md](./announcement-grading.md)(S1,公告分级)。
>
> **本文范围**:热榜源接入、独立表、独立页、采集调度。
> **本文不覆盖**:`morning-brief` 消费者(见 §7「与 source-tiering §6 的偏离」)。

---

## 0. 一句话结论

D 层**从零新建**(source-tiering §1.4 已核实:热榜源与 `morning-brief` 在代码库中**均不存在**,
既有 `market_snapshots.hot_topics` 是 `market-state` 派生的市场热点字段,不是热榜源)。

落地为:**2 个源、1 张独立表、1 个独立常驻采集进程、1 个独立页**——
**不进 `raw_documents`、不进聚类、不触碰 `events`**(§6 红线),
两源**各出各的、不合并、不加权、不排名**(方案 A,实测两源同题对为 0)。

---

## 1. 源选型(实测,勿凭想象增源)

issue #68 评论(2026-09-21)已双端验证。**只有两个源可用**:

| 源 | 端点 | 条数 | 热度值 | 备注 |
|---|---|---|---|---|
| **同花顺话题榜** | `dq.10jqka.com.cn/fuyao/hot_list_data/out/hot_list/v1/topic?stock_type=a&type=day` | **15(硬上限)** | `hot_value` 数值 | 含关联个股(`attach_info`) |
| **财联社首页热文** | 首页 SSR `__NEXT_DATA__.props.pageProps.hotArticleData` | **13(硬上限)** | `readNum` 数值 | 无独立 API,只能解析 SSR |

**其余候选均不可用**(实测,记录以免重复踩):

| 候选 | 结果 |
|---|---|
| 金十 | **无热榜端点** |
| 东财 / 雪球 / 百度 | **403 / WAF / 空** |
| 财联社「话题」 | 是**栏目名**不是事件(如「环球市场情报」)→ 不可用 |

源标识落 `hot_topic_items.source`,枚举**单一真源** = `internal/collector/hottopic.go` 的
`HotSourceThs = "ths-topic"` / `HotSourceCLS = "cls-hot-article"`;
前端显示名单一真源 = `internal/web/api_hot_topics.go` 的 `hotSourceMeta`。

---

## 2. 🔴 方案 A:分层各出各的、不合并、不加权、不排名

**实测依据**(issue #68 评论):用生产聚类同一把尺子(`NormalizeTitle` + `Jaccard@0.7`)
比对两源 615 对标题,**同题对 = 0**(放宽到 J≥0.5 仍为 0)。

**根因**:两源**粒度不同** —— 同花顺是**题材/事件**(「固态电池」、「算力租赁」),
财联社热文是**文章/复盘**(「三大指数集体收涨…」)。它们不是同一件事的两种说法,
**没有可合并的对象**。

→ 方案 A(采用):**结构上**各源一列,前端无法混排。

| | 方案 A ✅ 采用 | 方案 B ❌ 否决 |
|---|---|---|
| 展示 | 两源分列 | 混成一张榜 |
| 名次 | 各源自己榜内名次 | 合成一个跨源名次 |
| 热度 | 各源自己的值 | 归一化/加权后比 |
| 依据 | 同题对 = 0,无可比对象 | 会被实测否证 |

**否决方案 B 的理由**:两源热度量纲不同(同花顺 `hot_value` vs 财联社 `readNum`),
且各源自身尺度也不同(财联社榜内自己就相差 9 倍),跨源加权**没有共同单位**,
任何加权系数都是凭空捏造。

---

## 3. 🔴 红线:热度可被操纵,只作展示

沿用 source-tiering §6 的红线,并落为**结构约束**:

1. **不进 `raw_documents`、不进聚类、不触碰 `events`** —— 独立表 `hot_topic_items`。
   凡把热榜塞进事件链路的改动,都等于把「可被操纵的热度」混进事件印证度。
   → 实现层**刻意不复用** `Driver`/`RawNews` 接口(见 §4.1)。
2. **绝不作为「重要性」判定** —— 热度高 ≠ 重要。UI 顶部固定 `.notice-bar` 声明
   「可被商业力量操纵 / 非重要性判定 / 非推荐」。
3. **不得与印证度合并计算** —— 印证度 `source_count`/`cluster_sources` 与之**正交**
   (印证度 = 几家在报,热度 = 多少人在看)。本层不读任何事件侧字段。
4. **热度仅源内可比** —— 前端每个源各标「仅本榜内可比」;`hot_value` 为 `NULL` 时
   显示 `—`,**不填 0**(0 是一个热度的断言,NULL 是「上游没给」)。
5. **名次是事实,不得重排** —— `rank` = 上游数组下标 + 1。上游数组已按热度降序,
   下标即榜位。若某行因空标题被跳过,后续行**保留原榜位**(出现空号),
   **不得重排** —— 重排会把「上游第 3 名」谎报成「第 2 名」。

> ⚠️ **实测佐证「不得重排」不是洁癖**:财联社 `hotArticleData` 的**数组顺序 ≠ readNum 降序**
> (实测榜位 7 = 507051 > 榜位 1 = 304251)。若按 `hot_value` 重排,会篡改上游榜位。

---

## 4. 实现

### 4.1 采集:`internal/collector/hottopic.go`(刻意另立接口)

```
HotTopicSource { Name() string; Fetch(ctx) ([]HotTopicItem, error) }
      ↑ 与 Driver/RawNews **不同接口** —— 热榜数据在类型上就无法流入事件链路。
NewHotTopicSources() → [thsTopicSource, clsHotArticleSource]
```

**结构漂移须报错而非空成功**(#64 教训):财联社靠解析首页 SSR,结构一变就拿不到数据;
`parseCLSHotArticleHTML` 在**未找到 `__NEXT_DATA__` / JSON 解析失败 / `hotArticleData` 为空**
三种情况下**一律返回 error**,让 `task_runs` 记 `failed`,绝不静默退化成「采到 0 条 = 成功」。

**护栏复用**:C 层 per-host 三护栏(令牌桶 2/s·突发 3、空响应哨兵连 3 次 ×0.5 封顶 ×1/8、
熔断连 4 失败开路 60s)经 `httpSource` **自动生效**(同花顺话题榜走**新 host**
`dq.10jqka.com.cn`,独占一份 per-host 状态,不与快讯互相拖累)。

### 4.2 落库:独立表 `hot_topic_items`(迁移 `0018`)

```sql
id UUID PK · source TEXT · rank INT · title TEXT · hot_value BIGINT NULL
  · url TEXT · extra JSONB · snapshot_at TIMESTAMPTZ · created_at TIMESTAMPTZ
INDEX idx_hot_topic_items_src_snap (source, snapshot_at DESC, rank)
```

- **刻意不叫 `hot_topics`** —— `market_snapshots.hot_topics` 是无关的派生字段
  (涨停行业 top5 + 事件 top3),重名会混淆。
- **无唯一约束**:跨快照重复是**预期的**(热度走势正依赖它)。
- `hot_value` 可 `NULL` —— 上游没给就留 NULL,不猜、不填 0。
- `extra` 存上游原始字段(关联个股 / brief·author·ctime)留档,当前不下发。

**读回**(`ListHotTopicLatest`):按 source 各取**自己最新一批**(`max(snapshot_at) OVER
(PARTITION BY source)`),两源采集时刻差几秒不互相遮盖。

### 4.3 采集调度:`cmd/hot-topic`(常驻)

- **盘中每 30 分钟**(用户 2026-09-22 定):热榜分钟级变化小,30 分钟足够看出走势;
  每次 ~28 条 → 盘中约 11 次/日 ≈ 300 行。
- **必须常驻** —— 与 C 层同理:护栏状态(空响应哨兵、熔断)**靠跨轮累积**,
  一次性进程每轮从零开始,两条护栏形同虚设。
- session 默认 `09:15-15:05`,与 C 层同款 `parseSession`/`inSession`(**刻意重复实现**,
  两个命令各自独立,不跨包共享,避免 cmd 间耦合)。
- 另在 `scripts/pipeline.sh` **补一发收盘后 one-shot**:留一条稳定的「当日收盘态」记录,
  且常驻进程若挂了日管线仍保证每日至少一批。
- 复用 `piks-tools` 镜像(仍四镜像),compose 服务 `hot-topic`,与 `collector` 同形。

### 4.4 接口与前端

`GET /api/v1/hot-topics` → `{sources:[{key,name,note,items:[{rank,title,hot_value,url}]}], snapshot_at}`。

**按源分列下发而非拍平** —— 让前端**结构上无法**跨源混排。即使某源本轮无数据也**占位**
(整列消失会让用户以为这个榜不存在)。

- 页:`frontend/src/pages/hotTopics.tsx`(`/hot-topics`,导航「发现」组第 4 项,共 13 项)
- 组件:`components/hottopics/HotTopicColumn.tsx`(一源一列)+ `HotTopicRow.tsx`
- 布局:非对称栅格 `1.15fr 1fr`(同花顺列略宽,条数多),900px 以下折为单列
- 三态齐备(loading/error/empty);顶部 `.notice-bar` 固定红线声明

---

## 5. 与 C 层的关系(同 issue,不同层)

| | C 层(快讯) | D 层(热榜) |
|---|---|---|
| 去向 | `raw_documents`(`status='raw'`) | **独立表** `hot_topic_items` |
| 进 LLM | ✅ | ❌ |
| 进聚类 | ✅ | ❌ |
| 频率 | 盘中每 **3 分钟** | 盘中每 **30 分钟** |
| 进程 | `collector` 服务 | **独立** `hot-topic` 服务 |
| 接口 | `Driver`/`RawNews` | **另立** `HotTopicSource` |

**为什么独立进程而非并入 `collector`**:热榜与快讯的采集策略/去向/频率全不同
(热榜不进事件链)。合成一个进程会让「把热榜塞进事件链路」在实现上变得太容易 ——
独立进程 + 独立接口 = 把红线变成**结构性约束**。

---

## 6. 验收(dev 侧已完成)

| 项 | 结果 |
|---|---|
| 迁移 `0018` 干净应用(scratch 库 `piks_dtest`) | ✅ |
| 真实端到端采集 | ✅ 15(同花顺)+ 13(财联社) |
| 第二轮采集后接口仍只回**各源最新一批**(共 28 条,非累加) | ✅ |
| NULL `hot_value` 往返保持 NULL | ✅ 集成测试 |
| SSR 漂移三变体(缺脚本 / 坏 JSON / 空列表)**全部报错** | ✅ 单测 |
| rank 空号保留(跳行后 1 & 3,非 1 & 2) | ✅ 单测 |
| 无跨源合并辅助函数(读源码断言无 `MergeHotTopic` 等) | ✅ 守卫单测 |
| `go build` / `go vet` / `tsc --noEmit` / `vite build` / 镜像拓扑检查 | ✅ 全绿 |

---

## 7. 与 source-tiering §6 的偏离(如实记录)

source-tiering §6 的 D 层定性里,**保留**三条:不进 `raw_documents`/不进聚类、独立表、
红线(可操纵/不合并印证度)。**偏离**两条,理由如下:

| §6 原述 | 本次处置 | 理由 |
|---|---|---|
| 「一天两次快照」 | 改为**盘中每 30 分钟**(约 11 次/日) | 用户 2026-09-22 当面决定;热榜是**走势**信号,30 分钟粒度才有意义;量小(~300 行/日)不成负担 |
| 「只给 `morning-brief` 做排序权重」 | **建独立页**,`morning-brief` 不建 | ① `morning-brief` 消费者**不存在**(§1.4 已核实);② 它属 **#45 续篇**(评级排序/事件线)范围,与「数据源分层」是不同议题;③ 只做排序权重=用户看不到任何东西,独立页才让数据有去处。**排序权重仍可作为将来 #45 续篇的消费方**(数据已在表里,`rank`+`hot_value`+`snapshot_at` 齐备) |

> 即:**数据形状按「将来可作排序信号」设计**(rank + hot_value + snapshot_at 连续快照),
> 但**本期消费者是独立页**,不假装 morning-brief 已存在。

---

## 8. 红线汇总

- ⚠️ **热榜不进 `raw_documents`、不进聚类、不触碰 `events`** —— 独立表 + 独立接口(§3.1)
- ⚠️ **热榜可被操纵** —— 只作展示,不作「重要性」判定(§3.2)
- ⚠️ **不与印证度合并计算** —— 二者正交(§3.3)
- ⚠️ **热度仅源内可比** —— 分列展示,不跨源加权/排名(§2)
- ⚠️ **`hot_value` NULL ≠ 0** —— 上游没给就显示 `—`,不填 0(§3.4)
- ⚠️ **名次是事实,不得重排** —— 上游榜位原样保留,跳行留空号(§3.5)
- ⚠️ **结构漂移须报错** —— SSR 拿不到数据返回 error,不静默退化成「0 条 = 成功」(§4.1)
- ⚠️ **不得凭数组顺序 ≠ 热度降序就重排** —— 实测财联社即如此(§3.5)
