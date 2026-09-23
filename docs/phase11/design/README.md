# phase11 定稿设计索引

> 阶段:事件类多源交叉验证(epic issue #43 的任务卡)。已定稿设计在此登记,此后按它执行,改动需走变更。

## P11 跨源同事件判定与簇内来源可见 ✅ 已实现(2026-09-20)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [event-cross-source.md](./event-cross-source.md) | ✅ **已实现**(dev-only) | 2026-09-20 | issue **#48**(epic **#43** 任务卡 **T2**)。同一真实事件被多个机构源报道 → 聚到一簇 + **簇内可见各源来源**(机构名 + 原文链接 + 上游一级源)。核心 = **归一化加固**(抽取 `【】` 内嵌标题前缀,把被正文稀释的跨源相似度**还原**),**不改阈值 0.7**(红线「扩候选池不降门槛」;实测降门槛不增加专属召回)。**零 schema**(复用 `events.cluster_id` + `raw_documents.source_id/url/extra`)。阈值校准就地实测:163 条 6 源真实标题,0.7 下重复对 **36 → 65**,新增 29 对逐条核对**均为同一事件**。 |
| [event-cross-source.md](./event-cross-source.md) §8~§12 | ✅ **已实现**(dev-only) | 2026-09-20 | issue **#49**(任务卡 **T3**,同一文档续写)。**漏报/冲突检测**:① 单源可见 —— 新增 `source_count`(机构级去重计数;`cluster_sources` 是 `omitempty` **反推不出**单源,会与「未聚类」混淆),单源标「**单一来源**」(中性措辞,**不说可疑**,绝不丢弃事件);② 冲突可见 —— 纯规则数值比对(`internal/cluster/conflicts.go`,单位白名单 + 骨架 Jaccard 门控 **0.6**),**留双方原文**、显性展示,**禁止静默择一**。门控按**生产 1410 条真实事实句**标定:随机句对误报 **0.0750% @0.6**,8 组同标题重复 **0 差异**,对抗集 **0/4**。已知召回边界(远改写 J≈0.47~0.50 落在误报带内,规则不可分)如实登记并**钉测**,留给 #45。**零 schema**;顺修 `deploy.sh` 的 `scripts/` 同步(编排漂移根因)。⚠️ 生产无跨机构簇,99.1% 事件会被标单源 —— **UI 价值取决于 lab 多源采集落地**。 |

## P11 公告独立源(巨潮)✅ 已实现并合并(2026-09-21)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [announcement-source.md](./announcement-source.md) | ✅ **已实现并合并**(PR #63;**未上生产**) | 2026-09-21 | issue **#50**(epic **#43** 任务卡 **T4**,填 **G3** 公告缺口)。巨潮资讯网公告接入日管线采集,`source_type='announcement'` / `status='collected'`;**不进 LLM**(官方披露无真假问题);消息页新增「公告」tab(`GET /api/v1/announcements`)。⚠️ **只存标题 + 原文外链,不存正文** —— 东财正文接口实测被 IP 级限流(300 连发 209 失败;连接复用 60/60 RST),巨潮正文只在 PDF 里。⚠️ **含迁移 `0016`**:`raw_documents` 去重键按「源是否带 `external_id`」分派(公告标题会重名,实测 1377 行只有 1375 个不同标题,标题键会**静默丢行**);**该迁移与 `InsertRawDocument` 的 `ON CONFLICT` 必须成对改**,否则快讯采集全线崩。 |

## C 层 快讯提频(盘中 3 分钟)+ 反封禁护栏 ✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [flash-cadence.md](./flash-cadence.md) | ✅ **已实现**(dev-only) | 2026-09-21 | issue **#68** 数据源分层的 **C 层 = 快讯改用法**(S2 新浪去留 + S3 提频/护栏)。**S2**:用生产聚类同一把尺子(`NormalizeTitle`+`Jaccard`@0.7)实测新浪↔金十近同对仅 **11.2%/11.5% 且对称** → **保留新浪**;「转载≠独立源」修在 **#45 P2 剥转载**根因层,**不得**以「数据无用」删源。**S3**:提频到**盘中每 3 分钟**轮询,硬前置 **per-host 三护栏**(令牌桶 2/s·突发3、空响应哨兵连续3次×0.5封顶×1/8、熔断连续4失败开路60s)。⚠️ 哨兵与熔断**靠跨轮累积**,故采集**必须常驻**(仿 `research-worker`)而非 cron 每 3 分钟拉起 —— 后者护栏形同虚设 + 一天 ~4000 容器;新 `collector` 服务**复用 `piks-tools` 镜像**(仍四镜像),只跑快讯 6 源。连带:worker `-limit 300→800`;去掉「3 连败自动 PauseSource」(3 分钟就停源,且 `PauseSource` 无人读取)→ 改熔断承担 + **真正尊重 `sources.status='paused'`**。**零 schema**。 |

## 数据源分层与采集策略(A/B/C/D)📝 草案,待评审(2026-09-21)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [source-tiering.md](./source-tiering.md) | 📝 **草案,未定稿** | 2026-09-21 | 给已有/新增数据源**分层**并定采集策略。**核查出四处事实前提与代码不符**:① 公告现在**零成本**(T4 `status='collected'` 不进 LLM)→「分级降 80% 成本」不成立,分级降级为**只落标签**;② 巨潮 `announcementType` 实测**不可解**(`announcementTypeName` 逐条 null),分级只能靠**标题规则**;③ 互动平台**无免费全市场入口**(`stock_irm_cninfo` 是 per-stock;`newircs/index/latest`、`search/search` 均 404)→ B 层**暂缓**;④ **热榜源与 `morning-brief` 在代码库中均不存在** → D 层是**从零新建**非「降级」。 |

## S1 公告分级(issue #68)✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [announcement-grading.md](./announcement-grading.md) | ✅ **已实现**(dev-only) | 2026-09-21 | issue **#68** S1 = [source-tiering.md](./source-tiering.md) §3 落地。公告 ~1200 条/日全平铺「看不过来」→ `internal/announce` **纯规则标题分级**(must/important/routine/noise),**只落标签、不进 LLM**(公告零成本,分级无成本收益,价值纯展示层)。判定次序 **否定式→中介衍生件→必读→重要→噪音→常规→默认常规**(前两步不可省:实测「最近五年未被处罚」含 must 词但语义相反、「重大资产重组…核查意见」是券商衍生件)。校准(2026-09-18 单日 **1196** 条):**必读 14 / 重要 226 / 常规 678 / 噪音 278** → 必读+重要 **20.1%**,可折叠 **79.9%**(跨 5 交易日 79.9~87.5%);Go 规则与 Python 校准**逐条一致**。**含迁移 `0017`**(`grade` 列 + CHECK + 部分索引)。前端默认折叠为必读+重要,`gradeMatch` 把 `NULL` 当「常规」→ **只折叠、不隐藏**(红线)。⚠️ `status='collected'` 逐字不变,公告仍不进 LLM。**未上生产**。 |

## D 层 热榜(issue #68)✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [hot-topic.md](./hot-topic.md) | ✅ **已实现**(dev-only) | 2026-09-22 | issue **#68** **D 层 = [source-tiering.md](./source-tiering.md) §6 落地**。source-tiering §1.4 已核实「热榜源与 `morning-brief` **均不存在**」→ **从零新建**。落地:**2 源**(同花顺话题榜 15 条 / 财联社首页热文 13 条,SSR 解析)+ **独立表 `hot_topic_items`**(迁移 `0018`)+ **独立常驻进程 `cmd/hot-topic`**(盘中每 30 分钟)+ **独立页 `/hot-topics`**(导航「发现」组,13 项)。🔴 **方案 A:两源各出各的、不合并、不加权、不排名** —— 实测 615 对标题**同题对 = 0**(J≥0.5 仍 0),根因是粒度不同(题材/事件 vs 文章/复盘),混排必然误导。🔴 **不进 `raw_documents`、不进聚类、不触碰 `events`**:另立 `HotTopicSource` 接口(不复用 `Driver`/`RawNews`)+ 独立进程,把「可被操纵的热度」隔离成**结构性约束**。⚠️ **实测财联社数组顺序 ≠ `readNum` 降序**(榜位 7 = 507051 > 榜位 1 = 304251)→ `rank` 取**数组下标 + 1**、跳行留空号、**不得重排**(名次是事实);SSR 漂移**报错不空成功**(#64 教训);`hot_value` `NULL ≠ 0`。**零 token**、**零 schema 外溢**。**未上生产**。 |

## 自选自动同步(同花顺)✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [watchlist-sync.md](./watchlist-sync.md) | ✅ **已实现**(dev-only) | 2026-09-22 | issue **#87**(承接「截图导入太麻烦 → 要自动化」)。同花顺「我的自选」改**服务器每日 3 次自动拉取**(09:00/12:55/18:00),替代截图导入。**成员资格真源不变**(仍 `entities.status='watch'`,不迁表);**新增表 `watchlist_entries`**(迁移 `0020`)**只补加入价/日** —— 因 `entities.detail` 每轮被 entity-build **无条件覆盖**(本表存在的唯一理由,已钉集成回归)。**常驻 `watch-sync` 服务**(复用 tools 镜像,仍四镜像)重启去重靠 DB(`task_runs.meta.slot`)。🔴 两端无签名 GET(名单 + `selfstock_detail` 得加入价/日);🔴 **code→name 三链回退**(本地 → 同花顺 realhead → deferred,**宁缺毋假绝不拿 code 当名建实体**);🔴 **失败不静默**(空名单/过滤归零/无凭据/登录失败一律 failed,detail 失败降级须可见,#64 教训);🔴 **只读**(绝不向同花顺写);$NULL \ne 0$。✅ **账密登录端到端实测通过**(2026-09-22 探针:四步全通 / 43 项 / 38 A股 / 带价带日各 43 / 取名 38-38);cookie 注入与账密两路均可用(实现上注入优先)。会话实测寿命 ~31 天 ⇒ 登录稳态 ~1 次/月。**未上生产**。 |

## 剥转载:转载 ≠ 独立源(issue #83 P-1)✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [reprint-stripping.md](./reprint-stripping.md) | ✅ **已实现**(dev-only) | 2026-09-23 | issue **#83** 分期 **P-1**(根因层:转载 ≠ 独立源)。`raw_documents` 去重键含 `source_id`,同一篇通稿被两家近逐字转载会落两行 → 按机构名去重的 `source_count` 被刷高 → 印证度三级(单一来源/多家印证/广泛报道)**全废**。落地:**读路径派生**(与 `cluster_sources`/`source_count`/`event_conflicts` 同一范式),新增 `independent_count`(独立来源数)+ 逐来源 `reprint` 标记,**不改 `source_count`(机构数)语义**。🔴 **两条实测反例定死实现形态**:① 不得用**标题**指纹判转载(同事件标题天然收敛,会把独立报道误判成转载);② **不得复用 `NormalizeTitle`**(内含 `stripBracketWrap` 会抽掉【】内正文 → 裸标题与【同标题】正文 J=1.000,实测 **156 对**误判)→ 指纹取 **content 原文**(去电头/HTML)。校准(lab 库 398 多机构簇 / 2735 跨机构对):正文指纹**强双峰**,门控 **0.85**(下沿 0.80 全真转载,0.75~0.80 全改写);电头剥离覆盖 18.3% 成员行、只把真转载抬过门控;@0.85 **50.0%** 簇独立来源 < 机构数、**39.9%** 由「≥2 家」跌到「恰好 1 个独立来源」。残余上限(改写型转载漏判)**如实登记**。**零 schema / 零迁移 / 零管线步骤**。 |

## 事件管线 P-2:数据面地基(issue #83 P-2)✅ 已实现(dev-only)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [event-pipeline-p2.md](./event-pipeline-p2.md) | ✅ **已实现**(dev-only) | 2026-09-23 | issue **#83** 分期 **P-2**(数据面地基,与 P-1 读路径**不同层面**)。issue §三 P4 表列「迁移 8 项」,本篇**复核收窄为 4 列**:🔴 **不做** `raw_documents.origin`(可从 `extra->>'source'` 读,建列是**第二真源**)、`is_reprint`、`events.corroboration`(P-1 已读路径派生;**更根本:转载是簇相对属性,列级布尔不良定义**;corroboration 落库会与 `reexamine` 的 `MergeClusters` **并发漂移**⇒ 两处真源)。✅ **新增 4 列**(迁移 **`0021`**,全部**无 FK**,与决策边同纪律):`raw_documents.origin_kind`(`TEXT NOT NULL DEFAULT 'pipeline'`,**worker/reconcile 的全部 raw 层查询加 `='pipeline'`** —— 今日无 realtime 行,过滤是**契约非优化**,给 P-5 实时层立结构性隔离;⚠️ 去重键(0016)**不含本列**,P-5 须补「realtime→pipeline 提升」)/ `event_clusters.canonical_event_id`(`ApplyClusters` 早已算出却丢弃,此处持久化;**回填比较器与 `canonicalIndex` 逐字一致** —— `created_at ASC`,同则 `confidence DESC`;**代表冻结**:reexamine 不重选)/ `raw_documents.canonical_id`(**只建列**,填充属 P-8)/ `events.human_verdict`(**只建列**,取值域本版不定故**故意不加 CHECK**,**引擎永不碰**)。测试:`canonicalIndex` 单测 5 例 + `ApplyClusters` 落列集成测 + `origin_kind` 门控/**回填**/`human_verdict` 存活三集成测,全绿。**不含部署**。 |



## 阶段序列(epic #43)

| 任务卡 | 内容 | 状态 | 落地 |
|---|---|---|---|
| **T1** 事件多源采集(≥3 独立机构) | 6 机构源 + `extra` 落库 | ✅ 已完成 | PR #46 / `50ddaf3`(迁移 `0014`) |
| **T2** 同事件判定(复用 cluster) | 本篇 §1~§7 | ✅ 已实现(dev-only) | issue #48 |
| **T3** 漏报 / 冲突检测 | 单源标注(机构级)+ 数值冲突双源留原文 | ✅ 已实现(dev-only) | issue #49(本篇 §8~§12) |
| **T4** 公告独立源(巨潮) | 公告链路补独立机构 | ✅ **已实现并合并**(PR #63;**未上生产**) | issue #50([announcement-source.md](./announcement-source.md)) |

## 阶段序列(epic #83:事件管线与展示)

> issue **#83** 是**分期 epic**,按「采集 → 预去重 → 剥转载 → 同事件合并 → 印证分级 → 窗口 → 展示」
> 顺序推进。**P-1 与 P-2 均 dev-only、未上生产**;epic **保持 OPEN**。

| 分期 | 内容 | 状态 | 落地 |
|---|---|---|---|
| **P-1** 剥转载(转载 ≠ 独立源) | 读路径派生 `independent_count`/`reprint`,零 schema | ✅ 已实现并合入 dev | [reprint-stripping.md](./reprint-stripping.md) / PR #92(merge `7ff4da8`) |
| **P-2** 数据面地基(迁移补列) | 迁移 `0021` 四列 + `origin_kind` 门控 + `canonical_event_id` 回填 | ✅ 已实现(dev-only) | [event-pipeline-p2.md](./event-pipeline-p2.md) |
| P-3 ~ P-8 | 规范标题投票 / 实时层 / 展示单元等 | ⬜ 未开始 | 见 `event-pipeline-p2.md` §4 |

## 前序阶段

- **T1**(`docs/数据源总览.md` §2.1.1):6 个独立机构源(东财/金十/财联社/新浪/同花顺/富途),
  `sources.name` 改**机构名**,上游原始字段落 `raw_documents.extra`。本篇的 `origin` 读的就是
  `extra->>'source'`(金十自带的一级源)。
- **聚类质量**(`docs/phase2/design/cluster-quality.md`):「扩候选池不降门槛」纪律 + 重审视 Pass。
  本篇沿用该纪律(阈值 0.7 不变),把召回缺口修在归一化层。
