"""test_scorecard.py — 评分卡（档案维度自适应）单元测试

回归短线档案缺陷：短线的 sections 不含 financial/valuation/risk，
评分卡却仍无条件评六维、把缺失维度标 N/A 并降级为「结论受限」——短线永远没有结论。
现按 profile.scorecard.dimensions 只评声明维度，且「不在档案范围」不触发降级。
"""
import os
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.price import PriceMetrics
from src.analysis.financial import FinancialMetrics
from src.analysis.events import EventMetrics
from src.analysis.risk import RiskMetrics
from src.analysis.scorecard import analyze_scorecard, DEFAULT_DIMENSIONS

AS_OF = date(2026, 9, 13)
SHORT_TERM_DIMS = ["market_trend", "recent_events"]


def _price(period_return=0.0, vol=20.0):
    return PriceMetrics(
        symbol="sz000001", as_of=AS_OF, period_days=20,
        start_price=10.0, end_price=10.0, max_price=11.0, min_price=9.0,
        return_pct_5d=0.0, return_pct_10d=0.0, return_pct_20d=period_return,
        return_pct_60d=None, period_return_pct=period_return,
        max_drawdown_pct=5.0, max_rise_pct=3.0, volatility_annual=vol,
    )


def _events(total=0, major=0, earnings=0):
    return EventMetrics(
        symbol="sz000001", as_of=AS_OF, total_news=total,
        recent_news=[], earnings_mentions=earnings,
        major_events=[object()] * major,
    )


def _financial(roe=20.0, net_margin=10.0, debt=50.0, rev=0.0, profit=0.0, pe=30.0, pb=5.0):
    return FinancialMetrics(
        symbol="sz000001", as_of=AS_OF,
        latest_revenue_yoy=rev, latest_net_profit_yoy=profit,
        latest_roe=roe, latest_gross_margin=30.0, latest_net_margin=net_margin,
        latest_debt_ratio=debt,
        revenue_yoy_trend=[rev], net_profit_yoy_trend=[profit], roe_trend=[roe],
        revenue_yoy_change=0.0, profit_yoy_change=0.0, roe_change=0.0,
        pe_ttm=pe, pb=pb,
    )


def _risk(level="low"):
    return RiskMetrics(symbol="sz000001", as_of=AS_OF, overall_level=level, items=[], veto_buy=False)


class TestProfileDimensions(unittest.TestCase):
    def test_default_is_six_dims(self):
        sc = analyze_scorecard(_price(), None, None, _events(), None, AS_OF)
        self.assertEqual([d.dimension for d in sc.dimensions], DEFAULT_DIMENSIONS)

    def test_profile_subset_omits_out_of_scope(self):
        sc = analyze_scorecard(_price(period_return=26.0), None, None, _events(), None, AS_OF,
                               dimensions=SHORT_TERM_DIMS)
        self.assertEqual([d.dimension for d in sc.dimensions], SHORT_TERM_DIMS)
        # 财务/估值/风险不在档案范围 → 不再产出 N/A 占位行
        self.assertFalse(any(d.unavailable for d in sc.dimensions))

    def test_out_of_scope_missing_data_does_not_degrade(self):
        # 无 financial / risk 数据,但二者不在短线档案范围 → 不得标「结论受限」
        sc = analyze_scorecard(_price(period_return=26.0), None, None, _events(), None, AS_OF,
                               dimensions=SHORT_TERM_DIMS)
        self.assertNotEqual(sc.overall_label, "结论受限")

    def test_sensitive_dim_in_scope_still_degrades(self):
        # risk 在档案范围内但数据缺失 → 仍降级(「在本范围却缺」≠「不在本范围」)
        sc = analyze_scorecard(_price(), None, None, _events(), None, AS_OF,
                               dimensions=["market_trend", "risk"])
        self.assertEqual(sc.overall_label, "结论受限")

    def test_default_six_degrades_when_financial_missing(self):
        sc = analyze_scorecard(_price(), None, None, _events(), None, AS_OF)
        self.assertEqual(sc.overall_label, "结论受限")

    def test_empty_dimensions_falls_back_to_default(self):
        sc = analyze_scorecard(_price(), None, None, _events(), None, AS_OF, dimensions=[])
        self.assertEqual([d.dimension for d in sc.dimensions], DEFAULT_DIMENSIONS)


class TestDirectionScaling(unittest.TestCase):
    """方向按「平均维度分」判定：≥ +2/3 偏正面、≤ -2/3 偏负面。"""

    def _sc(self, period_return, major=0):
        return analyze_scorecard(_price(period_return=period_return), None, None,
                                 _events(total=3, major=major), None, AS_OF,
                                 dimensions=SHORT_TERM_DIMS)

    def test_strong_positive(self):
        sc = self._sc(26.0)          # market +2, events 0 → avg 1.0
        self.assertEqual(sc.overall, 2)
        self.assertEqual(sc.overall_label, "偏正面")

    def test_mild_negative_is_neutral(self):
        sc = self._sc(-10.0)         # market -1, events 0 → avg -0.5(未过 2/3 阈值)
        self.assertEqual(sc.overall, -1)
        self.assertEqual(sc.overall_label, "中性")

    def test_strong_negative(self):
        sc = self._sc(-20.0)         # market -2 → avg -1.0
        self.assertEqual(sc.overall_label, "偏负面")

    def test_flat_is_neutral(self):
        self.assertEqual(self._sc(0.0).overall_label, "中性")


class TestSixDimBackwardCompat(unittest.TestCase):
    """六维全可用时,avg ≥ 2/3 等价于原「总分 ≥ +4」阈值。"""

    def test_sum_plus4_is_positive(self):
        # market +2, events +1, bq +1, ft 0, val 0, risk 0 = +4 → 偏正面
        sc = analyze_scorecard(
            _price(period_return=26.0), None,
            _financial(roe=20.0, net_margin=10.0, debt=50.0, rev=0.0, profit=0.0, pe=30.0, pb=5.0),
            _events(total=3, major=3), _risk("low"), AS_OF,
        )
        self.assertEqual(sc.overall, 4)
        self.assertEqual(sc.overall_label, "偏正面")

    def test_sum_plus3_is_neutral(self):
        # 同上但 bq 归零 = +3 → 中性(严格复现原 ±4 阈值)
        sc = analyze_scorecard(
            _price(period_return=26.0), None,
            _financial(roe=10.0, net_margin=10.0, debt=50.0, rev=0.0, profit=0.0, pe=30.0, pb=5.0),
            _events(total=3, major=3), _risk("low"), AS_OF,
        )
        self.assertEqual(sc.overall, 3)
        self.assertEqual(sc.overall_label, "中性")


if __name__ == "__main__":
    unittest.main()
