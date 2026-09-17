# 公司研报与个股分析拆分（功能分工）

> 状态：**待定稿门**（本文档通过前，不写业务代码）。
> 阶段定位：phase9 续作 —— 在 P9 研报体裁与版面（`report-layout.md`，已上生产）之上，按**功能**把「公司研报」与「个股分析」拆开。
> 关联：issue **#11**；硬前置 = 主体轴泛化 #10（已合并 PR #15）、研报版面 P9-2（已合并 PR #16）。
> 前序文档：[`report-layout.md`](./report-layout.md)（版面规格，本文档**复用**它，不重做）。

---

## 1. 背景与问题

用户诉求（2026-09-17，原话）：

> 「我还是考虑功能上的拆分。」

即：不纠结 profile 命名或表结构，而是从**用户视角**回答「这是两个不同的功能」——它们该有各自的入口、各自的判断标准、各自的节奏。

### 1.1 为什么「重叠」是个误解

issue #11 的原始疑问是「公司研报和个股分析是不是重叠了？」。答案是**不重叠，是两种不同节奏的功能**：

| | **个股分析** | **公司研报** |
|---|---|---|
| 回答的问题 | 这票**现在**能不能动 | 这家公司**是什么质地** |
| 数据面 | 行情 / 量价 / 换手 / 形态 / 资金 | 财务 / 估值 / 行业地位 / 风险 |
| 时效 | **日频** —— 看今天的 | **季频** —— 看几季的趋势 |
| 呈现形态 | **速览**（卡片/仪表盘，一眼看完） | **文档**（成篇，可存、可回看、可对比） |
| 成本 | 便宜，天天跑无妨 | 贵、慢，几周一次 |
| 现有落点 | `/research`（已存在） | `/reports`（P9 版面已就位） |

**病症**：`complete-stock` 把 11 节揉成一份 —— 想快看一眼量价，得陪财务跑完全程；想读公司质地，又被量价噪音稀释。这不是功能重叠，是**两种节奏挤在一个容器里**。

### 1.2 现状：一份 profile 装两种功能

`research/profiles/complete-stock.yaml` 的 11 节：

```yaml
sections: [company, market, volume, turnover, financial, valuation,
           events, announcements, industry, risk, conclusion]
```

按功能归位：

- **公司面**（季频）：`company` / `financial` / `valuation` / `industry` / `risk`
- **个股面**（日频）：`market` / `volume` / `turnover`
- **共享**（都关心）：`events` / `announcements` / `conclusion`

---

## 2. 调研发现：量价章节是「承重墙」（关键）

「公司研报只要基本面、不要量价」听起来直接，但代码里**量价数据是风险分析与评分卡的输入**，不是可选装饰。实测（`research/src/`）：

| 依赖 | 位置 | 后果 |
|---|---|---|
| `risk` 分析要求 price + volume **都在** | `workflow/engine.py:304` `elif price and volume:` | 去掉 → `risk_metrics = None` → **风险章空** |
| 评分卡 `market_trend` 维度读 price | `analysis/scorecard.py:291` | 无 price → 该维度 `unavailable` |
| `fundamental_trend` / `risk` 是**敏感维度** | `analysis/scorecard.py:47` | risk 一缺 → 结论降级「**结论受限**」|
| `analyze_price(bars)` bars 空即抛错 | `analysis/price.py:51` | 不采行情就无法跑 price 分析 |
| `RiskMetrics.symbol` 取自 `price.symbol` | `analysis/risk.py:150` | price 为 None 时构造器缺 symbol |

**结论**：若公司研报彻底不采行情，会连带失去风险章、且结论退化成「结论受限」—— 对一份要「专业」的公司研报是硬伤。

### 2.1 但风险引擎**已有**基本面规则（重要）

细看 `analysis/risk.py` 的 7 条规则：

| # | 规则 | 依赖 | 类别 |
|---|---|---|---|
| 1 | 年化波动率 > 25/40 | price | 量价 |
| 2 | 区间跌幅 < -15% | price | 量价 |
| 3 | 低换手 + 低成交额 | volume | 量价 |
| 4 | 资产负债率 > 50/70% | financial | **基本面** |
| 5 | 净利润同比 < -5/-20% | financial | **基本面** |
| 6 | 近 30 天重大公告 | announcements | **基本面** |
| 7 | PE/PB 数据缺失 | financial | **基本面** |

**7 条里 4 条已是基本面**。所以「改风险引擎」不是新建规则，而是**把 1–3 条量价规则改为可选**，让 4–7 在无量价数据时照常独立运行。改动比预想小。

### 2.2 另一处历史耦合

```python
# report/sections.py:58
Chapter("price", "股价表现", ("fact", "calc"), ("market", "company")),
```

「股价表现」章的触发条件含 `company`。而 `company` 节**本身不渲染任何章节**（它只是机检的 `meta` 键，`quality_gate.py:131`）。这是个历史耦合：任何含 `company` 的 profile 都会冒出一个量价章 —— 公司研报若不修这里，会凭空多出「股价表现」。

> 实测：`build_manifest(['company','financial','valuation','industry','risk','conclusion'])` 确实产出了「股价表现」章。故此项**必须**在本次修掉。

---

## 3. 决策（2026-09-17 用户选定）

| # | 问题 | 选定 |
|---|---|---|
| D-1 | 公司研报用户主要在哪找到 | **以 `/reports` 为主** —— 研报体裁归 `/reports`；个股中心 `/stock/:code` 只留入口链接 |
| D-2 | `prebuy`（买入前速评）与「个股分析」关系 | **并存，职责分开** —— prebuy 是「决策前快动作」（含买入视角话术）；个股分析是「日常中立速览」 |
| D-3 | 公司研报内容深度 | **按现有内容拆**（现有基本面章节换专业版面呈现，不新增数据源） |
| D-4 | 公司研报是否保留行情采集 | **改风险引擎** —— 让风险分析在无量价时仍可用基本面规则（§2.1），公司研报**完全不采行情** |

---

## 4. 方案

### 4.1 功能与 profile 的分工

| profile | 功能 | 主体 | 入口 | sections（按功能定） |
|---|---|---|---|---|
| **`company`**（新） | **公司研报** | 6 位股票码 | `/reports` | `company, financial, valuation, industry, risk, conclusion` |
| **`stock`**（新） | **个股分析** | 6 位股票码 | `/research` | `market, volume, turnover, patterns, conclusion` |
| `prebuy` | 买入前速评（不变） | 6 位股票码 | `/stock/:code` 速评卡 | 维持现状 |
| `short-term` | 短线视角（不变） | 6 位股票码 | `/research` | 维持现状 |
| `complete-stock` | **兼容档案**（不废弃） | 6 位股票码 | 不出现在选择器 | 维持现状（历史 run 可读） |
| `industry` | 行业研报（已上生产） | `sw` + 6 位 | `/reports` | 维持现状 |

**关键**：`complete-stock` **保留不删** —— 生产有 7 份历史 run 用它（含 `run_id` 前缀），删除会破坏「多份 run = 时间序列」与决策记录 `based_on` 边。它只是从**新建选择器**里隐去。

### 4.2 风险引擎改造（D-4）

**签名改为可选量价**：

```python
# analysis/risk.py —— 现状
def analyze_risk(price: PriceMetrics, volume: VolumeMetrics,
                 financial=None, announcements=None) -> RiskMetrics:

# 改为
def analyze_risk(symbol: str, as_of: date,
                 price: Optional[PriceMetrics] = None,
                 volume: Optional[VolumeMetrics] = None,
                 financial: Optional[FinancialMetrics] = None,
                 announcements: Optional[List[Announcement]] = None) -> RiskMetrics:
```

- 规则 1–3 各自包 `if price:` / `if volume:` 守卫（量价缺失即跳过，**不猜、不补**）；
- `symbol` / `as_of` 由调用方显式传入（不再从 `price` 借）；
- 规则 4–7 无条件照跑。

**调用点同步**（`workflow/engine.py:304`）：

```python
# 现状：price 与 volume 缺一即 risk_metrics = None
elif price and volume:
    self.context["risk_metrics"] = analyze_risk(price, volume, financial, announcements)
else:
    self.context["risk_metrics"] = None

# 改为：有任一可用数据就产出（纯基本面路径也能出风险章）
elif price or volume or financial or announcements:
    self.context["risk_metrics"] = analyze_risk(
        self.symbol.code, self.as_of, price, volume, financial, announcements)
else:
    self.context["risk_metrics"] = None
```

**行业主体分支不变**（`industry_metrics` 走 `analyze_industry_risk`，已有独立实现）。

**副作用（正向）**：评分卡的 `risk` 敏感维度在公司研报路径上**变为可用** → 结论不再误报「结论受限」。

### 4.2.1 ⚠️ Evidence 缺口（同 P9-3 踩过的坑，必须在设计里解决）

**问题**：`run_analysis`（`analysis/engine.py:36`）同时负责算指标**和登记 Evidence**，而它的调用点被 bars 守住：

```python
# workflow/engine.py:335-347
def _refresh_evidence(self):
    ...
    bars = self.context.get("bars", [])
    if not bars:
        return          # ← 无 bars 直接返回,证据一条不登记
```

公司研报不采行情 ⇒ `bars` 空 ⇒ **Evidence 全空** ⇒ 机检 `evidence_completeness` 失败（`quality_gate.py:189`），且 `risk` 的 Evidence（`analysis/engine.py:100-112`，`run_analysis` 内）也登记不上 ⇒ 封面机检徽标显示「未过」。

**这不是新坑**：P9-3 行业研报撞的是同一个（其注释在 `engine.py:234` 明写「个股的 Evidence 由 `_refresh_evidence` 建（它要求 bars，对指数不适用）」），解法是**并列的独立登记函数** `add_industry_evidence`（`analysis/engine.py:125`）。

**解法**：照此先例，为基本面主体新增 `add_fundamental_evidence(store, financial, risk, events)`：

- 登记「基本面分析」章的来源数字（ROE / 净利率 / 负债率 / 营收同比 / 净利同比 / PE/PB）→ `section="financial"`；
- 登记风险项（`risk_level` / `risk_veto`）→ `section="risk"`；
- 在 `_execute_analyze` 的 `risk` 分支（`engine.py:287`）中，当 `industry_metrics is None` **且 bars 为空**时调用它（仿 `add_industry_evidence` 的挂载位置）。

**备选（更简）**：让 `_refresh_evidence` 在无 bars 但有 financial 时也走一条**财务专用**的登记分支。二选一，倾向独立函数（与行业先例对称，`run_analysis` 保持「必须 bars」的纯个股语义）。

> 验收已补：§7「公司研报机检**通过**（Evidence 非空）」。

### 4.3 章节清单解耦（§2.2）

```python
# report/sections.py:58 —— 去掉 company 触发
Chapter("price", "股价表现", ("fact", "calc"), ("market",)),
```

安全性：现有所有含 `company` 的 profile **同时**含 `market`（`complete-stock` / `short-term` / `prebuy`），故 `market` 触发照常命中，**零回归**；`industry.yaml` 两者都无。实测无 profile 出现「有 company 无 market」的形态。

### 4.4 前端

| 页面 | 改动 |
|---|---|
| `/reports`（列表） | 按 `subject_type` 已分组（P9-2 已完成）—— 公司研报自然落入「公司」组，**零改动** |
| `/reports/:runId`（阅读器） | 复用 P9-2 版面（封面头/目录/三域/免责）—— **零改动** |
| `/research`（个股分析） | 触发选择器加 `stock` profile；`complete-stock` 从选择器隐去 |
| `/stock/:code` | 「深研」区按 profile 分成**公司质地** / **个股分析** 两块，各自独立触发 |

`constants.ts` 的 `RESEARCH_PROFILES` 相应更新（新增 `company` / `stock` 标签，`complete-stock` 移出选择器但保留 `RESEARCH_PROFILE_LABEL` 映射供历史 run 回显）。

---

## 5. 范围

**做**：

- 新增 `research/profiles/company.yaml` 与 `research/profiles/stock.yaml`。
- 风险引擎量价规则可选化（§4.2）+ 调用点同步。
- **基本面 Evidence 登记**（§4.2.1）—— 否则公司研报机检必失败。
- 章节清单「股价表现」触发解耦（§4.3）。
- 前端 profile 选择器与 `/stock/:code` 分区。
- 单测：风险引擎在无量价时仍产出基本面风险项；`section_manifest` 对两个新 profile 的装配断言。

**不做**（见 §7）：

- 不改风险引擎的**规则内容**（只改可选性）——不新增财务风险规则。
- 不删 `complete-stock`、不迁移历史 run。
- 不动 `research_runs` schema。
- 不为 `stock` profile 新增数据源（形态分析用现成的 P7 `patterns`，已随 PR #20 并回 dev）。
- 不做宏观研报（#13）。

---

## 6. 阻塞与待确认

### 6.1 ✅ dev↔master 分叉（曾为硬阻塞，2026-09-17 已消解）

**原阻塞**：生产跑 master 线、开发在 dev 线，两条线 profile 集不一致 —— dev 缺 `prebuy`（P7）与 `patterns` 节注册，导致按设计含 `patterns` 的 `stock` 在 dev 上加载即校验失败。

**已消解**（2026-09-17）：`sync/master-into-dev`（PR #20，master→dev 方向）已把 P7/P8 并回 dev，**无冲突自动合并**。现 `dev` 为 `master` 内容超集：`research/profiles/prebuy.yaml`、`research/src/analysis/patterns.py`、`profile.py` 的 `patterns` 节注册均已在 dev。

| | dev（现开发基线） | master（生产） |
|---|---|---|
| `complete-stock` / `short-term` | ✅ | ✅ |
| `industry`（P9-3） | ✅ | ✅ |
| `prebuy`（P7） | ✅ **已并回** | ✅ |
| `patterns` 节注册 | ✅ **已并回** | ✅ |

根因（无「谁是源」的发布纪律）由 PR #21 固化修复（全局 `~/.claude/CLAUDE.md`「发布纪律」+ 项目 `CLAUDE.md`「分支与发布纪律」）。**本次 P9-4 可直接在 dev 开工，`stock` 含 `patterns` 不再受阻。**

### 6.2 其他待确认

- `stock`（个股分析）的评分卡维度：量价档应只含 `market_trend`（+ 可用时 `recent_events`），**不含** `fundamental_trend` —— 后者是敏感维度，缺失会误报「结论受限」。需确认维度子集。
- `company` 的评分卡：`business_quality` / `fundamental_trend` / `valuation` / `recent_events` / `risk`；**不含 `market_trend`**（无量价数据）。
- `/stock/:code` 现有「深研」按钮（`DeepResearchButton`，默认 `complete-stock`）的**默认 profile 归属**需重新指派。

---

## 7. 验收

- [ ] 同一 code 可分别产出「公司研报」与「个股分析」，各自 status / 产物 / 机检独立可追溯。
- [ ] 公司研报**不采集行情**（plan 无 `collect_market` 任务），且**风险章非空**（基本面规则生效）。
- [ ] 公司研报**机检通过**（`evidence_completeness` 有基本面 Evidence，§4.2.1）。
- [ ] 公司研报正文**不出现**「股价表现」「成交量与换手率」章。
- [ ] 公司研报评分卡**不误报**「结论受限」（`risk` 敏感维度可用）。
- [ ] `complete-stock` 历史 run 仍可读、可展示（零回归）。
- [ ] `prebuy` / `short-term` / `industry` 行为**逐字节不变**。
- [ ] `go build/vet/test` + `cd research && pytest` 全过。
- [ ] `scripts/check-research-isolation.sh` 通过。
- [ ] 前端 `npx tsc --noEmit && npx vite build` 通过。
- [ ] 旧报告（无 `section_manifest`）降级路径仍可用。

---

## 8. 实施阶段（建议）

| 阶段 | 内容 | 依赖 | 交付 |
|---|---|---|---|
| **P9-4a** 风险引擎可选化 | `analyze_risk` 签名 + 守卫 + 调用点 | — | 风险引擎可在无量价时工作 |
| **P9-4b** 章节清单解耦 | `sections.py` 去掉 `company` 触发 | — | 无凭空量价章 |
| **P9-4c** 新 profile | `company.yaml` / `stock.yaml` + 评分卡维度 | P9-4a/b | 两个功能可独立触发 |
| **P9-4d** 前端 | 选择器 + `/stock/:code` 分区 | P9-4c | 功能在界面上分离 |

> P9-4a 与 P9-4b 相互独立，可并行；P9-4c 依赖两者；P9-4d 依赖 P9-4c。
> §6.1 分叉阻塞已消解（PR #20），P9-4c 的 `stock` 内容范围即为设计全文（含量价形态）。

---

## 9. 关联

| 关联 | 说明 |
|---|---|
| #10 主体轴泛化 | ✅ 已合并（PR #15）—— 本拆分的硬前置 |
| P9-2 研报版面 | ✅ 已合并（PR #16）—— 本拆分**复用**版面，不重做 |
| #12 行业研报 | ✅ 已上生产 —— `company` 与 `industry` 在 `/reports` 并列 |
| #13 宏观研报 | 后续；同样复用版面，主体 = `macro:<key>` |
| dev↔master 分叉 | ✅ 已消解（PR #20 并轨 + PR #21 发布纪律），见 §6.1 |
| `report-layout.md` §7 | 「#11 的 profile 关系需在 #11 内单独定夺」—— 本文档即该定夺 |
