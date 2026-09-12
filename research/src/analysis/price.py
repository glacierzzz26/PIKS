"""价格分析引擎"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional
import numpy as np

from ..models import Bar


@dataclass
class PriceMetrics:
    """价格指标卡"""
    symbol: str
    as_of: date
    period_days: int

    # 区间统计
    start_price: float
    end_price: float
    max_price: float
    min_price: float

    # 涨跌幅
    return_pct_5d: Optional[float]
    return_pct_10d: Optional[float]
    return_pct_20d: Optional[float]
    return_pct_60d: Optional[float]
    period_return_pct: float

    # 回撤
    max_drawdown_pct: float
    max_rise_pct: float

    # 波动率
    volatility_annual: float

    # 相对指数（占位）
    vs_index_return_pct: Optional[float] = None

    # 特殊标记
    limit_up_days: int = 0
    limit_down_days: int = 0
    halted_days: int = 0


def analyze_price(bars: List[Bar], as_of: date) -> PriceMetrics:
    """
    计算价格指标。
    bars 必须按日期升序排列，且均为前复权数据。
    """
    if not bars:
        raise ValueError("bars 不能为空")

    closes = np.array([b.close for b in bars])
    highs = np.array([b.high for b in bars])
    lows = np.array([b.low for b in bars])

    n = len(bars)

    # 区间涨跌幅
    start_price = closes[0]
    end_price = closes[-1]
    period_return = (end_price - start_price) / start_price * 100

    # 分窗口涨跌幅（从终点往前数）
    def window_return(days: int) -> Optional[float]:
        if n < days:
            return None
        prev = closes[-days - 1] if n > days else closes[0]
        return (end_price - prev) / prev * 100

    # 最大回撤（从前高到后低）
    peak = closes[0]
    max_dd = 0.0
    max_rise = 0.0
    for i in range(1, n):
        if closes[i] > peak:
            peak = closes[i]
        dd = (peak - closes[i]) / peak * 100
        if dd > max_dd:
            max_dd = dd
        rise = (highs[i] - lows[i - 1]) / lows[i - 1] * 100 if i > 0 else 0
        if rise > max_rise:
            max_rise = rise

    # 年化波动率（日收益率标准差 × √250）
    if n >= 2:
        daily_returns = np.diff(closes) / closes[:-1]
        vol = np.std(daily_returns, ddof=1) * np.sqrt(250) * 100
    else:
        vol = 0.0

    # 涨跌停统计
    limit_up = sum(1 for b in bars if b.is_limit_up)
    limit_down = sum(1 for b in bars if b.is_limit_down)
    halted = sum(1 for b in bars if b.is_halted)

    return PriceMetrics(
        symbol=bars[0].symbol,
        as_of=as_of,
        period_days=n,
        start_price=start_price,
        end_price=end_price,
        max_price=float(np.max(highs)),
        min_price=float(np.min(lows)),
        return_pct_5d=window_return(5),
        return_pct_10d=window_return(10),
        return_pct_20d=window_return(20),
        return_pct_60d=window_return(60),
        period_return_pct=period_return,
        max_drawdown_pct=max_dd,
        max_rise_pct=max_rise,
        volatility_annual=vol,
        limit_up_days=limit_up,
        limit_down_days=limit_down,
        halted_days=halted,
    )
