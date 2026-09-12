from dataclasses import dataclass
from datetime import date
from typing import Optional


@dataclass(frozen=True)
class Bar:
    """K线数据"""
    symbol: str
    date: date
    open: float
    high: float
    low: float
    close: float
    volume: int          # 手（100股）
    amount: float        # 成交额（元）
    amplitude: float     # 振幅（%）
    pct_change: float    # 涨跌幅（%）
    chg_amount: float    # 涨跌额（元）
    turnover: float      # 换手率（%）

    # 复权与特殊状态标记
    adj_factor: Optional[float] = None   # 复权因子
    is_limit_up: bool = False
    is_limit_down: bool = False
    is_halted: bool = False

    @property
    def is_yizi(self) -> bool:
        """是否一字板（开盘=收盘=最高=最低，且涨停或跌停）"""
        if not (self.is_limit_up or self.is_limit_down):
            return False
        return abs(self.open - self.close) < 0.001 and abs(self.high - self.low) < 0.001
