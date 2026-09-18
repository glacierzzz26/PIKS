# P7 买入前「一键速评」+ 量价形态分析(设计)

> 状态:**已定稿(2026-09-14),按此执行,改动需走变更**。范围:把「个股分析」增强为**买入前可用的一份尽可能专业的即时研报**——一键现场重跑(新鲜度)、风险红线优先、财务/估值/消息面全给、并补上逐日**量价形态分析**(换手率与股价背离的形态标签 + 双轴图)。契约依据:`research/src/analysis/{price,volume,risk,financial,scorecard}.py`、`research/src/report/json_report.py`、`research/src/workflow/{plan,profile}.py`、`research/profiles/*.yaml`、`internal/research/orchestrator.go`、`internal/web/api_research.go`、`frontend/src/components/research/*`。前序:`docs/phase6/design/ux-ia.md`(P6 决绝重构 + 闭环,已上生产 镜像 `9454524`)。

## 定稿门结论(2026-09-14)

用户原话:「因为我在买入前可能查询一下个股,所以我需要这个项目在买入前能尽可能给我更多的分析,以及结论。」

| 议题 | 用户选择 | 结论 |
|---|---|---|
| 核心形态 | 快速「买入前体检」**且** 量价形态分析(截图那种) | 两者都做,同卡呈现 |
| 维度 | 风险红线(最重要)、估值、财务基本面、消息面/事件,「一个尽可能专业的研报,你了解的方面都需要考虑到」 | 全给;并**补渲染**现成但未上屏的 `risk.items` / `financial` |
| 落点 | 「我不是一定要买的,属于个股分析的功能增强」 | 个股页首屏置顶区块,**非**交易流程页 |
| 新鲜度 | **一键现场重跑** | `quick` 快速模式;结论取自规则,AI 可选 |
| 量价形态 | **标签 + 双轴图** | 新规则引擎 + ECharts 双轴图 |

---

## 1. 背景与现状

### 1.1 需求

用户买入前会查个股,要「尽可能多的分析 + 结论」。定位是**个股分析的功能增强**(不一定要买),不是交易录入流程。

### 1.2 探查结论(两件必须说清的事实)

**✅ 现成但未上屏(零新算的收益)。** 每次深研都算好了:
- `metrics.risk`:`overall_level`(low/medium/high)、`veto_buy`(一票否决)、`items[]`(category/level/evidence/description)。规则见 `analysis/risk.py:35-154`——高波动、大幅回撤、流动性差、高负债、业绩下滑、重大公告、估值缺失。
- `metrics.financial`:`latest_roe/net_margin/gross_margin/debt_ratio`、`revenue/net_profit_yoy`、趋势数组、`pe_ttm/pb`(`analysis/financial.py:9-33`)。

前端**只**渲染了评分卡的「风险」单维(`components/research/Scorecard.tsx`,经 `DIM_LABEL.risk`);`risk.items[]` 与 `financial` **从未上屏**(调查确认:无 RiskList 组件、`FactSection` 不含 `metrics.risk`)。故「风险红线 + 财务/估值」是**现成数据的呈现补齐**。

**❌ 真空缺——量价形态。**
- 逐日 K 线(含 close/turnover)在 Python 里算完即丢:**PG 无 bar 表**(全量 migrations 无 price/bar/kline 表;`internal/store/` 无 bar/quote 方法)。
- `analysis/volume.py` 只吐 8 个聚合标量(近 5/20/60 日均换手、最高/最低换手、放量异常日、量价相关 `price_volume_corr`),**无逐日序列、无形态标注**。
- 用户截图要的是「换手率峰值日 = 股价高点」这类**逐日形态 + 背离**判定 → 需新模块。

**数据源边界(诚实):** 换手率唯一来源 = akshare/腾讯 `stock_zh_a_hist_tx`(`providers/market/akshare_provider.py:68-71`,turnover×100),**A股流通股本口径**。⚠️ **2026-09-18 实测更正**:该口径与**同花顺逐日一致**(22 只 × 各 140 交易日,最大偏差 0.018%)——同花顺自家 feed 用的也是 A股流通股本,故 P7 当年「拿不到同花顺口径」的表述**不准确**;真正无免费源的是**自由流通**口径(同花顺 App「实际换手率」一列)。详见 `turnover-caliber-findings.md`。

### 1.3 与既定方向的关系

`docs/项目详解.md` 定位不变:Knowledge System,非 Trading System。速评**不下买卖建议**——它给**规则判定的事实 + 风险标记 + 评分**,把「买不买」留给用户。红线**不改**:Fact ≠ **Inference**(形态/评分是 Inference,规则可解释、与 Fact 分区)、PG 唯一 Source of Truth、AI 不直接写库、缺数据如实降级(宁缺毋假)。

---

## 2. 现状关键机制(设计依据)

### 2.1 深研管线(触发 → 落库)

`POST /api/v1/research-runs`(`internal/web/api_research.go:134`)→ 同步建 `pending` 行 → 后台 goroutine `orchestrator.Run`(`internal/research/orchestrator.go:75`)三步:
1. `gathering` — 一次 Python `research <code> --profile <p> --out-dir <d>`(`runner.go:116`),akshare 采集 + 确定性分析 + 报告组装;
2. `synthesizing` — Go 读 `synthesis_prompt.txt`,调 LLM,**写 `synthesis.json`**,跑 `synthesize` 子进程渲染 + Number Lint;
3. `verifying` — 跑 `gate` 子进程(6 检查)。

产物落 `research_runs`(JSONB:`metrics`/`synthesis`/`lint`/`gate`/`evidence`;TEXT:`markdown`)。**逐日 K 线不落库。**

**⚠️ 关键约束:`synthesizing` 无条件要求 LLM。** `synth.go:40-42` provider 为 nil → error;`orchestrator.go:238-242` 该 error 传播 → 整轮 `status=failed`。故**无 AI 配置的环境,深研必失败**。这是「一键速评」必须先解的前置。

### 2.2 Profile 机制

- 加载:`workflow/profile.py:119-154`(`<research>/profiles/<name>.yaml`)。
- **`mode` 字段不参与分支**(验证:`plan.py:61-211` 从不读 `mode`);**行为完全由 `sections` 驱动**(经 `SECTION_REQUIREMENTS` 决定采集/分析任务)。
- 现有两档:`complete-stock`(full,11 节含 `industry`)、`short-term`(express,7 节、**仅 2 评分维度、不含 risk**)。
- `short-term` 不足以做「体检」(无 risk);`complete-stock` 含 `industry`——**最重步骤**(`sw_provider.py` 循环拉三级行业成员,常 30~90s+)。

### 2.3 评分卡 / 结论来源

`analysis/scorecard.py:264-331`:按 profile 声明的维度打分,**overall = 可用维度简单加总**;**overall_label**(`偏正面/中性/偏负面/结论受限`):任一**敏感维度**(`risk`/`fundamental_trend`)不可得 → `结论受限`;否则按平均分 ±2/3 判方向。**全确定性**,不依赖 LLM。

故速评的「结论」= **`overall_label` + `risk.overall_level` + `veto_buy`**,规则给出、可复现;AI 综合研判是**可选的叙述层**。

---

## 3. 方案

### 3.1 后端

#### 3.1.1 新 Profile `research/profiles/prebuy.yaml`(快而不残)

**除 `industry` 外全要**,保留完整 6 维评分卡(含 `risk`):

```yaml
name: prebuy
version: "1.0.0"
description: 买入前速评（确定性优先,跳过行业对比以提速）
mode: express
period:
  display: { price: 60d, volume: 60d, turnover: 60d, news: 30d, announcement: 30d, financial: 4q, valuation: 3y }
  compute: { price: 260d, turnover: 260d }
sections: [company, market, volume, turnover, financial, valuation, events, announcements, risk, conclusion]
scorecard:
  dimensions: [business_quality, fundamental_trend, market_trend, valuation, recent_events, risk]
  overall_rule: simple_sum
  scale: [-2, -1, 0, +1, +2]
```

与 `complete-stock` 的唯一差别 = **少一个 industry 采集**(提速关键取舍)。行业对比仍由「完整深研」按钮覆盖。

#### 3.1.2 新模块 `research/src/analysis/patterns.py`(量价形态)

输入 `bars`(前复权、日期升序),输出确定性 `PatternMetrics`:

- **`series`**:最近 60 交易日 `[{date, close, turnover, volume}]` —— 双轴图数据源,**也是逐日序列首次落库的载体**(随 metrics 进 `research_runs.metrics` JSONB,**零 schema**,约 60×4 数)。
- **`labels`**:`[{code, label, from, to, evidence}]`,规则判定,每条**必带 evidence**(日期 + 数值,可核对)。拟定规则集:
  - `地量启动` — 区间换手处低位分位,随后价升量增;
  - `温和放量上行` — 价涨、量升未到峰;
  - `换手峰值见顶` — 换手窗口极大值点且邻近价格极大值;
  - `高位放量下跌` — 价从区间高点回落、换手仍高;
  - `价跌量未缩` — 近 N 日价跌、换手 ≥ 窗口中位数(抛压未减);
  - `缩量续跌` — 近 N 日价跌、换手 < 中位数(跌透迹象);
  - `放量见顶`(背离)— 见顶日换手与价格同步见顶。
  - 阈值随窗口**分位自适应**,不硬编码绝对值。
- **`divergence`**:`{price_volume_corr, peak_date, peak_turnover, peak_close}` 辅助字段。

接入 `report/json_report.py`(新增可选 `patterns` 键)+ `analysis/engine.py` 的 `run_analysis`(供 Evidence 记录)。

#### 3.1.3 快速模式:合成可选(`RequireSynthesis`)

- `internal/research` 的 `Options` 增 `RequireSynthesis bool`(**默认 true**,深研路径逐字不变)。
- `orchestrator.execute` 的 synthesizing 步:为 false 时 LLM 缺失/失败/超预算 **不 fail** → 记空 synthesis、继续;markdown 保留骨架 + `_（待 AI 综合研判）_` 占位(`synthesis.py` 已内建 fallback)。
- gate 照常跑,quick 模式下不过 gate **不 fail**(gate 本就非致命)。
- `POST /api/v1/research-runs` body 增可选 `quick bool`(默认 false)→ `quick:true` 即 `RequireSynthesis=false`。**向后兼容,非破坏性。**

**结论来源(关键):** 速评结论 = 评分卡 + 风险(veto),**规则确定性、不依赖 AI**;AI 有配置则锦上添花,无则如实空态。→ 速评在生产**不被 AI 网关阻塞**。

#### 3.1.4 无新端点

速评结果 = 一条 `research_runs`(profile=`prebuy`)。前端复用 `GET /research-runs?code=X`(取最新 prebuy `done`)+ `GET /research-runs/:runId`(全量 metrics)。唯一 API 改动 = POST body 加 `quick`。

### 3.2 前端

#### 3.2.1 落点

`frontend/src/pages/stock/[code].tsx`:`StockHeader` 之后、「当时在看什么」之前插 `<section className="section">`。带新鲜度「截至 {as_of}」+ 一键「快速分析」。

#### 3.2.2 组件(守 <150 行 / JSX >80 行拆分)

| 组件 | 预算 | 职责 |
|---|---|---|
| `hooks/usePrebuy.ts` | ~70 | 拉最新 prebuy done 详情;`trigger()` 发 `{code, profile:'prebuy', quick:true}` 并轮询(2s),**原地出结果不跳页**;三态 |
| `components/stock/StockPrebuy.tsx` | ~120 | 卡容器:新鲜度头 + 结论 + 风险 + 财务/估值 + 量价形态 + 图 + AI(有则显) |
| `components/stock/PrebuyVerdict.tsx` | ~55 | 结论横幅:评分卡 `overall_label`(Chip 分档色)+ 风险等级 + `veto_buy` 红标「一票否决」 |
| `components/stock/RiskList.tsx` | ~50 | **新增渲染** `metrics.risk.items[]` |
| `components/stock/PatternTags.tsx` | ~60 | 形态标签(规则判定分区)+ 每条 evidence |
| `components/charts/VolumePriceChart.tsx` | ~75 | 双轴:收盘价线(左轴)+ 换手率柱(右轴),色值走 `useChartTheme` |

**复用**:`MetricGroup`(label/value 表 + Evidence 脚注)、`Scorecard`(自 bail)、`OpinionSection`(有 AI 才渲染)、`StockSectionEmpty`(诚实空态)、`EChart`、`metricRows.ts`。

#### 3.2.3 图表

`components/charts/EChart.tsx` 现仅注册 `BarChart` → 追加 `LineChart` + `PiksChartOption` 联合加 `LineSeriesOption`(仍按需注册,禁全量)。双轴 = 两个 `yAxis`(左 price / 右 turnover),`series[1].yAxisIndex=1`。

#### 3.2.4 财务/估值上屏 + 类型

- `metricRows.ts` 增 `FINANCIAL_ROWS`(ROE/净利率/毛利率/负债率/营收同比/净利同比)、`VALUATION_ROWS`(PE-TTM/PB),复用 `MetricGroup`。
- `lib/types.ts`:`ResearchMetrics` 加 `patterns?: ResearchPatterns`;新增 `ResearchRisk`(现为 `Record<string,unknown>`);POST body / `ResearchTrigger` 加 `quick?`。

#### 3.2.5 诚实与新手友好

无记录 → 空态 + CTA;运行中 → 文字状态徽标(**核心数字区禁 skeleton**);AI 未配置 → 如实「未生成」,确定性结论照常;财务缺失 → 维度 `unavailable` → 如实「结论受限」;图注「换手率(A股流通股本口径 · 与同花顺一致)」;形态区标「量价形态(规则判定)」与 Fact 分离;术语经 `lib/glossary.ts` + `<Term>`。

---

## 4. 分阶段(每阶段独立可验收/可回滚)

### P7-1 后端:量价形态 + prebuy profile + 合成可选
- `patterns.py` + `tests/test_patterns.py`;`prebuy.yaml`;`json_report.py`/`engine.py` 接线。
- `orchestrator.go` `Options.RequireSynthesis` + `api_research.go` POST `quick`。
- **验收**:`research/.venv/bin/python -m src.cli research 600519 --profile prebuy --out-dir /tmp/x` → `metrics.json` 含 `patterns.series`(≤60 点)与 `labels`;`go test ./...` 过;`curl -XPOST /research-runs -d '{"code":"600519","profile":"prebuy","quick":true}'` → done 且 `metrics.patterns` 非空;**无 LLM 配置下也 done**。

### P7-2 前端:速评卡 + 双轴图 + 风险/财务上屏
- `EChart` 注册 LineChart;`VolumePriceChart`;`usePrebuy` + 5 组件;`metricRows` 增行;`types.ts` 补型;`glossary.ts` 补术语;接入 `/stock/:code`。
- **验收**:`npm run lint`(tsc)+ `npm run build` 过;`/stock/600519` 点「快速分析」→ 原地出卡(结论/评分卡/风险条目/财务/形态标签/双轴图);无 AI 配置结论仍完整;缺数据显示 `—`/「数据缺失」。

### P7-3 端到端 + 文档 + 上生产
- 联调(`npm run dev`:3100 + 本地 PG:5433);真实标的跑一遍核对标签与 K 线语义一致。
- 归档 `docs/phase7/`;登记 `docs/进度总表.md`;更新 `CLAUDE.md`(个股页新增「买入前速评」区块;profile 清单加 `prebuy`)。
- **上生产**:预存回滚 `piks-tools:rollback-pre-p7` → `./scripts/deploy.sh`(预期 **migrate 0**)。
- **验收**:17 页全 200 + Playwright 真实浏览器 0 console 错误;生产 `/stock/:code` 点速评出卡;容器日志干净。

---

## 5. 风险与回滚

| 风险 | 应对 |
|---|---|
| akshare 生产被限(现走腾讯源) | prebuy 去掉最重的 industry 采集;失败如实 `failed` + 原文错误,不静默 |
| 财务 provider 失败 → 评分卡「结论受限」 | 如实显示 + 前端提示,不编造 |
| 合成可选影响既有深研 | `RequireSynthesis` 默认 true;仅 `quick:true` 生效;深研路径逐字不变 |
| 形态规则误判 | 每条带 evidence 可核对;分区标「规则判定」;不入 Fact 区 |
| 双轴图单口径 | 图注明确「A股流通股本口径 · 与同花顺一致」;自由流通口径无免费源,不暗示可得 |
| 上生产回归 | 预存 `piks-tools:rollback-pre-p7`;回滚 `docker tag piks-tools:rollback-pre-p7 piks-tools:latest && docker compose up -d --force-recreate web` |

---

## 6. 明确不在本次范围(如实列出)

- 同花顺**自由流通口径**换手(2026-09-18 实测:无免费源;但 PIKS 现口径已与同花顺 feed 一致 —— 见 `turnover-caliber-findings.md`);
- **行业对比**实时(需重跑 `industry`,走「完整深研」);
- **龙虎榜资金面**(provider `capital/akshare_lhb.py` 存在但未接线 `plan.py`,`capital_metrics` 恒 None,死代码);
- **相对指数超额收益**(`vs_index_return_pct` 现为占位 None);
- **买卖建议**(速评只给事实/风险/评分,不给操作指令——定位约束)。
