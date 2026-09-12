from abc import ABC, abstractmethod
from typing import List
from datetime import date

from ..models import Symbol, Bar, Quote, Period


class MarketProvider(ABC):
    """行情数据 Provider 接口"""

    @property
    @abstractmethod
    def name(self) -> str:
        ...

    @abstractmethod
    def get_history(
        self,
        symbol: Symbol,
        start_date: date,
        end_date: date,
        adjust: str = "qfq",   # 前复权
    ) -> List[Bar]:
        """获取历史 K 线"""
        ...

    @abstractmethod
    def get_quote(self, symbol: Symbol) -> Quote:
        """获取实时行情快照"""
        ...


class FinancialProvider(ABC):
    """财务数据 Provider 接口（占位）"""

    @property
    @abstractmethod
    def name(self) -> str:
        ...


class NewsProvider(ABC):
    """新闻 Provider 接口（占位）"""

    @property
    @abstractmethod
    def name(self) -> str:
        ...


class AnnouncementProvider(ABC):
    """公告 Provider 接口（占位）"""

    @property
    @abstractmethod
    def name(self) -> str:
        ...
