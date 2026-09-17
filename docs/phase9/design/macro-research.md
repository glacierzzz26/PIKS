# 宏观研报：以宏观维度为主体（新建宏观数据管线）

> 状态：**草案，待定稿门**。阶段定位：phase9 续作 —— 在 P9-1 主体轴泛化（#10）/ P9-2 研报版面 / P9-3 行业数据管线（#12）之上，新建**宏观数据管线**，把研报主体从「个股 / 行业」扩到「宏观维度」。
> 关联：issue **#13**。硬前置 = #10（PR #15 已合并）、P9-2 版面（PR #16 已合并）。**结构模板 = #12 行业管线**（PR #15）。
> 前序文档：[`report-layout.md`](./report-layout.md)（版面规格，**复用不重做**）、[`p9-3-datasource-findings.md`](./p9-3-datasource-findings.md)（**口径禁令**的体例模板，本文档 §2/§4 照此写）、[`company-analysis-split.md`](./company-analysis-split.md)（分期与实施记录的体例）。

---

## 1. 背景与问题

### 1.1 诉求（issue #13 原文）

> 新增「宏观研报」：以宏观主题为主体（大盘 / 利率 / 汇率 / 政策 / 物价 等），而非个股或行业。
> **明确不做**：不做宏观预测 / 政策预判；不做资产配置建议；不接付费数据源。

### 1.2 现状：全站没有任何宏观数据源

`research/src/providers/` 只有 `market` / `financial` / `news` / `announcement` / `capital` / `industry` 六个 provider，**没有宏观**。issue 的判断成立：这不是加个 profile，而是**新建一整条数据管线**。

### 1.3 已经就位的地基（**不要重做**）

P9-1 主体轴泛化把「研报主体不限于个股」这件事在地基层已经做完了：

| 已就位 | 位置 | 说明 |
|---|---|---|
| Go 已认 `macro:<key>` | `internal/store/research_runs.go:112-129` | `NormalizeSubject` 识别序为 **行业 → 宏观 → 公司**；`SubjectMacro` 常量已定义 |
| 宏观码不加交易所前缀 | `internal/research/orchestrator.go:453-461` | `SubjectFullCode` 对 industry/macro **原样返回**（加 sh/sz/bj 会造出 `bj801010` 那类错码） |
| **零 schema** | `internal/web/api_research.go:318-321` | `subject_type` **不落库**，读时由 `code` 重算 ⇒ 无 migration |
| 前端类型与分组已就位 | `frontend/src/lib/types.ts:352`、`constants.ts:65-69`、`reportList.ts:36-40,73` | `ResearchSubjectType` 已含 `"macro"`；`RESEARCH_REPORT_TYPES.macro = "宏观研报"`；`TYPE_ORDER` 已有宏观分组且自动过滤空组 |
| 结论 chip 的兜底路径 | `frontend/src/lib/report.ts:148-155` | 取 `scorecard.overall_label ?? risk.overall_level` —— 宏观无评分卡，**risk 是 chip 唯一来源**（故 §5.7 的 risk 章非装饰） |

### 1.4 真正缺的五件事（本文档要解决的）

1. **Python 侧无宏观主体表示**：`resolve_symbol`（`models/symbol.py:69-101`）无宏观分支。实测 `resolve_symbol("macro:cn_cpi")` → `Market.UNKNOWN`、`full_code == "unknownmacro:cn_cpi"`（错码，会流进封面标题与合成提示）。`Market` 枚举（`:5-20`）无宏观成员。
2. **Go 侧无宏观展示名**：`subjectPresentation`（`api_research.go:320-339`）只对 `SubjectIndustry` 取 `display_name`；宏观落到 `CompanyNamesByCodes`（查 `type='company'`）→ 空名 → 前端退回裸码 `macro:cn_cpi`。
3. **封面与合成只有两个主体分支**：`markdown.py:134-151`（封面标题）、`synthesis.py:55-70, 82-95, 103-105`。⚠️ **个股分支字节冻结** —— 它是 `internal/ai/mock.go` 的关键词匹配依据（`synthesis.py:53-54` 已有警告）。
4. **机检有三处只认个股/行业**：`quality_gate.py` 的 `SECTION_TO_KEY`（`:130-146`）、`SECTION_FOR_SECTION`（`:170-183`）、`time_boundary`（`:223-245`，读 `price` / `industry_index.price` / `financial_snapshots` —— 宏观三者皆无）。
5. **前端没有任何入口能触发非个股档案**：`AnalystTrigger.tsx:25` 用 `isStockCode` 硬锁 6 位数字、`:43` 用 `replace(/\D/g, "")` 剥掉一切非数字。行业研报至今只能经 API/CLI 触发，`industry` 只进 `RESEARCH_PROFILE_LABEL`（`constants.ts:50-59`）供历史回显。

---

## 2. 数据源实测事实（Spike 结论）

> 2026-09-17 实测 akshare **1.18.94**（与 `research/requirements.txt` 的 `akshare>=1.18.0` 相符；该文件有意不锁上限，见其注释）。以下每条都是**跑出来的**，不是查文档得来的。

### 2.1 否决 issue 建议的接口（**关键发现**）

issue #13 建议用 `macro_china_cpi_yearly` / `macro_china_m2_yearly` / `macro_china_gdp_yearly`。实测**这三者不可用**：

| 接口 | 返回列 | 实测末个真实值 | 今天 |
|---|---|---|---|
| `macro_china_cpi_yearly` | 商品/日期/今值/预测值/前值 | **2025-08**（另有 2025-09 一行 `今值=NaN` 占位） | 2026-09 |
| `macro_china_m2_yearly` | 同上 | **2025-08** | 2026-09 |
| `macro_china_gdp_yearly` | 同上 | **2025-07** | 2026-09 |

三点原因使其不可用：

1. **陈旧一年**：末个真实值停在 2025 年年中。用它做「当前宏观读数」= 拿去年的数当今天。
2. `日期` 是**发布日期**（发布日历口径）而非统计期，与 `今值` 的统计期错位一整个月。
3. 末行常是 `今值=NaN` 的待发布占位行，直接取 `iloc[0]` 会取到空值。

→ **决策：V1 一律不用 `*_yearly` 系列**。provider 内写死「不得回退到 `*_yearly`」的注释，并配一条**断言它们从未被调用**的测试。

### 2.2 采用源（NBS 口径，经东方财富数据中心）

| 维度 | akshare 接口 | 实测形状 | 频率 | 最新期 | 序列起点 |
|---|---|---|---|---|---|
| **CPI** | `macro_china_cpi()` | 224 × 13 | 月 | **2026-08** | 2008-01 |
| **PPI** | `macro_china_ppi()` | 248 × 4：`月份/当月/当月同比增长/累计` | 月 | **2026-08** | 2006-01 |
| **M2** | `macro_china_money_supply()` | 224 × 10：M2/M1/M0 的 数量(亿元)+同比增长+环比增长 | 月 | **2026-08** | 2008-01 |
| **GDP** | `macro_china_gdp()` | 82 × 9 | 季（累计） | **2026 Q1-2** | 2006 Q1 |

备选（**V1 不做**，登记为延期）：`macro_china_lpr()`（`TRADE_DATE`/`LPR1Y`/`LPR5Y`，实测 `LPR5Y` 仅 2019-08 起 ⇒ 分位基准过短，须各自诚实标注）、`macro_china_shibor_all()`（2370 × 17，日频，另一条路径）。

### 2.3 硬事实一：CPI/PPI 的「当月」**不是百分比**，是「上年同月=100」的指数

实测逐行校验，全部 224 / 248 行成立：

| 期 | 全国-当月 | 全国-同比增长 | 当月 − 100 |
|---|---|---|---|
| 2026-08 | 100.8 | 0.8 | 0.8 |
| 2026-07 | 100.5 | 0.5 | 0.5 |
| 2008-02 | 108.7443 | 8.7443 | 8.7443 |

`|当月 − 100 − 同比|` 的**最大偏差**：CPI = **0.0**（全部精确相等）；PPI = **0.1**（248 行中 4 行有 0.1 的四舍五入余数）。

→ 口径规则：**只有 `同比`/`环比` 列按百分比入卡**；`当月`/`累计` 只能标为「指数（上年同月=100）」，**绝不以 % 呈现**。字段命名即体现差异（`level_index` 而非 `level_pct`）。断言须用**容差 0.1**，不能用精确相等。

### 2.4 硬事实二：GDP 是**累计口径**

实测：2026 年第1季度 = 334192.9 亿元 → 2026 年第1-2季度 = 695704.0 亿元（累计）。4 行/年，20 行标 `第1-4季度`。

三条硬规则：

- **(a) 绝对值序列不得直接当期数画图或排名** —— 每年年初归零，直接画会产出锯齿状的假「Q2 崩塌」。
- **(b) `国内生产总值-同比增长` 是**累计同比**，必须标注「累计同比」**，不得显示为「GDP 同比」。
- **(c) V1 只允许**把「单季水平 = 同年内 累计(to) − 累计(to−1)」作为**单季水平（亿元）**呈现，并标注「由累计差分所得，非原始披露值」；**禁止**差分出单季同比（需跨年两跳，且会在同一张表里印出第二个数值不同的「GDP 同比」，读者必混）。

**已实测差分正确**（按「同年内、按结束季度排序」，第 1 季度行不参与差分）：

```
2025 Q1  累计 318466.4 → 单季 318466.4
2025 Q1-2 累计 695704.0? 否 —— 实测 341395.2（= 累计(Q1-2) − 累计(Q1)）
2026 Q1-2 累计 695704.0 → 单季 361511.1   ← 量级合理，无负值/无锯齿
```

⚠️ 解析坑：季度标签有两种形态 `2026年第1-2季度`（累计区间）与 `2026年第1季度`（单季）。**排序键必须是 (年, 结束季度)**，按 (年, 起始季度) 排序会让 Q1 与 Q1-2 撞键。

### 2.5 硬事实三：四个接口**全部新→旧排序**

`analysis/industry.py:198-200` 的分位写法依赖 `closes[-1]` 是「最新」。宏观若沿用源站旧序，`[-1]` 会取到 2008 年 —— 分位数会算成「2008 年水平的历史分位」，一个不会报错但完全错误的数字。

→ **provider 必须升序化，并断言日期单调递增**（断言失败即抛，不静默继续）。

### 2.6 硬事实四：**发布日拿不到**

四个东财接口只有 `REPORT_DATE`（统计期，如 `2026-08-01`）+ `TIME`（`"2026年08月份"`），**没有发布日期字段**。带发布日的是 §2.1 那批陈旧一年的 `*_yearly`。

→ **V1 不得写任何「发布于 X 月 X 日」的表述**，也不得用 `*_yearly` 反推发布滞后（那是推断，不是事实）。

### 2.7 硬事实五：源站期号会被 Number Lint 当数据数字扫描

实测 `number_lint._mask_text`（`:148-155`）的日期掩码是 `\d{4}[-/]\d{1,2}[-/]\d{1,2}` 与 `\d{1,2}月\d{1,2}[日号]`，**两者都匹配不到**「2026年08月份」/「2026年第1-2季度」：

```
'2026年08月份 CPI 同比 0.8%'      → 掩码后原样返回
'2026年第1-2季度 GDP 同比增长 4.7%' → 掩码后原样返回
```

即期号里的 `2026`、`08`（或 `1`、`2`）会被当数据数字扫描，卡里没有即 **lint 必挂**。

→ **provider 必须同时产出可对账的期号字段**：`period_year`(2026) + `period_month`(8)（或 `period_quarter_from`/`period_quarter_to`），使源站期号能被 `collect_numbers_from_json` 命中。这是 §3 D-M6「引用值入 known **零新增代码**」得以成立的**前提**，也是 P9-5a/b 的必测项。

### 2.8 单位：M2 亿元 → 万亿展示可过 lint

M2 水平是**亿元**（2026-08 = 3568083.6 亿元 = 356.81 万亿元）。渲染成「万亿元」时文本值 × 1e4 落在 `number_lint.UNIT_SCALES = (1, 1e4, 1e8)`（`number_lint.py:53`）的对账带上（356.81e4 ≈ 3568100 vs 卡内 3568083.6，相对误差 0.005% ≪ 2%）⇒ 万亿展示**可以**过 lint，但**卡内必须存原始亿元值**。

---

## 3. 关键决策（D-M）

沿用 `report-layout.md` 的 D-R 编号体例。

| 编号 | 决策 | 理由 |
|---|---|---|
| **D-M1** | 主体码 = `macro:<key>`，`key` 为**扁平、自带命名空间**的单个 token：`cn_cpi` / `cn_ppi` / `cn_m2` / `cn_gdp`。**一个 run = 一个维度** | Go 侧 `CutPrefix(s, "macro:")`（`research_runs.go:120-123`）已定型；key 内再带冒号会成 `macro:china:cpi`（双冒号）。粒度论证见 §3.1 |
| **D-M2** | **维度表归 Python 独占**（provider 查表）；Go 只做**形态**判别，不校验 key 合法性 | 承 `api_research.go:316-318` 的既有原则：明确拒绝在 Go 复制申万表（D-11 独立性）。非法 key 在 Python 侧 `resolve` 返回 `None` → collect 抛错 → run 如实 `failed` |
| **D-M3** | `as_of` 语义**拆分**：`meta.as_of` = **报告生成日**（语义不变，`markdown.py:145` 逐字节不动）；**新增** `macro.period.*` 承载**数据边界**：`period_label`(源站原文) / `period_year` / `period_month` / `period_end` / `source_lag_days` / `history_start` / `history_days`。**不写 `released_at`** | §2.6 实测无发布日。行业先例已用 `history_start`/`history_days` 如实暴露边界（`analysis/industry.py:44-46`）。滞后用「报告生成日 − 统计期末」表达，是**可算的事实**而非猜测 |
| **D-M4** | **两章 + 复用 risk 章**：`macro_level`（读数与近期序列）+ `macro_position`（历史定位与趋势）+ `risk`（复用既有 key，走**宏观专用**风险引擎与渲染器） | 对齐行业「多节同源一个 provider」的写法；**不设「口径章」** —— 口径说明走各表下的 `> 口径说明：…`（行业先例 `markdown.py:303-305`）+ 末章免责的宏观专条 |
| **D-M5** | **无评分卡**（`scorecard.dimensions: []`） | 承 `profiles/industry.yaml:27-32` 的既有决策：个股评分卡维度对宏观无对应数据源，硬凑即编造。结论由 AI 三段（研判域）+ 风险等级 chip 承担 |
| **D-M6** | 引用值入 Number Lint **零新增代码**：宏观原始值进 `metrics.json` ⇒ `collect_numbers_from_json`（`number_lint.py:96-100`）自动纳入 known | 前提 = §2.7 的期号可对账字段。**不往 `TEMPLATE_CONSTANTS` 加常量**（会削弱精确匹配）⇒ 窗口期数必须写进卡 |
| **D-M7** | 禁止清单**结构性**杜绝：`MacroMetrics` **不设** `volatility_annual` 字段；`analyze_patterns` **不建任务** | 承 `p9-3-datasource-findings.md §2.1/§3` 的原则：数据源不支持的口径，靠**不产出该字段**杜绝，而非靠 lint 事后拦 |

### 3.1 为什么是「一维度一 run」而不是 `macro:china` 全维度合一

1. **`as_of` 是单值，而频率是混合的**：CPI/PPI/M2 月频，GDP 季频且累计。一份报告只有一个 `as_of`，无法同时诚实描述「数据截止 2026-08」与「数据截止 2026 Q2」。per-dimension 让 D-M3 的滞后声明是**单值、可核对**的。
2. **复用 `industry_index` 的结构而不发明新结构**：行业是「一个 provider、一个主体、多节同源」；宏观是「一个 provider、一个主体（= 一个维度）、两节同源」。`display_name` 也照行业从指标卡取（`macro.ref.name`），Go 改动 = 加一个分支。
3. **`macro:china` 会逼报告变成 4 个浅章**：行业三章之所以成立，是三章读**同一条序列**的不同侧面。CPI 与 GDP 之间**没有**可用的确定性连接（见 §4.2 的「跨维度因果禁止」）。全维度合一会把「跨维度因果」变成读者的默认期待 —— 而这恰是本项目明确不做的事。
4. **验收口径天然对齐**：issue #13 写「至少 3 个宏观维度可产出研报」——per-dimension 下这就是 3~4 个主体码 × 各一次 run；bundle 下「N 个维度」与「1 份报告」的数量关系反而需要额外解释。

> **反方意见（定稿门须确认）**：bundle 只需 1 次触发、1 次 LLM 成本。代价是 D-M3 的诚实性。**不建议**。

---

## 4. 分析层：可算什么 / 禁止什么（`research/src/analysis/macro.py`）

> 体例照搬 `analysis/industry.py:1-11`：模块 docstring 先写「与个股的关键差异」，`Number Lint` 口径一句，**未算出一律 `None`（不补 0，"N/A" 由渲染层负责）**。

### 4.1 合法（确定性、可从本期序列单源复现）

| 指标 | 定义 | 域 |
|---|---|---|
| `latest_level` / `latest_yoy` / `latest_mom` | 最新一期原始值。同比/环比**用源站已发布列**，**不**由水平序列自行重算 | fact |
| `prev_*` + `delta_yoy` | 上期值与差额（pct） | calc |
| `percentile_level` | `(count(x ≤ latest) / N) × 100`，N = 全历史 —— 照 `analysis/industry.py:198-200` 的 `point_percentile` 写法 | calc |
| `percentile_yoy` | 同上，但作用在**同比序列**上 | calc |
| `history_start` / `history_days` | 如实暴露序列起点与长度 —— 照 `IndustryPriceMetrics.history_start/history_days` | fact |
| `window_median` / `window_min` / `window_max`（近 N 期，N 明示且入卡） | 近 N 期中位与极值。**不单列均值** —— 照 `DispersionMetrics`「不做均值」的先例（`industry.py:51`），宏观序列极值影响大 | calc |
| `direction_run` | 连续同向期数（同比连续 N 期回落/回升），纯**状态描述**，无预测 | calc |
| `period_end` / `source_lag_days` | D-M3 | fact |
| `series_window` | 近 N 期明细（期号 + 水平 + 同比 + 环比），供序列展示 | fact |

**为什么同比例用源站已发布列而非自行重算**：自行由水平序列重算的同比会与源站已发布值不等（口径/修订差异），同一张表出现两个不同的「同比」必然误导。**两个分位分别命名、分别标注**（水平分位 ≠ 同比分位，混用即误导）。

### 4.2 禁止（数据源不支持 = 写了即编造）

| 禁止项 | 理由 |
|---|---|
| **年化波动率**（`√12 × std(月度序列)`） | 对齐 `p9-3-datasource-findings.md §3` 的「口径不支持就不写」先例。月频统计量年化是**双重计数**（CPI 同比本身已是变化率）。**结构性杜绝**：`MacroMetrics` 无此字段 |
| **技术形态 / 择时**（`analyze_patterns`、均线、量价共振、一字板） | 宏观序列**无 OHLC、无量** ⇒ 复用即产假字段。`_execute_analyze` 不建 `analyze_patterns` 任务 |
| **成交 / 换手 / 资金 / 龙虎榜** | 同行业先例（`profiles/industry.yaml:7-12`） |
| **任何预测**：未来值、目标位、「预计下月」、「政策将…」 | `report/synthesis.py:70` 与 issue §「明确不做」。`synthesis.py` 宏观分支**再加一条硬约束**（照 `:103-105` 行业专条的写法）：不得对宏观指标作预测或政策预判；不得表述「数据将于 X 月公布」（源站不提供发布日历） |
| **单季 GDP 同比** | §2.4(c)：会与同表已发布的累计同比并列成两个不同数值 |
| **跨维度因果 / 领先滞后**（「M2 领先 CPI」、相关系数） | 需模型 = 推断；且 D-M1 下单份报告只有一条序列，天然无处安放 |
| **「高于/低于政策目标」**（如「CPI 低于 3% 目标」） | 目标值**不在数据源内** = 外部知识 = Number Lint 不可溯源 |
| **季调后环比 / 自行重算同比** | 建模选择，非数据；用源站已发布的环比原值 |
| **周期定位判断**（「处于衰退期」） | 定性判断属**研判域**，只能由 LLM 三段写且受 lint 约束；确定性层不产出 |

### 4.3 两个数据陷阱的处理落点

- **CPI/PPI 的「当月」列**：provider 层字段命名即体现差异 —— 叫 `level_index`（不叫 `level_pct`），并在 `MacroRef.caliber` 写明「同比/环比为 %；当月/累计为『上年同月=100』的指数」。配断言测试：抽样行满足 `|level_index − 100 − yoy| ≤ 0.1`（**容差 0.1，非精确相等** —— PPI 实测有 4 行 0.1 余数）。
- **GDP 累计**：`MacroPoint.cumulative: bool` + `period_quarter_from`/`period_quarter_to`；单季差分按**季度序号**（同年内）而非行位置；排序键 **(年, 结束季度)**。

---

## 5. 指标卡与报告装配

### 5.1 provider（`research/src/providers/macro/macro_provider.py`）

照 `sw_index_provider.py` 的形制：

- `name` 属性；
- `resolve(key) -> Optional[MacroRef]` —— **未知 key → `None`，绝不臆造**（同 `sw_provider.py:11`「无法分类时返回 None（不脑补）」）；
- `get_series(ref) -> List[MacroPoint]`；
- `MacroRef` 带 `key` / `name` / `unit` / `cadence` / `source` / `caliber`。**名称查表，不靠 key 推断** —— 同 `sw_index_provider.py:204-227` 的「层级归属必须查表」原则。

采集**非 optional**（照 `plan.py:124-130` 的 `collect_industry_index`）：序列就是报告本体，采不到就**如实 `failed`**，不降级出空壳报告。

（可选，照行业「进程内 → 在线重试 → 磁盘」的三段式缓存 `sw_index_provider.py:159-202`：宏观是**月/季频**，当日重复触发无需重采，P9-5a 视实现成本决定是否上磁盘缓存。**不做**发布日历缓存 —— 无此数据。）

### 5.2 `profiles/macro.yaml`

```yaml
name: macro
sections: [macro_level, macro_position, risk]
period:
  display: {macro: 36m}      # 展示窗口：近 36 期（月频≈3 年；季频=9 年）
  compute: {macro: 600m}     # 历史分位用全历史（CPI 实测 224 期，上限即全量）
  unit:    {macro: calendar} # 月/季是日历期，不是交易日
scorecard: {dimensions: []}
```

⚠️ `period.unit` 只允许 `trading|calendar`（`workflow/profile.py:88-90`），宏观**必须**选 `calendar`。
⚠️ 现有窗口小助手只认两种后缀：`_display_days` 只认 `d`（`engine.py:29-37`）、`_get_display_quarters` 只认 `q`（`plan.py:52-55`）。**需新增 `_display_periods(profile, key, default)` 认 `m`**，或复用 `q` 表达季度维度、`m` 表达月度维度。

### 5.3 章节登记（**三处，漏一处即 KeyError 或缺章**）

`sections.py` 的 `CHAPTERS` 元组（`:57-82`）加两条 —— `key` / `title` / `domains` / `triggers` 四元组：

- `Chapter("macro_level", "宏观指标读数", ("fact", "calc"), ("macro_level",))`
- `Chapter("macro_position", "历史定位与趋势", ("fact", "calc"), ("macro_position",))`

`risk` 章**复用既有 key**（标题仍为「风险分析」），宏观走**专用渲染器** —— 完全照 `markdown.py:82-85` 的行业分发写法：`builders["risk"]` 按 `macro_metrics is not None` 三分支。

`markdown.py` 的 `builders` 字典（`:92-105`）新增两个 key。**未登记即抛 `KeyError`**（`:119-128` 是有意为之的装配断言：宁可显式报错，也不要静默少一章）。

### 5.4 装配约束（由 D-M6 派生，**必须在设计里写明**）

- **期号可对账**（§2.7）：`macro.period.period_year` / `period_month`（或 `period_quarter_from`/`period_quarter_to`）**必须**进卡，否则正文印源站期号即 lint 挂。
- **关键数字平铺到 `macro` 顶层**：合成提示的指标摘要是**单层**过滤（`synthesis.py:41-50` 只收 `isinstance(v, (int,float,str))`），嵌套一层的 dict 会被整体丢弃 → LLM 看不到任何数字。照 `json_report.py:114-141` 的做法：嵌套块**原样保留**（契约不变）+ 顶层平铺。
- **窗口期数必须入卡**：`period_days`（= 展示期数）写进卡。**不要**往 `TEMPLATE_CONSTANTS`（`number_lint.py:60-74`）加常量 —— 那会削弱精确匹配。（`12` 已在表内，若窗口用 12 期则天然命中；用 36 期则必须靠卡内 `period_days`。）

### 5.5 `quality_gate.py` 四处补齐

| 位置 | 改动 |
|---|---|
| `SECTION_TO_KEY`（`:130-146`） | `macro_level → macro`、`macro_position → macro` |
| `SECTION_FOR_SECTION`（`:170-183`） | `macro_level → macro_level`、`macro_position → macro_position`（与 `add_macro_evidence` 登记一致） |
| `time_boundary`（`:223-245`） | 加**第 4 条**来源 `(json_report.get("macro") or {}).get("period")`。语义仍是「有没有明确时间边界」，只是边界不必长在 `price` 上（同一修法见 P9-4 §10.2）。**并保留「无 period 块仍应失败」的反向守卫测试** |
| `citation_correctness`（`:206-213`） | **不改** —— 它读 `scorecard`，无评分卡（D-M5）⇒ 天然 0 issue |

### 5.6 Evidence（`analysis/engine.py`，模板 = `add_industry_evidence` `:125-171`）

新增 `add_macro_evidence(store, macro_metrics, macro_risk)`，登记 `section="macro_level"`（latest / period / percentile_level）、`"macro_position"`（percentile_yoy / history_days / window stats）、`"risk"`（overall_level）。

⚠️ **不得复用 `run_analysis`** —— 它会顺手对序列跑 `analyze_price`/`analyze_volume`，产出无口径字段。理由与行业逐字相同（`engine.py:130-134`）。

### 5.7 `risk` 分支（`engine.py:287-327`）

新增 `mm = self.context.get("macro_metrics")` 分支，置于**行业分支之后、个股分支之前**；风险引擎 `analyze_macro_risk(m, as_of)` 照 `analyze_industry_risk`（`industry.py:94-139`：只标**可从本卡溯源**的项，不做预测）。同时把基本面 Evidence 的守卫改成 `if im is None and mm is None and not bars:`（`:316`）。

**为什么 risk 章是承重而非装饰**：`frontend/src/lib/report.ts:148-155` 的封面结论 chip 取 `scorecard.overall_label ?? risk.overall_level`。宏观无评分卡 ⇒ **risk 是 chip 的唯一来源**。

### 5.8 `synthesis.py` 宏观分支

`build_synthesis_prompt` 加 `is_macro = "macro" in json_report` **三分支**：主体句（「你是一名宏观研究分析师，正在撰写 {name}（{unit}）的宏观研究报告」）+ 三段槽位说明 + D-M7/§4.2 的宏观专条约束。

⚠️ **个股分支（`:66-70`）与行业分支的字节不得改动** —— 个股文案是 `internal/ai/mock.go` 的关键词匹配依据（`:53-54` 已有警告）。

---

## 6. Go 侧改动（最小）

| 文件 | 改动 | 性质 |
|---|---|---|
| `internal/web/api_research.go:320-339` `subjectPresentation` | 加宏观分支：读 `metrics.macro.ref.name`（照行业读 `industry_index.ref.name` 的写法，**Go 侧不建维度表**，D-M2）；删掉 `:323` 的「宏观:#13 预留」注释 | **承重** |
| `internal/web/api_research.go:163-164`、`internal/research/orchestrator.go:65` | 错误文案补「宏观为 `macro:<key>`（如 `macro:cn_cpi`）」 | 承重（小） |
| `internal/research/artifacts_test.go:240` | 主体表加 `{"macro:cn_cpi", "macro"}` | 测试 |
| `internal/store/research_runs.go:112-129` `NormalizeSubject` | **不改** —— key 合法性归 Python（D-M2） | 不改 |
| `CONTRACT_VERSION`（`cli.py:27` / `artifacts.go:16`） | **不升** —— 只加字段/新 section（`research/README.md:55` 的规则） | 不改 |

---

## 7. 前端改动

| 文件 | 改动 | 性质 |
|---|---|---|
| `frontend/src/lib/report.ts:127-139` `coverSub` | 宏观分支：用 `metrics.macro.period.period_label` / `period_end` 替代泛用的「数据截至 {run.as_of}」；文案**区分**「数据截止」（统计期）与「报告生成」（as_of） | **承重（诚实性）** |
| `frontend/src/lib/constants.ts:56-60` `RESEARCH_PROFILE_LABEL` | 加 `macro: "宏观研报"`（照 `industry` 的历史回显先例） | 承重（小） |
| `frontend/src/components/research/AnalystTrigger.tsx:25,43` | **放开非 6 位输入**（当前 `isStockCode` 硬锁 + `replace(/\D/g,"")` 剥非数字）+ 宏观维度选择器 | 本期做（用户拍板） |
| `frontend/src/lib/constants.ts:42-46` `RESEARCH_PROFILES` | 加宏观档案 chip | 本期做 |
| `frontend/src/lib/reportList.ts` / `types.ts` / `RESEARCH_REPORT_TYPES` / `ReportCover.tsx:23` | **不改** —— 宏观分组与 `display_name \|\| name \|\| code` 兜底**全已就位** | 不改 |

⚠️ **触发 UI 的校验必须分档案**：个股档案仍要求 6 位；宏观档案接受 `macro:cn_*` 形态。**不得**为了放开宏观而让个股档案接受任意字符串（那会把 issue #2 的脏 code 从入口放回来）。

---

## 8. 实施阶段（P9-5a → P9-5d）

| 阶段 | 交付物 | 依赖 | 关闭 |
|---|---|---|---|
| **P9-5a** 主体解析 + 数据源 | `models/symbol.py` 宏观分支（**`full_code` 必须逐字节等于 Go 的规范码** `macro:cn_cpi`）；`providers/macro/macro_provider.py`；`tests/test_macro_provider.py` + `test_symbol.py` 扩例；本文档 §2 实测表定稿 | #10（已合） | #13「数据源」「主体」 |
| **P9-5b** 分析层 + 报告（月度） | `analysis/macro.py`；`engine.py::add_macro_evidence`；`workflow/{profile,plan,engine}.py`（含 `m` 后缀窗口小助手）；`profiles/macro.yaml`；`report/{sections,markdown,json_report,synthesis}.py`；`quality_gate.py` 四处；Go `subjectPresentation` + 文案；`tests/test_macro_*.py` | P9-5a | #13 主体验收（**≥3 维度**） |
| **P9-5c** GDP（季频/累计）+ 全链路 E2E | 季频解析（`第a-b季度` → `period_quarter_from/to` + `period_end`）、`cumulative` 标记、单季差分与披露语；`cn_gdp` 维度；Go→PG→`/api/v1/research-runs` 端到端（含 **run_id / 产物文件名含冒号**实测）；`research/README.md` 补 `as_of` 语义 | P9-5b | #13 字面清单「CPI/M2/GDP」 |
| **P9-5d** 前端触发 UI | `AnalystTrigger` 分档案校验 + 宏观维度选择器；`glossary.ts`/`help.tsx` 术语 | P9-5b（可与 5c 并行） | 残量 UI（用户拍板本期做） |

### 8.1 范围与防越界

**不做**：横向维度合成、跨维度相关/领先滞后、政策/汇率维度、LPR/SHIBOR、发布日历、季调、评分卡、任何预测、任何新 Evidence 类型、任何 schema 变更。

**不改**（防越界）：`analysis/{price,volume,patterns,scorecard}.py`、`models/bar.py`（⚠️ **不要**把宏观序列塞进 `Bar` —— 它无 OHLC/无量纲）、`number_lint.py` 的 `TEMPLATE_CONSTANTS`、`internal/store/research_runs.go` 的 `NormalizeSubject` 逻辑、个股/行业 synthesis 与 markdown 的既有分支（**逐字节冻结**）。

**明确延期并登记**：`macro:cn_lpr` / `cn_shibor`（日频路径；`LPR5Y` 实测仅 2019-08 起 ⇒ 分位基准过短，须各自诚实标注）。

---

## 9. 验收

对应 issue #13 的验收清单，逐条落法：

1. **≥3 个宏观维度可产出研报，含时间序列 + 历史分位** → `macro:cn_cpi` / `cn_ppi` / `cn_m2`（P9-5b）+ `cn_gdp`（P9-5c）；每份含 `series_window`（时间序列）+ `percentile_level`/`percentile_yoy`（历史分位，附 `history_start`/`history_days`）。
2. **引用值全部标注来源且入 Number Lint known 集** → **零新增代码**（D-M6）+ 一条 `lint_markdown_report(md, js).issues == 0` 的测试（维度各一例）+ 卡内 `macro.ref.source`。
3. **`as_of` 对宏观的语义在代码注释与 README 说清** → D-M3；落点：`profiles/macro.yaml` 注释、`analysis/macro.py` docstring、`markdown.py` 封面行、`research/README.md` 产物说明、`json_report.py` 的 `macro.period` 结构注释。
4. **`pytest` 有 macro provider 用例；契约版本不变**（`contract: 1`）。
5. **版面**：`/reports` 宏观分组下出现在期报告；封面副信息显示「数据截止 2026-08」且与「报告生成」区分；三域标记由 `section_manifest` 驱动。

---

## 10. 风险与定稿门待决

按影响排序，**定稿门须逐条确认**：

1. **一维度一 run**（D-M1）—— 决定 §3.1 / §5.2 / §7 全部。已倾向确认。
2. **首发维度集**：已定 **CPI / PPI / M2 / GDP 全进首发**（用户拍板 GDP 进首发）。代价：P9-5b/5c 合起来要吞下 §2.4 的季频/累计/差分路径。
3. **发布日**：接受「只披露统计期末 + 滞后天数、**不写发布日**」（D-M3，§2.6 实测根据充分）？若要发布日，须另接 NBS 发布日历（新 Spike，出 V1 范围）。
4. **`risk` 章**：接受「复用 risk 节 key + 宏观专用规则」？（砍掉则封面**没有**结论 chip —— `report.ts:148-155` 的唯一来源。）
5. **GDP 单季差分是否上屏**（须附「由累计差分、非原始披露值」披露语，§2.4c）。
6. **run_id / 产物文件名含冒号**（`macro:cn_cpi_metrics.json`）：Linux/Docker 合法，**Windows 不合法**。建议保留并在 P9-5c E2E 实测；若出问题，只在 `NewRunID`（`orchestrator.go:429-431`）内做 `:` → `-` 消毒，`code` 列**不变**。
7. **无评分卡**（D-M5）确认 —— 结论只由 AI 三段 + 风险 chip 承担，与行业一致。
8. **前端放开非 6 位输入**（§7）与 issue #2 脏 code 防线的关系 —— 校验**必须分档案**，个股仍锁 6 位。

---

## 11. 关联

| 关联 | 说明 |
|---|---|
| #10 主体轴泛化 | **硬前置**，已合并（PR #15）。`macro:<key>` 形态由它定型 |
| #12 行业研报 | **结构模板**（PR #15）。本文档的 provider / 分析层 / 风险引擎 / Evidence / gate / profile 六件套逐件对齐它 |
| #11 公司研报拆分 | 同一套分期体例（`company-analysis-split.md`）；其 §10.2 的 `time_boundary` 修法是本文档 §5.5 的先例 |
| [`report-layout.md`](./report-layout.md) | 版面规格（封面头 / 目录 / 三域 / 章节清单）。宏观**复用不重做**；它已为 `macro:<key>` 预留表示（D-R3） |
| [`p9-3-datasource-findings.md`](./p9-3-datasource-findings.md) | **口径禁令**的体例模板与判据来源 |
| #4 换手率真实口径 | 无关，但同属「口径不诚实即不写」这一族 |
| #13 本 issue | 宏观研报，本文档即其实施设计 |

---

## 12. 实施记录

> **本节留空待回填**（照 `company-analysis-split.md:304-352` 的体例）。实施完成后回填：各阶段实际交付、端到端实测结果（机检项数 / `data_completeness` / `pytest` 计数 / `go build,v et,test` / `tsc` + `vite build` / 独立性检查）、与本文档的偏离及原因。
