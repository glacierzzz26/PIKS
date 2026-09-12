"""Analysis Engine 阈值配置"""
from dataclasses import dataclass


@dataclass(frozen=True)
class AnalysisConfig:
    # 量能异常判定
    abnormal_volume_multiplier: float = 2.0   # 当日量 > N 倍 20 日均量
    abnormal_volume_lookback: int = 20

    # 换手率分位
    turnover_percentile_window: int = 520     # 约 2 年交易日

    # 价格回撤
    max_drawback_lookback: int = 260          # 1 年交易日

    # 波动率
    volatility_window: int = 60

    # 事件窗口
    event_window_short: int = 3   # 事件日 ±3 交易日
    event_window_long: int = 5    # 事件日 ±5 交易日
