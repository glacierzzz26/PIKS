"""事件分析引擎

在原始新闻/公告之上做降噪分级（§10），并保留轻量摘要字段供评分卡使用。
"""
from dataclasses import dataclass, field
from datetime import date
from typing import Any, Dict, List, Optional

from ..models.news import NewsItem
from ..models.announcement import Announcement
from ..models.bar import Bar
from .event_denoise import EventFilterResult, filter_events


@dataclass
class EventMetrics:
    """事件指标"""
    symbol: str
    as_of: date
    total_news: int
    recent_news: List[NewsItem]
    earnings_mentions: int
    major_events: List[NewsItem]  # 重大新闻（兼容旧字段）
    denoised: Optional[EventFilterResult] = None  # 降噪分级结果

    def to_dict(self) -> Dict[str, Any]:
        d: Dict[str, Any] = {
            "symbol": self.symbol,
            "as_of": self.as_of.isoformat(),
            "total_news": self.total_news,
            "earnings_mentions": self.earnings_mentions,
            "recent_news": [
                {
                    "title": n.title,
                    "source": n.source,
                    "published": n.published_at.isoformat(),
                }
                for n in self.recent_news
            ],
            "major_events": [
                {
                    "title": n.title,
                    "source": n.source,
                    "published": n.published_at.isoformat(),
                }
                for n in self.major_events
            ],
        }
        if self.denoised:
            d["denoised"] = self.denoised.to_dict()
        return d


def analyze_events(
    news: List[NewsItem],
    as_of: date,
    max_items: int = 5,
    announcements: Optional[List[Announcement]] = None,
    bars: Optional[List[Bar]] = None,
    window_days: int = 30,
) -> EventMetrics:
    """分析新闻事件 + 降噪分级

    Args:
        news          : 新闻列表
        announcements : 公告列表（参与降噪合并 + 分类）
        bars          : K线（用于事件→价格关联）
    """
    earnings = [n for n in news if n.is_earnings_related]
    sorted_news = sorted(news, key=lambda n: n.published_at, reverse=True)

    # 降噪分级（新闻 + 公告）
    denoised = None
    if news or announcements:
        denoised = filter_events(
            symbol=news[0].symbol if news else (announcements[0].symbol if announcements else ""),
            as_of=as_of,
            news=news,
            announcements=announcements,
            bars=bars,
            window_days=window_days,
        )

    return EventMetrics(
        symbol=news[0].symbol if news else (announcements[0].symbol if announcements else ""),
        as_of=as_of,
        total_news=len(news),
        recent_news=sorted_news[:max_items],
        earnings_mentions=len(earnings),
        major_events=earnings[:max_items],
        denoised=denoised,
    )