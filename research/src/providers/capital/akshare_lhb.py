"""龙虎榜数据 Provider"""
from dataclasses import dataclass
from datetime import date, timedelta
from typing import List

import akshare as ak

from ...models import Symbol


@dataclass(frozen=True)
class LHBRecord:
    """单条龙虎榜记录"""
    symbol: str
    name: str
    trade_date: date
    close_price: float
    pct_change: float
    volume: float
    amount: float
    reason: str           # 上榜原因，如"涨幅偏离值达7%的证券"


@dataclass(frozen=True)
class LHBDetail:
    """龙虎榜营业部明细"""
    trade_date: date
    dealer_name: str      # 营业部名称
    buy_amount: float     # 买入金额（元）
    buy_pct: float        # 买入占总成交比例
    sell_amount: float    # 卖出金额（元）
    sell_pct: float       # 卖出占总成交比例
    net_amount: float     # 净额（元）
    reason: str


class AkShareLHBProvider:
    """
    龙虎榜数据 Provider。
    获取个股在指定区间内的龙虎榜记录及营业部买卖明细。
    """

    @property
    def name(self) -> str:
        return "akshare_lhb"

    def search(self, symbol: Symbol, days: int = 30) -> List[LHBRecord]:
        """
        获取个股最近 N 天的龙虎榜记录。
        遍历区间内每个交易日，查询当日全部上榜股票，过滤目标个股。
        """
        end = date.today()
        start = end - timedelta(days=days + 10)

        records = []
        current = start
        while current <= end:
            date_str = current.strftime("%Y%m%d")
            try:
                df = ak.stock_lhb_detail_daily_sina(date=date_str)
                if df is None or df.empty:
                    current += timedelta(days=1)
                    continue

                df.columns = [str(c).strip() for c in df.columns]
                code_col = "股票代码" if "股票代码" in df.columns else None
                if code_col is None:
                    current += timedelta(days=1)
                    continue

                row = df[df[code_col] == symbol.code]
                if not row.empty:
                    r = row.iloc[0]
                    records.append(LHBRecord(
                        symbol=symbol.full_code,
                        name=str(r.get("股票名称", "")),
                        trade_date=current,
                        close_price=float(r.get("收盘价", 0) or 0),
                        pct_change=float(r.get("对应值", 0) or 0),
                        volume=float(r.get("成交量", 0) or 0),
                        amount=float(r.get("成交额", 0) or 0),
                        reason=str(r.get("指标", "")),
                    ))
            except Exception:
                pass
            current += timedelta(days=1)

        return records

    def get_detail(self, symbol: Symbol, trade_date: date) -> List[LHBDetail]:
        """
        获取个股某一天的龙虎榜营业部买卖明细。
        合并买入和卖出数据。
        """
        date_str = trade_date.strftime("%Y%m%d")
        details = []

        for flag in ["买入", "卖出"]:
            try:
                df = ak.stock_lhb_stock_detail_em(
                    symbol=symbol.code,
                    date=date_str,
                    flag=flag,
                )
                if df is None or df.empty:
                    continue

                for _, row in df.iterrows():
                    details.append(LHBDetail(
                        trade_date=trade_date,
                        dealer_name=str(row.get("交易营业部名称", "")),
                        buy_amount=float(row.get("买入金额", 0) or 0),
                        buy_pct=float(row.get("买入金额-占总成交比例", 0) or 0),
                        sell_amount=float(row.get("卖出金额", 0) or 0),
                        sell_pct=float(row.get("卖出金额-占总成交比例", 0) or 0),
                        net_amount=float(row.get("净额", 0) or 0),
                        reason=str(row.get("类型", "")),
                    ))
            except Exception:
                continue

        return details
