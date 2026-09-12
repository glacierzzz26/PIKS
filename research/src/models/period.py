from dataclasses import dataclass
from datetime import datetime, timedelta
from enum import Enum


class PeriodUnit(Enum):
    TRADING_DAY = "trading"   # 交易日
    CALENDAR_DAY = "calendar" # 自然日


@dataclass(frozen=True)
class Period:
    """时间周期定义"""
    value: int              # 数值，如 60
    unit: PeriodUnit        # 单位
    label: str              # 原始标签，如 "60d"

    @staticmethod
    def parse(text: str) -> "Period":
        """解析如 '60d', '30d', '4q', '3y'"""
        text = text.strip().lower()
        unit_char = text[-1]
        num = int(text[:-1])

        if unit_char == "d":
            return Period(value=num, unit=PeriodUnit.TRADING_DAY, label=text)
        if unit_char == "q":
            # 季度转交易日近似
            return Period(value=num * 60, unit=PeriodUnit.TRADING_DAY, label=text)
        if unit_char == "y":
            return Period(value=num * 250, unit=PeriodUnit.TRADING_DAY, label=text)

        raise ValueError(f"无法解析周期: {text}")

    def __str__(self) -> str:
        return self.label
