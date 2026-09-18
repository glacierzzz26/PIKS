"""龙虎榜数据 Provider

数据源:东方财富 `stock_lhb_detail_em(start_date, end_date)`(区间一次取回)+
`stock_lhb_stock_detail_em(code, date, flag)`(单日席位明细)。

⚠️ 2026-09-18 修正(issue #26) —— 原实现有三处会产出**假数字**的缺陷:

1. **`pct_change` 取错字段**。原用新浪 `stock_lhb_detail_daily_sina` 的「对应值」,
   但该字段的**含义随上榜原因变化** —— 换手率榜是换手率、振幅榜是振幅、
   连续三日榜是 3 日累计偏离值,**都不是当日涨跌幅**。实测桂林旅游 2026-09-17
   真实涨跌幅 -9.33%,原实现会渲染成 +35.24%(那其实是换手率)。
   现改用东财 `涨跌幅` 字段(逐行实测该列在全部上榜原因下均为当日真实涨跌幅)。

2. **同一交易日多条记录**。个股同日可因多个原因同时上榜(实测桂林旅游 09-17
   上榜 3 次),原实现 `row.iloc[0]` 只取第一条,且会为同一日产出多条记录 →
   下游按日展开时重复。现按日聚合,全部上榜原因合并进 `reasons`。

3. **席位明细跨原因重复计数**。`stock_lhb_stock_detail_em` 的返回**按上榜原因分块**
   (同一天 3 个原因 = 3 块,每块 5 席),同一营业部的同一笔交易在每块里数值完全相同。
   原实现直接 `+=` 累加 → 机构/游资净额最高虚高 2 倍(实测 30 行 → 去重后仅 10 席,
   虚高 67%)。现:
     - **丢弃「连续三个交易日」块** —— 那是跨 3 日的聚合口径,不是当日成交,
       混入当日资金流即口径错误;
     - 按 `(营业部, 买入, 卖出)` 去重相同交易。
   ⚠️ 买入/卖出两个 flag 的席位实测无交集(交集=0),故合并两侧安全;若某席位
   当日既买又卖,则分别来自两侧、金额不同,不会被误去重。
"""
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from typing import List, Optional, Tuple

import akshare as ak

from ...models import Symbol

# 跨日聚合口径的上榜原因前缀 —— 不是「当日」成交,混入单日资金流即口径错误。
_MULTI_DAY_REASON = "连续三个交易日"


@dataclass(frozen=True)
class LHBRecord:
    """单个交易日的一条龙虎榜记录(同日多个上榜原因已合并)"""
    symbol: str
    name: str
    trade_date: date
    close_price: float
    pct_change: float          # 当日**真实涨跌幅**(%) —— 东财「涨跌幅」
    turnover_pct: float        # 换手率(%) —— 东财「换手率」
    reasons: Tuple[str, ...]   # 当日全部上榜原因

    @property
    def reason(self) -> str:
        """上榜原因的展示串(多个原因以「；」连接)。"""
        return "；".join(self.reasons)


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


def _clean_columns(df):
    df.columns = [str(c).strip() for c in df.columns]
    return df


def _to_float(value) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


class AkShareLHBProvider:
    """
    龙虎榜数据 Provider。
    获取个股在指定区间内的龙虎榜记录及营业部买卖明细。
    """

    @property
    def name(self) -> str:
        return "akshare_lhb"

    def search(
        self,
        symbol: Symbol,
        days: int = 30,
        end: Optional[date] = None,
    ) -> List[LHBRecord]:
        """获取个股最近 N 天的龙虎榜记录(按交易日聚合)。

        区间端点一次取回全市场龙虎榜(实测 17 天回 910 行 / 0.54s,
        整年 20420 行 / 9.9s),再按代码过滤 —— 取代原先逐日历日遍历
        (40 天 = 41 次请求 / 10.7s)。
        """
        end = end or date.today()
        start = end - timedelta(days=days)

        df = ak.stock_lhb_detail_em(
            start_date=start.strftime("%Y%m%d"),
            end_date=end.strftime("%Y%m%d"),
        )
        if df is None or getattr(df, "empty", True):
            return []

        df = _clean_columns(df)
        if "代码" not in df.columns:
            return []

        rows = df[df["代码"].astype(str).str.zfill(6) == symbol.code]
        if rows.empty:
            return []

        # 按交易日聚合:同一天可因多个原因上榜(实测最多 3 条),
        # 收盘价/涨跌幅/换手率在同一日各行一致,上榜原因逐条收集。
        by_date = {}
        for _, r in rows.iterrows():
            d = _parse_date(r.get("上榜日"))
            if d is None:
                continue
            reason = str(r.get("上榜原因", "") or "").strip()
            if d not in by_date:
                by_date[d] = {
                    "name": str(r.get("名称", "") or ""),
                    "close": _to_float(r.get("收盘价")),
                    "pct": _to_float(r.get("涨跌幅")),
                    "turnover": _to_float(r.get("换手率")),
                    "reasons": [],
                }
            if reason and reason not in by_date[d]["reasons"]:
                by_date[d]["reasons"].append(reason)

        records = [
            LHBRecord(
                symbol=symbol.full_code,
                name=v["name"],
                trade_date=d,
                close_price=v["close"],
                pct_change=v["pct"],
                turnover_pct=v["turnover"],
                reasons=tuple(v["reasons"]),
            )
            for d, v in sorted(by_date.items())
        ]
        return records

    def get_detail(self, symbol: Symbol, trade_date: date) -> List[LHBDetail]:
        """获取个股某一天的龙虎榜营业部买卖明细。

        合并买入 + 卖出两侧,**丢弃跨日聚合口径的上榜原因块**(连续三个交易日),
        并按 (营业部, 买入, 卖出) 去重 —— 详见模块 docstring 第 3 条。
        """
        date_str = trade_date.strftime("%Y%m%d")
        seen = set()
        details = []

        for flag in ["买入", "卖出"]:
            try:
                df = ak.stock_lhb_stock_detail_em(
                    symbol=symbol.code,
                    date=date_str,
                    flag=flag,
                )
            except Exception:
                # 不可得即 unavailable:单侧明细缺失不阻断,不猜测。
                continue
            if df is None or getattr(df, "empty", True):
                continue

            df = _clean_columns(df)
            for _, row in df.iterrows():
                reason = str(row.get("类型", "") or "")
                if _MULTI_DAY_REASON in reason:
                    continue  # 跨日聚合块,非当日成交
                name = str(row.get("交易营业部名称", "") or "")
                buy = _to_float(row.get("买入金额"))
                sell = _to_float(row.get("卖出金额"))
                key = (name, buy, sell)
                if key in seen:
                    continue  # 同笔交易在其他上榜原因块中重复出现
                seen.add(key)
                details.append(LHBDetail(
                    trade_date=trade_date,
                    dealer_name=name,
                    buy_amount=buy,
                    buy_pct=_to_float(row.get("买入金额-占总成交比例")),
                    sell_amount=sell,
                    sell_pct=_to_float(row.get("卖出金额-占总成交比例")),
                    net_amount=_to_float(row.get("净额")),
                    reason=reason,
                ))

        return details


def _parse_date(value) -> Optional[date]:
    """东财「上榜日」形如 `2026-09-17`;解析不出返回 None(不猜)。"""
    if value is None:
        return None
    if isinstance(value, date):
        return value
    text = str(value).strip()
    for fmt in ("%Y-%m-%d", "%Y/%m/%d", "%Y%m%d"):
        try:
            return datetime.strptime(text, fmt).date()
        except ValueError:
            continue
    return None
