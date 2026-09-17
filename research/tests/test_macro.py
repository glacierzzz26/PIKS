"""test_macro.py — 宏观分析层 + 报告装配单元测试(P9-5b / issue #13)

不联网:provider 直接喂 fixture 序列,锁住设计文档 §4 的可算/禁算边界:

1. 分位基于**全历史**、且水平分位与同比分位**分别命名**(不混用)
2. **累计序列跨年不出 delta**(§4.2「数据源不支持 = 不产出字段」)
3. `MacroMetrics` **无** `volatility_annual`、无 OHLC/量字段(结构性禁止)
4. 期号字段是 **int** —— 这是「引用值入 known、零新增 lint 代码」的前提(§2.7)
5. 报告装配 + quality_gate 的宏观四态(含缺 period 块的反向守卫)
6. 值域格式化**不得**输出科学计数法(`number_lint._NUMBER_RE` 无指数部分)
"""
import os
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.macro import (  # noqa: E402
    MacroMetrics,
    analyze_macro,
    analyze_macro_risk,
)
from src.analysis.number_lint import (  # noqa: E402
    collect_numbers_from_json,
    lint_markdown_report,
)
from src.providers.macro.macro_provider import (  # noqa: E402
    MacroPoint,
    MacroProvider,
    MacroRef,
    MacroSeries,
    PeriodParts,
)
from src.quality_gate import run_quality_gate  # noqa: E402
from src.report.json_report import generate_json  # noqa: E402
from src.report.markdown import _fmt_level, generate_markdown  # noqa: E402


def _period_month(year: int, month: int) -> PeriodParts:
    return PeriodParts(label=f"{year}年{month:02d}月份", year=year,
                       end=date(year, month, 28), month=month)


def _period_quarter(year: int, q_to: int, q_from: int = 1) -> PeriodParts:
    label = (f"{year}年第{q_to}季度" if q_from == q_to
             else f"{year}年第{q_from}-{q_to}季度")
    return PeriodParts(label=label, year=year, end=date(year, q_to * 3, 28),
                       quarter_from=q_from, quarter_to=q_to)


def _ref(key="cn_cpi", **kw) -> MacroRef:
    base = dict(
        key=key, name="居民消费价格指数（CPI）", unit="%", cadence="month",
        level_label="指数（上年同月=100）", source="腾讯财经/akshare",
        caliber="全国口径。", level_percentile_meaningful=True,
    )
    base.update(kw)
    return MacroRef(**base)


def _series_cpi(n: int = 60) -> MacroSeries:
    """n 期月频序列,水平在 100 附近震荡、同比缓降 —— 便于断言分位与方向。"""
    points = []
    for i in range(n):
        year, month = 2021 + i // 12, i % 12 + 1
        level = 100.0 + (i % 7) * 0.4
        yoy = round(2.0 - i * 0.02, 2)
        points.append(MacroPoint(
            period=_period_month(year, month), level=round(level, 2), yoy=yoy,
            mom=round(0.1 * ((i % 5) - 2), 2),
        ))
    return MacroSeries(ref=_ref(), points=points)


def _series_gdp() -> MacroSeries:
    """累计口径季频序列,**含跨年边界**(2025 Q1-4 → 2026 Q1)。

    `single_quarter_level` 按 provider 的同年内差分规则在本 fixture 内算好
    —— 分析层只**读**该字段(差分属 provider 职责,其单测见
    `test_macro_provider.py::TestGdpDeCumulation`)。
    """
    raw = [
        (2025, 1, 318466.4, 5.4), (2025, 2, 660698.3, 5.3),
        (2025, 3, 1015303.6, 5.2), (2025, 4, 1349084.0, 5.0),
        (2026, 1, 334192.9, 5.0), (2026, 2, 695704.0, 4.7),
    ]
    points, start_of_year = [], None
    for y, q, lv, yoy in raw:
        if q == 1 or start_of_year is None:
            single = lv
        else:
            single = round(lv - start_of_year, 4)
        start_of_year = lv
        points.append(MacroPoint(period=_period_quarter(y, q), level=lv, yoy=yoy,
                                 single_quarter_level=single, cumulative=True))
    return MacroSeries(ref=_ref(key="cn_gdp", name="国内生产总值（GDP）", cadence="quarter",
                                level_label="亿元（累计）",
                                level_percentile_meaningful=False),
                       points=points)


class TestAnalyzeMacro(unittest.TestCase):
    def test_percentile_uses_full_history_not_window(self):
        """分位分母是全历史:展示窗口截短**不得**改变分位。"""
        s = _series_cpi(60)
        full = analyze_macro(s.ref, s, date(2026, 9, 17), window_periods=60)
        short = analyze_macro(s.ref, s, date(2026, 9, 17), window_periods=6)
        self.assertEqual(full.percentile_level, short.percentile_level)
        self.assertEqual(full.percentile_yoy, short.percentile_yoy)
        # 但展示窗口确实变短了
        self.assertEqual(len(short.readings), 6)
        self.assertEqual(short.period.window_periods, 6)

    def test_level_and_yoy_percentile_are_separate(self):
        """两个分位分别命名 —— 同比缓降时同比分位必低,水平分位不应被带偏。"""
        m = analyze_macro(_ref(), _series_cpi(60), date(2026, 9, 17), window_periods=60)
        self.assertIsNotNone(m.percentile_level)
        self.assertIsNotNone(m.percentile_yoy)
        # 同比自 2.0 单调降到 0.82 → 必为全历史最低
        self.assertLessEqual(m.percentile_yoy, 5.0)

    def test_period_fields_are_int(self):
        """期号必须是 int —— str 会被 collect_numbers_from_json 跳过 → lint 必挂。"""
        m = analyze_macro(_ref(), _series_cpi(12), date(2026, 9, 17))
        self.assertIsInstance(m.period.period_year, int)
        self.assertIsInstance(m.period.period_month, int)
        for r in m.readings:
            self.assertIsInstance(r.period_year, int)
            self.assertIsInstance(r.period_month, int)

    def test_source_lag_days_is_fact(self):
        m = analyze_macro(_ref(), _series_cpi(12), date(2026, 9, 17))
        self.assertGreater(m.period.source_lag_days, 0)
        self.assertEqual(m.period.source_lag_days,
                         (date(2026, 9, 17) - m.period.period_end).days)

    def test_no_structural_forbidden_fields(self):
        """结构性禁止:指标卡**没有**波动率/OHLC/量字段(§4.2)。"""
        m = analyze_macro(_ref(), _series_cpi(12), date(2026, 9, 17))
        for banned in ("volatility_annual", "open", "high", "low", "close",
                       "volume", "turnover", "limit_up_pct", "limit_down_pct"):
            self.assertFalse(hasattr(m, banned), f"MacroMetrics 不应有字段 {banned}")

    def test_empty_series_raises(self):
        s = MacroSeries(ref=_ref(), points=[])
        with self.assertRaises(ValueError):
            analyze_macro(s.ref, s, date(2026, 9, 17))


class TestCumulativeDelta(unittest.TestCase):
    """累计序列不作跨期差 —— 靠**不产出字段**杜绝(§4.2)。

    增量只经 `latest_single_quarter_level` 一种形式面世(§2.4c):
    同年内 delta_level 恒等于它(重复呈现),跨年则是两个不同跨度累计之差(不可比)。
    """

    def test_same_year_delta_not_produced(self):
        """2026 Q1 → Q1-2 同年:delta_level 仍**不产出** —— 它是单季水平的重复。"""
        s = _series_gdp()
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        self.assertIsNone(m.delta_level, "累计口径不出 delta_level(增量走单季水平)")
        # 增量本身照常可得,且等于同年内累计之差
        self.assertAlmostEqual(m.latest_single_quarter_level, 361511.1, places=1)
        self.assertAlmostEqual(
            m.latest_single_quarter_level, 695704.0 - 334192.9, places=1)

    def test_cross_year_delta_suppressed(self):
        """2025 Q1-4 → 2026 Q1 跨年:delta_level/delta_yoy 必须为 None。"""
        s = _series_gdp()
        # 只保留 2025 Q4 与 2026 Q1 两期,构成跨年相邻对
        s.points = [s.points[3], s.points[4]]
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        self.assertTrue(m.period.cumulative)
        self.assertIsNone(m.delta_level, "累计跨年的 delta_level 属不可比,必须不产出")
        self.assertIsNone(m.delta_yoy, "累计跨年的 delta_yoy 属不可比,必须不产出")
        # 但上期本身照常保留(可对账的事实)
        self.assertEqual(m.prev_label, "2025年第1-4季度")
        self.assertAlmostEqual(m.prev_level, 1349084.0, places=1)

    def test_non_cumulative_cross_year_delta_kept(self):
        """非累计序列(CPI 等)**跨年照样**可作差 —— 抑制只针对累计口径。"""
        s = _series_cpi(24)
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        self.assertFalse(m.period.cumulative)
        self.assertIsNotNone(m.delta_level)
        self.assertIsNotNone(m.delta_yoy)


class TestAnalyzeMacroRisk(unittest.TestCase):
    """规则风险:封面结论 chip 的**唯一来源**(宏观无评分卡,D-M5)。"""

    def test_always_yields_overall_level(self):
        """任何输入都必须给出 overall_level —— 否则封面 chip 无源可落。"""
        m = analyze_macro(_ref(), _series_cpi(12), date(2026, 9, 17))
        r = analyze_macro_risk(m, date(2026, 9, 17))
        self.assertIn(r.overall_level, ("low", "medium", "high"))

    def test_level_percentile_gate_respected(self):
        """level_percentile_meaningful=False 的维度**不得**报水平分位风险。

        实测背景:M2/GDP 是名义总量,水平分位恒 ~100% —— 报「位于历史上沿,
        需关注均值回归」是噪声,且「均值回归」对货币供应量/经济体量不成立。
        """
        # 构造水平分位 = 100% 的场景:名义总量单调上行
        s = _series_cpi(12)
        for i, p in enumerate(s.points):
            p.level = 1_000_000.0 + i * 50_000.0
        s.ref = _ref(key="cn_m2", name="货币和金融统计（M2）",
                     level_label="亿元", level_percentile_meaningful=False)
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        self.assertEqual(m.percentile_level, 100.0)
        r = analyze_macro_risk(m, date(2026, 9, 17))
        self.assertNotIn("宏观位置", [i.category for i in r.items])
        # 水平分位仍是**事实**,只是不当信号
        self.assertEqual(m.percentile_level, 100.0)

    def test_level_percentile_used_when_meaningful(self):
        """level_percentile_meaningful=True 时,水平分位照常触发风险项。"""
        s = _series_cpi(12)
        s.ref = _ref(key="cn_ppi", level_percentile_meaningful=True)
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        m.percentile_level = 95.0  # 人工置高,隔离被测规则
        r = analyze_macro_risk(m, date(2026, 9, 17))
        self.assertIn("宏观位置", [i.category for i in r.items])
        self.assertEqual(r.overall_level, "medium")

    def test_direction_run_medium_level(self):
        """连续同向回落 3 期以上 → medium。"""
        s = _series_cpi(12)
        m = analyze_macro(s.ref, s, date(2026, 9, 17))
        self.assertLessEqual(m.direction_run, -3)
        r = analyze_macro_risk(m, date(2026, 9, 17))
        self.assertIn("趋势", [i.category for i in r.items])

    def test_no_forecast_wording_in_risk(self):
        """风险文案不得含预测词(设计文档 §4.2)。"""
        m = analyze_macro(_ref(), _series_cpi(12), date(2026, 9, 17))
        r = analyze_macro_risk(m, date(2026, 9, 17))
        blob = " ".join(i.description + i.evidence for i in r.items)
        for banned in ("预计", "预期", "将会", "目标位", "预测"):
            self.assertNotIn(banned, blob)


class TestLevelFormatting(unittest.TestCase):
    """值域格式化:**禁止科学计数法**(实测 lint 挂点)。"""

    def test_no_scientific_notation(self):
        """3568083.6 曾渲染为 `3.568e+06` → lint 只匹配到 `3.568`,必挂。"""
        for v in (3568083.6, 695704.0, 100.8, 148311.98, 1005872.4):
            out = _fmt_level(v)
            self.assertNotIn("e", out.lower(), f"{v} 渲染成了科学计数法:{out}")

    def test_magnitude_dispatch(self):
        self.assertEqual(_fmt_level(3568083.6), "3,568,083.6")
        self.assertEqual(_fmt_level(100.8), "100.80")
        self.assertEqual(_fmt_level(3568083.6 - 3555077.24, signed=True), "+13,006.4")

    def test_none_and_signed(self):
        self.assertEqual(_fmt_level(None), "N/A")
        self.assertEqual(_fmt_level(-1234.5, signed=True), "-1,234.5")


class TestReportAssembly(unittest.TestCase):
    """正文装配 + JSON 指标卡 + Number Lint 对账。"""

    def _build(self, key="cn_cpi", series=None, **ref_kw):
        s = series or _series_cpi(24)
        ref = _ref(key=key, **ref_kw)
        s.ref = ref
        m = analyze_macro(ref, s, date(2026, 9, 17))
        risk = analyze_macro_risk(m, date(2026, 9, 17))
        md = generate_markdown(
            symbol=f"macro:{key}", as_of=date(2026, 9, 17),
            price_metrics=None, volume_metrics=None,
            risk_metrics=risk, macro_metrics=m,
            sections=["macro_level", "macro_position", "risk"],
        )
        js = generate_json(
            symbol=f"macro:{key}", as_of=date(2026, 9, 17),
            price_metrics=None, volume_metrics=None,
            risk_metrics=risk, macro_metrics=m,
            sections=["macro_level", "macro_position", "risk"],
        )
        return m, md, js

    def test_cover_title_and_data_cutoff(self):
        """封面必须是「宏观研究报告」+ 数据截止(不是报告生成日)。"""
        _, md, _ = self._build()
        self.assertIn("# 宏观研究报告：", md)
        self.assertIn("数据截止：", md)
        self.assertIn("较报告生成日滞后", md)

    def test_no_price_metrics_crash_without_adjustment_claim(self):
        """宏观报告无价格 —— 免责声明不得声称「前复权计算」。"""
        _, md, _ = self._build()
        self.assertNotIn("前复权", md)
        self.assertIn("数据截止", md)

    def test_lint_clean_on_all_dimensions(self):
        """三个位面各自 lint 全通过 —— 期号 + 序列入卡缺一不可。"""
        cases = [
            ("cn_cpi", _series_cpi(36), {}),
            ("cn_gdp", _series_gdp(),
             dict(name="国内生产总值（GDP）", level_label="亿元（累计）",
                  level_percentile_meaningful=False)),
        ]
        for key, series, kw in cases:
            with self.subTest(dim=key):
                _, md, js = self._build(key=key, series=series, **kw)
                report = lint_markdown_report(md, js)
                self.assertTrue(report.passed, f"{key} lint 未通过:\n" +
                                "\n".join(str(i) for i in report.issues))

    def test_readings_nested_in_card_for_lint(self):
        """序列读数必须**整段**进卡:正文逐行印全部窗口期读数与期号。"""
        m, _, js = self._build()
        readings = js["macro"]["readings"]
        self.assertEqual(len(readings), len(m.readings))
        self.assertIn(m.latest_level, collect_numbers_from_json(js))
        # 期号也须入 known(str 会被跳过,故存 int)
        known = collect_numbers_from_json(js)
        for r in readings:
            self.assertIn(r["period_year"], known)

    def test_json_has_no_volatility_key(self):
        """JSON 指标卡同样不得出现波动率/行情键。"""
        _, _, js = self._build()
        blob = str(js)
        for banned in ("volatility_annual", "price\"", "\"volume\"", "\"patterns\""):
            self.assertNotIn(banned, blob)

    def test_position_section_flags_meaningless_level_percentile(self):
        """名义总量维度:正文须显式标注水平分位「不具位置信息量」。"""
        _, md, _ = self._build(
            key="cn_m2", name="货币和金融统计（M2）", level_label="亿元",
            level_percentile_meaningful=False)
        self.assertIn("不具位置信息量", md)


class TestQualityGateMacro(unittest.TestCase):
    """quality_gate 的宏观三态 + 反向守卫。"""

    SECTIONS = ["macro_level", "macro_position", "risk"]

    def _js(self, with_period=True):
        m = analyze_macro(_ref(), _series_cpi(24), date(2026, 9, 17))
        risk = analyze_macro_risk(m, date(2026, 9, 17))
        js = generate_json(
            symbol="macro:cn_cpi", as_of=date(2026, 9, 17),
            price_metrics=None, volume_metrics=None,
            risk_metrics=risk, macro_metrics=m,
            sections=self.SECTIONS,
        )
        if not with_period:
            js["macro"]["period"] = {}
        js["evidence"] = [{"section": s, "type": "macro"} for s in self.SECTIONS]
        return js

    def test_gate_passes_for_macro(self):
        gate = run_quality_gate(self.SECTIONS, self._js(), self._js()["evidence"],
                                lint_passed=True, lint_issue_count=0)
        self.assertTrue(gate.passed, gate.issues)

    def test_time_boundary_from_macro_period(self):
        """宏观无 price/industry_index/financial —— 边界落在 macro.period。"""
        gate = run_quality_gate(self.SECTIONS, self._js(), self._js()["evidence"],
                                lint_passed=True, lint_issue_count=0)
        check = next(c for c in gate.checks if c.name == "time_boundary")
        self.assertTrue(check.passed)

    def test_time_boundary_fails_without_macro_period(self):
        """反向守卫:缺 period 块必须 FAIL,不能因「有 as_of」就放过。"""
        js = self._js(with_period=False)
        gate = run_quality_gate(self.SECTIONS, js, self._js()["evidence"],
                                lint_passed=True, lint_issue_count=0)
        check = next(c for c in gate.checks if c.name == "time_boundary")
        self.assertFalse(check.passed)

    def test_section_to_key_maps_macro(self):
        """macro_level / macro_position 两节同源于 `macro` 键(覆盖率才对)。"""
        js = self._js()
        js.pop("macro", None)  # 抽掉卡 → 覆盖率应掉到 1/3
        gate = run_quality_gate(self.SECTIONS, js, self._js()["evidence"],
                                lint_passed=True, lint_issue_count=0)
        check = next(c for c in gate.checks if c.name == "data_completeness")
        self.assertFalse(check.passed)
        self.assertEqual(sorted(check.meta["missing"]),
                         ["macro_level", "macro_position"])


class TestProviderIntegration(unittest.TestCase):
    """provider ↔ 分析层:维度表键即 Go 规范主体码后缀。"""

    def test_known_keys_match_run_codes(self):
        p = MacroProvider()
        self.assertEqual(p.known_keys(), ["cn_cpi", "cn_gdp", "cn_m2", "cn_ppi"])

    def test_unknown_dimension_returns_none(self):
        self.assertIsNone(MacroProvider().resolve("cn_housing"))


if __name__ == "__main__":
    unittest.main()
