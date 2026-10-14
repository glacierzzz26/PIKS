"""datahub 个股新闻 provider（P14-1 / B6；替 `AkShareNewsProvider`）。

读 `news_by_stock`。时间范围**在 datahub 侧过滤**（`start`/`end`），适配层不重滤
（见 `mappers.to_news_items`）。窗口锚点用 `as_of`（缺省 today）。
"""
from datetime import timedelta
from typing import List

from ...models import Symbol
from ...models.news import NewsItem
from ...providers.base import NewsProvider
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider


class DataHubNewsProvider(DataHubBackedProvider, NewsProvider):
    """经 datahub 取个股新闻（与 `AkShareNewsProvider` 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_news"

    def search(self, symbol: Symbol, days: int = 30) -> List[NewsItem]:
        end = self._anchor()
        start = end - timedelta(days=days)
        rows = self.client.get("news_by_stock", {
            "codes": symbol.code,
            "start": self._iso(start),
            "end": self._iso(end),
        })
        return M.to_news_items(symbol, rows)
