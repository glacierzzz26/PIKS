"""test_industry_index.py — 行业本体 Provider + 分析引擎单元测试(P9 / #12)

不联网:全部用 fixture/monkeypatch 打桩 akshare,锁住
「层级归属靠查表」「估值只做横截面」「不采集成交额」三条硬约束。
"""
import json
import os
import sys
import unittest
from datetime import date
from unittest.mock import patch

import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.industry import (
    Constituent as AConstituent,
    analyze_industry,
    analyze_industry_risk,
)
from src.providers.industry.sw_index_provider import (
    Constituent,
    IndustryRef,
    SwIndexProvider,
)


# ---- 申万行业表 fixture(真实结构,取自 2026-09-16 实测) ----
def _first_info() -> pd.DataFrame:
    return pd.DataFrame([
        {"行业代码": "801010.SI", "行业名称": "农林牧渔", "成份个数": 104,
         "静态市盈率": 30.67, "TTM(滚动)市盈率": 32.45, "市净率": 1.95, "静态股息率": 2.22},
        {"行业代码": "801030.SI", "行业名称": "基础化工", "成份个数": 412,
         "静态市盈率": 25.77, "TTM(滚动)市盈率": 22.08, "市净率": 2.30, "静态股息率": 1.59},
        {"行业代码": "801040.SI", "行业名称": "钢铁", "成份个数": 44,
         "静态市盈率": -8.0, "TTM(滚动)市盈率": -12.5, "市净率": 0.9, "静态股息率": 0.5},
        {"行业代码": "801050.SI", "行业名称": "有色金属", "成份个数": 130,
         "静态市盈率": 40.0, "TTM(滚动)市盈率": 55.0, "市净率": 3.1, "静态股息率": 0.8},
    ])


def _second_info() -> pd.DataFrame:
    return pd.DataFrame([
        {"行业代码": "801016.SI", "行业名称": "种植业", "上级行业": "农林牧渔", "成份个数": 20,
         "静态市盈率": 39.69, "TTM(滚动)市盈率": 34.27, "市净率": 2.17, "静态股息率": 1.66},
    ])


def _third_info() -> pd.DataFrame:
    return pd.DataFrame([
        {"行业代码": "850111.SI", "行业名称": "种子", "上级行业": "种植业", "成份个数": 8,
         "静态市盈率": 80.4, "TTM(滚动)市盈率": 82.91, "市净率": 2.6, "静态股息率": 0.53},
    ])


def _third_cons() -> pd.DataFrame:
    return pd.DataFrame([
        {"股票代码": "600313.SH", "股票简称": "农发种业", "市值": 76.77, "市盈率ttm": 105.02,
         "市净率": 3.15, "ROE(%)": 2.37, "股息率": 0.14, "净利润增速(%)": -16.64, "营收增速(%)": 32.84},
        {"股票代码": "000713.SZ", "股票简称": "国投丰乐", "市值": 51.09, "市盈率ttm": 81.29,
         "市净率": 1.65, "ROE(%)": -1.0, "股息率": 0.47, "净利润增速(%)": -11.33, "营收增速(%)": 3.88},
        {"股票代码": "600371.SH", "股票简称": "万向德农", "市值": 37.83, "市盈率ttm": None,
         "市净率": None, "ROE(%)": 7.27, "股息率": 0.77, "净利润增速(%)": -44.47, "营收增速(%)": -15.14},
    ])


_FIXTURES = {1: _first_info, 2: _second_info, 3: _third_info}


def _provider_with_fixtures() -> SwIndexProvider:
    p = SwIndexProvider()
    p._info_cache = {lvl: fn() for lvl, fn in _FIXTURES.items()}
    return p


class TestResolveLevel(unittest.TestCase):
    """层级归属**必须查表** —— 801010 是农林牧渔,不是食品饮料。"""

    def test_level1_lookup(self):
        ref = _provider_with_fixtures().resolve("801010")
        self.assertEqual(ref.name, "农林牧渔")   # 不是「食品饮料」
        self.assertEqual(ref.level, 1)
        self.assertEqual(ref.level_label, "申万一级")
        self.assertIsNone(ref.parent)             # 一级无上级
        self.assertEqual(ref.member_count, 104)

    def test_level2_lookup(self):
        ref = _provider_with_fixtures().resolve("801016")
        self.assertEqual(ref.name, "种植业")
        self.assertEqual(ref.level, 2)
        self.assertEqual(ref.parent, "农林牧渔")

    def test_level3_lookup(self):
        ref = _provider_with_fixtures().resolve("850111")
        self.assertEqual(ref.name, "种子")
        self.assertEqual(ref.level, 3)

    def test_unknown_code_returns_none(self):
        # 不在任何层级表中 → None(不臆造一个行业名)
        self.assertIsNone(_provider_with_fixtures().resolve("999999"))


class TestValuationRank(unittest.TestCase):
    """D-R7:估值**只做横截面排名**,且亏损行业(PE<0)不进分母。"""

    def test_rank_ascending_excludes_negative_pe(self):
        p = _provider_with_fixtures()
        ref = p.resolve("801010")          # PE 32.45
        r = p.valuation_rank(ref)
        # 有效正 PE: 32.45, 22.08, 55.0 → 钢铁 -12.5 排除
        self.assertEqual(r.universe, 3)
        self.assertEqual(r.rank, 2)        # 22.08 < 32.45 < 55.0
        self.assertIn("申万一级", r.text)
        self.assertIn("第 2", r.text)

    def test_negative_pe_industry_has_no_rank(self):
        p = _provider_with_fixtures()
        ref = p.resolve("801040")          # 钢铁 PE -12.5(亏损)
        r = p.valuation_rank(ref)
        self.assertIsNone(r.rank)
        self.assertEqual(r.text, "N/A")

    def test_text_has_no_percentile_language(self):
        """D-R7 红线:不得出现「分位」二字(那是历史分位语义)。"""
        p = _provider_with_fixtures()
        r = p.valuation_rank(p.resolve("801010"))
        self.assertNotIn("分位", r.text)


class TestGetHistoryUnits(unittest.TestCase):
    """指数不采集成交额/成交量(量纲不可靠),且无涨跌停标记。"""

    @patch("akshare.index_hist_sw")
    def test_volume_amount_zeroed_and_no_limit_flag(self, mock_hist):
        mock_hist.return_value = pd.DataFrame([
            {"代码": "801010", "日期": "2026-09-14", "收盘": 2612.41, "开盘": 2630.95,
             "最高": 2634.81, "最低": 2584.83, "成交量": 44.01, "成交额": 353.63},
            {"代码": "801010", "日期": "2026-09-15", "收盘": 2573.25, "开盘": 2606.05,
             "最高": 2612.46, "最低": 2572.25, "成交量": 39.27, "成交额": 299.57},
        ])
        from src.models import Symbol, Market
        p = SwIndexProvider()
        bars = p.get_history(Symbol(code="801010", market=Market.SI),
                             date(2026, 9, 1), date(2026, 9, 16))
        self.assertEqual(len(bars), 2)
        for b in bars:
            self.assertEqual(b.volume, 0)        # 量纲不可靠 → 不采集
            self.assertEqual(b.amount, 0.0)
            self.assertEqual(b.turnover, 0.0)
            self.assertFalse(b.is_limit_up)      # 指数无涨跌停制度
            self.assertFalse(b.is_limit_down)
        # 涨跌幅自算: (2573.25-2612.41)/2612.41*100 = -1.498999...
        self.assertAlmostEqual(bars[1].pct_change, -1.498999, places=4)


class TestAggregateConstituents(unittest.TestCase):
    """一级/二级成分由三级聚合(申万只有三级 cons 接口),按代码去重。"""

    def test_level3_direct(self):
        p = _provider_with_fixtures()
        p._cons_cache["850111"] = [
            Constituent(symbol="600313", name="农发种业", market_cap=76.77, roe=2.37),
            Constituent(symbol="000713", name="国投丰乐", market_cap=51.09, roe=-1.0),
        ]
        out = p.get_constituents(p.resolve("850111"))
        self.assertEqual([c.symbol for c in out], ["600313", "000713"])

    def test_level1_aggregates_via_two_hops(self):
        p = _provider_with_fixtures()
        p._cons_cache["850111"] = [Constituent(symbol="600313", name="农发种业", market_cap=76.77)]
        out = p.get_constituents(p.resolve("801010"))   # 一级 → 种植业 → 种子
        self.assertEqual([c.symbol for c in out], ["600313"])


def _bars(n=300, base=2000.0, step=1.0):
    from src.models import Bar
    out = []
    for i in range(n):
        close = base + i * step
        out.append(Bar(symbol="sw801010", date=date(2025, 1, 1), open=close, high=close,
                       low=close, close=close, volume=0, amount=0.0, amplitude=0.0,
                       pct_change=0.0, chg_amount=0.0, turnover=0.0))
    return out


class TestAnalyzeIndustry(unittest.TestCase):
    def _run(self, bars, history):
        ref = IndustryRef(code="801010", name="农林牧渔", level=1, level_label="申万一级",
                          parent=None, member_count=104, pe_ttm=32.45, pe_static=30.67,
                          pb=1.95, dividend_yield=2.22)
        rank = _provider_with_fixtures().valuation_rank(ref)
        cons = [
            AConstituent(symbol="600313", name="A", market_cap=76.77, roe=2.37,
                         net_profit_growth=-16.64, revenue_growth=32.84, pe_ttm=105.02),
            AConstituent(symbol="000713", name="B", market_cap=51.09, roe=-1.0,
                         net_profit_growth=-11.33, revenue_growth=3.88, pe_ttm=81.29),
            AConstituent(symbol="600371", name="C", market_cap=37.83, roe=7.27,
                         net_profit_growth=-44.47, revenue_growth=-15.14, pe_ttm=None),
        ]
        return analyze_industry(ref, bars, history, cons, rank, date(2026, 9, 16))

    def test_metrics(self):
        m = self._run(_bars(300), _bars(600, base=1000.0))
        self.assertEqual(m.price.period_days, 300)
        self.assertEqual(m.dispersion.count, 3)
        # ROE 中位: -1.0, 2.37, 7.27 → 2.37
        self.assertAlmostEqual(m.dispersion.roe_median, 2.37, places=2)
        self.assertIsNotNone(m.dispersion.roe_q1)
        # 市值合计
        self.assertAlmostEqual(m.dispersion.cap_sum, 165.69, places=2)
        # PE 中位只用正值: 105.02, 81.29 → 93.155
        self.assertAlmostEqual(m.dispersion.pe_ttm_median, 93.155, places=2)
        self.assertEqual(m.constituent_count, 3)
        self.assertEqual(m.declared_count, 104)

    def test_point_percentile_uses_full_history(self):
        """点位分位基于**全历史**,且如实暴露历史起点/天数。"""
        m = self._run(_bars(300), _bars(600, base=1000.0))
        self.assertEqual(m.price.history_days, 600)
        self.assertEqual(m.price.history_start, date(2025, 1, 1))
        self.assertIsNotNone(m.price.point_percentile)

    def test_history_metrics_has_no_percentile_when_empty(self):
        m = self._run(_bars(300), [])
        # 无全历史时退回展示窗口,不谎称有历史序列
        self.assertEqual(m.price.history_days, 300)


class TestIndustryRisk(unittest.TestCase):
    def _metrics(self, vol, ret, drawdown, roe_median, rank=None, universe=31):
        from src.analysis.industry import (
            DispersionMetrics, IndustryMetrics, IndustryPriceMetrics, ValuationRank,
        )
        ref = IndustryRef(code="801010", name="农林牧渔", level=1, level_label="申万一级")
        return IndustryMetrics(
            ref=ref,
            price=IndustryPriceMetrics(
                symbol="801010", as_of=date(2026, 9, 16), period_days=250,
                start_point=2000.0, end_point=2500.0, period_return_pct=ret,
                return_pct_5d=None, return_pct_20d=None, return_pct_60d=None,
                max_drawdown_pct=drawdown, volatility_annual=vol,
                point_percentile=50.0, history_start=date(1999, 12, 30), history_days=6456,
            ),
            valuation_rank=ValuationRank(rank=rank, universe=universe,
                                         level_label="申万一级", value=32.45),
            dispersion=DispersionMetrics(count=10, roe_median=roe_median),
        )

    def test_high_volatility_and_drawdown(self):
        r = analyze_industry_risk(self._metrics(45.0, -20.0, 25.0, 5.0), date(2026, 9, 16))
        self.assertEqual(r.overall_level, "high")
        cats = [i.category for i in r.items]
        self.assertIn("Market", cats)

    def test_expensive_rank_flagged(self):
        # rank 30 / universe 31 → 后 25% 分位 → 估值偏贵
        r = analyze_industry_risk(self._metrics(10.0, 5.0, 5.0, 5.0, rank=30), date(2026, 9, 16))
        cats = [i.category for i in r.items]
        self.assertIn("Valuation", cats)
        # 依据里必须是横截面位次,不得出现「分位」
        val = next(i for i in r.items if i.category == "Valuation")
        self.assertIn("第 30", val.evidence)
        self.assertNotIn("分位", val.evidence)

    def test_no_risk_signals(self):
        r = analyze_industry_risk(self._metrics(10.0, 3.0, 5.0, 5.0), date(2026, 9, 16))
        self.assertEqual(r.overall_level, "low")
        self.assertEqual(len(r.items), 0)

    def test_negative_roe_median_flagged(self):
        r = analyze_industry_risk(self._metrics(10.0, 3.0, 5.0, -2.0), date(2026, 9, 16))
        cats = [i.category for i in r.items]
        self.assertIn("Fundamental", cats)


class TestSynthesisPromptSubjectAware(unittest.TestCase):
    def test_industry_prompt_has_cross_sectional_constraint(self):
        from src.report.synthesis import build_synthesis_prompt
        p = build_synthesis_prompt(
            "sw801010",
            {"industry_index": {"ref": {"name": "农林牧渔", "level_label": "申万一级"},
                                "price": {"end_point": 2573.25}}},
        )
        self.assertIn("行业研究报告", p)
        self.assertIn("不得表述行业估值的历史分位", p)

    def test_company_prompt_unchanged(self):
        """个股分支必须**逐字节**保留原首行 —— internal/ai/mock.go 靠它分流。"""
        from src.report.synthesis import build_synthesis_prompt
        p = build_synthesis_prompt("sh600519", {"price": {"end_price": 1290.88}})
        self.assertIn("你是一名 A 股研究分析师，正在撰写 sh600519 的个股研究报告。", p)
        self.assertNotIn("不得表述行业估值的历史分位", p)


class TestIndustryMarkdownSections(unittest.TestCase):
    def _im(self):
        from src.analysis.industry import (
            DispersionMetrics, IndustryMetrics, IndustryPriceMetrics, ValuationRank,
        )
        ref = IndustryRef(code="801010", name="农林牧渔", level=1, level_label="申万一级",
                          member_count=104, pe_static=30.67, pe_ttm=32.45, pb=1.95,
                          dividend_yield=2.22)
        return IndustryMetrics(
            ref=ref,
            price=IndustryPriceMetrics(
                symbol="801010", as_of=date(2026, 9, 16), period_days=250,
                start_point=2400.0, end_point=2573.25, period_return_pct=7.22,
                return_pct_5d=-1.5, return_pct_20d=3.0, return_pct_60d=5.0,
                max_drawdown_pct=8.1, volatility_annual=22.4,
                point_percentile=88.0, history_start=date(1999, 12, 30), history_days=6456,
            ),
            valuation_rank=ValuationRank(rank=26, universe=31, level_label="申万一级", value=32.45),
            dispersion=DispersionMetrics(count=104, roe_median=5.5, roe_q1=1.0, roe_q3=9.0,
                                        net_profit_growth_median=3.2, revenue_growth_median=4.1,
                                        pe_ttm_median=28.3, cap_sum=12345.0),
            declared_count=104, constituent_count=104,
        )

    def _md(self, sections):
        from src.report.markdown import generate_markdown
        return generate_markdown(
            "sw801010", date(2026, 9, 16), None, None,
            industry_metrics=self._im(), sections=sections,
        )

    def test_price_section(self):
        md = self._md(["industry_index"])
        self.assertIn("行业行情", md)
        self.assertIn("农林牧渔", md)
        self.assertIn("2573.25", md)
        self.assertIn("点位分位，非估值分位", md)
        # 不采集成交额:表中不得出现成交额**数值行**(口径说明里会提到「不提供成交额/换手率」)
        self.assertNotIn("区间总成交", md)
        self.assertNotIn("换手", md.split("口径说明")[0])
        # 封面头是行业报告,不是个股
        self.assertIn("行业研究报告", md)
        self.assertNotIn("个股研究报告", md)

    def test_valuation_section_uses_rank_not_percentile(self):
        md = self._md(["industry_valuation"])
        self.assertIn("估值定位", md)
        self.assertIn("横截面位次", md)
        self.assertIn("第 26", md)
        self.assertIn("不作估值历史分位判断", md)

    def test_structure_section(self):
        md = self._md(["industry_structure"])
        self.assertIn("成分结构", md)
        self.assertIn("ROE 中位", md)
        self.assertIn("12,345", md)     # 合计市值(千分位)

    def test_stock_path_untouched(self):
        """个股路径:给 price_metrics、不给 industry_metrics → 输出仍是「个股研究报告」。"""
        from src.analysis.price import PriceMetrics
        from src.analysis.volume import VolumeMetrics
        from src.report.markdown import generate_markdown
        price = PriceMetrics(symbol="sh600519", as_of=date(2026, 9, 10), period_days=60,
                            start_price=1240.0, end_price=1290.88, max_price=1363.35,
                            min_price=1151.01, return_pct_5d=-0.51, return_pct_10d=-0.91,
                            return_pct_20d=-3.88, return_pct_60d=4.1, period_return_pct=4.1,
                            max_drawdown_pct=6.53, max_rise_pct=9.0, volatility_annual=26.28)
        vol = VolumeMetrics(symbol="sh600519", as_of=date(2026, 9, 10), period_days=60,
                           total_volume=27634, avg_volume_5d=5000.0, avg_volume_20d=5000.0,
                           avg_volume_60d=None, total_amount=1e8, avg_amount_5d=5e6,
                           avg_amount_20d=5e6, avg_turnover_5d=0.3, avg_turnover_20d=0.25,
                           avg_turnover_60d=None, max_turnover=1.2, min_turnover=0.1,
                           abnormal_volume_days=0, abnormal_volume_dates=[],
                           price_volume_corr=0.3)
        md = generate_markdown("sh600519", date(2026, 9, 10), price, vol, sections=["market"])
        self.assertIn("个股研究报告：sh600519", md)
        self.assertIn("最新收盘价", md)      # 个股路径的量价章节仍在
        self.assertIn("1290.88", md)


class TestIndustryJsonReport(unittest.TestCase):
    """指标卡形状:行业无 price/volume 节,且关键数字必须**平铺到顶层**。"""

    def _json(self):
        from src.report.json_report import generate_json
        from src.analysis.industry import (
            DispersionMetrics, IndustryMetrics, IndustryPriceMetrics, ValuationRank,
        )
        ref = IndustryRef(code="801010", name="农林牧渔", level=1, level_label="申万一级",
                          member_count=104, pe_ttm=32.45)
        im = IndustryMetrics(
            ref=ref,
            price=IndustryPriceMetrics(
                symbol="801010", as_of=date(2026, 9, 16), period_days=250,
                start_point=2400.0, end_point=2573.25, period_return_pct=7.22,
                return_pct_5d=-1.5, return_pct_20d=3.0, return_pct_60d=5.0,
                max_drawdown_pct=8.1, volatility_annual=22.4,
                point_percentile=88.0, history_start=date(1999, 12, 30), history_days=6456,
            ),
            valuation_rank=ValuationRank(rank=26, universe=31, level_label="申万一级", value=32.45),
            dispersion=DispersionMetrics(count=104, roe_median=5.5, cap_sum=12345.0),
        )
        return generate_json("sw801010", date(2026, 9, 16), None, None, industry_metrics=im)

    def test_no_price_volume_keys_when_absent(self):
        d = self._json()
        # asdict(None) 会抛 TypeError;此处应为「省键」而非「空值」
        self.assertNotIn("price", d)
        self.assertNotIn("volume", d)

    def test_scalars_are_flattened_to_top_level(self):
        """合成提示的指标摘要是**单层**过滤 —— 嵌套一层的数字会被整体丢弃。"""
        from src.report.synthesis import build_synthesis_prompt
        prompt = build_synthesis_prompt("sw801010", self._json())
        for v in ("32.45", "2573.25", "22.4", "6456"):
            self.assertIn(v, prompt, f"{v} 未进入合成提示(未平铺到 industry_index 顶层?)")

    def test_top_level_rank_does_not_clobber_nested_block(self):
        d = self._json()["industry_index"]
        self.assertIsInstance(d["valuation_rank"], dict)      # 嵌套块原样保留
        self.assertEqual(d["valuation_rank_num"], 26)          # 平铺值另用键名

    def test_subject_line_is_well_formed(self):
        """回归:.strip("（）") 曾把刚拼上的右括号也剥掉。"""
        from src.report.synthesis import build_synthesis_prompt
        first = build_synthesis_prompt("sw801010", self._json()).splitlines()[0]
        self.assertIn("农林牧渔（申万一级）", first)
        self.assertEqual(first.count("（"), first.count("）"))


class TestIndustryQualityGate(unittest.TestCase):
    """行业 profile 必须能过六项机检(此前 3 项按个股假设写死)。"""

    def test_gate_passes_for_industry(self):
        from src.quality_gate import run_quality_gate
        from src.report.json_report import generate_json
        from src.analysis.industry import (
            DispersionMetrics, IndustryMetrics, IndustryPriceMetrics,
            IndustryRiskMetrics, ValuationRank,
        )
        ref = IndustryRef(code="801010", name="农林牧渔", level=1, level_label="申万一级",
                          member_count=104, pe_ttm=32.45)
        im = IndustryMetrics(
            ref=ref,
            price=IndustryPriceMetrics(
                symbol="801010", as_of=date(2026, 9, 16), period_days=250,
                start_point=2400.0, end_point=2573.25, period_return_pct=7.22,
                return_pct_5d=-1.5, return_pct_20d=3.0, return_pct_60d=5.0,
                max_drawdown_pct=8.1, volatility_annual=22.4,
                point_percentile=88.0, history_start=date(1999, 12, 30), history_days=6456,
            ),
            valuation_rank=ValuationRank(rank=26, universe=31, level_label="申万一级", value=32.45),
            dispersion=DispersionMetrics(count=104, roe_median=5.5, cap_sum=12345.0),
        )
        risk = IndustryRiskMetrics(symbol="801010", as_of=date(2026, 9, 16),
                                   overall_level="medium")
        js = generate_json("sw801010", date(2026, 9, 16), None, None,
                           risk_metrics=risk, industry_metrics=im)
        self.assertIn("risk", js)          # 行业风险走同一 risk 键
        evidence = [
            {"section": "industry_index"}, {"section": "industry_valuation"},
            {"section": "industry_structure"}, {"section": "risk"},
        ]
        gate = run_quality_gate(
            ["industry_index", "industry_valuation", "industry_structure", "risk"],
            js, evidence, lint_passed=True, lint_issue_count=0,
        )
        failed = [c.name for c in gate.checks if not c.passed]
        self.assertEqual(failed, [], f"机检未过: {failed}")


class TestInfoCacheOrder(unittest.TestCase):
    """行业表是**当期快照** —— 在线必须优先,磁盘只在官网失败时兜底。

    回归风险:若改成「磁盘优先」,估值/成份数会被冻死在首次抓取的那天,
    且**测试不会失败**(fixture 直接塞 _info_cache,绕过 _info)。
    """

    def _fresh(self):
        p = SwIndexProvider()
        p._info_cache = {}
        return p

    def test_online_wins_over_stale_disk(self):
        """官网可用时,返回值必须是**在线**的,不是磁盘里的旧快照。"""
        import tempfile
        from src.providers.industry import sw_index_provider as mod
        fresh = pd.DataFrame([{"行业代码": "801010.SI", "行业名称": "在线新值",
                               "成份个数": 1, "TTM(滚动)市盈率": 9.9}])
        stale = [{"行业代码": "801010.SI", "行业名称": "磁盘旧值",
                  "成份个数": 1, "TTM(滚动)市盈率": 1.1}]
        with tempfile.TemporaryDirectory() as d:
            with open(os.path.join(d, "sw_index_first_info.json"), "w") as f:
                json.dump(stale, f)
            with patch.object(mod, "_CACHE_DIR", d), \
                 patch("akshare.sw_index_first_info", return_value=fresh):
                df = self._fresh()._info(1)
        self.assertEqual(df.iloc[0]["行业名称"], "在线新值")

    def test_disk_fallback_when_online_fails(self):
        """官网 508 时仍能用磁盘快照解析(不返回 None)。"""
        import tempfile
        from src.providers.industry import sw_index_provider as mod
        stale = [{"行业代码": "801010.SI", "行业名称": "磁盘旧值", "成份个数": 1,
                  "TTM(滚动)市盈率": 1.1}]
        with tempfile.TemporaryDirectory() as d:
            with open(os.path.join(d, "sw_index_first_info.json"), "w") as f:
                json.dump(stale, f)
            with patch.object(mod, "_CACHE_DIR", d), \
                 patch("akshare.sw_index_first_info", side_effect=Exception("508")):
                df = self._fresh()._info(1)
        self.assertIsNotNone(df)
        self.assertEqual(df.iloc[0]["行业名称"], "磁盘旧值")

    def test_returns_none_without_cache_and_online(self):
        """两者都没有 → None(不臆造行业表)。"""
        import tempfile
        from src.providers.industry import sw_index_provider as mod
        with tempfile.TemporaryDirectory() as d:
            with patch.object(mod, "_CACHE_DIR", d), \
                 patch("akshare.sw_index_first_info", side_effect=Exception("508")):
                self.assertIsNone(self._fresh()._info(1))


if __name__ == "__main__":
    unittest.main()
