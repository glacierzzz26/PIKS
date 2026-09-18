"""分析引擎总控

整合所有分析模块，统一输出带 Evidence 的指标卡。
"""
from dataclasses import dataclass
from datetime import date
from typing import List, Optional

from ..models import Bar
from ..models.financial import FinancialSnapshot, ValuationMetrics
from ..models.news import NewsItem
from ..models.announcement import Announcement
from ..evidence import EvidenceStore, evidence_from_metric
from .price import PriceMetrics
from .volume import VolumeMetrics
from .financial import FinancialMetrics
from .events import EventMetrics
from .risk import RiskMetrics, analyze_risk
from .price import analyze_price
from .volume import analyze_volume
from .financial import analyze_financial
from .events import analyze_events


@dataclass
class AnalysisResult:
    """分析结果总包"""
    price: PriceMetrics
    volume: VolumeMetrics
    financial: Optional[FinancialMetrics]
    events: Optional[EventMetrics]
    risk: RiskMetrics
    evidence_store: EvidenceStore


def run_analysis(
    bars: List[Bar],
    snapshots: Optional[List[FinancialSnapshot]] = None,
    valuation: Optional[ValuationMetrics] = None,
    news: Optional[List[NewsItem]] = None,
    announcements: Optional[List[Announcement]] = None,
    as_of: Optional[date] = None,
) -> AnalysisResult:
    """
    运行全部分析，同时生成 Evidence。
    """
    if as_of is None:
        as_of = date.today()

    store = EvidenceStore()

    # 价格分析
    price = analyze_price(bars, as_of)
    store.add(evidence_from_metric(
        "period_return_pct", price.period_return_pct,
        "analysis_engine", f"{price.period_days}d",
        {"start_price": price.start_price, "end_price": price.end_price},
        section="price",
    ))
    store.add(evidence_from_metric(
        "max_drawdown_pct", price.max_drawdown_pct,
        "analysis_engine", f"{price.period_days}d",
        {"max_price": price.max_price, "min_price": price.min_price},
        section="price",
    ))

    # 成交量分析
    volume = analyze_volume(bars, as_of)
    store.add(evidence_from_metric(
        "avg_turnover_20d", volume.avg_turnover_20d,
        "analysis_engine", f"{volume.period_days}d",
        {"turnovers": [b.turnover for b in bars[-20:]]},
        section="volume",
    ))

    # 财务分析
    financial = None
    if snapshots:
        financial = analyze_financial(snapshots, valuation, as_of)
        if financial.latest_roe is not None:
            store.add(evidence_from_metric(
                "roe", financial.latest_roe,
                "akshare_financial", financial.latest_roe,
                {"report_date": snapshots[0].report_date.isoformat() if snapshots else None},
                section="financial",
            ))

    # 事件分析
    events = None
    if news:
        events = analyze_events(news, as_of)
        store.add(evidence_from_metric(
            "news_count", events.total_news,
            "akshare_news", "30d",
            {"earnings_mentions": events.earnings_mentions},
            section="events",
        ))

    # 风险分析 —— 无论是否触发 veto 都记录 Evidence（无风险也是结论）
    risk = analyze_risk(price.symbol, as_of, price, volume, financial, announcements)
    store.add(evidence_from_metric(
        "risk_level", risk.overall_level,
        "risk_engine", "current",
        {"risk_count": len(risk.items), "veto_buy": risk.veto_buy},
        section="risk",
    ))
    if risk.veto_buy:
        store.add(evidence_from_metric(
            "risk_veto", True,
            "risk_engine", "current",
            {"overall_level": risk.overall_level, "risk_count": len(risk.items)},
            section="risk",
        ))

    return AnalysisResult(
        price=price,
        volume=volume,
        financial=financial,
        events=events,
        risk=risk,
        evidence_store=store,
    )


def add_industry_evidence(
    store: EvidenceStore,
    industry_metrics,
    industry_risk,
) -> None:
    """行业主体(P9 #12)的 Evidence。

    与 `run_analysis` 并列而非并入:行业指数的 bars 不是个股 bars —— 那个函数
    会顺手跑 analyze_price/analyze_volume,对指数产出「换手率」等无口径字段。
    此处只登记行业报告真正上屏的那几个数字。
    """
    if industry_metrics is not None:
        p = industry_metrics.price
        store.add(evidence_from_metric(
            "period_return_pct", p.period_return_pct,
            "sw_index", f"{p.period_days}d",
            {"start_point": p.start_point, "end_point": p.end_point},
            section="industry_index",
        ))
        store.add(evidence_from_metric(
            "point_percentile", p.point_percentile,
            "sw_index", f"{p.history_days}d 全历史",
            {"history_start": p.history_start.isoformat() if p.history_start else None},
            section="industry_index",
        ))
        rank = industry_metrics.valuation_rank
        store.add(evidence_from_metric(
            "valuation_rank", rank.rank,
            "sw_index", "当期快照",
            {"universe": rank.universe, "level_label": rank.level_label,
             "pe_ttm": rank.value},
            section="industry_valuation",
        ))
        d = industry_metrics.dispersion
        store.add(evidence_from_metric(
            "roe_median", d.roe_median,
            "sw_index", "当期快照",
            {"count": d.count, "roe_q1": d.roe_q1, "roe_q3": d.roe_q3},
            section="industry_structure",
        ))
    if industry_risk is not None:
        store.add(evidence_from_metric(
            "risk_level", industry_risk.overall_level,
            "industry_risk_engine", "current",
            {"risk_count": len(industry_risk.items)},
            section="risk",
        ))


def add_macro_evidence(
    store: EvidenceStore,
    macro_metrics,
    macro_risk,
) -> None:
    """宏观主体(P9-5 / #13)的 Evidence。

    与 `run_analysis` 并列,理由同 `add_industry_evidence`:宏观序列不是个股 bars
    —— 那个函数会顺手跑 analyze_price/analyze_volume,对宏观序列产出「换手率」
    「年化波动率」等**无口径依据**的字段。此处只登记宏观报告真正上屏的数字。

    登记范围与 `markdown.py` 的三个 builder 一一对应:
      - `_build_macro_level_section`(最新读数 + 序列)→ section="macro_level"
      - `_build_macro_position_section`(分位/窗口/趋势)→ section="macro_position"
      - `_build_macro_risk_section`(等级 + 逐条依据)→ section="risk"
    """
    if macro_metrics is not None:
        m = macro_metrics
        p = m.period
        hist_span = f"{p.history_periods} 期" if p else "全历史"
        store.add(evidence_from_metric(
            "latest_level", m.latest_level,
            "akshare_macro", m.ref.name, {"level_label": m.ref.level_label},
            section="macro_level",
        ))
        store.add(evidence_from_metric(
            "latest_yoy", m.latest_yoy,
            "akshare_macro", m.ref.name, {"unit": "%"},
            section="macro_level",
        ))
        if m.latest_mom is not None:
            store.add(evidence_from_metric(
                "latest_mom", m.latest_mom,
                "akshare_macro", m.ref.name, {"unit": "%"},
                section="macro_level",
            ))
        if m.latest_single_quarter_level is not None:
            store.add(evidence_from_metric(
                "latest_single_quarter_level", m.latest_single_quarter_level,
                "akshare_macro", "同年内累计差分所得",
                {"cumulative_disclosure": "非原始披露值"},
                section="macro_level",
            ))

        store.add(evidence_from_metric(
            "percentile_level", m.percentile_level,
            "macro_analysis", hist_span,
            {"history_start": p.history_start.isoformat() if p and p.history_start else None,
             "meaningful": m.ref.level_percentile_meaningful},
            section="macro_position",
        ))
        store.add(evidence_from_metric(
            "percentile_yoy", m.percentile_yoy,
            "macro_analysis", hist_span,
            {"history_periods": p.history_periods if p else None},
            section="macro_position",
        ))
        store.add(evidence_from_metric(
            "window_median_level", m.window_median_level,
            "macro_analysis", f"近 {p.window_periods} 期" if p else "展示窗口",
            {"window_min": m.window_min_level, "window_max": m.window_max_level},
            section="macro_position",
        ))
        store.add(evidence_from_metric(
            "direction_run", m.direction_run,
            "macro_analysis", "同比连续同向期数",
            {"from": m.direction_from_label},
            section="macro_position",
        ))

    if macro_risk is not None:
        store.add(evidence_from_metric(
            "risk_level", macro_risk.overall_level,
            "macro_risk_engine", "current",
            {"risk_count": len(macro_risk.items)},
            section="risk",
        ))


def add_fundamental_evidence(
    store: EvidenceStore,
    financial: Optional[FinancialMetrics],
    risk: Optional[RiskMetrics],
    events: Optional[EventMetrics] = None,
) -> None:
    """纯基本面主体(P9-4,issue #11)的 Evidence。

    与 `run_analysis` 并列,理由同 `add_industry_evidence`:公司研报不采行情,
    `_refresh_evidence` 被 `if not bars: return` 挡住 ⇒ Evidence 全空 ⇒ 机检
    `evidence_completeness` 必失败。此处只登记基本面报告真正上屏的那几个数字。

    登记范围与 `markdown.py` 两个 builder 一一对应:
      - `_build_financial_section`(最近季度表 + 估值)→ section="financial"
      - `_build_risk_section`(等级 + 逐条依据)→ section="risk"
    """
    if financial is not None:
        # 与 _build_financial_section 的「最近季度」表逐行对齐
        for name, value, period in (
            ("revenue_yoy", financial.latest_revenue_yoy, "最新季报"),
            ("net_profit_yoy", financial.latest_net_profit_yoy, "最新季报"),
            ("roe", financial.latest_roe, "最新季报"),
            ("gross_margin", financial.latest_gross_margin, "最新季报"),
            ("net_margin", financial.latest_net_margin, "最新季报"),
            ("debt_ratio", financial.latest_debt_ratio, "最新季报"),
            ("pe_ttm", financial.pe_ttm, "当期快照"),
            ("pb", financial.pb, "当期快照"),
        ):
            if value is not None:
                store.add(evidence_from_metric(
                    name, value, "akshare_financial", period,
                    section="financial",
                ))

    if risk is not None:
        store.add(evidence_from_metric(
            "risk_level", risk.overall_level,
            "risk_engine", "current",
            {"risk_count": len(risk.items), "veto_buy": risk.veto_buy},
            section="risk",
        ))
        if risk.veto_buy:
            store.add(evidence_from_metric(
                "risk_veto", True,
                "risk_engine", "current",
                {"overall_level": risk.overall_level, "risk_count": len(risk.items)},
                section="risk",
            ))

    if events is not None:
        store.add(evidence_from_metric(
            "news_count", events.total_news,
            "akshare_news", "30d",
            {"earnings_mentions": events.earnings_mentions},
            section="events",
        ))
