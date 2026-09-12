"""分析引擎总控

整合所有分析模块，统一输出带 Evidence 的指标卡。
"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional

from ..models import Bar
from ..models.financial import FinancialSnapshot, ValuationMetrics
from ..models.news import NewsItem
from ..models.announcement import Announcement
from ..evidence import EvidenceStore, evidence_from_metric
from .price import PriceMetrics
from .volume import VolumeMetrics
from .financial import FinancialMetrics
from .events import EventMetrics
from .risk import RiskMetrics, analyze_risk
from .price import analyze_price
from .volume import analyze_volume
from .financial import analyze_financial
from .events import analyze_events


@dataclass
class AnalysisResult:
    """分析结果总包"""
    price: PriceMetrics
    volume: VolumeMetrics
    financial: Optional[FinancialMetrics]
    events: Optional[EventMetrics]
    risk: RiskMetrics
    evidence_store: EvidenceStore


def run_analysis(
    bars: List[Bar],
    snapshots: Optional[List[FinancialSnapshot]] = None,
    valuation: Optional[ValuationMetrics] = None,
    news: Optional[List[NewsItem]] = None,
    announcements: Optional[List[Announcement]] = None,
    as_of: Optional[date] = None,
) -> AnalysisResult:
    """
    运行全部分析，同时生成 Evidence。
    """
    if as_of is None:
        as_of = date.today()

    store = EvidenceStore()

    # 价格分析
    price = analyze_price(bars, as_of)
    store.add(evidence_from_metric(
        "period_return_pct", price.period_return_pct,
        "analysis_engine", f"{price.period_days}d",
        {"start_price": price.start_price, "end_price": price.end_price},
        section="price",
    ))
    store.add(evidence_from_metric(
        "max_drawdown_pct", price.max_drawdown_pct,
        "analysis_engine", f"{price.period_days}d",
        {"max_price": price.max_price, "min_price": price.min_price},
        section="price",
    ))

    # 成交量分析
    volume = analyze_volume(bars, as_of)
    store.add(evidence_from_metric(
        "avg_turnover_20d", volume.avg_turnover_20d,
        "analysis_engine", f"{volume.period_days}d",
        {"turnovers": [b.turnover for b in bars[-20:]]},
        section="volume",
    ))

    # 财务分析
    financial = None
    if snapshots:
        financial = analyze_financial(snapshots, valuation, as_of)
        if financial.latest_roe is not None:
            store.add(evidence_from_metric(
                "roe", financial.latest_roe,
                "akshare_financial", financial.latest_roe,
                {"report_date": snapshots[0].report_date.isoformat() if snapshots else None},
                section="financial",
            ))

    # 事件分析
    events = None
    if news:
        events = analyze_events(news, as_of)
        store.add(evidence_from_metric(
            "news_count", events.total_news,
            "akshare_news", "30d",
            {"earnings_mentions": events.earnings_mentions},
            section="events",
        ))

    # 风险分析 —— 无论是否触发 veto 都记录 Evidence（无风险也是结论）
    risk = analyze_risk(price, volume, financial, announcements)
    store.add(evidence_from_metric(
        "risk_level", risk.overall_level,
        "risk_engine", "current",
        {"risk_count": len(risk.items), "veto_buy": risk.veto_buy},
        section="risk",
    ))
    if risk.veto_buy:
        store.add(evidence_from_metric(
            "risk_veto", True,
            "risk_engine", "current",
            {"overall_level": risk.overall_level, "risk_count": len(risk.items)},
            section="risk",
        ))

    return AnalysisResult(
        price=price,
        volume=volume,
        financial=financial,
        events=events,
        risk=risk,
        evidence_store=store,
    )
