"""风险分析引擎"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional

from ..analysis.price import PriceMetrics
from ..analysis.volume import VolumeMetrics
from ..analysis.financial import FinancialMetrics
from ..models.announcement import Announcement


@dataclass
class RiskItem:
    """单项风险"""
    category: str           # Business / Financial / Valuation / Policy / Management / Event / Liquidity / Market
    level: str              # low / medium / high
    evidence: str
    description: str


@dataclass
class RiskMetrics:
    """风险指标卡"""
    symbol: str
    as_of: date
    overall_level: str      # low / medium / high
    items: List[RiskItem]
    veto_buy: bool = False


def analyze_risk(
    symbol: str,
    as_of: date,
    price: Optional[PriceMetrics] = None,
    volume: Optional[VolumeMetrics] = None,
    financial: Optional[FinancialMetrics] = None,
    announcements: Optional[List[Announcement]] = None,
) -> RiskMetrics:
    """
    基于规则的风险分析。
    不做预测，只基于已有数据标记已知风险。

    量价可选(P9-4,issue #11):公司研报不采行情,此时 price/volume 为 None,
    规则 1-3(波动率/回撤/流动性)整条跳过 —— 不猜、不补 —— 而规则 4-7
    (负债率/净利同比/重大公告/估值缺失)纯基本面,照常独立运行。
    `symbol`/`as_of` 由调用方显式传入(原先从 price 借,无量价时无法借)。
    """
    items = []

    # 1. 市场风险：高波动
    if price is not None:
        if price.volatility_annual > 40:
            items.append(RiskItem(
                category="Market",
                level="high",
                evidence=f"年化波动率 {price.volatility_annual:.1f}%",
                description="股价波动剧烈，短期风险较高",
            ))
        elif price.volatility_annual > 25:
            items.append(RiskItem(
                category="Market",
                level="medium",
                evidence=f"年化波动率 {price.volatility_annual:.1f}%",
                description="股价波动中等，注意仓位控制",
            ))

        # 2. 市场风险：大幅回撤
        if price.period_return_pct < -15:
            items.append(RiskItem(
                category="Market",
                level="high",
                evidence=f"区间跌幅 {price.period_return_pct:.1f}%",
                description="近期股价大幅下跌，趋势偏弱",
            ))

    # 3. 流动性风险：低换手 + 低成交额
    # 低换手不等于流动性差（大盘股常态），需结合成交额判断
    if volume is not None:
        avg_turnover = volume.avg_turnover_20d or volume.avg_turnover_5d
        avg_amount = volume.avg_amount_20d or volume.avg_amount_5d
        if avg_turnover is not None and avg_turnover < 0.15:
            if avg_amount is None or avg_amount < 1e8:  # 日均成交 < 1亿元
                items.append(RiskItem(
                    category="Liquidity",
                    level="medium",
                    evidence=f"20日平均换手率 {avg_turnover:.2f}%，日均成交额 {avg_amount/1e8:.2f}亿" if avg_amount else f"20日平均换手率 {avg_turnover:.2f}%",
                    description="交易活跃度较低，大额进出可能影响价格",
                ))

    # 4. 财务风险：高负债
    if financial and financial.latest_debt_ratio is not None:
        if financial.latest_debt_ratio > 70:
            items.append(RiskItem(
                category="Financial",
                level="high",
                evidence=f"资产负债率 {financial.latest_debt_ratio:.1f}%",
                description="杠杆率偏高，财务风险较大",
            ))
        elif financial.latest_debt_ratio > 50:
            items.append(RiskItem(
                category="Financial",
                level="medium",
                evidence=f"资产负债率 {financial.latest_debt_ratio:.1f}%",
                description="杠杆率中等偏高",
            ))

    # 5. 财务风险：业绩下滑
    if financial and financial.latest_net_profit_yoy is not None:
        if financial.latest_net_profit_yoy < -20:
            items.append(RiskItem(
                category="Business",
                level="high",
                evidence=f"净利润同比 {financial.latest_net_profit_yoy:.1f}%",
                description="业绩大幅下滑，需关注原因",
            ))
        elif financial.latest_net_profit_yoy < -5:
            items.append(RiskItem(
                category="Business",
                level="medium",
                evidence=f"净利润同比 {financial.latest_net_profit_yoy:.1f}%",
                description="业绩小幅下滑",
            ))

    # 6. 事件风险：重大公告
    if announcements:
        major = [a for a in announcements if a.is_major]
        if major:
            items.append(RiskItem(
                category="Event",
                level="medium",
                evidence=f"最近30天 {len(major)} 条重大公告",
                description="近期有重要公告，关注事件影响",
            ))

    # 7. 估值风险：数据缺失
    if financial and financial.pe_ttm is None:
        items.append(RiskItem(
            category="Valuation",
            level="low",
            evidence="PE/PB 数据缺失",
            description="估值指标暂不可得，无法判断估值水平",
        ))

    # 综合评级
    high_count = sum(1 for r in items if r.level == "high")
    medium_count = sum(1 for r in items if r.level == "medium")

    if high_count >= 2:
        overall = "high"
        veto = True
    elif high_count >= 1 or medium_count >= 2:
        overall = "medium"
        veto = False
    else:
        overall = "low"
        veto = False

    return RiskMetrics(
        symbol=symbol,
        as_of=as_of,
        overall_level=overall,
        items=items,
        veto_buy=veto,
    )
