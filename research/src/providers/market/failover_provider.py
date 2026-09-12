"""多数据源 Failover Market Provider

按优先级自动尝试多个数据源，前一个失败后切换到下一个。
"""
from datetime import date
from typing import List, Callable, Any, Tuple

from ...models import Symbol, Bar, Quote
from ...providers.base import MarketProvider


class FailoverMarketProvider(MarketProvider):
    """
    多数据源自动容灾 Provider。
    按优先级依次尝试，返回第一个成功的结果。
    """

    def __init__(self, providers: List[MarketProvider] = None):
        if providers is None:
            # 默认优先级链
            from .akshare_provider import AkShareMarketProvider
            providers = [
                AkShareMarketProvider(),
            ]
        self._providers = providers

    @property
    def name(self) -> str:
        names = [p.name for p in self._providers]
        return f"failover[{','.join(names)}]"

    def get_history(
        self,
        symbol: Symbol,
        start_date: date,
        end_date: date,
        adjust: str = "qfq",
    ) -> List[Bar]:
        return self._try_each(
            f"get_history({symbol.full_code})",
            lambda p: p.get_history(symbol, start_date, end_date, adjust),
        )

    def get_quote(self, symbol: Symbol) -> Quote:
        return self._try_each(
            f"get_quote({symbol.full_code})",
            lambda p: p.get_quote(symbol),
        )

    def _try_each(self, label: str, fn: Callable[[MarketProvider], Any]) -> Any:
        """按顺序尝试每个 Provider，返回第一个成功的结果。"""
        last_err = None
        for provider in self._providers:
            try:
                result = fn(provider)
                # 空结果也算失败，继续下一个
                if result is None or (isinstance(result, list) and len(result) == 0):
                    continue
                return result
            except Exception as e:
                last_err = e
                # 静默失败，尝试下一个
                continue
        # 全部失败
        if last_err:
            raise RuntimeError(
                f"所有数据源均失败 ({label}): {type(last_err).__name__}: {last_err}"
            )
        raise RuntimeError(f"所有数据源均返回空结果 ({label})")
