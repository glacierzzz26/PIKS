"""Markdown 报告生成器"""
from datetime import date
from typing import List, Optional

from ..analysis.price import PriceMetrics
from ..analysis.volume import VolumeMetrics
from ..analysis.financial import FinancialMetrics
from ..analysis.events import EventMetrics
from ..analysis.risk import RiskMetrics, RiskItem
from ..analysis.capital import CapitalMetrics
from ..analysis.scorecard import Scorecard
from ..models.financial import FinancialSnapshot


def format_optional(value, fmt="{:.2f}", suffix="") -> str:
    if value is None:
        return "N/A"
    if isinstance(value, (int, float)):
        return fmt.format(value) + suffix
    return str(value)


def _fmt_pct(value) -> str:
    if value is None:
        return "N/A"
    return f"{value:+.2f}%"


def generate_markdown(
    symbol: str,
    as_of: date,
    price_metrics: PriceMetrics,
    volume_metrics: VolumeMetrics,
    fin_metrics: Optional[FinancialMetrics] = None,
    snapshots: Optional[List[FinancialSnapshot]] = None,
    event_metrics: Optional[EventMetrics] = None,
    risk_metrics: Optional[RiskMetrics] = None,
    capital_metrics: Optional[CapitalMetrics] = None,
    scorecard: Optional[Scorecard] = None,
    industry: Optional[Any] = None,
    data_source: str = "腾讯财经/akshare",
    sections: Optional[List[str]] = None,
) -> str:
    """生成 Markdown 报告（模板槽位渲染）

    sections: Profile 要求的报告章节（如 ["market","volume","events",...]），
              决定渲染哪些章节；默认全部（兼容旧调用）。
    """
    sections = sections or [
        "market", "volume", "turnover", "financial", "valuation",
        "events", "announcements", "capital", "risk", "conclusion",
    ]

    fin_section = _build_financial_section(fin_metrics, snapshots)
    event_section = _build_event_section(event_metrics)
    risk_section = _build_risk_section(risk_metrics)
    capital_section = _build_capital_section(capital_metrics)
    scorecard_section = _build_scorecard_section(scorecard)

    # 按 Profile sections 决定渲染哪些章节（中文序号自动编号）
    body: List[str] = []
    cn_nums = ["一", "二", "三", "四", "五", "六", "七", "八", "九", "十"]

    def add_section(title: str, content: str) -> None:
        idx = len(body)
        body.append(f"## {cn_nums[idx] if idx < len(cn_nums) else idx + 1}、{title}\n\n{content}")

    if any(s in ("market", "company") for s in sections):
        add_section("股价表现", f"""| 指标 | 数值 |
|---|---|
| 最新收盘价 | {price_metrics.end_price:.2f} 元 |
| 区间涨跌幅 | {price_metrics.period_return_pct:+.2f}% |
| 5 日涨跌幅 | {format_optional(price_metrics.return_pct_5d, '{:+.2f}', '%')} |
| 20 日涨跌幅 | {format_optional(price_metrics.return_pct_20d, '{:+.2f}', '%')} |
| 60 日涨跌幅 | {format_optional(price_metrics.return_pct_60d, '{:+.2f}', '%')} |
| 区间最高价 | {price_metrics.max_price:.2f} 元 |
| 区间最低价 | {price_metrics.min_price:.2f} 元 |
| 最大回撤 | {price_metrics.max_drawdown_pct:.2f}% |
| 年化波动率 | {price_metrics.volatility_annual:.2f}% |
| 涨停天数 | {price_metrics.limit_up_days} 天 |
| 跌停天数 | {price_metrics.limit_down_days} 天 |""")

    if any(s in ("volume", "turnover") for s in sections):
        add_section("成交量与换手率", f"""| 指标 | 数值 |
|---|---|
| 区间总成交量 | {volume_metrics.total_volume:,.0f} 手 |
| 5 日均量 | {format_optional(volume_metrics.avg_volume_5d, '{:,.0f}')} 手 |
| 20 日均量 | {format_optional(volume_metrics.avg_volume_20d, '{:,.0f}')} 手 |
| 5 日平均换手 | {format_optional(volume_metrics.avg_turnover_5d, '{:.2f}', '%')} |
| 20 日平均换手 | {format_optional(volume_metrics.avg_turnover_20d, '{:.2f}', '%')} |
| 60 日平均换手 | {format_optional(volume_metrics.avg_turnover_60d, '{:.2f}', '%')} |
| 最大单日换手 | {volume_metrics.max_turnover:.2f}% |
| 最小单日换手 | {volume_metrics.min_turnover:.2f}% |
| 量能异常天数 | {volume_metrics.abnormal_volume_days} 天 |""")

    if any(s in ("financial", "valuation") for s in sections):
        add_section("基本面分析", fin_section)

    if any(s in ("events", "announcements") for s in sections):
        add_section("近期事件与新闻", event_section)

    if "industry" in sections:
        add_section("行业对比", _build_industry_section(industry))

    if "capital" in sections:
        add_section("资金面分析（龙虎榜）", capital_section)

    if "risk" in sections:
        add_section("风险分析", risk_section)

    if "conclusion" in sections:
        add_section("综合评分卡", scorecard_section)

    add_section("数据说明与免责声明", f"""- 本报告数据来源于 {data_source}，仅供参考，不构成投资建议。
- 价格指标基于前复权计算。
- 报告生成时间：{as_of.isoformat()}。""")
    # AI 综合研判槽位（普通字符串，避免 f-string 转义）
    body.append("""

{{ai_synthesis}}
""")

    return f"""# 个股研究报告：{symbol}

> 报告生成时间：{as_of.isoformat()}
> 研究周期：近 {price_metrics.period_days} 个交易日
> 数据来源：{data_source}

---

{"\n\n".join(body)}
"""


def _build_financial_section(
    fin_metrics: Optional[FinancialMetrics],
    snapshots: Optional[List[FinancialSnapshot]],
) -> str:
    if fin_metrics is None:
        return "_财务数据暂不可得。_\n"

    lines = []
    lines.append("### 最近季度")
    lines.append("")
    lines.append("| 指标 | 数值 |")
    lines.append("|---|---|")
    lines.append(f"| 营收同比 | {_fmt_pct(fin_metrics.latest_revenue_yoy)} |")
    lines.append(f"| 净利润同比 | {_fmt_pct(fin_metrics.latest_net_profit_yoy)} |")
    lines.append(f"| ROE | {format_optional(fin_metrics.latest_roe, '{:.2f}', '%')} |")
    lines.append(f"| 毛利率 | {format_optional(fin_metrics.latest_gross_margin, '{:.2f}', '%')} |")
    lines.append(f"| 净利率 | {format_optional(fin_metrics.latest_net_margin, '{:.2f}', '%')} |")
    lines.append(f"| 资产负债率 | {format_optional(fin_metrics.latest_debt_ratio, '{:.2f}', '%')} |")
    lines.append("")

    if fin_metrics.revenue_yoy_trend:
        trend_str = " → ".join([f"{v:.1f}%" for v in fin_metrics.revenue_yoy_trend[:4]])
        lines.append(f"**营收同比趋势**: {trend_str}")
        lines.append("")

    if fin_metrics.net_profit_yoy_trend:
        trend_str = " → ".join([f"{v:.1f}%" for v in fin_metrics.net_profit_yoy_trend[:4]])
        lines.append(f"**净利润同比趋势**: {trend_str}")
        lines.append("")

    if fin_metrics.pe_ttm or fin_metrics.pb:
        lines.append("### 估值")
        lines.append("")
        lines.append(f"- PE(TTM): {format_optional(fin_metrics.pe_ttm, '{:.2f}')}")
        lines.append(f"- PB: {format_optional(fin_metrics.pb, '{:.2f}')}")
        lines.append("")

    if fin_metrics.revenue_yoy_change is not None:
        direction = "↑" if fin_metrics.revenue_yoy_change > 0 else "↓"
        lines.append(f"**营收同比变化**: {direction} {abs(fin_metrics.revenue_yoy_change):.2f} 个百分点")
        lines.append("")

    return "\n".join(lines)


def _build_industry_section(industry: Optional[Any]) -> str:
    """行业对比章节（Phase 1 降级：分类 + 同业财务对比）"""
    if industry is None:
        return "_行业数据暂不可得。_\n"
    if not industry.industry_code:
        return "_行业分类不可得（数据源受限），暂无同业对比。_\n"

    lines = []
    lines.append(f"**行业分类**: {industry.industry_name}（申万三级，共 {industry.peer_count} 只成分股）")
    lines.append("")

    peers = industry.peers
    if not peers:
        lines.append("_同业成分股暂不可得。_")
        return "\n".join(lines)

    # 只展示头部 8 只（按市值降序），全部有财务数据的才出表
    shown = [p for p in peers if p.roe is not None or p.pe_ttm is not None][:8]
    if not shown:
        # 财务全 N/A：只给名单
        names = "、".join(p.symbol for p in peers[:10])
        lines.append(f"同业成分（{len(peers)} 只）：{names}")
        return "\n".join(lines)

    lines.append("| 公司 | 市值(亿) | PE(TTM) | PB | ROE(%) | 净利增速(%) | 营收增速(%) | 股息率(%) |")
    lines.append("|---|---|---|---|---|---|---|---|")
    for p in shown:
        cap = f"{p.market_cap:.0f}" if p.market_cap else "N/A"
        pe = f"{p.pe_ttm:.2f}" if p.pe_ttm else "N/A"
        pb = f"{p.pb:.2f}" if p.pb else "N/A"
        roe = f"{p.roe:.2f}" if p.roe is not None else "N/A"
        npg = f"{p.net_profit_growth:.1f}" if p.net_profit_growth is not None else "N/A"
        rg = f"{p.revenue_growth:.1f}" if p.revenue_growth is not None else "N/A"
        dy = f"{p.dividend_yield:.2f}" if p.dividend_yield is not None else "N/A"
        lines.append(f"| {p.name[:10]} | {cap} | {pe} | {pb} | {roe} | {npg} | {rg} | {dy} |")
    lines.append("")
    lines.append("> Phase 1 降级：仅做同业横比，不做景气度判断。财务数据缺失标 N/A，不推测。")
    lines.append("")
    return "\n".join(lines)


def _build_event_section(event_metrics: Optional[EventMetrics]) -> str:
    if event_metrics is None:
        return "_近期事件暂不可得。_\n"

    lines = []

    # 优先展示降噪分级结果（§10 先降噪再研究）
    d = event_metrics.denoised
    if d:
        lines.append(
            f"最近 30 天原始事件 {d.total_raw} 条（新闻 + 公告），去重合并后 {d.total_events} 个独立事件"
            f"（合并同一事件重复稿件 {d.dedup_merged} 条）。"
        )
        lines.append("")

        # 分类分布
        dist = "、".join(f"{k} {v}" for k, v in sorted(d.category_distribution.items()))
        if dist:
            lines.append(f"分类分布：{dist}")
            lines.append("")

        # 「30 天无重大事件」是合法结论（禁止 padding）
        if d.no_major_event:
            lines.append("**近期无高/中级事件**：30 天内无财报、重组、增减持等重大事项，"
                         "亦无可归因于事件的显著价格变化。")
            lines.append("")
        else:
            lines.append("### 重大事件（高 / 中级）")
            lines.append("")
            for ev in d.major_events:
                sev = {"high": "高", "medium": "中"}.get(ev.severity.value, "-")
                title = ev.title if len(ev.title) <= 60 else ev.title[:57] + "…"
                merged = f"（合并 {len(ev.source_items)} 条来源）" if len(ev.source_items) > 1 else ""
                lines.append(f"- **[{sev}]** {title} {merged}")
                if ev.price_window:
                    pw = ev.price_window
                    ret = pw.get("window_return_pct")
                    if ret is not None:
                        complete = "" if pw.get("window_complete") else "（窗口未走完）"
                        lines.append(
                            f"  - 事件日 {pw['event_date']} 后 {pw['window_days']} 个交易日"
                            f"区间涨跌 **{ret:+.2f}%**{complete}"
                        )
            lines.append("")

        # 低级事件聚合为一行统计
        if d.low_events:
            low = "、".join(e.title[:20] for e in d.low_events[:5])
            more = " 等" if len(d.low_events) > 5 else ""
            lines.append(f"**例行事务**（{len(d.low_events)} 条）：{low}{more}")
            lines.append("")
        return "\n".join(lines)

    # 回退：无降噪时的旧逻辑
    lines.append(f"最近 30 天共 {event_metrics.total_news} 条新闻，其中财报相关 {event_metrics.earnings_mentions} 条。")
    lines.append("")

    if event_metrics.major_events:
        lines.append("### 重点事件")
        lines.append("")
        for i, news in enumerate(event_metrics.major_events[:3], 1):
            lines.append(f"{i}. **{news.title}** ({news.source}, {news.published_at.strftime('%m-%d')})")
        lines.append("")

    if event_metrics.recent_news:
        lines.append("### 最新动态")
        lines.append("")
        for i, news in enumerate(event_metrics.recent_news[:3], 1):
            lines.append(f"{i}. {news.title} ({news.source}, {news.published_at.strftime('%m-%d')})")
        lines.append("")

    return "\n".join(lines)


def _build_capital_section(capital_metrics: Optional[CapitalMetrics]) -> str:
    if capital_metrics is None:
        return "_龙虎榜数据暂不可得。_\n"

    lines = []

    if capital_metrics.lhb_count_30d == 0:
        lines.append("最近 5 天未登上龙虎榜（未触发异动条件：涨幅偏离±7%、换手率20%、连续三日累计±20%）。")
        lines.append("")
        return "\n".join(lines)

    lines.append(f"最近 5 天登上龙虎榜 **{capital_metrics.lhb_count_30d}** 次。")
    lines.append("")

    # 按天展开
    for day in capital_metrics.daily:
        lines.append(f"#### {day.trade_date.strftime('%m-%d')}  {day.close_price:.2f}元  {day.pct_change:+.2f}%  |  {day.reason}")
        lines.append("")

        # 当日资金方汇总
        lines.append("当日席位资金流向：")
        lines.append("")
        lines.append(f"- 机构: {day.institution_net/1e4:+.0f}万 | 北向: {day.northbound_net/1e4:+.0f}万 | 游资: {day.hot_money_net/1e4:+.0f}万 | 散户: {day.retail_net/1e4:+.0f}万 | **合计: {day.day_net_buy/1e4:+.0f}万**")
        lines.append("")

        # 当日净买入前五
        if day.top_buyers:
            lines.append("净买入前五：")
            lines.append("")
            lines.append("| 营业部 | 类型 | 净买入（万元） |")
            lines.append("|---|---|---|")
            for d in day.top_buyers:
                lines.append(f"| {d.name[:22]} | {d.dealer_type} | {d.net_amount/1e4:+.0f} |")
            lines.append("")

        # 当日净卖出前五
        if day.top_sellers:
            lines.append("净卖出前五：")
            lines.append("")
            lines.append("| 营业部 | 类型 | 净卖出（万元） |")
            lines.append("|---|---|---|")
            for d in day.top_sellers:
                lines.append(f"| {d.name[:22]} | {d.dealer_type} | {d.net_amount/1e4:+.0f} |")
            lines.append("")

    # 全时段汇总
    lines.append("### 全时段汇总")
    lines.append("")
    lines.append(f"**主导力量**: {capital_metrics.dominant_force} | **合计净额**: {capital_metrics.total_net_buy/1e4:+.0f} 万元")
    lines.append("")

    return "\n".join(lines)


def _build_risk_section(risk_metrics: Optional[RiskMetrics]) -> str:
    if risk_metrics is None:
        return "_风险分析暂不可得。_\n"

    lines = []

    level_emoji = {"low": "🟢", "medium": "🟡", "high": "🔴"}
    emoji = level_emoji.get(risk_metrics.overall_level, "⚪")
    lines.append(f"**综合风险等级**: {emoji} {risk_metrics.overall_level.upper()}")
    lines.append("")

    if risk_metrics.veto_buy:
        lines.append("⚠️ **一票否决**: 当前风险水平不建议入场。")
        lines.append("")

    if risk_metrics.items:
        lines.append("| 风险类别 | 等级 | 依据 | 说明 |")
        lines.append("|---|---|---|---|")
        for item in risk_metrics.items:
            lines.append(f"| {item.category} | {item.level.upper()} | {item.evidence} | {item.description} |")
        lines.append("")
    else:
        lines.append("未发现显著风险信号。")
        lines.append("")

    return "\n".join(lines)


def _build_scorecard_section(scorecard: Optional[Scorecard]) -> str:
    if scorecard is None:
        return "_评分卡暂不可得。_\n"

    lines = []
    lines.append(f"**Overall: {scorecard.overall_label}** （总分 {scorecard.overall:+d}）")
    lines.append("")
    lines.append("| 维度 | 评分 | 理由 |")
    lines.append("|---|---|---|")

    dim_labels = {
        "business_quality": "业务质量",
        "fundamental_trend": "基本面趋势",
        "market_trend": "市场趋势",
        "valuation": "估值",
        "recent_events": "近期事件",
        "risk": "风险",
    }

    for d in scorecard.dimensions:
        label = dim_labels.get(d.dimension, d.dimension)
        score_str = "N/A" if d.unavailable else f"{d.score:+d}"
        lines.append(f"| {label} | {score_str} | {d.reason} |")

    lines.append("")
    lines.append("> 评分说明：-2（极弱）~ +2（极强），0 为中性。N/A 表示数据暂不可得。")
    lines.append("")

    return "\n".join(lines)
