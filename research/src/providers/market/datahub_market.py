"""datahub 行情 provider（P14-1 / B6；替 `AkShareMarketProvider`）。

读 `daily_price`（**含指数行**）。单位约定见 `datahub/mappers.py`：
`volume` 已是**手**、`turnover_rate` 已是 **%** —— 与本模块**不做**腾讯源的
`//100` / `×100`（那是 `akshare_provider` 的专有转换，datahub 侧已归一到 A 股口径）。
`close` 用不复权价，不乘 `adj_factor`。

派生字段（`pct_change`/`amplitude`/`chg_amount`/涨跌停）在 `mappers.to_bars` 里
按 `Symbol.limit_up_pct` 派生，datahub 不内嵌 PIKS 涨跌停规则（B4）。
"""
from datetime import date
from typing import List

from ...models import Bar, Quote, Symbol
from ...providers.base import MarketProvider
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider


class DataHubMarketProvider(DataHubBackedProvider, MarketProvider):
    """经 datahub `daily_price` 取行情（与 `AkShareMarketProvider` 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_market"

    def get_history(
        self,
        symbol: Symbol,
        start_date: date,
        end_date: date,
        adjust: str = "qfq",
    ) -> List[Bar]:
        rows = self.client.get("daily_price", {
            "codes": symbol.code,
            "start": self._iso(start_date),
            "end": self._iso(end_date),
        })
        return M.to_bars(symbol, rows)

    def get_quote(self, symbol: Symbol) -> Quote:
        rows = self.client.get("daily_price", {
            "codes": symbol.code,
            "last": 2,
            "asof": self._iso(self.as_of),
        })
        quote = M.to_quote(symbol, rows)
        if quote is None:
            raise ValueError(
                f"无法获取 {symbol.full_code} 的行情（datahub daily_price 不足两行）"
            )
        return quote
