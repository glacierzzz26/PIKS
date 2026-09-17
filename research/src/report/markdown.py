"""Markdown 报告生成器

版面装配(P9 #12 / D-R6:「摘要前置、免责置尾」)由 `sections.py` 的章节清单驱动 ——
章节顺序与标题是**单一真源**,与 `json_report.py` 产出的 `section_manifest`(供前端
目录/三域标签)同源,故两者不会再错位。改造前本章节顺序散落在下面的 if 链里。
"""
from datetime import date
from typing import Any, Callable, Dict, List, Optional

from ..analysis.price import PriceMetrics
from ..analysis.volume import VolumeMetrics
from ..analysis.financial import FinancialMetrics
from ..analysis.events import EventMetrics
from ..analysis.risk import RiskMetrics, RiskItem
from ..analysis.capital import CapitalMetrics
from ..analysis.scorecard import Scorecard
from ..analysis.patterns import PatternMetrics
from ..models.financial import FinancialSnapshot
from .sections import (
    AI_SYNTHESIS_SLOT,
    DEFAULT_SECTIONS,
    DISCLAIMER_TITLE,
    EXEC_SUMMARY_TITLE,
    active_chapters,
)

# 中文序号:章节数超十后用阿拉伯数字兜底(与改造前一致)。
CN_NUMS = ["一", "二", "三", "四", "五", "六", "七", "八", "九", "十"]


def _ordinal(idx: int) -> str:
    return CN_NUMS[idx] if idx < len(CN_NUMS) else str(idx + 1)


def format_optional(value, fmt="{:.2f}", suffix="") -> str:
    if value is None:
        return "N/A"
    if isinstance(value, (int, float)):
        return fmt.format(value) + suffix
    return str(value)


def _fmt_pct(value) -> str:
    if value is None:
        return "N/A"
    return f"{value:+.2f}%"


def _fmt_level(value, signed: bool = False) -> str:
    """宏观水平读数格式化(P9-5 / #13) —— **禁止科学计数法**。

    历史上用 `'{:,.4g}'`,大数(亿元量级)会输出 `3.568e+06`,两个问题:
    1. 财经正文不该出现科学计数法;
    2. `number_lint._NUMBER_RE` 的正则**没有指数部分**,只匹配到 `3.568`
       丢掉 `e+06`,于是永远对不上指标卡里的 `3568083.6` → lint 必挂。

    故按量级分派:千以上用千分位 + 1 位小数(亿元级足够),千以下保留 2 位
    (CPI/PPI 水平是「上年同月=100」的指数,两位小数可读且可对账)。
    """
    if value is None:
        return "N/A"
    if not isinstance(value, (int, float)):
        return str(value)
    big = abs(value) >= 1000
    if signed:
        return ("{:+,.1f}" if big else "{:+,.2f}").format(value)
    return ("{:,.1f}" if big else "{:,.2f}").format(value)


def generate_markdown(
    symbol: str,
    as_of: date,
    price_metrics: PriceMetrics,
    volume_metrics: VolumeMetrics,
    fin_metrics: Optional[FinancialMetrics] = None,
    snapshots: Optional[List[FinancialSnapshot]] = None,
    event_metrics: Optional[EventMetrics] = None,
    risk_metrics: Optional[RiskMetrics] = None,
    capital_metrics: Optional[CapitalMetrics] = None,
    scorecard: Optional[Scorecard] = None,
    industry: Optional[Any] = None,
    patterns: Optional[PatternMetrics] = None,
    industry_metrics: Optional[Any] = None,
    macro_metrics: Optional[Any] = None,
    data_source: str = "腾讯财经/akshare",
    sections: Optional[List[str]] = None,
) -> str:
    """生成 Markdown 报告（模板槽位渲染）

    industry_metrics: 行业**本体**指标卡(P9 #12,主体=申万行业指数)。给了它就渲染
        行业三章(行情/估值/成分),而非个股的「行业对比」章。
    macro_metrics: 宏观**维度**指标卡(P9-5 / #13,主体=宏观指标序列)。给了它就渲染
        宏观两章(读数/定位),而非个股或行业章节。
    sections: Profile 要求的报告章节（如 ["market","volume","events",...]），
              决定渲染哪些章节；默认全部（兼容旧调用）。

    版面装配由 `sections.py` 的章节清单驱动(D-R6:摘要前置、免责置尾):首章恒为
    「执行摘要」(AI 三段容器)、末章恒为「数据说明与免责声明」,中间按清单顺序。
    """
    sections = sections or list(DEFAULT_SECTIONS)

    fin_section = _build_financial_section(fin_metrics, snapshots)
    event_section = _build_event_section(event_metrics)
    # 风险章节:三种主体各走独立渲染器(个股/行业/宏观)。行业主体的 risk_metrics 是
    # IndustryRiskMetrics、宏观是 MacroRiskMetrics(两者都无 veto_buy、无涨跌停/
    # 流动性依据);个股路径逐字节不变。
    if macro_metrics is not None:
        risk_section = _build_macro_risk_section(risk_metrics)
    elif industry_metrics is not None:
        risk_section = _build_industry_risk_section(risk_metrics)
    else:
        risk_section = _build_risk_section(risk_metrics)
    capital_section = _build_capital_section(capital_metrics)
    pattern_section = _build_pattern_section(patterns)
    scorecard_section = _build_scorecard_section(scorecard)

    # 章节 key → 渲染函数。key 与 `sections.py` 的 Chapter.key 一一对应
    # (新增章节两处都要登记:清单定顺序与标题,此处只提供内容)。
    builders: Dict[str, Callable[[], str]] = {
        "price": lambda: _build_price_section(price_metrics),
        "volume": lambda: _build_volume_section(volume_metrics),
        "patterns": lambda: pattern_section,
        "financial": lambda: fin_section,
        "events": lambda: event_section,
        "industry": lambda: _build_industry_section(industry),
        "industry_index": lambda: _build_industry_price_section(industry_metrics),
        "industry_valuation": lambda: _build_industry_valuation_section(industry_metrics),
        "industry_structure": lambda: _build_industry_structure_section(industry_metrics),
        "macro_level": lambda: _build_macro_level_section(macro_metrics),
        "macro_position": lambda: _build_macro_position_section(macro_metrics),
        "capital": lambda: capital_section,
        "risk": lambda: risk_section,
        "conclusion": lambda: scorecard_section,
    }

    # 序号由清单位置决定,不再靠人工对齐散落在各 if 分支里。
    body: List[str] = []

    def add_section(title: str, content: str) -> None:
        idx = len(body)
        body.append(f"## {_ordinal(idx)}、{title}\n\n{content}")

    # 一、执行摘要 —— AI 三段定性(研判域)。槽位在此,由 cli.py 的 synthesize 步
    # 替换为「### 执行摘要 / ### 趋势解读 / ### 综合结论」三子段。
    # ⚠️ 与末章免责一样**无条件**渲染:它们是研报体裁的固有骨架,不受 sections 影响。
    add_section(EXEC_SUMMARY_TITLE, AI_SYNTHESIS_SLOT)

    for ch in active_chapters(sections):
        builder = builders.get(ch.key)
        if builder is None:
            # 清单登记了章节却没给渲染函数 = 装配缺件。宁可显式报错,也不要静默少一章
            # (TOC 仍会列出它,前端将指向不存在的锚点)。
            raise KeyError(
                f"章节 {ch.key!r}({ch.title})未在 markdown.py 的 builders 中登记渲染函数"
            )
        add_section(ch.title, builder())

    # 免责声明末行按主体分化:宏观报告**没有价格**,写「价格指标基于前复权计算」
    # 是对不存在口径的虚假声明(行业研报同样无 price,故一并分化)。
    if macro_metrics is not None:
        disclaimer_extra = f"- 统计期为 {macro_metrics.period.label}，数据截止见封面「数据截止」。"
    elif industry_metrics is not None:
        disclaimer_extra = "- 行业指数点位由源站直接提供，未做复权处理。"
    else:
        disclaimer_extra = "- 价格指标基于前复权计算。"
    add_section(DISCLAIMER_TITLE, f"""- 本报告数据来源于 {data_source}，仅供参考，不构成投资建议。
{disclaimer_extra}
- 报告生成时间：{as_of.isoformat()}。""")

    # 封面头:主体感知(P9 #12 / P9-5)。行业主体没有 price_metrics(profile 不含
    # price 分析),硬套「个股研究报告」与 price_metrics.period_days 会既错又崩;
    # 宏观主体两者皆无,故排在第一个判断。
    if macro_metrics is not None:
        ref, p = macro_metrics.ref, macro_metrics.period
        title = f"宏观研究报告：{ref.name}"
        # ⚠️ 宏观的**数据边界**与**报告生成日**必须分开写(D-M3):统计期总落后于
        # 生成日(实测 CPI 滞后 17 天、GDP 滞后 79 天)。混成一句会让人误读
        # 「数据到今天」。下面第一行是数据边界,上方保留生成日。
        period_line = (
            f"> 数据截止：{p.period_end.isoformat()}（{p.label}，"
            f"较报告生成日滞后 {p.source_lag_days} 天）"
        )
    elif industry_metrics is not None:
        ref = industry_metrics.ref
        title = f"行业研究报告：{ref.name}（{ref.level_label} {ref.code}）"
        period_line = f"> 研究周期：近 {industry_metrics.price.period_days} 个交易日"
    else:
        title = f"个股研究报告：{symbol}"
        period_line = f"> 研究周期：近 {price_metrics.period_days} 个交易日"

    return f"""# {title}

> 报告生成时间：{as_of.isoformat()}
{period_line}
> 数据来源：{data_source}

---

{"\n\n".join(body)}
"""


def _build_price_section(price_metrics: PriceMetrics) -> str:
    """个股行情章(个股 profile)。行业主体走 _build_industry_price_section。"""
    return f"""| 指标 | 数值 |
|---|---|
| 最新收盘价 | {price_metrics.end_price:.2f} 元 |
| 区间涨跌幅 | {price_metrics.period_return_pct:+.2f}% |
| 5 日涨跌幅 | {format_optional(price_metrics.return_pct_5d, '{:+.2f}', '%')} |
| 20 日涨跌幅 | {format_optional(price_metrics.return_pct_20d, '{:+.2f}', '%')} |
| 60 日涨跌幅 | {format_optional(price_metrics.return_pct_60d, '{:+.2f}', '%')} |
| 区间最高价 | {price_metrics.max_price:.2f} 元 |
| 区间最低价 | {price_metrics.min_price:.2f} 元 |
| 最大回撤 | {price_metrics.max_drawdown_pct:.2f}% |
| 年化波动率 | {price_metrics.volatility_annual:.2f}% |
| 涨停天数 | {price_metrics.limit_up_days} 天 |
| 跌停天数 | {price_metrics.limit_down_days} 天 |"""


def _build_volume_section(volume_metrics: VolumeMetrics) -> str:
    return f"""| 指标 | 数值 |
|---|---|
| 区间总成交量 | {volume_metrics.total_volume:,.0f} 手 |
| 5 日均量 | {format_optional(volume_metrics.avg_volume_5d, '{:,.0f}')} 手 |
| 20 日均量 | {format_optional(volume_metrics.avg_volume_20d, '{:,.0f}')} 手 |
| 5 日平均换手 | {format_optional(volume_metrics.avg_turnover_5d, '{:.2f}', '%')} |
| 20 日平均换手 | {format_optional(volume_metrics.avg_turnover_20d, '{:.2f}', '%')} |
| 60 日平均换手 | {format_optional(volume_metrics.avg_turnover_60d, '{:.2f}', '%')} |
| 最大单日换手 | {volume_metrics.max_turnover:.2f}% |
| 最小单日换手 | {volume_metrics.min_turnover:.2f}% |
| 量能异常天数 | {volume_metrics.abnormal_volume_days} 天 |"""


def _build_financial_section(
    fin_metrics: Optional[FinancialMetrics],
    snapshots: Optional[List[FinancialSnapshot]],
) -> str:
    if fin_metrics is None:
        return "_财务数据暂不可得。_\n"

    lines = []
    lines.append("### 最近季度")
    lines.append("")
    lines.append("| 指标 | 数值 |")
    lines.append("|---|---|")
    lines.append(f"| 营收同比 | {_fmt_pct(fin_metrics.latest_revenue_yoy)} |")
    lines.append(f"| 净利润同比 | {_fmt_pct(fin_metrics.latest_net_profit_yoy)} |")
    lines.append(f"| ROE | {format_optional(fin_metrics.latest_roe, '{:.2f}', '%')} |")
    lines.append(f"| 毛利率 | {format_optional(fin_metrics.latest_gross_margin, '{:.2f}', '%')} |")
    lines.append(f"| 净利率 | {format_optional(fin_metrics.latest_net_margin, '{:.2f}', '%')} |")
    lines.append(f"| 资产负债率 | {format_optional(fin_metrics.latest_debt_ratio, '{:.2f}', '%')} |")
    lines.append("")

    if fin_metrics.revenue_yoy_trend:
        trend_str = " → ".join([f"{v:.1f}%" for v in fin_metrics.revenue_yoy_trend[:4]])
        lines.append(f"**营收同比趋势**: {trend_str}")
        lines.append("")

    if fin_metrics.net_profit_yoy_trend:
        trend_str = " → ".join([f"{v:.1f}%" for v in fin_metrics.net_profit_yoy_trend[:4]])
        lines.append(f"**净利润同比趋势**: {trend_str}")
        lines.append("")

    if fin_metrics.pe_ttm or fin_metrics.pb:
        lines.append("### 估值")
        lines.append("")
        lines.append(f"- PE(TTM): {format_optional(fin_metrics.pe_ttm, '{:.2f}')}")
        lines.append(f"- PB: {format_optional(fin_metrics.pb, '{:.2f}')}")
        lines.append("")

    if fin_metrics.revenue_yoy_change is not None:
        direction = "↑" if fin_metrics.revenue_yoy_change > 0 else "↓"
        lines.append(f"**营收同比变化**: {direction} {abs(fin_metrics.revenue_yoy_change):.2f} 个百分点")
        lines.append("")

    return "\n".join(lines)


def _build_industry_section(industry: Optional[Any]) -> str:
    """行业对比章节（Phase 1 降级：分类 + 同业财务对比）"""
    if industry is None:
        return "_行业数据暂不可得。_\n"
    if not industry.industry_code:
        return "_行业分类不可得（数据源受限），暂无同业对比。_\n"

    lines = []
    lines.append(f"**行业分类**: {industry.industry_name}（申万三级，共 {industry.peer_count} 只成分股）")
    lines.append("")

    peers = industry.peers
    if not peers:
        lines.append("_同业成分股暂不可得。_")
        return "\n".join(lines)

    # 只展示头部 8 只（按市值降序），全部有财务数据的才出表
    shown = [p for p in peers if p.roe is not None or p.pe_ttm is not None][:8]
    if not shown:
        # 财务全 N/A：只给名单
        names = "、".join(p.symbol for p in peers[:10])
        lines.append(f"同业成分（{len(peers)} 只）：{names}")
        return "\n".join(lines)

    lines.append("| 公司 | 市值(亿) | PE(TTM) | PB | ROE(%) | 净利增速(%) | 营收增速(%) | 股息率(%) |")
    lines.append("|---|---|---|---|---|---|---|---|")
    for p in shown:
        cap = f"{p.market_cap:.0f}" if p.market_cap else "N/A"
        pe = f"{p.pe_ttm:.2f}" if p.pe_ttm else "N/A"
        pb = f"{p.pb:.2f}" if p.pb else "N/A"
        roe = f"{p.roe:.2f}" if p.roe is not None else "N/A"
        npg = f"{p.net_profit_growth:.1f}" if p.net_profit_growth is not None else "N/A"
        rg = f"{p.revenue_growth:.1f}" if p.revenue_growth is not None else "N/A"
        dy = f"{p.dividend_yield:.2f}" if p.dividend_yield is not None else "N/A"
        lines.append(f"| {p.name[:10]} | {cap} | {pe} | {pb} | {roe} | {npg} | {rg} | {dy} |")
    lines.append("")
    lines.append("> Phase 1 降级：仅做同业横比，不做景气度判断。财务数据缺失标 N/A，不推测。")
    lines.append("")
    return "\n".join(lines)


def _build_industry_price_section(im: Optional[Any]) -> str:
    """行业行情章节(P9 #12)。只写点位派生量 —— 不含成交额/换手(量纲不可靠)。"""
    if im is None:
        return "_行业行情暂不可得。_\n"
    p = im.price
    ref = im.ref

    # 历史起点如实标注:三级新设码历史短(851251 实测仅 2021-12-13 起),不能一律写「1999 年以来」。
    hist = ""
    if p.point_percentile is not None and p.history_start is not None:
        hist = (f"\n当前点位处于 **{p.history_start.isoformat()} 以来 {p.history_days} 个交易日"
                f"的 {p.point_percentile:.1f}% 分位**（点位分位，非估值分位）。\n")
    elif p.point_percentile is not None:
        hist = f"\n当前点位处于历史 {p.history_days} 个交易日的 {p.point_percentile:.1f}% 分位。\n"

    return f"""**{ref.name}**（{ref.level_label} {ref.code}{'，上级：' + ref.parent if ref.parent else ''}）

| 指标 | 数值 |
|---|---|
| 最新收盘点位 | {p.end_point:.2f} |
| 区间涨跌幅 | {p.period_return_pct:+.2f}% |
| 5 日涨跌幅 | {format_optional(p.return_pct_5d, '{:+.2f}', '%')} |
| 20 日涨跌幅 | {format_optional(p.return_pct_20d, '{:+.2f}', '%')} |
| 60 日涨跌幅 | {format_optional(p.return_pct_60d, '{:+.2f}', '%')} |
| 区间最大回撤 | {p.max_drawdown_pct:.2f}% |
| 年化波动率 | {p.volatility_annual:.2f}% |
{hist}
> 口径说明：行业指数无涨跌停制度，亦**不提供成交额/换手率**（申万指数该字段量纲与 A 股个股不一致，本报告不采集、不推测）。
"""


def _build_industry_valuation_section(im: Optional[Any]) -> str:
    """估值定位章节(P9 #12 / D-R7)。

    ⚠️ **只做横截面排名,禁止历史分位表述** —— 申万行业估值只有当期快照
    (sw_index_*_info),无历史序列。写「近 5 年 X% 分位」= 编造。
    """
    if im is None:
        return "_行业估值暂不可得。_\n"
    ref = im.ref
    r = im.valuation_rank

    rank_line = "N/A"
    if r.rank is not None:
        rank_line = f"{r.value:.2f}（{r.level_label} {r.universe} 个行业中第 {r.rank}）"

    return f"""| 指标 | 数值 |
|---|---|
| PE（静态） | {format_optional(ref.pe_static, '{:.2f}')} |
| PE（TTM 滚动） | {format_optional(ref.pe_ttm, '{:.2f}')} |
| PB | {format_optional(ref.pb, '{:.2f}')} |
| 股息率（静态） | {format_optional(ref.dividend_yield, '{:.2f}', '%')} |
| **PE(TTM) 横截面位次** | {rank_line} |

> 口径说明：以上为**当期快照**，横截面位次在{ref.level_label}同层级行业内比较。
> 数据源不提供行业估值的历史序列，故本报告**不作估值历史分位判断**。
"""


def _build_industry_structure_section(im: Optional[Any]) -> str:
    """成分结构章节(P9 #12)。中位 + 四分位,不做均值(财务比率受极值影响)。"""
    if im is None:
        return "_成分结构暂不可得。_\n"
    d = im.dispersion
    ref = im.ref

    if d.count == 0:
        return (f"_成分股明细暂不可得_（{ref.level_label}成分需经三级行业聚合；"
                f"行业表挂牌成份数 {im.declared_count or 'N/A'}）。\n")

    roe_q = ""
    if d.roe_q1 is not None and d.roe_q3 is not None:
        roe_q = f"（四分位 {d.roe_q1:.2f}% ~ {d.roe_q3:.2f}%）"

    count_note = f"{d.count} 只"
    if im.declared_count is not None and im.declared_count != d.count:
        # 不等就如实说 —— 不假装聚合完整(新成份未纳入三级表等)
        count_note = f"{d.count} 只（行业表挂牌 {im.declared_count} 只）"

    return f"""| 指标 | 数值 |
|---|---|
| 成分股数量 | {count_note} |
| 成分合计市值 | {format_optional(d.cap_sum, '{:,.0f}', ' 亿元')} |
| ROE 中位 | {format_optional(d.roe_median, '{:.2f}', '%')}{roe_q} |
| 净利润增速中位 | {format_optional(d.net_profit_growth_median, '{:.1f}', '%')} |
| 营收增速中位 | {format_optional(d.revenue_growth_median, '{:.1f}', '%')} |
| PE(TTM) 中位 | {format_optional(d.pe_ttm_median, '{:.2f}')} |

> 口径说明：用**中位数**而非均值（财务比率受极值影响大）。成分财务为个股当期快照。
"""


def _build_macro_level_section(mm: Optional[Any]) -> str:
    """宏观读数章(P9-5 / #13)。

    ⚠️ 期号字段(2026 / 08)与数值**都必须进指标卡** —— 正文印源站期号
    (「2026年08月份」)时,`number_lint._mask_text` 掩不掉它,其中的数字会被当
    数据数字扫描。指标卡里 `period_year`/`period_month` 存为 **int** 正是为此
    (str 会被 `collect_numbers_from_json` 跳过)。
    """
    if mm is None:
        return "_宏观数据暂不可得。_\n"
    ref = mm.ref

    lines = [
        f"**{ref.name}**",
        "",
        "| 指标 | 数值 |",
        "|---|---|",
        f"| 统计期 | {mm.period.label} |",
        f"| 最新读数 | {_fmt_level(mm.latest_level)}（{ref.level_label}） |",
        f"| 同比 | {format_optional(mm.latest_yoy, '{:+.2f}', '%')} |",
        f"| 环比 | {format_optional(mm.latest_mom, '{:+.2f}', '%')} |",
    ]
    # GDP 专用:累计差分出的单季水平(附披露语,见下)
    if mm.latest_single_quarter_level is not None:
        lines.append(
            f"| **单季水平** | {mm.latest_single_quarter_level:,.1f} 亿元 |"
        )
    if mm.prev_label:
        lines.append(f"| 上期（{mm.prev_label}） | {_fmt_level(mm.prev_level)}"
                     f"（同比 {format_optional(mm.prev_yoy, '{:+.2f}', '%')}） |")
        # 累计口径不呈现「读数较上期」:同年内该增量恒等于「单季水平」(重复),
        # 跨年则是两个不同跨度累计总量之差(不可比)。分析层已不产出该字段,
        # 此处如实说明「为什么不给」—— 读者看到残缺行会以为缺数据。
        if mm.period.cumulative:
            lines.append("| 读数较上期 | 见「单季水平」（累计口径不作跨期差） |")
        else:
            lines.append(f"| 读数较上期 | {_fmt_level(mm.delta_level, signed=True)} |")
        lines.append(f"| 同比较上期 | {format_optional(mm.delta_yoy, '{:+.2f}', ' 个百分点')} |")

    # 序列明细(近 N 期)
    if mm.readings:
        lines += ["", f"### 近 {mm.period.window_periods} 期读数", ""]
        lines.append("| 统计期 | 读数 | 同比 | 环比 |")
        lines.append("|---|---|---|---|")
        for r in mm.readings:
            lines.append(
                f"| {r.period_label} | "
                f"{_fmt_level(r.level)} | "
                f"{format_optional(r.yoy, '{:+.2f}', '%')} | "
                f"{format_optional(r.mom, '{:+.2f}', '%')} |"
            )

    # 口径说明:量纲差异(指数 vs 百分比)必须逐维度讲清,否则读者会把
    # 「100.8」当百分比读。GDP 另加累计差分的披露语。
    notes = [ref.caliber]
    if mm.latest_single_quarter_level is not None:
        from ..analysis.macro import GDP_CUMULATIVE_NOTE
        notes.append(GDP_CUMULATIVE_NOTE)
    lines += ["", f"> 口径说明：{chr(10).join('> ' + n for n in notes).lstrip()}",
              f"> 数据来源：{ref.source}。统计期与报告生成日不同，见封面「数据截止」。"]
    return "\n".join(lines) + "\n"


def _build_macro_position_section(mm: Optional[Any]) -> str:
    """宏观历史定位章(P9-5 / #13)。分位与趋势均为**确定性计算**,无预测。"""
    if mm is None:
        return "_宏观定位数据暂不可得。_\n"
    ref, p = mm.ref, mm.period

    # 水平分位:仅在**有意义**的维度上呈现为定位依据(M2/GDP 是名义总量,
    # 分位恒接近 100%,见 provider 的 level_percentile_meaningful)。
    if ref.level_percentile_meaningful:
        level_pct_line = (
            f"| 水平历史分位 | {format_optional(mm.percentile_level, '{:.1f}', '%')} |"
        )
        level_note = "水平为其**指数/可平稳比较**读数，故水平分位具位置信息量。"
    else:
        level_pct_line = (
            f"| 水平历史分位 | {format_optional(mm.percentile_level, '{:.1f}', '%')}"
            f"（**不具位置信息量**，见口径） |"
        )
        level_note = ("本维度为**名义总量**，随经济增长长期上行，水平分位恒接近 100%，"
                      "故定位判断以**同比**分位为准。")

    dir_line = "N/A"
    if mm.direction_run > 0:
        dir_line = f"同比已连续 {mm.direction_run} 期回升"
    elif mm.direction_run < 0:
        dir_line = f"同比已连续 {abs(mm.direction_run)} 期回落"
    elif mm.direction_from_label:
        dir_line = "无连续同向变化"

    return f"""| 指标 | 数值 |
|---|---|
| 全历史区间 | {p.history_start.isoformat() if p.history_start else 'N/A'} 起，共 {p.history_periods} 期 |
{level_pct_line}
| 同比历史分位 | {format_optional(mm.percentile_yoy, '{:.1f}', '%')} |
| 近 {p.window_periods} 期中位 | {_fmt_level(mm.window_median_level)} |
| 近 {p.window_periods} 期最小 | {_fmt_level(mm.window_min_level)} |
| 近 {p.window_periods} 期最大 | {_fmt_level(mm.window_max_level)} |
| 连续同向 | {dir_line} |

> 口径说明：分位基于**全部可用历史**（{p.history_periods} 期，自
> {p.history_start.isoformat() if p.history_start else 'N/A'} 起），非展示窗口。
> 水平分位与同比分位**刻度不同**，分别命名、不可混读。{level_note}
> 近 {p.window_periods} 期用**中位数与极值**，不用均值（宏观序列极值影响大）。
> 「连续同向」是**状态描述**（可从序列直接数出），不含任何外推或预测。
"""


def _build_macro_risk_section(risk_metrics: Optional[Any]) -> str:
    """宏观风险章(P9-5 / #13):纯文字,无 emoji(强制规则 3)。

    ⚠️ 本章是封面结论 chip 的**唯一来源** —— 宏观无评分卡(D-M5),前端取
    `scorecard.overall_label ?? risk.overall_level`。
    """
    if risk_metrics is None:
        return "_风险分析暂不可得。_\n"

    level_word = {"low": "低", "medium": "中", "high": "高"}
    lines = [f"**综合风险等级**: {level_word.get(risk_metrics.overall_level, risk_metrics.overall_level)}", ""]

    if risk_metrics.items:
        lines.append("| 风险类别 | 等级 | 依据 | 说明 |")
        lines.append("|---|---|---|---|")
        for item in risk_metrics.items:
            lines.append(f"| {item.category} | {item.level.upper()} | {item.evidence} | {item.description} |")
    else:
        lines.append("未触发任何风险规则：当前读数**未处于**历史极端位置，且同比无连续同向变化。")
    lines.append("")
    lines.append("> 口径说明：以上全部为**确定性规则**判定（分位位置 + 连续同向状态），"
                 "不构成对宏观走势的预测，亦不构成任何资产配置建议。")
    return "\n".join(lines) + "\n"


def _build_event_section(event_metrics: Optional[EventMetrics]) -> str:
    if event_metrics is None:
        return "_近期事件暂不可得。_\n"

    lines = []

    # 优先展示降噪分级结果（§10 先降噪再研究）
    d = event_metrics.denoised
    if d:
        lines.append(
            f"最近 30 天原始事件 {d.total_raw} 条（新闻 + 公告），去重合并后 {d.total_events} 个独立事件"
            f"（合并同一事件重复稿件 {d.dedup_merged} 条）。"
        )
        lines.append("")

        # 分类分布
        dist = "、".join(f"{k} {v}" for k, v in sorted(d.category_distribution.items()))
        if dist:
            lines.append(f"分类分布：{dist}")
            lines.append("")

        # 「30 天无重大事件」是合法结论（禁止 padding）
        if d.no_major_event:
            lines.append("**近期无高/中级事件**：30 天内无财报、重组、增减持等重大事项，"
                         "亦无可归因于事件的显著价格变化。")
            lines.append("")
        else:
            lines.append("### 重大事件（高 / 中级）")
            lines.append("")
            for ev in d.major_events:
                sev = {"high": "高", "medium": "中"}.get(ev.severity.value, "-")
                title = ev.title if len(ev.title) <= 60 else ev.title[:57] + "…"
                merged = f"（合并 {len(ev.source_items)} 条来源）" if len(ev.source_items) > 1 else ""
                lines.append(f"- **[{sev}]** {title} {merged}")
                if ev.price_window:
                    pw = ev.price_window
                    ret = pw.get("window_return_pct")
                    if ret is not None:
                        complete = "" if pw.get("window_complete") else "（窗口未走完）"
                        lines.append(
                            f"  - 事件日 {pw['event_date']} 后 {pw['window_days']} 个交易日"
                            f"区间涨跌 **{ret:+.2f}%**{complete}"
                        )
            lines.append("")

        # 低级事件聚合为一行统计
        if d.low_events:
            low = "、".join(e.title[:20] for e in d.low_events[:5])
            more = " 等" if len(d.low_events) > 5 else ""
            lines.append(f"**例行事务**（{len(d.low_events)} 条）：{low}{more}")
            lines.append("")
        return "\n".join(lines)

    # 回退：无降噪时的旧逻辑
    lines.append(f"最近 30 天共 {event_metrics.total_news} 条新闻，其中财报相关 {event_metrics.earnings_mentions} 条。")
    lines.append("")

    if event_metrics.major_events:
        lines.append("### 重点事件")
        lines.append("")
        for i, news in enumerate(event_metrics.major_events[:3], 1):
            lines.append(f"{i}. **{news.title}** ({news.source}, {news.published_at.strftime('%m-%d')})")
        lines.append("")

    if event_metrics.recent_news:
        lines.append("### 最新动态")
        lines.append("")
        for i, news in enumerate(event_metrics.recent_news[:3], 1):
            lines.append(f"{i}. {news.title} ({news.source}, {news.published_at.strftime('%m-%d')})")
        lines.append("")

    return "\n".join(lines)


def _build_capital_section(capital_metrics: Optional[CapitalMetrics]) -> str:
    if capital_metrics is None:
        return "_龙虎榜数据暂不可得。_\n"

    lines = []

    if capital_metrics.lhb_count_30d == 0:
        lines.append("最近 5 天未登上龙虎榜（未触发异动条件：涨幅偏离±7%、换手率20%、连续三日累计±20%）。")
        lines.append("")
        return "\n".join(lines)

    lines.append(f"最近 5 天登上龙虎榜 **{capital_metrics.lhb_count_30d}** 次。")
    lines.append("")

    # 按天展开
    for day in capital_metrics.daily:
        lines.append(f"#### {day.trade_date.strftime('%m-%d')}  {day.close_price:.2f}元  {day.pct_change:+.2f}%  |  {day.reason}")
        lines.append("")

        # 当日资金方汇总
        lines.append("当日席位资金流向：")
        lines.append("")
        lines.append(f"- 机构: {day.institution_net/1e4:+.0f}万 | 北向: {day.northbound_net/1e4:+.0f}万 | 游资: {day.hot_money_net/1e4:+.0f}万 | 散户: {day.retail_net/1e4:+.0f}万 | **合计: {day.day_net_buy/1e4:+.0f}万**")
        lines.append("")

        # 当日净买入前五
        if day.top_buyers:
            lines.append("净买入前五：")
            lines.append("")
            lines.append("| 营业部 | 类型 | 净买入（万元） |")
            lines.append("|---|---|---|")
            for d in day.top_buyers:
                lines.append(f"| {d.name[:22]} | {d.dealer_type} | {d.net_amount/1e4:+.0f} |")
            lines.append("")

        # 当日净卖出前五
        if day.top_sellers:
            lines.append("净卖出前五：")
            lines.append("")
            lines.append("| 营业部 | 类型 | 净卖出（万元） |")
            lines.append("|---|---|---|")
            for d in day.top_sellers:
                lines.append(f"| {d.name[:22]} | {d.dealer_type} | {d.net_amount/1e4:+.0f} |")
            lines.append("")

    # 全时段汇总
    lines.append("### 全时段汇总")
    lines.append("")
    lines.append(f"**主导力量**: {capital_metrics.dominant_force} | **合计净额**: {capital_metrics.total_net_buy/1e4:+.0f} 万元")
    lines.append("")

    return "\n".join(lines)


def _build_industry_risk_section(risk_metrics: Optional[Any]) -> str:
    """行业风险章节(P9 #12):纯文字,无 emoji(强制规则 3)。"""
    if risk_metrics is None:
        return "_风险分析暂不可得。_\n"

    level_word = {"low": "低", "medium": "中", "high": "高"}
    lines = [f"**综合风险等级**: {level_word.get(risk_metrics.overall_level, risk_metrics.overall_level)}", ""]

    if risk_metrics.items:
        lines.append("| 风险类别 | 等级 | 依据 | 说明 |")
        lines.append("|---|---|---|---|")
        for item in risk_metrics.items:
            lines.append(f"| {item.category} | {item.level.upper()} | {item.evidence} | {item.description} |")
        lines.append("")
    else:
        lines.append("未发现显著风险信号。")
        lines.append("")
    return "\n".join(lines)


def _build_risk_section(risk_metrics: Optional[RiskMetrics]) -> str:
    if risk_metrics is None:
        return "_风险分析暂不可得。_\n"

    lines = []

    level_emoji = {"low": "🟢", "medium": "🟡", "high": "🔴"}
    emoji = level_emoji.get(risk_metrics.overall_level, "⚪")
    lines.append(f"**综合风险等级**: {emoji} {risk_metrics.overall_level.upper()}")
    lines.append("")

    if risk_metrics.veto_buy:
        lines.append("⚠️ **一票否决**: 当前风险水平不建议入场。")
        lines.append("")

    if risk_metrics.items:
        lines.append("| 风险类别 | 等级 | 依据 | 说明 |")
        lines.append("|---|---|---|---|")
        for item in risk_metrics.items:
            lines.append(f"| {item.category} | {item.level.upper()} | {item.evidence} | {item.description} |")
        lines.append("")
    else:
        lines.append("未发现显著风险信号。")
        lines.append("")

    return "\n".join(lines)


def _build_pattern_section(patterns: Optional[PatternMetrics]) -> str:
    """量价形态（规则判定）。只打印 JSON 中确有的数值，保证 Number Lint 可溯源；
    逐日序列与每条标签的详细依据在个股页「买入前速评」卡片渲染（规则判定分区）。"""
    if patterns is None:
        return "_量价形态暂不可得。_\n"

    lines = ["**换手率口径**：流通股本口径（数据源 腾讯财经/akshare），非自由流通口径。", ""]

    if patterns.note:
        lines.append(f"_{patterns.note}_")
        lines.append("")
    elif patterns.labels:
        lines.append(f"近 {patterns.window_days} 个交易日识别到的量价形态（规则判定）：")
        lines.append("")
        for lb in patterns.labels:
            lines.append(f"- {lb['date']} {lb['label']}")
        lines.append("")
    else:
        lines.append(f"近 {patterns.window_days} 个交易日内未识别出显著量价形态。")
        lines.append("")

    d = patterns.divergence or {}
    if d:
        coincide = "是" if d.get("peak_high_coincide") else "否"
        lines.append(
            f"换手率峰值 {d.get('peak_turnover')}%（{d.get('peak_date')}）"
            f"与股价高点 {d.get('high_close')} 元（{d.get('high_date')}）"
            f"共振 {coincide}；峰值后至今 {d.get('after_peak_return_pct')}%。"
        )
        lines.append("")
    return "\n".join(lines)


def _build_scorecard_section(scorecard: Optional[Scorecard]) -> str:
    if scorecard is None:
        return "_评分卡暂不可得。_\n"

    lines = []
    lines.append(f"**Overall: {scorecard.overall_label}** （总分 {scorecard.overall:+d}）")
    lines.append("")
    lines.append("| 维度 | 评分 | 理由 |")
    lines.append("|---|---|---|")

    dim_labels = {
        "business_quality": "业务质量",
        "fundamental_trend": "基本面趋势",
        "market_trend": "市场趋势",
        "valuation": "估值",
        "recent_events": "近期事件",
        "risk": "风险",
    }

    for d in scorecard.dimensions:
        label = dim_labels.get(d.dimension, d.dimension)
        score_str = "N/A" if d.unavailable else f"{d.score:+d}"
        lines.append(f"| {label} | {score_str} | {d.reason} |")

    lines.append("")
    lines.append("> 评分说明：-2（极弱）~ +2（极强），0 为中性。N/A 表示数据暂不可得。")
    lines.append("")

    return "\n".join(lines)
