"""基于 akshare 的新闻 Provider"""
import akshare as ak
import pandas as pd
from datetime import datetime, timedelta
from typing import List

from ...models import Symbol
from ...models.news import NewsItem
from ..base import NewsProvider


class AkShareNewsProvider(NewsProvider):
    """基于 akshare 的个股新闻 Provider"""

    @property
    def name(self) -> str:
        return "akshare_news"

    def search(self, symbol: Symbol, days: int = 30) -> List[NewsItem]:
        """
        获取个股最近 N 天的新闻。
        akshare 的 stock_news_em 返回最近 10 条，不保证时间范围。
        返回结果在内存中按日期过滤。
        """
        df = ak.stock_news_em(symbol=symbol.code)
        if df is None or df.empty:
            return []

        # 列名映射（中文 → 英文）
        df = df.rename(columns={
            "新闻标题": "title",
            "新闻内容": "content",
            "发布时间": "published",
            "文章来源": "source",
            "新闻链接": "url",
        })

        cutoff = datetime.now() - timedelta(days=days)
        items = []
        for i, row in df.iterrows():
            try:
                pub = pd.to_datetime(row["published"])
            except Exception:
                continue

            if pub < cutoff:
                continue

            items.append(NewsItem(
                id=f"news_{symbol.code}_{pub.strftime('%Y%m%d%H%M%S')}",
                symbol=symbol.full_code,
                title=str(row.get("title", "")),
                content=str(row.get("content", "")),
                published_at=pub,
                source=str(row.get("source", "")),
                url=str(row.get("url", "")) if pd.notna(row.get("url")) else None,
            ))

        return items
