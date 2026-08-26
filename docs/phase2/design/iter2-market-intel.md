# 迭代 2 — 市场情报 设计文档

> 状态:**已定稿冻结(2026-08-26,D12~D17 用户确认)**。此后按它执行,改动需走变更。
> 日期:2026-08-26。契约依据:`PIKS架构设计文档.md` §9.6 Observation / §9.7 Market / §9.8 Emotion / §22.1 每日复盘 12 项 / §22.5 情绪模型 / §33.2 Phase 5;`docs/进度总表.md` 迭代 2 行。
> 前置:迭代 0/1 已闭环(全链 + 真实 AI provider 验证),本迭代**不新增 AI 调用**。

## 1. 目标与范围

### 目标

把「新闻 → Event → 知识卡片」闭环升级为「新闻 + 行情 → Event + 每日市场状态」:让市场每天**实际发生了什么**(Observation)可查询、可复盘,并以架构 §22.1 的 12 项框架产出**每日复盘聚合页**。

### 范围(§22.1 每日复盘 12 项 × 自动化程度)

| # | 项 | 数据来源 | 自动化 |
|---|---|---|---|
| 1 | 指数 | 东财 `stock/get` 指数(收盘+涨跌幅) | ✅ 自动 |
| 2 | 成交额 | 两市成交额(端点实现时核验,见 §3.1) | ✅ 自动* |
| 3 | 涨跌家数 | 涨跌家数(端点实现时核验) | ✅ 自动* |
| 4 | 涨停/跌停/炸板 | 东财 涨停池/跌停池/炸板池 | ✅ 自动 |
| 5 | 连板高度 | 涨停池 `lbc` 最大值 | ✅ 自动 |
| 6 | 昨日强势股表现 | 昨日涨停池 → 今日涨跌幅均值(需昨日快照) | ✅ 自动 |
| 7 | 行业表现 | 涨停池 `hybk` 行业分布 | ✅ 自动(涨停维度) |
| 8 | 热点 | 当日 events + 涨停行业派生(不建 topic 表) | 半自动 |
| 9 | 市场情绪 | 情绪模型(纯规则加权,§2.2) | ✅ 自动 |
| 10 | 资金 | 北向/主力资金(源待定) | 🔶 待源(可空) |
| 11 | 重要事件 | `events` 表当日事件 | ✅ 自动 |
| 12 | 我的判断 | **个人回写,走 09-Personal 分域**(§3.3) | ✋ 手动 |

> \* 成交额/涨跌家数端点 2026-08-26 实测 `push2` 的 `clist/get`、`ulist.np/get` 返回 SSL RST(不稳定),`stock/get` 稳定。设计如实标注:**实现时先探针核验真实形状,再填 §3.1 契约**;候选 `stock/get` f104/f105/f106、重试 ulist.np、或 `push2his` 分时;宁缺毋假(源不可用时对应项留空)。

### 边界(明确不做)

- ❌ 不引入 Tushare(token)/AkShare(Python)/高频行情/实时行情推送 —— 与 G1 同族选**东财免 token 静态 JSON**(拍板点 D12)
- ❌ 情绪模型不接 LLM —— 纯规则加权,可解释、零增量 AI 成本(拍板点 D13)
- ❌ 不建 `topics` 实体表(迭代 3 实体补全再做);热点由 events + 涨停行业派生
- ❌ 每日复盘不自动写「我的判断」—— 那是个人层,分域到 09-Personal
- ❌ 不做交易信号、不做自动荐股、不做回测(架构 §33.1 禁止清单)
- G3 公告(巨潮)不并进本迭代 —— 它复用的是事件抽取链路,范围独立,移后续迭代(拍板点 D16)

## 2. 数据模型变更(迁移 `0003_market.sql`)

### 2.1 `market_snapshots` — 每日市场状态快照(架构 §9.7 Market + §9.8 Emotion 聚合)

一日一行,JSONB 存结构化明细,**行级字段只放最常用的标量**。`emotion` 作为字段并入(不拆表,拍板点 D14)。

```sql
CREATE TABLE market_snapshots (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  trade_date          DATE NOT NULL UNIQUE,       -- 交易日(东财 qdate 确认,非交易日不建行)
  index_json          JSONB,          -- {"sh":{"close":3912.52,"pct":0.59},"sz":{...},"cyb":{...}}
  turnover_amt        NUMERIC(16,2),  -- 两市成交额(亿),源待核验,可 NULL
  breadth             JSONB,          -- {"advance":n,"decline":n,"flat":n}
  limit_up_count      INT,
  limit_down_count    INT,
  broken_limit_count  INT,            -- 炸板
  max_board           INT,            -- 最高连板
  zt_pool             JSONB,          -- 涨停池精简快照 [{code,name,lbc,hybk,fund}]
  strong_yesterday    JSONB,          -- {"avg_ret":x,"count":n} 昨日涨停今日表现
  industry_dist       JSONB,          -- {"家居用品":5,"其他电源":2,...}
  hot_topics          JSONB,          -- [{name:"...", event_ids:[...]}] 从 events 派生
  top_events          JSONB,          -- [event_id,...] 当日重要事件
  capital_flow        JSONB,          -- 资金(源待定),可 NULL
  emotion_score       NUMERIC,        -- 规则加权得分(§2.2)
  emotion_state       TEXT,           -- Extreme_Fear/Fear/Weak/Neutral/Warm/Strong/Extreme_Greed
  emotion_detail      JSONB,          -- 各组件分值明细,可解释(§2.2)
  my_judgment         TEXT,           -- 预留(不自动写;个人判断在 09-Personal)
  evidence            JSONB,          -- 数据源清单 [{endpoint,fetched_at,count}](可信度/血缘)
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_market_snapshots_date ON market_snapshots (trade_date DESC);
```

**血缘链**:quote-collector → `observations`(原始层,架构 §9.6)→ market-state → `market_snapshots`(派生层)。`evidence` 记录每个快照来自哪些端点与抓取时间,满足「每个展示字段知道来源」。

### 2.2 情绪模型(架构 §22.5,纯规则加权)

`emotion_detail` 记录每个组件 → 分值,加权求和得 `emotion_score`,按阈值映射枚举。**初版阈值,校准留待 ~20 个交易日真实数据**(数据诚实:不凭想象标定)。

| 组件 | 口径 | 分值映射 | 权重 |
|---|---|---|---|
| 涨停数 | 当日涨停家数 | ≥80:3 / 40~79:2 / 20~39:1 / 5~19:0 / ≤4:-1 | 2 |
| 跌停数 | 当日跌停家数 | 0:2 / 1~5:0 / ≥6:-2 | 2 |
| 涨跌家数比 | advance/(advance+decline) | ≥0.75:3 / 0.6~:2 / 0.45~:1 / 0.4~:0 / 0.3~:-1 / 0.15~:-2 / <0.15:-3 | 3 |
| 炸板率 | broken/(broken+limit_up) | <0.2:1 / 0.2~0.4:0 / >0.4:-2 | 2 |
| 连板高度 | max_board | ≥5:3 / 3~4:2 / 2:1 / 1:0 | 2 |
| 昨日涨停今表现 | 昨日涨停池今日涨跌幅均值 | ≥3%:2 / 0~3%:1 / -2%~0:0 / <-2%:-2 | 2 |
| 涨停行业分布 | 出现涨停的行业数 | ≥10:1 / 5~9:0 / <5:-1 | 1 |

得分范围约 -24 ~ +32,枚举映射:

| score | state |
|---|---|
| ≥20 | Extreme_Greed |
| 12~19 | Strong |
| 6~11 | Warm |
| -2~5 | Neutral |
| -8~-3 | Weak |
| -14~-9 | Fear |
| ≤-15 | Extreme_Fear |

> **Emotion State ≠ Buy/Sell Signal**(架构 §9.8):只描述市场状态,供个人复盘,不做任何交易建议。

### 2.3 `observations` 填充契约(指标字典,架构 §9.6)

复用迭代 0 建表(§3.5),本迭代开始填充。`indicator` 用**枚举字典**(常量表,写进代码,保证可查询一致):

| indicator | market 取值 | value 口径 | source |
|---|---|---|---|
| `index_close` / `index_pct` | `sh` / `sz` / `cyb` | 收盘点位 / 涨跌幅% | eastmoney |
| `market_turnover` | `all` | 两市成交额(亿) | eastmoney(核验) |
| `breadth_advance` / `breadth_decline` / `breadth_flat` | `all` | 家数 | eastmoney(核验) |
| `limit_up_count` / `limit_down_count` / `broken_limit_count` | `all` | 家数 | eastmoney |
| `max_board` | `all` | 连板高度 | eastmoney |
| `zt_pool` | `all` | 涨停池 JSON 快照 | eastmoney |
| `industry_dist` | `all` | 涨停行业分布 JSON | eastmoney |
| `yesterday_zt_avg_ret` | `all` | 昨日涨停今日均值% | eastmoney |
| `capital_flow` | `all` | 资金 JSON(待源,可空) | 待定 |

`observed_at` = 交易日 15:00+08:00(收盘后);`change` 视指标填与前一日差值或空。

## 3. 命令与组件

### 3.1 `cmd/quote-collector` — G2 行情采集(东财,免 token)

**探明端点(2026-08-26 实测,数据诚实):**

| 数据 | 端点 | 实测 DTO |
|---|---|---|
| 涨停池 | `push2ex.eastmoney.com/getTopicZTPool?ut=…&dpt=wz.ztzt&Pageindex=0&pagesize=N&sort=fbt:asc&date=YYYYMMDD` | `data.tc`(家数)、`data.qdate`、`data.pool[]`: `c`代码 / `n`名称 / `zdp`涨跌幅 / `lbc`连板数 / `fbt`/`lbt`封板时间 / `fund`封板资金 / `hybk`行业 / `zttj` |
| 炸板池 | `push2ex.eastmoney.com/getTopicZBPool?…&date=YYYYMMDD` | `data.tc`、`data.pool[]`: `c/n/zdp/zbc/zf/hybk` |
| 跌停池 | `push2ex.eastmoney.com/getTopicDTPool?…&date=YYYYMMDD` | `data.tc`、`data.pool[]`(同族) |
| 指数 | `push2.eastmoney.com/api/qt/stock/get?secid=1.000001&fields=f43,f57,f58,f60,f170` | `f43` 最新 / `f57` 代码 / `f58` 名 / `f60` 昨收 / `f170` 涨跌幅%(实测:上证 3912.52 +0.59%) |
| 涨跌家数/成交额 | 🔶 **待实现核验**:`ulist.np/get`(f104/f105/f106)、`clist/get` 实测 SSL RST;`stock/get` f6/f104~106 实测为空 | 探针确认后填真形状;不确认就留空项,不造假 |

**归一化 DTO**(采集适配器唯一输出):
```go
type MarketSnapshotRaw struct {
    TradeDate   time.Time
    LimitUp     []ZTItem  // {Code,Name,Zdp,Lbc,Fund,Hybk}
    LimitDown   []ZTItem
    Broken      []ZTItem
    Indexes     map[string]IndexQuote // sh/sz/cyb → {Close, Pct}
    Breadth     *Breadth             // Advance/Decline/Flat(可 nil)
    TurnoverAmt *float64             // 亿(可 nil)
}
```

**非交易日处理**:交易日以上证指数当日有交易数据 + 涨停池 `qdate` == 请求日为准;非交易日**不建 snapshot、不写 observations**(不产生周末脏数据)。`-date` 参数默认今天,可回补历史(一次性回填用)。

**幂等**:同 `-date` 重跑 → upsert observations(按 market+indicator+observed_at 去重),同值跳过。

### 3.2 `cmd/market-state` — 快照聚合 + 情绪

读当日 observations + 昨日 snapshot → 计算 12 项 → 写 `market_snapshots`(upsert by trade_date)。情绪组件逐项打分入 `emotion_detail`。纯规则,无 AI。**昨日强势股**依赖昨日快照的 `zt_pool`,首日无昨日数据则该组件留空并计入 detail 标注 `missing`。

### 3.3 `cmd/daily-review` — 每日复盘聚合页(02-Market/)

读 `market_snapshots` + `events`(当日) → 渲染 `02-Market/YYYY-MM-DD.md` → git commit。复用 publisher 的**内容 md5 跳过**(重跑零提交)。模板 12 节:

```markdown
---
id: market-2026-08-26
type: market-daily
date: 2026-08-26
emotion: Neutral
pipeline: market-state@<git-short>
---

# 每日复盘 2026-08-26

## 指数
| 指数 | 收盘 | 涨跌幅 |
| 上证指数 | 3912.52 | +0.59% |
| 深证成指 | … | … |

## 成交额   ## 涨跌家数   ## 涨停/跌停/炸板
## 连板高度  ## 昨日强势股表现  ## 行业表现(涨停分布)
## 热点      ## 市场情绪(得分+组件明细)  ## 资金
## 重要事件
- [[event-xxx]] 标题…

## 我的判断
> 个人判断请写入 09-Personal/复盘/2026-08-26.md(分域双源:Generated 不承载个人内容,可 wikilink 互链)
```

**分域**:02-Market/ = Generated(服务器生成,重复运行覆盖);「我的判断」落在 09-Personal/复盘/(个人仓库,Obsidian 编辑),双链互通(项目详解 §分域双源)。首次生成 daily-review 时可在 09-Personal 侧放一个空模板提示(可选,不强制)。

### 3.4 采集契约与源健康

- G2 与 G1 同族(东财,**非官方、无 SLA**)。源健康沿用迭代 1 `reconcile` 思路:`quote-collector` 连续失败 ≥3 次 → 当日跳过并记录 `task_runs`,不污染快照。
- 每次运行记 `task_runs`(command/meta):`quote-collector` 记 `{date, zt_count, zd_count, broken_count, fetched_at}`;`market-state` 记 `{date, emotion_state, emotion_score}`;`daily-review` 记 `{date, published/unchanged}`。

## 4. AI 成本策略

**迭代 2 AI 调用 = 0**。情绪/状态/热点均规则派生,「重要事件」复用已有 events(迭代 0/1 已抽取)。每日复盘不接 LLM 写综述 —— 保持 Fact 层数据诚实,个人判断留给用户。后续迭代(周报/复盘综述)再按需接推理档(迭代 4)。

## 5. 验收标准(smoke test)

| # | 标准 | 判定 |
|---|---|---|
| §5.1 | 迁移 0003:`market_snapshots` 建表(index 存在) | `information_schema` + `pg_indexes` |
| §5.2 | G2 采集:真实交易日 `quote-collector` 拉到涨停池(>0)、observations 落库;非交易日跳过 | 当日实际运行 |
| §5.3 | `market-state` 生成快照:12 项字段齐全,`emotion_detail` 逐组件有值 | 单元测试 + 当日运行 |
| §5.4 | 情绪模型边界:构造极端输入 → 枚举/得分符合 §2.2 表 | 单元测试(mock observations) |
| §5.5 | 昨日强势股:两日数据 → `strong_yesterday.avg_ret` 计算正确;首日缺昨日 → 标注 missing | 单元测试 + 连续两日运行 |
| §5.6 | `daily-review` 生成 02-Market 页:12 节齐全、emotion 正确、git 提交;重跑 hash 跳过零提交 | 运行 2 次对比 |
| §5.7 | 回归:迭代 0/1 全链(mock 采集/抽取/聚类/发布/reconcile)不受影响 | 全命令 smoke |
| §5.8 | 诚实:成交额/涨跌家数若源未核验 → 对应项留空或标 `pending`,不写假数字 | 抽查快照 |

## 6. 定稿拍板点(已确认,2026-08-26)

- **D12 ✅ G2 数据源 = 东财静态 JSON(与 G1 同族,免 token,Go 直接 HTTP)**;备选 Tushare(需 token)/AkShare(需 Python),不进 V1 技术栈。涨跌家数/成交额端点实现时探针核验,可留空不造假。
- **D13 ✅ 情绪模型 = 纯规则加权(零 LLM、可解释、可调)**,初版阈值,~20 交易日真实数据后校准。Emotion 仅描述市场,非交易信号。
- **D14 ✅ 聚合形态 = `market_snapshots` 单表一日一行,emotion 并入(score/state/detail),不拆 `emotions` 表。**
- **D15 ✅ 热点 = events + 涨停行业派生,不建 `topics` 表**(迭代 3 实体补全再建)。
- **D16 ✅ G3 公告(巨潮)不并入本迭代**,独立源,后续迭代再做;本迭代聚焦市场情报核心,一次不做完。
- **D17 ✅「我的判断」走分域**:Generated 页只留提示占位,个人判断在 09-Personal/复盘/(个人仓库)。

## 7. 交付物清单(定稿后按此执行)

| 交付物 | 说明 |
|---|---|
| `0003_market.sql` | market_snapshots + index |
| `internal/collector/quotemarket.go` + 探针 | G2 东财行情驱动(涨停/炸板/跌停/指数)+ probe |
| `cmd/quote-collector` | G2 采集 → observations |
| `internal/marketstate/` + `cmd/market-state` | 聚合 + 情绪规则模型 |
| `cmd/daily-review` | 每日复盘页 → 02-Market/ + git |
| `internal/publish/` market 模板 | 复盘页渲染 |
| 归档 `docs/phase2/stages/iter2.md` + 进度总表更新 | |
