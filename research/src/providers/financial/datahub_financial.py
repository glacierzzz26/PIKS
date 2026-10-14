"""datahub 财务/估值 provider（P14-1 / B6；替 `AkShareFinancialProvider`）。

- 财务：读 `financial_indicator`（`as_of` 防未来函数 → 截断 `announce_date ≤ as_of`；
  `last=quarters` 取最近 N 期）。`net_margin` 来自 datahub 侧表，未采集时为 None。
- 估值：读 `daily_valuation` 末行（`asof` 锚点），映射为 `ValuationMetrics`。
"""
from datetime import date
from typing import List, Optional

from ...models import Symbol
from ...models.financial import FinancialSnapshot, ValuationMetrics
from ...providers.base import FinancialProvider
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider


class DataHubFinancialProvider(DataHubBackedProvider, FinancialProvider):
    """经 datahub 取财务/估值（与原 akshare provider 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_financial"

    def get_financials(self, symbol: Symbol, quarters: int = 4) -> List[FinancialSnapshot]:
        rows = self.client.get("financial_indicator", {
            "codes": symbol.code,
            "last": quarters,
            "as_of": self._iso(self.as_of),
        })
        return M.to_financial_snapshots(symbol, rows)

    def get_valuation(self, symbol: Symbol) -> Optional[ValuationMetrics]:
        rows = self.client.get("daily_valuation", {
            "codes": symbol.code,
            "last": 1,
            "asof": self._iso(self.as_of),
        })
        return M.to_valuation(symbol, rows[-1] if rows else None)
