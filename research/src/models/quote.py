from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class Quote:
    """实时行情快照"""
    symbol: str
    name: str
    price: float
    pre_close: float
    open: float
    high: float
    low: float
    volume: int
    amount: float
    pct_change: float
    turnover: float
    pe_ttm: float
    pb: float
    market_cap: float      # 总市值（元）
    float_cap: float       # 流通市值（元）
    timestamp: datetime
