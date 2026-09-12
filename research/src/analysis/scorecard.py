"""评分卡引擎

六维度评分 + overall 聚合规则。
所有评分由确定性规则计算，LLM 只负责叙述。
"""
from dataclasses import dataclass, field
from datetime import date
from typing import List, Optional

from .price import PriceMetrics
from .volume import VolumeMetrics
from .financial import FinancialMetrics
from .events import EventMetrics
from .risk import RiskMetrics


@dataclass
class DimensionScore:
    """单维度评分"""
    dimension: str
    score: int          # -2 ~ +2, 或 None 表示 unavailable
    reason: str
    unavailable: bool = False


@dataclass
class Scorecard:
    """评分卡"""
    symbol: str
    as_of: date
    dimensions: List[DimensionScore]
    overall: int        # 简单加总分
    overall_label: str  # 偏正面 / 中性 / 偏负面 / 结论受限


def _business_quality(fin: Optional[FinancialMetrics]) -> DimensionScore:
    """业务质量：基于 ROE、净利率、负债率"""
    if fin is None:
        return DimensionScore("business_quality", 0, "财务数据暂不可得", unavailable=True)

    roe = fin.latest_roe
    net_margin = fin.latest_net_margin
    debt = fin.latest_debt_ratio

    if roe is None and net_margin is None:
        return DimensionScore("business_quality", 0, "核心盈利指标缺失", unavailable=True)

    score = 0
    reasons = []

    if roe is not None:
        if roe > 15:
            score += 1
            reasons.append(f"ROE {roe:.1f}% 优秀")
        elif roe < 8:
            score -= 1
            reasons.append(f"ROE {roe:.1f}% 偏低")
        else:
            reasons.append(f"ROE {roe:.1f}% 正常")

    if net_margin is not None:
        if net_margin > 20:
            score += 1
            reasons.append(f"净利率 {net_margin:.1f}% 优秀")
        elif net_margin < 5:
            score -= 1
            reasons.append(f"净利率 {net_margin:.1f}% 偏低")
        else:
            reasons.append(f"净利率 {net_margin:.1f}% 正常")

    if debt is not None:
        if debt > 70:
            score -= 1
            reasons.append(f"负债率 {debt:.1f}% 偏高")
        elif debt < 30:
            reasons.append(f"负债率 {debt:.1f}% 低，财务稳健")

    score = max(-2, min(2, score))
    return DimensionScore("business_quality", score, "；".join(reasons))


def _fundamental_trend(fin: Optional[FinancialMetrics]) -> DimensionScore:
    """基本面趋势：基于营收/净利润同比变化"""
    if fin is None:
        return DimensionScore("fundamental_trend", 0, "财务数据暂不可得", unavailable=True)

    rev = fin.latest_revenue_yoy
    profit = fin.latest_net_profit_yoy
    rev_change = fin.revenue_yoy_change
    profit_change = fin.profit_yoy_change

    if rev is None and profit is None:
        return DimensionScore("fundamental_trend", 0, "增长指标缺失", unavailable=True)

    score = 0
    reasons = []

    if rev is not None:
        if rev > 20:
            score += 2
            reasons.append(f"营收同比 {rev:+.1f}%，高增长")
        elif rev > 5:
            score += 1
            reasons.append(f"营收同比 {rev:+.1f}%，稳健增长")
        elif rev < -10:
            score -= 1
            reasons.append(f"营收同比 {rev:+.1f}%，下滑明显")
        else:
            reasons.append(f"营收同比 {rev:+.1f}%")

    if profit is not None:
        if profit > 30:
            score += 1
            reasons.append(f"净利润同比 {profit:+.1f}%，高增长")
        elif profit < -20:
            score -= 1
            reasons.append(f"净利润同比 {profit:+.1f}%，大幅下滑")

    # 边际变化
    if rev_change is not None:
        if rev_change > 5:
            score += 1
            reasons.append(f"营收增速环比提升 {rev_change:+.1f}pct")
        elif rev_change < -5:
            score -= 1
            reasons.append(f"营收增速环比下降 {abs(rev_change):.1f}pct")

    score = max(-2, min(2, score))
    return DimensionScore("fundamental_trend", score, "；".join(reasons))


def _market_trend(price: PriceMetrics) -> DimensionScore:
    """市场趋势：基于区间涨跌幅"""
    ret = price.period_return_pct
    vol = price.volatility_annual

    score = 0
    reasons = []

    if ret > 15:
        score = +2
        reasons.append(f"区间涨幅 {ret:.1f}%，强势")
    elif ret > 5:
        score = +1
        reasons.append(f"区间涨幅 {ret:.1f}%，偏强")
    elif ret < -15:
        score = -2
        reasons.append(f"区间跌幅 {abs(ret):.1f}%，弱势")
    elif ret < -5:
        score = -1
        reasons.append(f"区间跌幅 {abs(ret):.1f}%，偏弱")
    else:
        reasons.append(f"区间波动 {ret:.1f}%，中性")

    if vol > 40:
        reasons.append(f"高波动 {vol:.1f}%")
    elif vol < 15:
        reasons.append(f"低波动 {vol:.1f}%，走势平稳")

    return DimensionScore("market_trend", score, "；".join(reasons))


def _valuation(fin: Optional[FinancialMetrics]) -> DimensionScore:
    """估值：基于 PE/PB（当前数据常缺失）"""
    if fin is None or (fin.pe_ttm is None and fin.pb is None):
        return DimensionScore("valuation", 0, "估值数据暂不可得", unavailable=True)

    pe = fin.pe_ttm
    pb = fin.pb
    score = 0
    reasons = []

    if pe is not None:
        if pe < 15:
            score += 1
            reasons.append(f"PE(TTM) {pe:.1f}，偏低")
        elif pe > 50:
            score -= 1
            reasons.append(f"PE(TTM) {pe:.1f}，偏高")
        else:
            reasons.append(f"PE(TTM) {pe:.1f}")

    if pb is not None:
        if pb < 2:
            score += 1
            reasons.append(f"PB {pb:.1f}，偏低")
        elif pb > 10:
            score -= 1
            reasons.append(f"PB {pb:.1f}，偏高")
        else:
            reasons.append(f"PB {pb:.1f}")

    score = max(-2, min(2, score))
    return DimensionScore("valuation", score, "；".join(reasons))


def _recent_events(events: Optional[EventMetrics]) -> DimensionScore:
    """近期事件：基于新闻/公告重大性"""
    if events is None:
        return DimensionScore("recent_events", 0, "近期事件数据暂不可得", unavailable=True)

    score = 0
    reasons = []

    total = events.total_news
    major = len(events.major_events)

    if major >= 3:
        score += 1
        reasons.append(f"重大事件 {major} 条，催化较多")
    elif major == 0 and total > 0:
        reasons.append(f"{total} 条新闻，无重大事件")
    elif total == 0:
        reasons.append("30 天内无新闻")
    else:
        reasons.append(f"重大事件 {major} 条")

    # 财报相关事件通常偏中性，如果有业绩超预期/低于预期可再细分
    if events.earnings_mentions > 0:
        reasons.append(f"财报相关 {events.earnings_mentions} 条")

    return DimensionScore("recent_events", score, "；".join(reasons))


def _risk(risk_metrics: Optional[RiskMetrics]) -> DimensionScore:
    """风险：基于风险引擎输出"""
    if risk_metrics is None:
        return DimensionScore("risk", 0, "风险分析暂不可得", unavailable=True)

    level = risk_metrics.overall_level
    veto = risk_metrics.veto_buy

    mapping = {"low": 0, "medium": -1, "high": -2}
    score = mapping.get(level, 0)

    reasons = []
    if veto:
        reasons.append("一票否决触发")
    reasons.append(f"综合风险等级 {level.upper()}")
    if risk_metrics.items:
        cats = ", ".join([r.category for r in risk_metrics.items[:3]])
        reasons.append(f"主要风险: {cats}")

    return DimensionScore("risk", score, "；".join(reasons))


def analyze_scorecard(
    price: Optional[PriceMetrics],
    volume: Optional[VolumeMetrics],
    financial: Optional[FinancialMetrics],
    events: Optional[EventMetrics],
    risk_metrics: Optional[RiskMetrics],
    as_of: date,
) -> Scorecard:
    """
    计算六维度评分卡。

    overall 聚合规则（设计文档 §13）：
    - 六维度简单加总（范围 -12 ~ +12）
    - overall ≥ +4 → 偏正面
    - overall ≤ -4 → 偏负面
    - 之间 → 中性
    - 若任一权重敏感维度标记 unavailable 且影响判断，降级为「结论受限」
    """
    dims = []
    dims.append(_business_quality(financial))
    dims.append(_fundamental_trend(financial))
    dims.append(_market_trend(price) if price else DimensionScore("market_trend", 0, "价格数据缺失", unavailable=True))
    dims.append(_valuation(financial))
    dims.append(_recent_events(events))
    dims.append(_risk(risk_metrics))

    # 只计算有确定评分的维度
    valid_scores = [d.score for d in dims if not d.unavailable]
    overall = sum(valid_scores) if valid_scores else 0

    # 判断是否有敏感维度 unavailable
    sensitive_unavailable = any(
        d.unavailable for d in dims if d.dimension in ("risk", "fundamental_trend")
    )

    if sensitive_unavailable:
        overall_label = "结论受限"
    elif overall >= 4:
        overall_label = "偏正面"
    elif overall <= -4:
        overall_label = "偏负面"
    else:
        overall_label = "中性"

    symbol = price.symbol if price else (financial.symbol if financial else "")
    return Scorecard(
        symbol=symbol,
        as_of=as_of,
        dimensions=dims,
        overall=overall,
        overall_label=overall_label,
    )
