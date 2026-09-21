# T4 公告独立源(巨潮资讯网)✅ 已实现并合并(PR #63)

> 阶段:事件类多源交叉验证(epic issue #43 的任务卡 **T4**,顺带 **G3** 公告缺口)。2026-09-21。
> 前置:[event-cross-source.md](./event-cross-source.md)(T2/T3)。

## 1. 目标与结论

**目标**:公告补一个**独立机构源**,填 G3(公告缺口)。

**结论**:接入 **巨潮资讯网**(证监会指定信息披露网站)作为公告源,进**节日管线采集**,
消息页新增「公告」tab。**零 schema 变更之外只加一条迁移(去重键,见 §5)**。

| 决策点 | 定夺 | 依据 |
|---|---|---|
| 采哪家 | **巨潮**(非东财) | 巨潮 `adjunctUrl` 指向**原始 PDF**(法定披露原文,`200/application/pdf`);东财信息页是二次呈现。详见 `docs/数据源总览.md` §2.1.2 |
| 存什么 | **只存标题 + 外链,不存正文** | 巨潮正文只在 PDF 里(`announcementContent` 实测恒空);东财正文接口实测被限流(§3) |
| 是否进 LLM | **不进** | 公告是官方披露,无真假问题;抽取纯属浪费 token |
| Python 侧 | **完全不动** | 研报按需取数(`research/src/providers/announcement/`)原样保留,与本管线**互不影响、不合并** |

## 2. 全链

```
cmd/collector -driver cninfo-announce
  → raw_documents (source_type='announcement', status='collected')
  → GET /api/v1/announcements        (按 source_type 过滤)
  → 消息页「公告」tab:  时间 | 代码 名称 | 标题 | 查看原文↗(PDF)
```

落点:`internal/collector/cninfo_announce.go`(驱动)、`cmd/collector/main.go`(注册 +
`status='collected'`)、`internal/store/raw_documents.go`(`ListAnnouncementsWithSource` +
快讯投影排除 announcement)、`internal/web/api_v1.go`(`handleAPIAnnouncements`)、
`frontend/src/pages/announcements.tsx` + `messages.tsx`(第三个 tab)。

## 3. 🔴 关键实测:公告正文抓不得

三轮探测(本机,非估算):

| 试验 | 结果 |
|---|---|
| 东财正文 `np-cnotice-stock` 300 条 @300ms(HTTP/2) | **209/300 失败** |
| 强制 HTTP/1.1,同样 300 条 | **211/300 失败**(HTTP/1.1 不是解药) |
| 单条孤立请求 @3s ×5 | ✅ 全 200 |
| **连接复用(模拟 Go client)60 连发** | ❌ **60/60 `Connection reset by peer`** |

→ 与 `quotemarket.go` 记录的「东财 push2 对频繁请求返回 SSL RST(IP 级限流)」**同源**。
**降速无用 —— 持续打同一 host 即触发**。故**不做全量正文抓取**;
「看原文」由外链满足(巨潮给的是原始 PDF,体验更好)。

> ⚠️ **留此记录防重蹈**:曾评估「存全文供深研用」(1.38 GB/年),因上述实测**否决**。
> 若将来确需正文,应走**离线 PDF 解析**而非逐条打正文接口。

## 4. 巨潮接口实测(勿凭想象改)

- **必须 POST**:同参数 GET 返回 **500 HTML 页**。故 `internal/collector/http.go` 新增
  `postFormJSON`,并把 `once` 重构为 GET/POST 共用(避免两份退避逻辑漂移)。
- **scope**:`column=szse`(=**仅股票**,非「深市」)+ `plate=szsh`(=深+沪+京 A股)。
  2026-09-17 单日实测:`szsh`=1184 = `sz`625 + `sh`476 + `bj`83;去掉 `column=szse` 会多出 761 条
  基金/债券/港股。
- **`pageSize` 上限 30**(请求 50/100 均只回 30);单日 ~1200 条 = **40 页**,`minGap=1s` ≈ 40s/日。
- **`announcementType` 是不可解数字码**(`01010503||010112||…`),`announcementTypeName` = null,
  字典端点不可得 → `extra` **原样留存、不猜测含义**。

## 5. 去重键(迁移 `0016`,本任务的**要害**)

`raw_documents` 原为 `UNIQUE(source_id, content_hash)`,以**归一化正文**为键。
快讯 `content`=正文,适用;但公告 `content` 是**标题占位**,而**公告标题由交易所模板生成、会重名** ——
实测 2026-09-17 东财 **1377 行只有 1375 个不同标题**,按标题键去重会**静默丢 2 条**
(「关于N沈鼓(601091)盘中临时停牌的公告」等)。

**修法**:按源是否带稳定上游标识分派,不做一刀切:

| 情形 | 去重键 | 影响 |
|---|---|---|
| `external_id IS NULL`(file 保底驱动) | `UNIQUE(source_id, content_hash)` | 快讯行为**逐字不变** |
| `external_id` 非空(快讯各源 + 公告) | `UNIQUE(source_id, external_id, content_hash)` | 同题不同公告不再互撞 |

两条均为 **partial unique index**(`UNIQUE` 视 NULL 互不相同,旧键无法表达「NULL 时退化」)。
**插入侧必须改成无目标的 `ON CONFLICT DO NOTHING`** —— partial index 不能作为推断目标,
且两侧键不同。经核查 `content_hash` 全仓仅用作去重键,无读取方。

> ⚠️ **踩坑记录**:最初只删旧约束、加两条 partial index,结果**快讯采集全线崩**
> (每条插入报 `no unique or exclusion constraint matching the ON CONFLICT specification`,
> 且被 `runOne` 记为 `failed` 后**整体仍报 success**)。**二者必须成对改** —— 这正是
> 本任务最容易被漏掉的一处。

## 6. 状态隔离:`status='collected'`

公告**既不能进 LLM,也不能报对账异常**,而两个既有机制都会误伤:

| status | `worker` 抽取(取 `raw`) | reconcile 告警 | 用途 |
|---|---|---|---|
| `raw` | ✅ | — | 待抽取(快讯) |
| **`collected`** | ❌ | ❌ | **公告** |
| `processed` | ❌ | 报 `processed_no_event` | 快讯已抽取 |

→ 新增 `collected`。`InsertRawDocument` 已支持显式 `Status`,**零 schema 变更**;
`worker` 与 `reconcile` **零改动**(二者天然都不匹配 `collected`)。
`cmd/collector` 按 `SourceType=='announcement'` 传该值。

**实测验证**(scratch 库跑真驱动 + 真接口):
`worker` 待抽取数 = **0**;reconcile `stale_raw` / `processed_no_event` = **0**;
快讯投影条数 = **0**(公告不混入)。

## 7. 已知边界(如实登记)

- **只覆盖个股公告**(`column=szse`):基金/债券/港股公告约占 761 条/日,本任务**不采**。
  若将来需要,加一个 `column=&plate=` 的变体源即可(`extra.page_column` 已留存板块标记)。
- **正文不可检索**:只有标题可搜;深读需点开 PDF。
- **与东财公告通道不合并**:巨潮(本管线,标题级)与东财(研报按需,经 akshare)是两条独立链路,
  互不影响;若将来要「公告双源比对」,需另开任务卡(本 T4 的原始验收含该条,**已于 2026-09-21 从 issue #50 正文改判移除**,改为单源接入)。
- **`pageSize` 硬上限 30** 上游随时可能再收窄;若单日翻页失败,按既有纪律该源连续失败 3 次即暂停。
