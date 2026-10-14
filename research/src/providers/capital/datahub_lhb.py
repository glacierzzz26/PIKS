"""datahub 龙虎榜 provider（P14-1 / B6；替 `AkShareLHBProvider`）。

读 `capital_flow`（记录，每 code 每日一行）+ `capital_flow_detail`（营业部明细）。
口径红线已在 datahub 采集器内落定（同日多因合并为一行、明细按 `(营业部,买入,卖出)`
去重、丢弃「连续三个交易日」块）—— 适配层只做行 → `LHBRecord`/`LHBDetail` 映射。
"""
from datetime import date, timedelta
from typing import List, Optional

from ...models import Symbol
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider
from .akshare_lhb import LHBDetail, LHBRecord


class DataHubLHBProvider(DataHubBackedProvider):
    """经 datahub 取龙虎榜记录/明细（与 `AkShareLHBProvider` 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_lhb"

    def search(
        self,
        symbol: Symbol,
        days: int = 30,
        end: Optional[date] = None,
    ) -> List[LHBRecord]:
        end = end or self._anchor()
        start = end - timedelta(days=days)
        rows = self.client.get("capital_flow", {
            "codes": symbol.code,
            "start": self._iso(start),
            "end": self._iso(end),
        })
        return M.to_lhb_records(symbol, rows)

    def get_detail(self, symbol: Symbol, trade_date: date) -> List[LHBDetail]:
        rows = self.client.get("capital_flow_detail", {
            "codes": symbol.code,
            "date": self._iso(trade_date),
        })
        return M.to_lhb_details(rows)
