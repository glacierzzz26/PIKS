"""JSON 结构化报告生成器"""
from dataclasses import asdict
from datetime import date
from typing import Dict, Any, List, Optional

from ..analysis.price import PriceMetrics
from ..analysis.volume import VolumeMetrics
from ..analysis.financial import FinancialMetrics
from ..analysis.events import EventMetrics
from ..analysis.risk import RiskMetrics
from ..analysis.capital import CapitalMetrics
from ..analysis.scorecard import Scorecard
from ..analysis.industry import IndustryMetrics
from ..models.financial import FinancialSnapshot
from .sections import DEFAULT_SECTIONS, build_manifest


def generate_json(
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
    industry_metrics: Optional[IndustryMetrics] = None,
    sections: Optional[List[str]] = None,
) -> Dict[str, Any]:
    """生成 JSON 结构化报告

    industry_metrics: 行业**本体**指标卡(P9 #12,主体=申万行业指数)。
      与 industry(个股所属行业的同业对比)是两回事,JSON 键分别为
      `industry_index` / `industry`。
    sections: Profile 章节清单 —— 用于产出 `meta.section_manifest`(前端 TOC +
      三域标记的数据源,D-R8)。缺省则用 markdown.py 的历史默认(兼容旧调用)。
    """
    result = {
        "meta": {
            "symbol": symbol,
            "as_of": as_of.isoformat(),
            "data_source": "akshare/tencent",
            # 章节清单(D-R8,零 schema):前端据此渲染目录与三域标签,**不解析
            # markdown 标题**(裸 react-markdown 无锚点、不可靠)。与 markdown.py
            # 的正文装配同源(都出自 sections.py),故不会错位。新增字段不升 contract。
            "section_manifest": build_manifest(sections or list(DEFAULT_SECTIONS)),
        },
    }

    # 行业主体(industry profile)无 market/volume 章节 → 两者为 None。
    # 省键而非写 null:指标卡是 Number Lint 的对账基准,缺席应表现为「没有」,
    # 而不是一个会被遍历到的空值。(个股路径两者恒非空,输出逐字节不变。)
    if price_metrics is not None:
        result["price"] = asdict(price_metrics)
    if volume_metrics is not None:
        result["volume"] = asdict(volume_metrics)

    if fin_metrics:
        result["financial"] = asdict(fin_metrics)
        # 趋势与变化数值（Number Lint 对账基准）
        for key, val in (
            ("revenue_yoy_trend", fin_metrics.revenue_yoy_trend),
            ("net_profit_yoy_trend", fin_metrics.net_profit_yoy_trend),
            ("revenue_yoy_change", fin_metrics.revenue_yoy_change),
            ("profit_yoy_change", fin_metrics.profit_yoy_change),
            ("roe_change", fin_metrics.roe_change),
        ):
            if val:
                result["financial"][key] = val

    if snapshots:
        result["financial_snapshots"] = [
            {
                "report_date": s.report_date.isoformat(),
                "report_type": s.report_type,
                "revenue_yoy": s.revenue_yoy,
                "net_profit_yoy": s.net_profit_yoy,
                "roe": s.roe,
                "gross_margin": s.gross_margin,
                "net_margin": s.net_margin,
                "debt_ratio": s.debt_ratio,
            }
            for s in snapshots
        ]

    if event_metrics:
        result["events"] = event_metrics.to_dict()
        # 保持向后兼容的顶层字段
        result["events"]["total_news"] = event_metrics.total_news
        result["events"]["earnings_mentions"] = event_metrics.earnings_mentions

    if risk_metrics:
        result["risk"] = asdict(risk_metrics)

    if industry:
        result["industry"] = industry.to_dict()

    if industry_metrics:
        ref, price = industry_metrics.ref, industry_metrics.price
        rank, disp = industry_metrics.valuation_rank, industry_metrics.dispersion
        result["industry_index"] = {
            "ref": asdict(ref),
            "price": asdict(price),
            "valuation_rank": asdict(rank),
            "dispersion": asdict(disp),
            "declared_count": industry_metrics.declared_count,
            "constituent_count": industry_metrics.constituent_count,
        }
        # 数值**拍平到顶层**。原因:合成提示的指标摘要是**单层**过滤
        # (`synthesis.py` 只收 isinstance(v, (int,float,str))),嵌套一层的
        # price/valuation_rank 会被整体丢弃 → LLM 看不到任何数字。
        # 个股各 section 的关键数字本就平铺在 section 顶层,此处对齐同一形状。
        # 只加不改:上面嵌套块原样保留(指标卡契约不变),顶部仅为平铺视图。
        result["industry_index"].update({
            "name": ref.name,
            "level_label": ref.level_label,
            "pe_ttm": ref.pe_ttm,
            "pe_static": ref.pe_static,
            "pb": ref.pb,
            "dividend_yield": ref.dividend_yield,
            "end_point": price.end_point,
            "period_return_pct": price.period_return_pct,
            "return_pct_5d": price.return_pct_5d,
            "return_pct_20d": price.return_pct_20d,
            "return_pct_60d": price.return_pct_60d,
            "max_drawdown_pct": price.max_drawdown_pct,
            "volatility_annual": price.volatility_annual,
            "point_percentile": price.point_percentile,
            "history_days": price.history_days,
            "valuation_rank_num": rank.rank,
            "valuation_universe": rank.universe,
            "roe_median": disp.roe_median,
            "net_profit_growth_median": disp.net_profit_growth_median,
            "revenue_growth_median": disp.revenue_growth_median,
            "cap_sum": disp.cap_sum,
        })

    if capital_metrics:
        result["capital"] = asdict(capital_metrics)

    if scorecard:
        result["scorecard"] = {
            "overall": scorecard.overall,
            "overall_label": scorecard.overall_label,
            "dimensions": [
                {
                    "dimension": d.dimension,
                    "score": None if d.unavailable else d.score,
                    "reason": d.reason,
                    "unavailable": d.unavailable,
                }
                for d in scorecard.dimensions
            ],
        }

    return result
