"""成交量/成交额/换手率分析引擎"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional
import numpy as np

from ..models import Bar


@dataclass
class VolumeMetrics:
    """量价指标卡"""
    symbol: str
    as_of: date
    period_days: int

    # 成交量统计
    total_volume: int
    avg_volume_5d: Optional[float]
    avg_volume_20d: Optional[float]
    avg_volume_60d: Optional[float]

    # 成交额统计
    total_amount: float
    avg_amount_5d: Optional[float]
    avg_amount_20d: Optional[float]

    # 换手率统计
    avg_turnover_5d: Optional[float]
    avg_turnover_20d: Optional[float]
    avg_turnover_60d: Optional[float]
    max_turnover: float
    min_turnover: float

    # 量能异常
    abnormal_volume_days: int
    abnormal_volume_dates: List[date]

    # 量价关系（相关系数）
    price_volume_corr: Optional[float]


def analyze_volume(bars: List[Bar], as_of: date) -> VolumeMetrics:
    """计算成交量/成交额/换手率指标"""
    if not bars:
        raise ValueError("bars 不能为空")

    n = len(bars)
    volumes = np.array([b.volume for b in bars], dtype=float)
    amounts = np.array([b.amount for b in bars], dtype=float)
    turnovers = np.array([b.turnover for b in bars], dtype=float)
    closes = np.array([b.close for b in bars], dtype=float)

    def avg_last(days: int) -> Optional[float]:
        if n < days:
            return None
        return float(np.mean(volumes[-days:]))

    def avg_turnover_last(days: int) -> Optional[float]:
        if n < days:
            return None
        return float(np.mean(turnovers[-days:]))

    # 量能异常：当日量 > max(前20日均量 × 2, 前5日峰值)
    # 使用滚动窗口，每一天的 threshold 基于该日之前 20 个交易日
    abnormal_dates = []
    for i in range(n):
        if i < 20:
            continue  # 前20天无足够历史，跳过
        vol_20_avg = np.mean(volumes[i - 20:i])
        vol_5_max = np.max(volumes[i - 5:i]) if i >= 5 else vol_20_avg
        threshold = max(vol_20_avg * 2, vol_5_max)
        if volumes[i] > threshold:
            abnormal_dates.append(bars[i].date)

    # 量价相关系数
    corr = None
    if n >= 5:
        with np.errstate(invalid='ignore'):
            c = np.corrcoef(closes[-min(n, 20):], volumes[-min(n, 20):])[0, 1]
            corr = float(c) if not np.isnan(c) else None

    return VolumeMetrics(
        symbol=bars[0].symbol,
        as_of=as_of,
        period_days=n,
        total_volume=int(np.sum(volumes)),
        avg_volume_5d=avg_last(5),
        avg_volume_20d=avg_last(20),
        avg_volume_60d=avg_last(60),
        total_amount=float(np.sum(amounts)),
        avg_amount_5d=float(np.mean(amounts[-5:])) if n >= 5 else None,
        avg_amount_20d=float(np.mean(amounts[-20:])) if n >= 20 else None,
        avg_turnover_5d=avg_turnover_last(5),
        avg_turnover_20d=avg_turnover_last(20),
        avg_turnover_60d=avg_turnover_last(60),
        max_turnover=float(np.max(turnovers)),
        min_turnover=float(np.min(turnovers)),
        abnormal_volume_days=len(abnormal_dates),
        abnormal_volume_dates=abnormal_dates,
        price_volume_corr=corr,
    )
