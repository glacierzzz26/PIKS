# 迭代 3 — 实体补全 归档

> 状态:**已完成**。本阶段只读,续接请看 `进度总表.md` 与 `../../phase2/design/iter3-entities.md`。
> 日期:2026-08-26~27。实现以 `phase2/design/iter3-entities.md` 定稿契约为准(D18~D23)。

## 1. 目标与交付物

把「事件 + 行情」升级为「事件 + **实体** + 行情」:从事实层(events.affected / 东财涨停池)沉淀可链接、可查询的实体对象(Company/Industry/Concept/Topic),闭环:事件卡 affected 变可跳转 wikilink、hot_topics 补 event_ids、实体知识卡。**本迭代唯一 AI 调用 = 未匹配 affected 词的一次便宜档批量分类**(首跑触发,重跑全匹配归零)。

| 交付物 | 状态 |
|---|---|
| `0004_entities.sql`:entities 表(单表 type 判别 + detail JSONB)+ 索引 | ✅ |
| `internal/entityextract`:便宜档批量分类(company/industry/concept/topic)+ 单元测试 | ✅ |
| `cmd/entity-build`:种子源 zt_pool→Company/Industry+belongs_to;affected 收割/匹配/分类/unknown 诚实落点 | ✅ |
| `internal/publish/entities.go`:实体卡渲染 + TermResolver(affected→wikilink) | ✅ |
| `internal/store`:ListAllEntities / ListAffectedTermEvents / ListZTAppearances / GetEntityByTerm 等 | ✅ |
| `cmd/publisher` 扩展:实体卡 03-Entities/{type}/ + 事件卡 affected wikilink;全量候选重渲染 | ✅ |
| `cmd/market-state` 扩展:hot_topics 补 event_ids(§3.4 降级兼容) | ✅ |
| 迭代 3 验收(设计 §5 七项) | ✅ |

## 2. 验收结果(设计 §5)

| # | 标准 | 结果 |
|---|---|---|
| §5.1 | 迁移 0004:entities 建表 + 索引;relationships 复用 | ✅ `information_schema` 9 列(NOT NULL 除 description);entities_pkey/entities_type_name_key/idx_entities_type/idx_entities_name 在 pg_indexes |
| §5.2 | 种子源实体:zt_pool 建 Company/Industry + belongs_to;重跑幂等零新增 | ✅ 52 公司/32 行业 + belongs_to 52 条;重跑 created=0 affects=11 恒定 |
| §5.3 | 事件↔实体:affected 匹配/分类 → affects;未分类如实 unknown/pending | ✅ 7 个 affected 词 → 6 实体 11 条 affects(银行×4 房地产×2 新能源×2 LPR×1 宁德时代×1 金融机构×1);unknown=0 如实入 meta |
| §5.4 | hot_topics 补链:market-state 重算 → 行业热点 event_ids 非空 | ✅ 事件条目携带 event_ids 并渲染 `[[event-xxx]]`;行业条目在 affects 存在时补链(银行/房地产/金融机构各有事件,SQL 复核);top-5 涨停行业当前无事件关系 → 如实空(§3.4 降级,不假造) |
| §5.5 | 实体卡发布:03-Entities/ 卡片生成;事件卡 affected 变 `[[entity-xxx|…]]`;重跑零提交 | ✅ 90 张实体卡(company 53/industry 35/concept 1/topic 1);affected 抽样 `[[entity-f789e08b|银行]]`;连续重跑 git commits=0 |
| §5.6 | 回归:迭代 0/1/2 全链不受影响 | ✅ worker processed=0 tokens=0;cluster events=3 llm_pairs=0 tokens=0;market-state 08-26 正常;daily-review published/unchanged 幂等;reconcile 异常=0 结论=通过;`go test ./...` 全绿 |
| §5.7 | 诚实:AI 分类失败/非法输出 → 建 unknown;匹配失败不假造关系 | ✅ 单测 TestClassifyInvalidTypeDropped / TestClassifyProviderFailure(非法/失败→error→调用方 unknown);真机匹配全真实事件关系,零伪造 |

## 3. 关键实现决策(落地细节)

- **两阶段写入防 churn**:entity-build 先在内存收集全部贡献(别名并集 + 关系),再一次性 upsert。避免「种子写别名 → 收割又改回」的来回写(§5.2 重跑零 DB churn)。
- **affected 词 → aliases 补全**:匹配命中的原字符串(含后缀剥离形式如"银行板块")补进实体 aliases。发布器 TermResolver 按 name/alias 精确 + 后缀剥离命中 → `[[entity-xxx|原词]]`;未命中保持纯文本(诚实,不假造链接)。
- **发布器全量候选重渲染**:`ListEventsForPublishWithSource` 从「增量(updated_at>published_at)」改为「全量有效事件」。实体层建立/更新后,已发布事件卡自动升级 wikilink;幂等靠 md5 内容比对跳过写盘 + 仅首次发布打 published_at 标记 → git 零提交、DB 零 churn。
- **空别名序列化 `[]` 非 `null`**:`json.Marshal(nil)` 对空 slice 产出 `null`,违反 NOT NULL 语义且污染数据 → 显式 `[]`。真机曾发现 1 例 null,修复后归零。
- **AI 成本(§4)**:唯一 AI 调用 = 首跑 7 个未匹配词的一次批量分类(deepseek-v4-flash 便宜档);此后全匹配,AI 调用归零。task_runs 记 token;复用 `AIDailyTokenBudget` 护栏(耗尽 → 未匹配词全部 unknown,诚实)。
- **AI 识别实体 detail 留空(架构 §9.3)**:银行/房地产/金融机构/宁德时代等 AI 分类实体无 code/source → 卡片渲染 `_未知(非东财种子,AI 识别)_`。真实上市公司 code 待其在涨停池出现后由种子路径补全。
- **行业深度 = 仅东财叶子行业(D20)**:种子 industry 只来自 zt_pool hybk(32 个,detail {source:eastmoney});不建 parent/产业链。

## 4. 真实验证记录(2026-08-26~27)

```
entity-build 首跑:  种子 52 公司/32 行业,7 个 affected 词 → 6 实体 11 affects;概念 1(LPR)题材 1(新能源)
entity-build 重跑:  seed=52/32 affected=7 matched=7 classified=0 unknown=0 created=0 affects=11(幂等)
publisher 首跑:     90 张实体卡,git commits=1
publisher 重跑:     published=0 updated=0 unchanged=6 commits=0(幂等)
market-state 08-26: emotion=Strong score=15.0 events=9;hot_topics 事件条目带 event_ids
daily-review 08-26: published;重跑 unchanged(零提交);热点渲染 [[event-xxx]]
reconcile 08-27:    异常=0 结论=通过
worker:             processed=0 events=0 tokens=0
cluster:            events=3 llm_pairs=0 tokens=0
go test ./...:      全绿(含 entityextract 新增 4 用例)
```

代码仓库 dev 提交:`beb6ea2`(entity-build)、`7036fd1`(publisher 实体卡)、`63b86a9`(market-state 补链)、后续归档提交。vault 发布:实体卡 + 事件卡 wikilink + 复盘页热点链接(2026-08-27)。

## 5. 待续(非本迭代范围)

- G3 巨潮公告独立链路(D21 延后)。
- AI 识别实体的 code/source 自动补全(出现于涨停池后由种子路径覆盖,或迭代 4+ 财务 G4 锚点)。
- 实体属性自动补全(business_model/valuation 等,架构 §9.3 目前留 Unknown)。
- Topic 生命周期 / industry parent 链(D20 不做全量产业链)。
- 迭代 4+ 财务数据 G4 / 宏观政策 G5(进度总表契约缺口后移)。
