# 数据源扩展工作计划（免费公开源）

> 状态：待推进 | 依据：`spike/findings-data-sources.md`（2026-09-10 实测，20+ 接口逐一验证）
> 原则：只用免费公开数据源；任何接口**接入前先跑 spike 验证**；程序算数值、LLM 只写定性；新增维度必须带 Evidence 与时间边界。
> 推进方式：按任务卡逐项做，每项完成 = 单测通过 + `python3 -m src.cli research 600519` 与 `000560` 冒烟 + `gate` 机检通过 + commit。

---

## 总体规则（所有任务通用）

1. **多源冲突**：同一指标多源数值不一致时，两个 Evidence 都记（`source.provider` 区分），报告用主源、脚注备源。
2. **单位归一**：两融单位是**元**、龙虎榜是**万元**、腾讯市值是**亿**——入 `analysis/` 前统一（建议统一到元）。
3. **限频与缓存**：新 provider 统一加请求间隔 + 指数退避重试；全市场级接口（两融/业绩报表/披露日历）落 `python/src/cache/` 当日缓存（按日期 key）。
4. **不可得即 unavailable**：任何源失败不猜测，沿用现有降级约定。
5. **东财接口分域名**：部分域被限制（全市场实时快照、`stock_zh_index_daily_em`、`stock_individual_info_em`），新接口先跑 `python/spike/data_source_eval.py` 同款探针。

---

## P0：补齐现有缺口（1-2 天，全部实测可用）

### P0-1 估值快照：腾讯 qt.gtimg.cn ⭐ 最优先

- **问题**：`get_valuation` 的 fallback #1（`stock_individual_info_em`）实测连接被断，估值基本一直 `unavailable`。
- **方案**：新增 `providers/market/qt_snapshot.py`（纯 `urllib`，GBK，无新依赖），`akshare_financial.get_valuation` 改为首选该源。
- **实测字段位**（600519）：`[3]`=最新价 1275.16、`[39]`=PE 19.57、`[46]`=PB 6.34、`[44]`/`[45]`=流通/总市值（亿）15940.54。注意字段位无文档，接入时对 2-3 只票回归校验。
- **改动点**：providers/market/qt_snapshot.py（新）、providers/financial/akshare_financial.py、tests。
- **验收**：`research 600519` 报告估值 section 有 PE/PB/市值；JSON 中 `valuation` 无 unavailable。

### P0-2 经营现金流（设计文档 §M3 承诺项）

- **方案**：`get_cashflow` 用 `stock_financial_report_sina(sh600519, "现金流量表")`（主）/ `stock_cash_flow_sheet_by_report_em`（备，254 列）。
- **改动点**：providers/financial、models/financial.py（`FinancialSnapshot` 增 `operating_cashflow`）、analysis/financial.py（现金流趋势 + 净利润含金量=经营现金流/净利润，纯计算）、report/markdown.py 基本面 section、json_report.py、tests。
- **验收**：近 4 期经营现金流序列出现在报告与 JSON；含金量比值为程序计算值。

### P0-3 毛利率 NaN 修复 + 财务交叉验证（完成标准第 14 条）

- **方案**：`stock_financial_abstract`（同花顺，105 列）交叉填充 `stock_financial_analysis_indicator` 的 NaN 毛利率；同时用新浪利润表抽查营收/净利润。
- **改动点**：providers/financial、analysis（冲突记双 Evidence）、tests。
- **验收**：600519 报告毛利率有值；至少 1 个指标实现双源 Evidence。

### P0-4 相对指数表现 + 市场状态地基

- **方案**：`stock_zh_index_daily("sh000300")`（腾讯，实测 0.1s）接进 market provider；`analysis/price.py` 增加区间相对收益（个股 − 沪深300）；事件降噪的事件窗口收益改用相对收益（设计文档 §882）。
- **改动点**：providers/market（index）、analysis/price.py、analysis/event_denoise.py、report、tests。
- **验收**：报告股价表现 section 出现「相对沪深300 超额收益」；事件关联用相对收益表述。

---

## P1：新维度（3-5 天）

### P1-1 资金面 section（资金流 + 两融，与龙虎榜合并）

- **数据**：`stock_individual_fund_flow`（主力/超大单/大单净额+占比，120 日）✅；两融 SSE/SZSE 明细（官方源，2000+ 行/日，日期参数格式 `20260908`）✅。
- **改动点**：providers/capital 扩展或新建 fund_flow provider；analysis/fund_flow.py（融资余额趋势、融资余额/流通市值占比、主力净流入连续性）；profile `sections` 增 `fund_flow`；report 新 section（放在龙虎榜同级）。
- **验收**：000560（两融标的）报告出现资金面 section；600519 沪市两融数据可查。

### P1-2 事件前瞻：披露日历 + 业绩预告

- **数据**：`stock_yysj_em(沪深A股, 报告期)` ⚠️ 只对**已披露期**有效（未来期返回 None）；`stock_yjyg_em(报告期)`。
- **改动点**：新 calendar provider（全市场日历当日缓存）、analysis/events.py 增 `upcoming_disclosure`、报告事件区加「下次财报预约日 / 业绩预告摘要」。
- **验收**：报告能给出下一次财报预约日期；有业绩预告时展示预测区间。

### P1-3 筹码结构 + 股东回报

- **数据**：`stock_zh_a_gdhs_detail_em(symbol)`（个股 63 期，含区间涨跌幅）✅；`stock_fhps_detail_em`（分红送配）✅。⚠️ 全市场版 `stock_zh_a_gdhs` 参数是**报告期**不是代码。
- **改动点**：analysis/shareholder.py（户数变化 vs 区间涨跌联动、户均持股市值趋势）；报告新增小节。
- **验收**：报告出现股东户数趋势及与涨跌的对照。

### P1-4 行情 Failover 第二源

- **数据**：`stock_zh_a_daily(sh600519, qfq)`（新浪，6003 行）✅。
- **改动点**：`failover_provider.py` 挂腾讯→新浪双源 + 健康度记录；对账（两源收盘价 diff 超阈值记 Evidence 警示）。
- **验收**：屏蔽腾讯源时自动切新浪跑通全流程。

---

## P2：深度能力（1 周+）

| 任务 | 方案 | 注意 |
|---|---|---|
| **P2-1 券商研报评级** | `stock_research_report_em`（600519 有 771 条：评级/机构/日期） | **只能落 Opinion tier**，禁止进 fact/Evidence 数值集合；报告标注「卖方观点，非事实」 |
| **P2-2 市场情绪温度计** | `stock_zt_pool_em(date)`（涨停家数/连板高度）+ 指数状态 | 评分卡 market_trend 维度的情绪修正项；只影响评分理由，不改机检规则 |
| **P2-3 公告正文提取** | 东财 notices HTML（32KB，含 `notice_content` 字段标记）或 PDF 直链 + pdfplumber | 完成后 `event_denoise` 从「标题关键词」升级为「内容判断」，重大性分级精度提升 |
| **P2-4 delta 复研** | SQLite 已有归档；diff 引擎对比两次 run 的指标卡与评分 | Phase 1.5 本体；fingerprint 已存 |
| **P2-5 PE/PB 历史序列** | 腾讯快照每日落库自建（乐咕接口本版 akshare 无）；或 `stock_cash_flow_sheet_by_report_em` 推算 | 需要积累时间，先建存储再谈分析 |

## 已知受限（不要投入）

- **北向个股持股**：仅历史至 **2024-08-16**（交易所停发每日数据）。历史回溯可用（1683 条），增量不可得，报告引用必须注明截止日。
- **全市场实时快照 / 东财指数 / `stock_individual_info_em`**：连接被断，维持现状（腾讯 K 线推导 + P0-1 快照）。
- **乐咕估值历史接口**：本版 akshare 无。

---

## 推进检查清单（每项任务完成后）

- [ ] 新接口进了 spike 探针脚本并有 ✅ 记录
- [ ] 单元测试（mock DataFrame，不依赖网络）
- [ ] 真实冒烟：`research 600519` + `research 000560`（大盘/中小盘各一）
- [ ] `python3 -m src.cli gate <run_id>` 七项机检通过
- [ ] 单位归一 & 双源冲突处理符合总体规则
- [ ] 更新本文档任务卡状态与 `spike/findings-data-sources.md`
