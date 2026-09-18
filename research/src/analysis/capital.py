"""资金面分析引擎"""
from dataclasses import dataclass
from datetime import date
from typing import List, Dict

from ..providers.capital.akshare_lhb import LHBRecord, LHBDetail


@dataclass
class DealerSummary:
    """单个营业部的汇总"""
    name: str
    total_buy: float
    total_sell: float
    net_amount: float
    appearances: int
    dealer_type: str   # 机构/北向/游资/散户/未知

@dataclass
class DailyCapital:
    """单日龙虎榜分析结果"""
    trade_date: date
    reason: str           # 上榜原因(多个以「；」连接)
    close_price: float
    pct_change: float     # 当日**真实涨跌幅**(%)
    top_buyers: List[DealerSummary]   # 当日净买入前五
    top_sellers: List[DealerSummary]  # 当日净卖出前五
    day_net_buy: float    # 当日全部席位净买入合计

    # 当日资金方汇总
    institution_net: float
    northbound_net: float
    retail_net: float
    hot_money_net: float


@dataclass
class CapitalMetrics:
    """资金面指标卡"""
    symbol: str
    as_of: date

    # 采集窗口(天),供报告如实陈述「最近 N 天」
    window_days: int

    # 龙虎榜统计
    lhb_count: int
    lhb_records: List[LHBRecord]

    # 按日明细（核心）
    daily: List[DailyCapital]

    # 全时段汇总
    total_net_buy: float
    dominant_force: str


def classify_dealer(name: str) -> str:
    """识别营业部类型"""
    if "机构专用" in name:
        return "机构"
    if "深股通专用" in name or "沪股通专用" in name:
        return "北向"
    if "拉萨" in name:
        return "散户"
    hot_money_keywords = [
        "中金公司", "华泰证券", "国泰君安",
        "中信证券", "招商证券", "银河证券",
    ]
    for kw in hot_money_keywords:
        if kw in name:
            return "游资"
    return "未知"


def _analyze_one_day(
    trade_date: date,
    details: List[LHBDetail],
    record: LHBRecord,
) -> DailyCapital:
    """分析单日龙虎榜数据"""
    dealer_map: Dict[str, DealerSummary] = {}

    for d in details:
        dtype = classify_dealer(d.dealer_name)
        key = d.dealer_name
        if key not in dealer_map:
            dealer_map[key] = DealerSummary(
                name=d.dealer_name,
                total_buy=0.0,
                total_sell=0.0,
                net_amount=0.0,
                appearances=0,
                dealer_type=dtype,
            )
        s = dealer_map[key]
        s.total_buy += d.buy_amount
        s.total_sell += d.sell_amount
        s.net_amount += d.net_amount
        s.appearances += 1

    # 排序
    sorted_dealers = sorted(dealer_map.values(), key=lambda x: x.net_amount, reverse=True)
    top_buyers = [d for d in sorted_dealers if d.net_amount > 0][:5]
    top_sellers = [d for d in sorted_dealers if d.net_amount < 0][:5]

    # 当日资金方汇总
    inst = north = retail = hot = 0.0
    for s in dealer_map.values():
        if s.dealer_type == "机构":
            inst += s.net_amount
        elif s.dealer_type == "北向":
            north += s.net_amount
        elif s.dealer_type == "散户":
            retail += s.net_amount
        else:
            hot += s.net_amount

    day_total = inst + north + retail + hot

    return DailyCapital(
        trade_date=trade_date,
        reason=record.reason,
        close_price=record.close_price,
        pct_change=record.pct_change,
        top_buyers=top_buyers,
        top_sellers=top_sellers,
        day_net_buy=day_total,
        institution_net=inst,
        northbound_net=north,
        retail_net=retail,
        hot_money_net=hot,
    )


def analyze_capital(
    records: List[LHBRecord],
    details_map: Dict[date, List[LHBDetail]],
    as_of: date,
    window_days: int = 30,
) -> CapitalMetrics:
    """
    分析龙虎榜数据，按日展开机构/游资/北向/散户行为。

    window_days: 采集窗口(天),仅用于报告如实陈述「最近 N 天」——
    原先渲染层硬编码「最近 5 天」而 provider 实采 30 天,三方不一致(issue #26)。
    """
    if not records:
        return CapitalMetrics(
            symbol="",
            as_of=as_of,
            window_days=window_days,
            lhb_count=0,
            lhb_records=[],
            daily=[],
            total_net_buy=0.0,
            dominant_force="无数据",
        )

    symbol = records[0].symbol

    # 按日分析
    daily_list = []
    for rec in sorted(records, key=lambda r: r.trade_date):
        details = details_map.get(rec.trade_date, [])
        if not details:
            continue
        daily_list.append(_analyze_one_day(rec.trade_date, details, rec))

    # 全时段汇总
    total_net = sum(d.day_net_buy for d in daily_list)

    # 全时段主导力量
    inst_total = sum(d.institution_net for d in daily_list)
    north_total = sum(d.northbound_net for d in daily_list)
    retail_total = sum(d.retail_net for d in daily_list)
    hot_total = sum(d.hot_money_net for d in daily_list)

    forces = {
        "机构": inst_total,
        "北向": north_total,
        "散户": retail_total,
        "游资": hot_total,
    }
    dominant = max(forces, key=forces.get)
    if forces[dominant] <= 0:
        dominant = "无明确主导"

    return CapitalMetrics(
        symbol=symbol,
        as_of=as_of,
        window_days=window_days,
        lhb_count=len(records),
        lhb_records=records,
        daily=daily_list,
        total_net_buy=total_net,
        dominant_force=dominant,
    )
