"""评分卡引擎

按 Profile 声明的维度评分（默认六维）+ overall 聚合规则。
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


# 默认六维（完整档案）；档案可在 profile.scorecard.dimensions 里声明子集
# （如 short-term 无财务/估值/风险章节，只保留 market_trend + recent_events）。
DEFAULT_DIMENSIONS = [
    "business_quality",
    "fundamental_trend",
    "market_trend",
    "valuation",
    "recent_events",
    "risk",
]
# 缺失即判定「结论受限」的敏感维度（数据不足以支撑结论，宁可不下结论）
SENSITIVE_DIMENSIONS = ("risk", "fundamental_trend")
# 方向判定阈值：平均维度分 ≥ +2/3 偏正面、≤ -2/3 偏负面。
# 六维时即原设计 §13 的「总分 ≥ +4 / ≤ -4」（4/6 = 2/3），维度更少时自动缩放。
DIRECTION_AVG_THRESHOLD = 2 / 3


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
    dimensions: Optional[List[str]] = None,
) -> Scorecard:
    """
    计算评分卡。

    dimensions: Profile 声明的评分维度（顺序即展示顺序）。
                空/None → 默认六维（完整档案）。
                只评估声明维度：档案未纳入的字段（如 short-term 无 financial）
                既不出现在结果里，也不触发「结论受限」——「不在本档案范围」≠「数据缺失」。

    overall 聚合规则（设计文档 §13，按档案维度数自适应）：
    - overall = 各可用维度简单加总
    - 方向按「平均维度分」判定：≥ +2/3 → 偏正面，≤ -2/3 → 偏负面，之间 → 中性
      （六维时 2/3 × 6 = +4，与原「总分 ≥ +4」阈值一致）
    - 若任一敏感维度（risk / fundamental_trend）不可得 → 降级为「结论受限」
    """
    by_dim = {
        "business_quality": _business_quality(financial),
        "fundamental_trend": _fundamental_trend(financial),
        "market_trend": (
            _market_trend(price) if price
            else DimensionScore("market_trend", 0, "价格数据缺失", unavailable=True)
        ),
        "valuation": _valuation(financial),
        "recent_events": _recent_events(events),
        "risk": _risk(risk_metrics),
    }

    wanted = [d for d in (dimensions or DEFAULT_DIMENSIONS) if d in by_dim]
    if not wanted:  # 声明为空或全非法 → 回退默认六维，避免产出空评分卡
        wanted = DEFAULT_DIMENSIONS
    dims = [by_dim[d] for d in wanted]

    # 只计算有确定评分的维度
    valid_scores = [d.score for d in dims if not d.unavailable]
    overall = sum(valid_scores) if valid_scores else 0

    # 敏感维度不可得（且在本档案范围内）→ 结论受限
    sensitive_unavailable = any(
        d.unavailable for d in dims if d.dimension in SENSITIVE_DIMENSIONS
    )

    if sensitive_unavailable or not valid_scores:
        overall_label = "结论受限"
    else:
        avg = overall / len(valid_scores)
        if avg >= DIRECTION_AVG_THRESHOLD:
            overall_label = "偏正面"
        elif avg <= -DIRECTION_AVG_THRESHOLD:
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
