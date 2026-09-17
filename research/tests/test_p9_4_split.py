"""test_p9_4_split.py — 公司研报 / 个股分析功能拆分（issue #11, P9-4）回归

覆盖三处地基改动：
  1. 风险引擎量价可选化（§4.2）—— 无量价时规则 1-3 跳过、规则 4-7 照跑
  2. 章节清单去 `company` 触发（§4.3）—— 公司研报不再凭空多出「股价表现」
  3. 基本面 Evidence 登记（§4.2.1）—— 无 bars 主体的机检不再必失败
"""
import os
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.price import PriceMetrics
from src.analysis.volume import VolumeMetrics
from src.analysis.financial import FinancialMetrics
from src.analysis.risk import analyze_risk
from src.report.sections import build_manifest
from src.workflow.profile import load_profile
from src.workflow.plan import PlanGenerator

AS_OF = date(2026, 9, 17)


def _price(vol=20.0, ret=0.0):
    return PriceMetrics(
        symbol="sh600519", as_of=AS_OF, period_days=20,
        start_price=100.0, end_price=100.0, max_price=105.0, min_price=95.0,
        return_pct_5d=0.0, return_pct_10d=0.0, return_pct_20d=ret,
        return_pct_60d=None, period_return_pct=ret,
        max_drawdown_pct=-5.0, max_rise_pct=5.0, volatility_annual=vol,
    )


def _volume(turnover=1.0, amount=5e8):
    return VolumeMetrics(
        symbol="sh600519", as_of=AS_OF, period_days=20,
        total_volume=1e6, avg_volume_5d=1e6, avg_volume_20d=1e6, avg_volume_60d=1e6,
        total_amount=1e10, avg_amount_5d=amount, avg_amount_20d=amount,
        avg_turnover_5d=turnover, avg_turnover_20d=turnover, avg_turnover_60d=turnover,
        max_turnover=2.0, min_turnover=0.5,
        abnormal_volume_days=0, abnormal_volume_dates=[],
        price_volume_corr=0.1,
    )


def _financial(debt=None, net_yoy=None, pe=20.0, pb=3.0):
    return FinancialMetrics(
        symbol="sh600519", as_of=AS_OF,
        latest_revenue_yoy=10.0, latest_net_profit_yoy=net_yoy,
        latest_roe=15.0, latest_gross_margin=50.0, latest_net_margin=30.0,
        latest_debt_ratio=debt,
        revenue_yoy_trend=[10.0], net_profit_yoy_trend=[net_yoy] if net_yoy is not None else [],
        roe_trend=[15.0],
        revenue_yoy_change=None, profit_yoy_change=None, roe_change=None,
        pe_ttm=pe, pb=pb,
    )


class TestRiskEngineOptionalPriceVolume(unittest.TestCase):
    """§4.2 风险引擎量价可选 —— 纯基本面路径仍出风险项。"""

    def test_no_price_volume_skips_market_rules(self):
        """无量价：规则 1-3 全部跳过（不猜、不补），不出 Market/Liquidity 项。"""
        risk = analyze_risk("sh600519", AS_OF, financial=_financial())
        cats = {i.category for i in risk.items}
        self.assertNotIn("Market", cats)
        self.assertNotIn("Liquidity", cats)

    def test_no_price_volume_still_applies_fundamental_rules(self):
        """无量价但基本面规则照跑：高负债 + 业绩下滑。"""
        risk = analyze_risk(
            "sh600519", AS_OF,
            financial=_financial(debt=80.0, net_yoy=-30.0),
        )
        cats = {i.category for i in risk.items}
        self.assertIn("Financial", cats)   # 规则 4 负债率
        self.assertIn("Business", cats)    # 规则 5 净利同比
        self.assertEqual(risk.overall_level, "high")
        self.assertTrue(risk.veto_buy)     # 两条 high → 一票否决

    def test_symbol_as_of_taken_from_args(self):
        """symbol/as_of 由调用方传入（原先从 price 借，无量价时借不到）。"""
        risk = analyze_risk("sh600519", AS_OF, financial=_financial())
        self.assertEqual(risk.symbol, "sh600519")
        self.assertEqual(risk.as_of, AS_OF)

    def test_price_volume_still_produce_market_rules(self):
        """有量价时行为不变：高波动 + 大回撤仍出 Market 项。"""
        risk = analyze_risk(
            "sh600519", AS_OF,
            price=_price(vol=50.0, ret=-20.0),
            volume=_volume(),
            financial=_financial(),
        )
        cats = {i.category for i in risk.items}
        self.assertIn("Market", cats)
        self.assertEqual(risk.symbol, "sh600519")

    def test_valuation_missing_rule_still_fires(self):
        """规则 7（PE 缺失）不依赖量价。"""
        risk = analyze_risk(
            "sh600519", AS_OF,
            financial=_financial(pe=None, pb=None),
        )
        self.assertIn("Valuation", {i.category for i in risk.items})


class TestChapterDecoupling(unittest.TestCase):
    """§4.3 「股价表现」章不再由 `company` 触发。"""

    def test_company_profile_has_no_price_chapter(self):
        ms = load_profile("company")
        titles = [c["title"] for c in build_manifest(ms.sections)]
        self.assertNotIn("股价表现", titles)
        self.assertNotIn("成交量与换手率", titles)
        self.assertIn("基本面分析", titles)
        self.assertIn("风险分析", titles)

    def test_company_alone_does_not_trigger_price_chapter(self):
        """回归本体：含 company 但不含 market 的 sections 不产出量价章。"""
        titles = [c["title"] for c in build_manifest(["company", "financial"])]
        self.assertNotIn("股价表现", titles)

    def test_legacy_profiles_unaffected(self):
        """零回归：含 company 的旧 profile 同时含 market，量价章照常出现。"""
        for name in ("complete-stock", "short-term", "prebuy"):
            p = load_profile(name)
            titles = [c["title"] for c in build_manifest(p.sections)]
            self.assertIn("股价表现", titles, f"{name} 丢了股价表现章")


class TestNewProfiles(unittest.TestCase):
    """§4.1 两个新 profile 的装配契约。"""

    def test_company_does_not_collect_market(self):
        """公司研报**不采集行情** —— plan 无 collect_market 任务。"""
        plan = PlanGenerator.generate(load_profile("company"))
        names = [t.name for t in plan.tasks]
        self.assertNotIn("collect_market", names)
        self.assertNotIn("analyze_price", names)
        self.assertNotIn("analyze_volume", names)
        self.assertNotIn("analyze_patterns", names)
        self.assertIn("collect_financial", names)
        self.assertIn("collect_announcement", names)  # 风险规则 6 依赖公告

    def test_company_scorecard_excludes_market_trend(self):
        """company 无价格数据 → 评分卡不含 market_trend（纳入即恒 N/A）。"""
        dims = load_profile("company").scorecard.dimensions
        self.assertNotIn("market_trend", dims)
        self.assertIn("risk", dims)  # risk 保留且在无量价时可用

    def test_stock_does_not_collect_financial(self):
        """个股分析只用量价侧 —— 不采财务。"""
        plan = PlanGenerator.generate(load_profile("stock"))
        names = [t.name for t in plan.tasks]
        self.assertIn("collect_market", names)
        self.assertIn("analyze_patterns", names)
        self.assertNotIn("collect_financial", names)
        self.assertNotIn("analyze_financial", names)

    def test_stock_scorecard_excludes_sensitive_fundamental_trend(self):
        """stock 不采财务 → 评分卡不含 fundamental_trend（敏感维度，纳入即误报「结论受限」）。"""
        dims = load_profile("stock").scorecard.dimensions
        self.assertNotIn("fundamental_trend", dims)
        self.assertNotIn("business_quality", dims)
        self.assertNotIn("valuation", dims)
        self.assertIn("market_trend", dims)

    def test_stock_has_no_financial_chapter(self):
        titles = [c["title"] for c in build_manifest(load_profile("stock").sections)]
        self.assertNotIn("基本面分析", titles)
        self.assertIn("股价表现", titles)
        self.assertIn("量价形态", titles)


class TestFundamentalEvidence(unittest.TestCase):
    """§4.2.1 无 bars 主体的 Evidence 登记 —— 否则机检 evidence_completeness 必失败。"""

    def test_add_fundamental_evidence_covers_financial_and_risk(self):
        from src.evidence import EvidenceStore
        from src.analysis.engine import add_fundamental_evidence

        store = EvidenceStore()
        fin = _financial(debt=80.0, net_yoy=-30.0)
        risk = analyze_risk("sh600519", AS_OF, financial=fin)
        add_fundamental_evidence(store, fin, risk, None)

        sections = {e["section"] for e in store.to_json() if e.get("section")}
        self.assertIn("financial", sections)
        self.assertIn("risk", sections)

    def test_no_bars_means_refresh_evidence_is_skipped(self):
        """确认坑的存在：没有 bars 时 _refresh_evidence 一条都不登记。

        这是 §4.2.1 之所以必要的原因 —— 若不另走 add_fundamental_evidence，
        公司研报的 Evidence 就是空的。
        """
        import inspect
        from src.workflow.engine import WorkflowEngine
        src = inspect.getsource(WorkflowEngine._refresh_evidence)
        self.assertIn("if not bars:", src)
        self.assertIn("return", src)


class TestTimeBoundaryForFundamentalReport(unittest.TestCase):
    """机检 time_boundary 对公司研报的适配（设计 §7 未预见的结构缺口）。

    原判定只认 `price.period_days` / `industry_index.price.period_days`。
    公司研报不采行情 → 两者皆无 → 机检恒失败。补财报报告期作第三条时间边界。
    """

    def _gate_check(self, sections, js):
        from src.quality_gate import run_quality_gate
        gate = run_quality_gate(sections, js, [{"section": "risk"}], True, 0)
        return next(c for c in gate.checks if c.name == "time_boundary")

    def test_financial_report_period_satisfies_time_boundary(self):
        """有财报快照（report_date）→ time_boundary 通过。"""
        js = {
            "meta": {"as_of": "2026-09-17"},
            "financial": {"as_of": "2026-09-17"},
            "financial_snapshots": [{"report_date": "2026-06-30"}],
        }
        chk = self._gate_check(["company", "financial", "risk"], js)
        self.assertTrue(chk.passed, chk.detail)

    def test_no_boundary_still_fails(self):
        """无价格、无财报快照 → 仍应判失败（不放水）。"""
        js = {"meta": {"as_of": "2026-09-17"}, "financial": {"as_of": "2026-09-17"}}
        chk = self._gate_check(["company", "financial"], js)
        self.assertFalse(chk.passed, chk.detail)

    def test_price_path_unaffected(self):
        """有价格的主体走原路径，行为不变。"""
        js = {"meta": {"as_of": "2026-09-17"}, "price": {"period_days": 60}}
        chk = self._gate_check(["market", "volume"], js)
        self.assertTrue(chk.passed, chk.detail)


if __name__ == "__main__":
    unittest.main()
