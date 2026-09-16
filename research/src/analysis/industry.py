"""行业分析引擎(P9 / issue #12)

输入是**行业指数序列 + 成分**,输出确定性指标卡。与个股分析的关键差异:

1. 无成交额/换手(申万指数该字段量纲与 A 股不一致,见 sw_index_provider docstring)
2. 估值只做**横截面排名**,**绝不**做历史分位 —— 估值快照无历史序列(D-R7)
3. 点位**可以**做历史分位 —— `index_hist_sw` 有 6456 个交易日(1999-12-30 起)

Number Lint 口径:本模块产出的每个数字都会进 metrics.json,故均可溯源。
未算出的字段一律 None(不补 0、"N/A" 由渲染层负责)。
"""
from dataclasses import dataclass, field
from datetime import date
from typing import List, Optional, Sequence

import numpy as np

from ..models import Bar
from ..providers.industry.sw_index_provider import (
    Constituent,
    IndustryRef,
    ValuationRank,
)


@dataclass
class IndustryPriceMetrics:
    """行业行情指标卡。单位:指数点(无量纲)、百分比。"""
    symbol: str
    as_of: date
    period_days: int

    start_point: float
    end_point: float
    period_return_pct: float
    return_pct_5d: Optional[float]
    return_pct_20d: Optional[float]
    return_pct_60d: Optional[float]

    max_drawdown_pct: float
    volatility_annual: float

    # 点位历史分位 —— 基于**全部可用历史**(非展示窗口)
    point_percentile: Optional[float]      # 0~100,当前点位在历史收盘序列中的分位
    history_start: Optional[date]          # 历史序列首日(**如实暴露起点**:三级新码短)
    history_days: int                      # 参与分位计算的历史交易日数


@dataclass
class DispersionMetrics:
    """成分离散度:中位 + 四分位。**不做均值** —— 财务比率受极值影响,中位更稳。"""
    count: int
    roe_median: Optional[float] = None
    roe_q1: Optional[float] = None
    roe_q3: Optional[float] = None
    net_profit_growth_median: Optional[float] = None
    revenue_growth_median: Optional[float] = None
    pe_ttm_median: Optional[float] = None
    cap_sum: Optional[float] = None        # 成分合计市值(亿)


@dataclass
class IndustryMetrics:
    """行业研报的完整指标卡(= Fact 唯一源)。"""
    ref: IndustryRef
    price: IndustryPriceMetrics
    valuation_rank: ValuationRank
    dispersion: DispersionMetrics
    declared_count: Optional[int] = None   # 行业表挂牌成份数
    constituent_count: int = 0             # 实际聚合到的成分数


@dataclass
class IndustryRiskItem:
    """行业风险项(与个股 RiskItem 同构,但**不臆造**不适用类别)。

    不套用个股风险引擎:个股的流动性风险依据「换手率 + 成交额」,而申万指数这两个字段
    量纲不可靠(见 provider docstring)→ 套用会产出一句基于假数字的结论。
    """
    category: str
    level: str          # low / medium / high
    evidence: str
    description: str


@dataclass
class IndustryRiskMetrics:
    symbol: str
    as_of: date
    overall_level: str
    items: List[IndustryRiskItem] = field(default_factory=list)


def analyze_industry_risk(m: IndustryMetrics, as_of: date) -> IndustryRiskMetrics:
    """基于规则的行业风险。只标记**可从本指标卡溯源**的风险,不做预测。"""
    items: List[IndustryRiskItem] = []
    p = m.price

    if p.volatility_annual > 40:
        items.append(IndustryRiskItem(
            "Market", "high", f"年化波动率 {p.volatility_annual:.1f}%",
            "行业指数波动剧烈，短期回撤风险较高"))
    elif p.volatility_annual > 25:
        items.append(IndustryRiskItem(
            "Market", "medium", f"年化波动率 {p.volatility_annual:.1f}%",
            "行业指数波动中等，注意仓位与节奏"))

    if p.period_return_pct < -15:
        items.append(IndustryRiskItem(
            "Market", "high", f"区间涨跌 {p.period_return_pct:+.1f}%",
            "行业近期跌幅较大，趋势偏弱"))
    elif p.max_drawdown_pct > 20:
        items.append(IndustryRiskItem(
            "Market", "medium", f"区间最大回撤 {p.max_drawdown_pct:.1f}%",
            "区间内回撤较深，需关注支撑与资金面"))

    # 估值风险:横截面**相对**位置(D-R7 口径)。只在有排名时标记,不做历史分位判断。
    r = m.valuation_rank
    if r.rank is not None and r.universe >= 10:
        top_quartile = r.rank > r.universe * 0.75
        if top_quartile:
            items.append(IndustryRiskItem(
                "Valuation", "medium", f"TTM PE {r.value:.2f}，{r.level_label} {r.universe} 个行业中第 {r.rank}",
                "估值在同层级行业中偏贵，需警惕均值回归压力"))

    # 盈利质量:成分中位 ROE 为负 → 板块整体盈利能力弱(用中位抗极值)
    if m.dispersion.roe_median is not None and m.dispersion.roe_median < 0:
        items.append(IndustryRiskItem(
            "Fundamental", "medium", f"成分 ROE 中位 {m.dispersion.roe_median:.2f}%",
            "行业成分整体盈利为负，基本面承压"))

    if any(i.level == "high" for i in items):
        overall = "high"
    elif any(i.level == "medium" for i in items):
        overall = "medium"
    else:
        overall = "low"

    return IndustryRiskMetrics(symbol=m.ref.code, as_of=as_of, overall_level=overall, items=items)


def _window_return(closes: np.ndarray, days: int) -> Optional[float]:
    n = len(closes)
    if n < days:
        return None
    prev = closes[-days - 1] if n > days else closes[0]
    return float((closes[-1] - prev) / prev * 100)


def _median(xs: Sequence[Optional[float]]) -> Optional[float]:
    vals = [x for x in xs if x is not None]
    return float(np.median(vals)) if vals else None


def _quartiles(xs: Sequence[Optional[float]]):
    vals = [x for x in xs if x is not None]
    if len(vals) < 2:
        return None, None
    q1, q3 = np.percentile(vals, [25, 75])
    return float(q1), float(q3)


def analyze_industry(
    ref: IndustryRef,
    bars: List[Bar],
    history_bars: List[Bar],
    constituents: List[Constituent],
    valuation_rank: ValuationRank,
    as_of: date,
) -> IndustryMetrics:
    """算行业指标卡。

    bars          : 展示窗口(profile 的 display 天数)
    history_bars  : 全部可用历史(算点位分位;**与展示窗口分离**,否则分位被窗口污染)
    """
    if not bars:
        raise ValueError("行业指数序列为空,无法计算行业指标")

    closes = np.array([b.close for b in bars], dtype=float)
    highs = np.array([b.high for b in bars], dtype=float)
    lows = np.array([b.low for b in bars], dtype=float)

    # 最大回撤(展示窗口内,与个股口径一致)
    peak, max_dd = closes[0], 0.0
    for i in range(1, len(closes)):
        if closes[i] > peak:
            peak = closes[i]
        dd = (peak - closes[i]) / peak * 100
        max_dd = max(max_dd, dd)

    # 年化波动率
    if len(closes) >= 2:
        daily = np.diff(closes) / closes[:-1]
        vol = float(np.std(daily, ddof=1) * np.sqrt(250) * 100)
    else:
        vol = 0.0

    # 点位历史分位:当前收盘在全历史收盘序列中的位次(0~100)
    h_closes = np.array([b.close for b in history_bars], dtype=float) if history_bars else closes
    point_pct = float((h_closes <= closes[-1]).sum() / len(h_closes) * 100) if len(h_closes) else None
    hist_start = history_bars[0].date if history_bars else (bars[0].date if bars else None)

    price = IndustryPriceMetrics(
        symbol=ref.code,
        as_of=as_of,
        period_days=len(bars),
        start_point=float(closes[0]),
        end_point=float(closes[-1]),
        period_return_pct=float((closes[-1] - closes[0]) / closes[0] * 100),
        return_pct_5d=_window_return(closes, 5),
        return_pct_20d=_window_return(closes, 20),
        return_pct_60d=_window_return(closes, 60),
        max_drawdown_pct=float(max_dd),
        volatility_annual=vol,
        point_percentile=round(point_pct, 1) if point_pct is not None else None,
        history_start=hist_start,
        history_days=len(h_closes),
    )

    roe_q1, roe_q3 = _quartiles([c.roe for c in constituents])
    dispersion = DispersionMetrics(
        count=len(constituents),
        roe_median=_median([c.roe for c in constituents]),
        roe_q1=roe_q1,
        roe_q3=roe_q3,
        net_profit_growth_median=_median([c.net_profit_growth for c in constituents]),
        revenue_growth_median=_median([c.revenue_growth for c in constituents]),
        pe_ttm_median=_median([c.pe_ttm for c in constituents if (c.pe_ttm or 0) > 0]),
        cap_sum=(float(sum(c.market_cap for c in constituents if c.market_cap is not None))
                 if any(c.market_cap is not None for c in constituents) else None),
    )

    return IndustryMetrics(
        ref=ref,
        price=price,
        valuation_rank=valuation_rank,
        dispersion=dispersion,
        declared_count=ref.member_count,
        constituent_count=len(constituents),
    )
