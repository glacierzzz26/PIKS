"""datahub 个股公告 provider（P14-1 / B6；替 `AkShareAnnouncementProvider`）。

读 `announcement_by_stock`。时间范围**在 datahub 侧过滤**（`start`/`end`），
适配层不重滤（见 `mappers.to_announcements`）。空标题已在 datahub 侧丢弃。
"""
from datetime import timedelta
from typing import List

from ...models import Symbol
from ...models.announcement import Announcement
from ...providers.base import AnnouncementProvider
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider


class DataHubAnnouncementProvider(DataHubBackedProvider, AnnouncementProvider):
    """经 datahub 取个股公告（与 `AkShareAnnouncementProvider` 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_announcement"

    def search(self, symbol: Symbol, days: int = 30) -> List[Announcement]:
        end = self._anchor()
        start = end - timedelta(days=days)
        rows = self.client.get("announcement_by_stock", {
            "codes": symbol.code,
            "start": self._iso(start),
            "end": self._iso(end),
        })
        return M.to_announcements(symbol, rows)
