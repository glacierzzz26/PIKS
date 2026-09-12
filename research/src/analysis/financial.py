"""财务分析引擎"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional

from ..models.financial import FinancialSnapshot, ValuationMetrics


@dataclass
class FinancialMetrics:
    """财务指标卡"""
    symbol: str
    as_of: date

    # 最近季度
    latest_revenue_yoy: Optional[float]
    latest_net_profit_yoy: Optional[float]
    latest_roe: Optional[float]
    latest_gross_margin: Optional[float]
    latest_net_margin: Optional[float]
    latest_debt_ratio: Optional[float]

    # 趋势（最近4个季度）
    revenue_yoy_trend: List[float]
    net_profit_yoy_trend: List[float]
    roe_trend: List[float]

    # 变化
    revenue_yoy_change: Optional[float]   # 最新 vs 上季度
    profit_yoy_change: Optional[float]
    roe_change: Optional[float]

    # 估值
    pe_ttm: Optional[float]
    pb: Optional[float]


def analyze_financial(
    snapshots: List[FinancialSnapshot],
    valuation: Optional[ValuationMetrics],
    as_of: date,
) -> FinancialMetrics:
    """计算财务指标"""
    if not snapshots:
        raise ValueError("snapshots 不能为空")

    # 按日期降序
    snaps = sorted(snapshots, key=lambda s: s.report_date, reverse=True)

    latest = snaps[0]

    def trend(field: str) -> List[float]:
        vals = []
        for s in snaps:
            v = getattr(s, field)
            if v is not None:
                vals.append(v)
        return vals

    def change(field: str) -> Optional[float]:
        if len(snaps) >= 2:
            cur = getattr(snaps[0], field)
            prev = getattr(snaps[1], field)
            if cur is not None and prev is not None:
                return cur - prev
        return None

    return FinancialMetrics(
        symbol=latest.symbol,
        as_of=as_of,
        latest_revenue_yoy=latest.revenue_yoy,
        latest_net_profit_yoy=latest.net_profit_yoy,
        latest_roe=latest.roe,
        latest_gross_margin=latest.gross_margin,
        latest_net_margin=latest.net_margin,
        latest_debt_ratio=latest.debt_ratio,
        revenue_yoy_trend=trend("revenue_yoy"),
        net_profit_yoy_trend=trend("net_profit_yoy"),
        roe_trend=trend("roe"),
        revenue_yoy_change=change("revenue_yoy"),
        profit_yoy_change=change("net_profit_yoy"),
        roe_change=change("roe"),
        pe_ttm=valuation.pe_ttm if valuation else None,
        pb=valuation.pb if valuation else None,
    )
