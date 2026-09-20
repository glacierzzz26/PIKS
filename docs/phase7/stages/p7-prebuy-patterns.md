# P7 买入前「一键速评」+ 量价形态分析 —— 实现与验收

> 阶段:前端重构 P7(单阶段跨后端/前端)。设计依据 `docs/phase7/design/ux-ia.md`。
> 目标:个股页新增「买入前速评」—— 现场重跑一份新鲜体检(风险红线 / 评分卡 / 财务估值 / 量价形态 / AI),
> 一键原地出卡不跳页。**零 schema**(逐日序列入 metrics JSONB)。

## 交付

### 1. 后端:量价形态规则模块(`research/src/analysis/patterns.py`,新)
- 输入逐日 K 线(前复权、升序),输出确定性 `PatternMetrics`:
  - `series`:`[{date, close, turnover, volume}]`,最近 60 交易日 —— **双轴图数据源,也是逐日序列首次落库的载体**;
  - `labels`:`[{code, label, date, evidence}]`,规则判定(地量启动 / 换手峰值见顶 / 高位放量下跌 / 价跌量未缩 / 缩量续跌 / 放量上行 / 缩量上行 / 温和放量上行),每条必带 evidence(日期 + 数值,可核对);
  - `divergence`:`{peak_date, peak_turnover, high_date, high_close, peak_high_gap_days, peak_high_coincide, turnover_price_corr, after_peak_return_pct}`(换手峰值与股价高点的对齐关系);
  - `note`:窗口不足 `MIN_BARS=20` 时如实说明样本不足,不出标签。
- 阈值**分位自适应**(`median_t` / `p20` / `p80`),不硬编码绝对值。
- **诚实边界(硬约束,前端据此分区)**:换手率仅**流通口径**(akshare/腾讯),拿不到同花顺自由流通口径;形态是**规则判定(Inference)**,不是事实。
  - ⚠️ **2026-09-18 更正(issue #4 实测)**:前半句表述不准确。腾讯 = **A股流通股本**口径,与**同花顺 feed 逐日一致**(22 只 × 各 140 交易日,最大偏差 0.018%);真正无免费源的是**自由流通**口径。详见 `../design/turnover-caliber-findings.md`。
- 单测 `research/tests/test_patterns.py`:10 条(序列截断 / 窗口不足 / 各形态分支 / divergence 字段),全过。

### 2. 后端:prebuy profile + 接线
- `research/profiles/prebuy.yaml`(新):`express`,除 `industry` 外全要 —— **去掉最重的 SW 行业成员循环以提速**,保留完整 6 维评分卡(含 risk)。
- `json_report.py` 接 `patterns` 键 → 随 metrics 落 `research_runs.metrics` JSONB;**零 schema**(60×4 个数,体积可忽略)。
- `engine.py` 接入 `patterns` 分析步;`plan.py` 增 `analyze_patterns` 任务;`profile.py` 增 section 需求。

### 3. 后端:合成可选(快速模式)
- `research.Options` 增 `RequireSynthesis bool`(默认 true = 深研逐字不变)。
- `quick:true`(RequireSynthesis=false)时:LLM 缺失 / 失败 / 超预算**不 fail** —— 记空 synthesis、markdown 用骨架,确定性结论(评分卡 + 风险规则)**照常产出**。
- `api_research.go` POST body 增可选 `quick bool`(向后兼容);`cmd/research-run` 恒 `RequireSynthesis: true`。
- 结论来源 = **评分卡 `overall_label` + 风险等级 + `veto_buy`** —— 全部规则确定性,不依赖 AI;AI 有则锦上添花。
- 集成测试 `TestOrchestratorQuickNoProvider`:nil provider + 快速模式 → done(骨架),对照深研应 failed。

### 4. 前端:速评卡 + 双轴图 + 风险/财务首次上屏
- `EChart.tsx` 注册 `LineChart`(仍按需注册);`charts/VolumePriceChart.tsx`(新):收盘价折线(左轴)+ 换手率柱(右轴),柱按当日涨跌着色(A 股习惯),色值走 `useChartTheme`。
- `hooks/usePrebuy.ts`(新):挂载即拉最近 prebuy done 报告自动渲染;`trigger()` 发 `{code, profile:'prebuy', quick:true}` 并轮询(2s),**原地出卡不跳页**;三态。
- 组件:`StockPrebuy`(容器,121 行)/ `PrebuyVerdict`(结论横幅:评分卡 + 风险 + 一票否决)/ `RiskList`(风险条目,**首次上屏**)/ `PatternTags`(形态标签 + evidence + 背离)。
- `metricRows.ts` 增 `FINANCIAL_ROWS` / `VALUATION_ROWS`,复用 `MetricGroup` 渲染 `metrics.financial` —— **风险与财务此前每轮都算好了却从没上屏,这是「零新算」的现成收益**。
- `types.ts` 补 `ResearchPatterns` / `ResearchRisk` / `ResearchTriggerBody`;`glossary.ts` 补术语(买入前速评 / 评分卡结论 / 一票否决 / 换手率流通口径 / 量价形态)。
- `OpinionSection` 增可选 `heading` prop(深研页标题不变),速评卡复用同组件。
- 个股页 `pages/stock/[code].tsx` 在 `StockHeader` 之后插入「买入前速评」区块。

## 验收

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet` / `go test ./...` | ✅ 全过(含 `TestOrchestratorQuickNoProvider` 集成) |
| `research/.venv/bin/python -m pytest tests/test_patterns.py` | ✅ 10 passed |
| `tsc --noEmit`(`npm run lint`) | ✅ |
| `npm run build` | ✅ |
| CLI 真跑(600519,prebuy) | ✅ 出 `patterns.series`(60 点)+ `labels` + `divergence` |
| **POST `/research-runs` {prebuy, quick:true} → done,metrics.patterns 非空** | ✅ `sh600519_prebuy_20260914_152024` done;series 60 / label 缩量续跌 / divergence 齐全 |
| **无 AI 也可用(快速模式降级)** | ✅ 实测 LLM 网关 429(月额度耗尽)/ TLS 超时,快速模式下**不 fail**:骨架报告 + 确定性评分卡照常,日志如实记「快速模式跳过 AI 合成」 |
| Playwright `/stock/600519` 速评卡 | ✅ 9/9:区块存在 / 已有速评自动渲染 / 风险红线上屏 / 双轴图 canvas / 形态标签 / 点「重新分析」原地出卡不跳页 / 0 console 错误 |
| 全站 e2e 冒烟 | ✅ 28 通过 / 0 失败(17 页 + 导航) |
| 缺数据如实降级 | ✅ 600519 毛利率/PE/PB 缺失显 `—`;评分卡估值维「数据不可得」→ 结论如实 |
| 图注口径诚实 | ✅ 「换手率:流通口径(腾讯)」;形态区标「规则判定 · 非事实」,与数字区分区 |

### 实测样本(600519,2026-09-14)
- 速评结论:**中性**(总分 0);风险:**中**(市场波动 25.6% / 近期 2 条重大公告 / 估值数据缺失)。
- 量价形态标签:**缩量续跌**(2026-09-11,近 3 日 -2.6%、换手 0.23% < 中位数 0.30%)。
- 背离事实:换手峰值 0.85%(07-20)与股价高点 1361.76 元(07-30)相隔 8 日,**未共振**;峰值后 -3.94%。

## 偏差与遗留
- **单口径换手**:仅流通口径,拿不到同花顺自由流通口径 —— UI 已明示,不暗示双口径。⚠️ **2026-09-18 更正**:与同花顺**一致**(见上);详见 `../design/turnover-caliber-findings.md`。
- **行业对比不在 prebuy**:需重跑最重的 `industry` 采集;仍由「完整深研」按钮覆盖。
- **龙虎榜资金面**:provider 存在但未接线 `plan.py`,留待后续。
- **相对指数超额收益**:`vs_index_return_pct` 现为占位。
- **快速模式 markdown 为骨架**:AI 网关不可用时正文不含 AI 段落,但确定性结论完整;配好 AI 后同一按钮即出完整研判。
