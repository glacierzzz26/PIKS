"""基于 akshare 的公告 Provider"""
import akshare as ak
import pandas as pd
from datetime import date, timedelta
from typing import List

from ...models import Symbol
from ...models.announcement import Announcement
from ..base import AnnouncementProvider


class AkShareAnnouncementProvider(AnnouncementProvider):
    """基于 akshare 的公告 Provider"""

    @property
    def name(self) -> str:
        return "akshare_announcement"

    def search(self, symbol: Symbol, days: int = 30) -> List[Announcement]:
        """
        获取个股最近 N 天的公告。
        akshare 的 stock_individual_notice_report 返回全部历史，按日期过滤。
        """
        df = ak.stock_individual_notice_report(security=symbol.code)
        if df is None or df.empty:
            return []

        # 列名映射
        df = df.rename(columns={
            "公告标题": "title",
            "公告类型": "category",
            "公告日期": "publish_date",
            "网址": "url",
        })

        cutoff = date.today() - timedelta(days=days)
        items = []
        for i, row in df.iterrows():
            try:
                pub = pd.to_datetime(row["publish_date"]).date()
            except Exception:
                continue

            if pub < cutoff:
                continue

            items.append(Announcement(
                id=f"ann_{symbol.code}_{pub.strftime('%Y%m%d')}_{i}",
                symbol=symbol.full_code,
                title=str(row.get("title", "")),
                category=str(row.get("category", "")),
                publish_date=pub,
                url=str(row.get("url", "")) if pd.notna(row.get("url")) else None,
            ))

        return items
