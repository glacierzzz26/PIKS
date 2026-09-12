"""基于 akshare 的财务数据 Provider"""
import akshare as ak
import pandas as pd
from datetime import date
from typing import List, Optional

from ...models import Symbol
from ...models.financial import FinancialSnapshot, ValuationMetrics
from ..base import FinancialProvider


class AkShareFinancialProvider(FinancialProvider):
    """基于 akshare 的财务/估值数据 Provider"""

    @property
    def name(self) -> str:
        return "akshare_financial"

    def get_financials(self, symbol: Symbol, quarters: int = 4) -> List[FinancialSnapshot]:
        """
        获取最近 N 个季度的财务指标。
        使用 stock_financial_analysis_indicator 接口。
        """
        df = ak.stock_financial_analysis_indicator(symbol=symbol.code)
        if df is None or df.empty:
            return []

        df = df.sort_values("日期", ascending=False)

        # 只取财报日期（季度末/年末）
        df["date"] = pd.to_datetime(df["日期"])
        # 取最近 quarters 条
        df = df.head(quarters * 2)  # 多取一些，过滤非季度数据

        snapshots = []
        for _, row in df.iterrows():
            d = row["date"]
            if pd.isna(d):
                continue

            # 判断报表类型
            month_day = d.month * 100 + d.day
            if month_day == 331:
                rtype = "Q1"
            elif month_day == 630:
                rtype = "Q2"
            elif month_day == 930:
                rtype = "Q3"
            elif month_day == 1231:
                rtype = "annual"
            else:
                continue  # 跳过非财报日期

            snap = FinancialSnapshot(
                symbol=symbol.full_code,
                report_date=d.date(),
                report_type=rtype,
                revenue_yoy=_to_float(row.get("主营业务收入增长率(%)")),
                net_profit_yoy=_to_float(row.get("净利润增长率(%)")),
                gross_margin=_to_float(row.get("销售毛利率(%)")),
                net_margin=_to_float(row.get("销售净利率(%)")),
                roe=_to_float(row.get("净资产收益率(%)")),
                debt_ratio=_to_float(row.get("资产负债率(%)")),
                operating_cashflow_per_share=_to_float(row.get("每股经营性现金流(元)")),
                total_assets=_to_float(row.get("总资产(元)")),
            )
            snapshots.append(snap)

        # 去重按 report_date，取最近的 quarters 个
        seen = set()
        unique = []
        for s in snapshots:
            if s.report_date not in seen:
                seen.add(s.report_date)
                unique.append(s)
        return unique[:quarters]

    def get_valuation(self, symbol: Symbol) -> Optional[ValuationMetrics]:
        """
        获取当前估值指标。
        由于东财/新浪接口在当前环境常被限制，采用多接口 fallback。
        全部失败时返回 None，由报告标记 unavailable。
        """
        # fallback 1: 个股信息接口（含 PE/PB/市值）
        try:
            df = ak.stock_individual_info_em(symbol=symbol.code)
            if df is not None and not df.empty:
                info = {row["item"]: row["value"] for _, row in df.iterrows()}
                return ValuationMetrics(
                    symbol=symbol.full_code,
                    as_of=date.today(),
                    pe_ttm=_to_float(info.get("市盈率-动态")),
                    pb=_to_float(info.get("市净率")),
                    market_cap=_to_float(info.get("总市值")),
                    float_cap=_to_float(info.get("流通市值")),
                )
        except Exception:
            pass

        # fallback 2: 实时行情全市场快照
        try:
            df = ak.stock_zh_a_spot_em()
            row = df[df["代码"] == symbol.code]
            if not row.empty:
                r = row.iloc[0]
                return ValuationMetrics(
                    symbol=symbol.full_code,
                    as_of=date.today(),
                    pe_ttm=_to_float(r.get("市盈率-动态")),
                    pb=_to_float(r.get("市净率")),
                    market_cap=_to_float(r.get("总市值")),
                    float_cap=_to_float(r.get("流通市值")),
                )
        except Exception:
            pass

        # 全部失败，降级返回 None
        return None


def _to_float(val) -> Optional[float]:
    if val is None or pd.isna(val):
        return None
    try:
        return float(val)
    except (ValueError, TypeError):
        return None
