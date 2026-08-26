# 迭代 2 — 市场情报 归档

> 状态:**已完成**。本阶段只读,续接请看 `进度总表.md` 与 `../../phase2/design/iter2-market-intel.md`。
> 日期:2026-08-26。实现以 `phase2/design/iter2-market-intel.md` 定稿契约为准(D12~D17)。

## 1. 目标与交付物

把「新闻 → Event → 知识卡片」闭环升级为「新闻 + 行情 → Event + 每日市场状态」:让市场每天实际发生了什么(Observation)可查询、可复盘,并按架构 §22.1 的 12 项框架产出每日复盘聚合页。**本迭代 AI 调用 = 0**(情绪/状态/热点全规则派生)。

| 交付物 | 状态 |
|---|---|
| `0003_market.sql`:market_snapshots 表(22 列,一日一行)+ 索引 | ✅ |
| G2 东财行情探针 + 驱动 `internal/collector/quotemarket.go`(涨停/跌停/炸板池 + 指数) | ✅ |
| `cmd/quote-collector`:G2 采集 → observations(指标字典 §2.3) | ✅ |
| `internal/marketstate`:情绪规则模型 + 快照聚合 | ✅ |
| `cmd/market-state`:聚合 → market_snapshots | ✅ |
| `internal/publish/market.go` + `cmd/daily-review`:12 节复盘页 → 02-Market/ + git | ✅ |
| 迭代 2 验收(设计 §5 八项) | ✅ |

## 2. 验收结果(设计 §5)

| # | 标准 | 结果 |
|---|---|---|
| §5.1 | 迁移 0003:market_snapshots 建表(index 存在) | ✅ `information_schema` 22 列;`idx_market_snapshots_date`/`idx_observations_market_indicator` 在 pg_indexes |
| §5.2 | G2 采集:真实交易日 quote-collector 拉到涨停池(>0)、observations 落库;非交易日跳过 | ✅ 2026-08-26 zt=52 zd=0 broken=20 obs=6 落库;2026-08-29(周六)返回「非交易日,跳过」 |
| §5.3 | market-state 生成快照:12 项字段齐全,emotion_detail 逐组件有值 | ✅ 快照行 22 列;emotion_detail 7 组件,missing 明确标注;单测 TestComputeSnapshotFields |
| §5.4 | 情绪模型边界:构造极端输入 → 枚举/得分符合 §2.2 表 | ✅ 单测 Extreme_Greed(score≥20)/Extreme_Fear(score≤-15)/阈值边界 13 点抽查 |
| §5.5 | 昨日强势股:两日数据 → strong_yesterday.avg_ret 正确;首日缺昨日 → 标注 missing | ✅ 单测 avg_ret=3.25 count=2;首日 snapshot.StrongYesterday=nil + pending + emotion missing。真机首日即缺,如实 missing |
| §5.6 | daily-review 生成 02-Market 页:12 节齐全、emotion 正确、git 提交;重跑 hash 跳过零提交 | ✅ 页 12 节齐全;首跑 published(提交),重跑 unchanged(零提交,git status 空) |
| §5.7 | 回归:迭代 0/1 全链不受影响 | ✅ worker processed=0 tokens=0;cluster llm_pairs=0 tokens=0;reconcile 异常=0 结论=通过;publisher 全 0 零提交 |
| §5.8 | 诚实:成交额/涨跌家数若源未核验 → 对应项留空或标 pending,不写假数字 | ✅ turnover_amt/breadth/index_json 均 NULL(源被 WAF 限流);daily-review 页渲染 `_数据缺失(pending,源未核验)_` |

## 3. 关键实现决策(落地细节)

- **源健康分层(§3.4)**:push2ex 池端点稳定(zt/zb/dt);push2 的 stock/get 偶通、ulist.np/clist 持续 SSL RST → 指数/涨跌家数/成交额**尽力而为**,失败如实记 pending,不造假(§5.8)。
- **非交易日判定(§3.1)**:涨停池 `data.qdate == 请求日` 才视为交易日;否则跳过(不建快照、不写观测,不产生周末脏数据)。
- **情绪模型(§2.2)**:7 组件纯规则加权(涨停/跌停/涨跌比/炸板率/连板/昨日强势/行业数),得分 -24~+32,7 档枚举。missing 组件不参与求和,detail 内保留 `missing:true` 可解释。**初版阈值,~20 交易日真实数据后校准**(数据诚实)。
- **昨日强势股**:昨日快照 zt_pool 代码 → 今日批量涨跌幅(ulist.np,尽力而为)。首次运行无昨日数据 → 组件 missing。已留 `FetchDailyReturns` 接线,源恢复即自动填充。
- **热点(§2.1/D15)**:不建 topics 表。hot_topics = 涨停行业 top5 + 当日事件 top3(`{name,event_ids}` 形),事件↔行业自动关联留待迭代 3 实体补全。
- **幂等纪律**:① observations 按 market+indicator+observed_at 去重,同值跳过;② market_snapshots 按 trade_date upsert;③ 复盘页渲染内容逐字节稳定 → git status 空 → 零提交。**坑**:行业 top-N 排序曾只按 count(Go map 随机序破坏幂等),加 name 次级排序修复。
- **分域(D17)**:02-Market/ 属 Generated,重复运行覆盖;「我的判断」只留提示占位,个人内容在 09-Personal/复盘/(个人仓库,双链互通)。
- **AI 成本**:迭代 2 全部命令 tokens=0。恢复节流:worker/cluster 回归时已确认无多余 AI 调用。

## 4. 真实验证记录(2026-08-26)

```
quote-collector 2026-08-26: zt=52 zd=0 broken=20 indexes=0 obs=6 pending=[index_sh index_sz index_cyb breadth turnover]
quote-collector 2026-08-29: 非交易日,跳过
market-state   2026-08-26: emotion=Strong score=15.0 events=9 pending=[index_sh index_sz index_cyb turnover breadth strong_yesterday]
daily-review   2026-08-26: published;重跑 unchanged(零提交)
```

emotion 15 = limit_up(2×2) + limit_down(2×2) + broken_rate(0×2) + max_board(3×2) + industry(1×1),breadth/strong_yesterday missing。行业分布 32 个行业(工业金属 4 家居 3 …),连板最高 5(深中华A)。top_events 9 条(央行降准 ×3 措辞 + 宁德麒麟5.0 + 星河固态电池 ×2 + 异常甲/乙)。vault 提交:`publish: 每日复盘 2026-08-26`。

## 5. 待续(非本迭代范围)

- 指数/涨跌家数/成交额端点源核验:push2 WAF 恢复或换源(Tushare token 等),填入真实数字。
- 昨日强势股连续两日运行后自动填充(需两个交易日快照)。
- 事件↔行业自动关联(迭代 3 实体补全后补 hot_topics 的 event_ids)。
- 情绪阈值 ~20 交易日真实数据校准(架构 §9.8 要求)。
- 迭代 3 实体补全 / 迭代 4 个人学习闭环(进度总表)。
