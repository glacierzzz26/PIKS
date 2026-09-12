"""新闻数据模型"""
from dataclasses import dataclass
from datetime import datetime
from typing import Optional


@dataclass(frozen=True)
class NewsItem:
    """单条新闻"""
    id: str
    symbol: str
    title: str
    content: str
    published_at: datetime
    source: str
    url: Optional[str]

    @property
    def is_earnings_related(self) -> bool:
        """是否与财报相关"""
        keywords = ["年报", "中报", "季报", "业绩", "净利润", "营收", "盈利"]
        text = self.title + self.content
        return any(k in text for k in keywords)
