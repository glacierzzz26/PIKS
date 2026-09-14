"""JSON 结构化报告生成器"""
from dataclasses import asdict
from datetime import date
from typing import Dict, Any, List, Optional

from ..analysis.price import PriceMetrics
from ..analysis.volume import VolumeMetrics
from ..analysis.financial import FinancialMetrics
from ..analysis.events import EventMetrics
from ..analysis.risk import RiskMetrics
from ..analysis.capital import CapitalMetrics
from ..analysis.scorecard import Scorecard
from ..analysis.patterns import PatternMetrics
from ..models.financial import FinancialSnapshot


def generate_json(
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
) -> Dict[str, Any]:
    """生成 JSON 结构化报告"""
    result = {
        "meta": {
            "symbol": symbol,
            "as_of": as_of.isoformat(),
            "data_source": "akshare/tencent",
        },
        "price": asdict(price_metrics),
        "volume": asdict(volume_metrics),
    }

    if fin_metrics:
        result["financial"] = asdict(fin_metrics)
        # 趋势与变化数值（Number Lint 对账基准）
        for key, val in (
            ("revenue_yoy_trend", fin_metrics.revenue_yoy_trend),
            ("net_profit_yoy_trend", fin_metrics.net_profit_yoy_trend),
            ("revenue_yoy_change", fin_metrics.revenue_yoy_change),
            ("profit_yoy_change", fin_metrics.profit_yoy_change),
            ("roe_change", fin_metrics.roe_change),
        ):
            if val:
                result["financial"][key] = val

    if snapshots:
        result["financial_snapshots"] = [
            {
                "report_date": s.report_date.isoformat(),
                "report_type": s.report_type,
                "revenue_yoy": s.revenue_yoy,
                "net_profit_yoy": s.net_profit_yoy,
                "roe": s.roe,
                "gross_margin": s.gross_margin,
                "net_margin": s.net_margin,
                "debt_ratio": s.debt_ratio,
            }
            for s in snapshots
        ]

    if event_metrics:
        result["events"] = event_metrics.to_dict()
        # 保持向后兼容的顶层字段
        result["events"]["total_news"] = event_metrics.total_news
        result["events"]["earnings_mentions"] = event_metrics.earnings_mentions

    if risk_metrics:
        result["risk"] = asdict(risk_metrics)

    if industry:
        result["industry"] = industry.to_dict()

    if capital_metrics:
        result["capital"] = asdict(capital_metrics)

    if patterns:
        result["patterns"] = asdict(patterns)

    if scorecard:
        result["scorecard"] = {
            "overall": scorecard.overall,
            "overall_label": scorecard.overall_label,
            "dimensions": [
                {
                    "dimension": d.dimension,
                    "score": None if d.unavailable else d.score,
                    "reason": d.reason,
                    "unavailable": d.unavailable,
                }
                for d in scorecard.dimensions
            ],
        }

    return result
