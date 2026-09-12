import akshare as ak
import pandas as pd
from datetime import date, datetime, timedelta
from typing import List

from ...models import Symbol, Bar, Quote, Market
from ...providers.base import MarketProvider


class AkShareMarketProvider(MarketProvider):
    """基于 akshare 的行情数据 Provider"""

    @property
    def name(self) -> str:
        return "akshare_market"

    def get_history(
        self,
        symbol: Symbol,
        start_date: date,
        end_date: date,
        adjust: str = "qfq",
    ) -> List[Bar]:
        """
        获取历史 K 线（日频）。
        当前环境东财/新浪接口被限制，改用腾讯接口 stock_zh_a_hist_tx。
        """
        # 腾讯接口返回全部历史，按日期过滤
        df = ak.stock_zh_a_hist_tx(symbol=symbol.code)

        if df is None or df.empty:
            return []

        # 按日期过滤
        df["date"] = pd.to_datetime(df["date"])
        mask = (df["date"].dt.date >= start_date) & (df["date"].dt.date <= end_date)
        df = df.loc[mask].copy()

        if df.empty:
            return []

        df = df.sort_values("date").reset_index(drop=True)

        bars = []
        prev_close = None
        for _, row in df.iterrows():
            bar_date = pd.to_datetime(row["date"]).date()
            close = float(row["close"])

            # 涨跌幅自己算（腾讯接口不提供）
            if prev_close is not None and prev_close > 0:
                pct = (close - prev_close) / prev_close * 100
            else:
                pct = 0.0

            # 振幅
            high = float(row["high"])
            low = float(row["low"])
            open_ = float(row["open"])
            amplitude = ((high - low) / prev_close * 100) if prev_close and prev_close > 0 else 0.0

            # 涨跌停判断
            is_limit_up = pct >= symbol.limit_up_pct * 0.99
            is_limit_down = pct <= symbol.limit_down_pct * 0.99

            # 腾讯接口数据转换：
            # - volume 是股数，转为手（÷100）
            # - turnover 是小数，转为百分比（×100）
            # - amount 是元，保持不变
            volume_shou = int(row["volume"]) // 100
            turnover_pct = float(row.get("turnover", 0) or 0) * 100

            bar = Bar(
                symbol=symbol.full_code,
                date=bar_date,
                open=open_,
                high=high,
                low=low,
                close=close,
                volume=volume_shou,
                amount=float(row.get("amount", 0) or 0),
                amplitude=amplitude,
                pct_change=pct,
                chg_amount=close - prev_close if prev_close else 0.0,
                turnover=turnover_pct,
                is_limit_up=is_limit_up,
                is_limit_down=is_limit_down,
                is_halted=False,
            )
            bars.append(bar)
            prev_close = close

        return bars

    def get_quote(self, symbol: Symbol) -> Quote:
        """
        获取实时行情快照。
        由于东财/新浪实时接口受限且慢，用腾讯 K 线最后两条推导快照。
        """
        df = ak.stock_zh_a_hist_tx(symbol=symbol.code)
        if df is None or len(df) < 2:
            raise ValueError(f"无法获取 {symbol.full_code} 的行情")

        df["date"] = pd.to_datetime(df["date"])
        df = df.sort_values("date")
        today = df.iloc[-1]
        prev = df.iloc[-2]

        # 涨跌幅
        pct = (today["close"] - prev["close"]) / prev["close"] * 100 if prev["close"] else 0

        return Quote(
            symbol=symbol.full_code,
            name=symbol.code,  # 腾讯接口无名称，用代码代替
            price=float(today["close"]),
            pre_close=float(prev["close"]),
            open=float(today["open"]),
            high=float(today["high"]),
            low=float(today["low"]),
            volume=int(today["volume"]),
            amount=float(today.get("amount", 0) or 0),
            pct_change=pct,
            turnover=float(today.get("turnover", 0) or 0),
            pe_ttm=0.0,   # 腾讯接口无估值，后续接入财务 Provider
            pb=0.0,
            market_cap=0.0,
            float_cap=0.0,
            timestamp=datetime.now(),
        )
