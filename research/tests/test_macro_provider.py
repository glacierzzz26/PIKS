"""test_macro_provider.py — 宏观 Provider 单元测试(P9-5 / issue #13)

不联网:全部用 fixture 打桩 akshare 的**已解析 DataFrame**(即 akshare 返回的
中文列名形态),锁住四条硬约束:

1. 期号解析(含两类季度形态)—— 解析失败即抛,不静默丢期
2. 升序化 + 单调断言 —— 源站是新→旧,沿用旧序会拿 2008 年当「最新」
3. GDP 累计差分 —— 单季水平对,且不做单季同比
4. 不使用 `*_yearly` 接口 —— 那批实测陈旧一年
"""
import os
import sys
import unittest
from datetime import date
from unittest.mock import patch

import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.providers.macro.macro_provider import (
    MacroProvider,
    parse_period,
)


# ---- fixtures:真实 akshare 返回结构(取自 2026-09-17 实测) ----

def _cpi_df() -> pd.DataFrame:
    """macro_china_cpi() 形态:13 列,含城市/农村分项。新→旧序(与源站一致)。"""
    return pd.DataFrame([
        {"月份": "2026年08月份", "全国-当月": 100.8, "全国-同比增长": 0.8,
         "全国-环比增长": 0.4, "全国-累计": 100.9, "城市-当月": 100.8,
         "城市-同比增长": 0.8, "城市-环比增长": 0.4, "城市-累计": 100.9,
         "农村-当月": 100.7, "农村-同比增长": 0.7, "农村-环比增长": 0.4,
         "农村-累计": 100.7},
        {"月份": "2026年07月份", "全国-当月": 100.5, "全国-同比增长": 0.5,
         "全国-环比增长": -0.1, "全国-累计": 100.9, "城市-当月": 100.5,
         "城市-同比增长": 0.5, "城市-环比增长": -0.1, "城市-累计": 101.0,
         "农村-当月": 100.4, "农村-同比增长": 0.4, "农村-环比增长": -0.2,
         "农村-累计": 100.7},
        {"月份": "2026年06月份", "全国-当月": 101.0, "全国-同比增长": 1.0,
         "全国-环比增长": -0.3, "全国-累计": 101.0, "城市-当月": 101.0,
         "城市-同比增长": 1.0, "城市-环比增长": -0.4, "城市-累计": 101.0,
         "农村-当月": 100.8, "农村-同比增长": 0.8, "农村-环比增长": -0.3,
         "农村-累计": 100.8},
    ])


def _ppi_df() -> pd.DataFrame:
    """macro_china_ppi() 形态:仅 4 列,**无环比**。"""
    return pd.DataFrame([
        {"月份": "2026年08月份", "当月": 103.8, "当月同比增长": 3.8, "累计": 102.0},
        {"月份": "2026年07月份", "当月": 103.5, "当月同比增长": 3.5, "累计": 101.8},
        {"月份": "2026年06月份", "当月": 104.1, "当月同比增长": 4.1, "累计": 101.5},
    ])


def _m2_df() -> pd.DataFrame:
    """macro_china_money_supply() 形态:10 列,M2/M1/M0 各 3 个数。"""
    return pd.DataFrame([
        {"月份": "2026年08月份", "货币和准货币(M2)-数量(亿元)": 3568083.60,
         "货币和准货币(M2)-同比增长": 7.5, "货币和准货币(M2)-环比增长": 0.365853,
         "货币(M1)-数量(亿元)": 1157741.43, "货币(M1)-同比增长": 4.1,
         "货币(M1)-环比增长": 0.270082, "流通中的现金(M0)-数量(亿元)": 148311.98,
         "流通中的现金(M0)-同比增长": 11.2, "流通中的现金(M0)-环比增长": 0.073629},
        {"月份": "2026年07月份", "货币和准货币(M2)-数量(亿元)": 3555077.24,
         "货币和准货币(M2)-同比增长": 7.7, "货币和准货币(M2)-环比增长": -0.337281,
         "货币(M1)-数量(亿元)": 1154623.00, "货币(M1)-同比增长": 4.0,
         "货币(M1)-环比增长": -2.544999, "流通中的现金(M0)-数量(亿元)": 148202.86,
         "流通中的现金(M0)-同比增长": 11.6, "流通中的现金(M0)-环比增长": 0.568704},
    ])


def _gdp_df() -> pd.DataFrame:
    """macro_china_gdp() 形态:季度列有**两种**标签(累计区间 / 单季)。新→旧序。

    含跨年边界(2025 Q1-4 → 2026 Q1),用于锁「差分只在同年内」。
    """
    return pd.DataFrame([
        {"季度": "2026年第1-2季度", "国内生产总值-绝对值": 695704.0,
         "国内生产总值-同比增长": 4.7, "第一产业-绝对值": 31521.8,
         "第一产业-同比增长": 3.7, "第二产业-绝对值": 250472.9,
         "第二产业-同比增长": 3.9, "第三产业-绝对值": 413709.2,
         "第三产业-同比增长": 5.2},
        {"季度": "2026年第1季度", "国内生产总值-绝对值": 334192.9,
         "国内生产总值-同比增长": 5.0, "第一产业-绝对值": 11940.8,
         "第一产业-同比增长": 3.8, "第二产业-绝对值": 116134.9,
         "第二产业-同比增长": 4.9, "第三产业-绝对值": 206117.2,
         "第三产业-同比增长": 5.2},
        {"季度": "2025年第1-4季度", "国内生产总值-绝对值": 1401879.2,
         "国内生产总值-同比增长": 5.0, "第一产业-绝对值": 93346.8,
         "第一产业-同比增长": 3.9, "第二产业-绝对值": 499653.0,
         "第二产业-同比增长": 4.5, "第三产业-绝对值": 808879.3,
         "第三产业-同比增长": 5.4},
        {"季度": "2025年第1-3季度", "国内生产总值-绝对值": 1013967.9,
         "国内生产总值-同比增长": 5.2, "第一产业-绝对值": 68017.6,
         "第一产业-同比增长": 3.7, "第二产业-绝对值": 364773.8,
         "第二产业-同比增长": 4.8, "第三产业-绝对值": 581176.5,
         "第三产业-同比增长": 5.5},
        {"季度": "2025年第1季度", "国内生产总值-绝对值": 318466.4,
         "国内生产总值-同比增长": 5.4, "第一产业-绝对值": 11713.3,
         "第一产业-同比增长": 3.5, "第二产业-绝对值": 111335.8,
         "第二产业-同比增长": 5.9, "第三产业-绝对值": 195417.3,
         "第三产业-同比增长": 5.3},
    ])


_FIXTURES = {
    "macro_china_cpi": _cpi_df,
    "macro_china_ppi": _ppi_df,
    "macro_china_money_supply": _m2_df,
    "macro_china_gdp": _gdp_df,
}


def _provider_with(fn_name: str) -> MacroProvider:
    """打桩指定 akshare 接口;其余接口返回空(任何意外调用都会炸出来)。"""
    def fake(name):
        def _call():
            if name == fn_name:
                return _FIXTURES[name]()
            raise AssertionError(f"未预期的 akshare 调用: {name}")
        return _call
    return MacroProvider()


def _series(fn_name: str, key: str):
    ref = MacroProvider().resolve(key)
    with patch(f"akshare.{fn_name}", _FIXTURES[fn_name]):
        return MacroProvider().get_series(ref)


class TestParsePeriod(unittest.TestCase):
    """期号解析:两类月度/季度形态;失败即抛,不静默丢期。"""

    def test_month_label(self):
        p = parse_period("2026年08月份")
        self.assertEqual(p.year, 2026)
        self.assertEqual(p.month, 8)
        self.assertEqual(p.end, date(2026, 8, 31))
        self.assertIsNone(p.quarter_to)
        self.assertFalse(p.cumulative_span)

    def test_month_single_digit(self):
        """源站零填充,但解析不依赖填充宽度。"""
        p = parse_period("2008年01月份")
        self.assertEqual((p.year, p.month), (2008, 1))
        self.assertEqual(p.end, date(2008, 1, 31))

    def test_quarter_single(self):
        p = parse_period("2026年第1季度")
        self.assertEqual((p.year, p.quarter_from, p.quarter_to), (2026, 1, 1))
        self.assertEqual(p.end, date(2026, 3, 31))
        self.assertFalse(p.cumulative_span)

    def test_quarter_range(self):
        p = parse_period("2026年第1-2季度")
        self.assertEqual((p.quarter_from, p.quarter_to), (1, 2))
        self.assertEqual(p.end, date(2026, 6, 30))
        self.assertTrue(p.cumulative_span)

    def test_quarter_four(self):
        p = parse_period("2025年第1-4季度")
        self.assertEqual(p.end, date(2025, 12, 31))
        self.assertTrue(p.cumulative_span)

    def test_unparseable_raises(self):
        """**不静默跳过**:静默丢期会让分位数无声偏移。"""
        for bad in ("2026-08", "2026年08月", "2026年第5季度", "", "2026Q1"):
            with self.assertRaises(ValueError, msg=bad):
                parse_period(bad)


class TestResolveDimension(unittest.TestCase):
    """维度定位靠**查表**,未知 key 返回 None(不臆造)。"""

    def test_known_keys(self):
        self.assertEqual(MacroProvider().known_keys(),
                         ["cn_cpi", "cn_gdp", "cn_m2", "cn_ppi"])

    def test_resolve_returns_ref_with_caliber(self):
        ref = MacroProvider().resolve("cn_cpi")
        self.assertEqual(ref.name, "居民消费价格指数（CPI）")
        self.assertEqual(ref.cadence, "month")
        self.assertIn("国家统计局", ref.source)
        self.assertIn("上年同月=100", ref.caliber)

    def test_unknown_key_returns_none(self):
        for bad in ("cn_xxx", "china:cpi", "", "cpi"):
            self.assertIsNone(MacroProvider().resolve(bad), bad)

    def test_key_is_case_insensitive(self):
        self.assertIsNotNone(MacroProvider().resolve("CN_CPI"))


class TestAscendingOrder(unittest.TestCase):
    """源站新→旧 ⇒ 必须升序化。沿用旧序会拿 2008 年当「最新」。"""

    def test_cpi_ascending(self):
        s = _series("macro_china_cpi", "cn_cpi")
        self.assertEqual([p.period.month for p in s.points], [6, 7, 8])
        self.assertEqual(s.latest.period.label, "2026年08月份")
        self.assertEqual(s.history_periods, 3)
        self.assertEqual(s.history_start, date(2026, 6, 30))

    def test_gdp_ascending_by_end_quarter(self):
        """排序键是**期末** —— 按起始季度排会让「第1季度」与「第1-2季度」撞键。"""
        s = _series("macro_china_gdp", "cn_gdp")
        ends = [p.period.end for p in s.points]
        self.assertEqual(ends, sorted(ends))
        self.assertEqual(s.latest.period.label, "2026年第1-2季度")

    def test_monotonic_assertion_raises_on_duplicate_period(self):
        """重复期号 = 源站语义已变 ⇒ 抛,不静默继续(分位数不可信)。"""
        ref = MacroProvider().resolve("cn_cpi")
        dup = pd.concat([_cpi_df(), _cpi_df().head(1)], ignore_index=True)
        with patch("akshare.macro_china_cpi", return_value=dup):
            with self.assertRaises(ValueError) as cm:
                MacroProvider().get_series(ref)
        self.assertIn("非严格递增", str(cm.exception))


class TestCaliberFields(unittest.TestCase):
    """CPI/PPI 的「当月」是**指数**非百分比 —— 字段名与量纲说明都必须体现。"""

    def test_cpi_level_is_index_not_pct(self):
        s = _series("macro_china_cpi", "cn_cpi")
        latest = s.latest
        self.assertAlmostEqual(latest.level, 100.8, places=4)
        self.assertAlmostEqual(latest.yoy, 0.8, places=4)
        # 逐行校验 caliber:|当月 − 100 − 同比| 应为 0(CPI 实测全精确)
        for p in s.points:
            self.assertAlmostEqual(p.level - 100, p.yoy, places=4)

    def test_level_label_says_index(self):
        ref = MacroProvider().resolve("cn_cpi")
        self.assertIn("指数", ref.level_label)
        self.assertNotIn("%", ref.level_label)

    def test_m2_level_is_yi_yuan(self):
        s = _series("macro_china_money_supply", "cn_m2")
        self.assertAlmostEqual(s.latest.level, 3568083.60, places=2)
        self.assertEqual(s.ref.level_label, "亿元")

    def test_nan_becomes_none_not_zero(self):
        """缺失 = 不知道,不是 0。"""
        df = _cpi_df()
        df.loc[0, "全国-同比增长"] = float("nan")
        ref = MacroProvider().resolve("cn_cpi")
        with patch("akshare.macro_china_cpi", return_value=df):
            s = MacroProvider().get_series(ref)
        self.assertIsNone(s.latest.yoy)
        self.assertIsNotNone(s.latest.level)


class TestPpiHasNoMom(unittest.TestCase):
    """PPI 源站不提供环比 —— 字段留 None,**不回退重算**。"""

    def test_mom_is_none(self):
        s = _series("macro_china_ppi", "cn_ppi")
        for p in s.points:
            self.assertIsNone(p.mom, p.period.label)

    def test_caliber_discloses_missing_mom(self):
        ref = MacroProvider().resolve("cn_ppi")
        self.assertIn("不提供环比", ref.caliber)

    def test_yoy_and_level_present(self):
        s = _series("macro_china_ppi", "cn_ppi")
        self.assertAlmostEqual(s.latest.level, 103.8, places=4)
        self.assertAlmostEqual(s.latest.yoy, 3.8, places=4)


class TestGdpDeCumulation(unittest.TestCase):
    """GDP 累计 → 单季水平差分(同年内),且**不做**单季同比。"""

    def test_single_quarter_level(self):
        s = _series("macro_china_gdp", "cn_gdp")
        by_label = {p.period.label: p for p in s.points}
        # 「第1季度」本身即单季
        self.assertAlmostEqual(by_label["2026年第1季度"].single_quarter_level,
                               334192.9, places=1)
        # 「第1-2季度」= 累计 − 前一段累计 = 695704.0 − 334192.9
        self.assertAlmostEqual(by_label["2026年第1-2季度"].single_quarter_level,
                               361511.1, places=1)

    def test_diff_does_not_cross_year(self):
        """跨年差分会把上年 Q4 累计混进来 —— 2026 Q1 必须是自身累计值。"""
        s = _series("macro_china_gdp", "cn_gdp")
        q1 = next(p for p in s.points if p.period.label == "2026年第1季度")
        self.assertAlmostEqual(q1.single_quarter_level, q1.level, places=6)
        self.assertAlmostEqual(q1.level, 334192.9, places=1)   # 不是 334192.9 − 1401879.2

    def test_all_single_quarter_levels_positive(self):
        """差分正确性的量级检查:任何单季 GDP 都不该是负数。"""
        s = _series("macro_china_gdp", "cn_gdp")
        vals = [p.single_quarter_level for p in s.points if p.single_quarter_level is not None]
        self.assertTrue(vals)
        self.assertTrue(all(v > 0 for v in vals), f"出现非正单季水平: {vals}")

    def test_no_single_quarter_yoy_field(self):
        """禁止差分出单季同比 —— 结构上就没有这个字段(MacroPoint 无此属性)。"""
        s = _series("macro_china_gdp", "cn_gdp")
        for p in s.points:
            self.assertFalse(hasattr(p, "single_quarter_yoy"))

    def test_cumulative_flag_and_caliber_disclosure(self):
        s = _series("macro_china_gdp", "cn_gdp")
        self.assertTrue(all(p.cumulative for p in s.points))
        ref = s.ref
        self.assertIn("累计", ref.caliber)
        self.assertIn("非原始披露值", ref.caliber)


class TestNoYearlyFallback(unittest.TestCase):
    """**结构性**杜绝回退到 `*_yearly` —— 那批实测陈旧一年(设计文档 §2.1)。"""

    def test_no_ak_name_contains_yearly(self):
        from src.providers.macro import macro_provider as mod
        for key, spec in mod._DIMENSIONS.items():
            self.assertNotIn("_yearly", spec.ak_name, key)

    def test_no_source_references_yearly(self):
        """源码不得提及 `*_yearly` 接口名(注释里的历史说明除外,故按调用形态查)。"""
        from src.providers.macro import macro_provider as mod
        import inspect
        src = inspect.getsource(mod)
        for lib in ("macro_china_cpi_yearly", "macro_china_m2_yearly",
                    "macro_china_gdp_yearly"):
            # 只允许出现在 docstring 禁则里(即紧跟「不可用/陈旧」语境),不得是调用
            self.assertNotIn(f"ak.{lib}", src)
            self.assertNotIn(f'"{lib}"', src)


class TestMissingColumnRaises(unittest.TestCase):
    """源站结构变了 → 显式报错(提示重新 Spike),而不是产出空序列。"""

    def test_missing_label_column(self):
        ref = MacroProvider().resolve("cn_cpi")
        with patch("akshare.macro_china_cpi", return_value=pd.DataFrame([{"x": 1}])):
            with self.assertRaises(ValueError) as cm:
                MacroProvider().get_series(ref)
        self.assertIn("缺期号列", str(cm.exception))

    def test_empty_frame_raises(self):
        ref = MacroProvider().resolve("cn_cpi")
        with patch("akshare.macro_china_cpi", return_value=pd.DataFrame()):
            with self.assertRaises(ValueError) as cm:
                MacroProvider().get_series(ref)
        self.assertIn("无数据", str(cm.exception))


if __name__ == "__main__":
    unittest.main()
