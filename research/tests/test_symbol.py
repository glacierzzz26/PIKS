"""test_symbol.py — 主体码解析回归(P9 主体轴泛化 / issue #10)

覆盖:
- 股票解析逐字节不变(零回归)
- 申万行业码 sw+6 位 → Market.SI,**绝不**落到 BJ(否则套 30% 涨跌停)
- 行业指数无涨跌停制度 → limit_up/down = 0
- 裸申万码(无 sw 前缀)仍是既有 BJ 行为(记录地雷:调用方必须带前缀)
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.models.symbol import Market, resolve_symbol


class TestStockRegression(unittest.TestCase):
    """股票解析零回归:既有行为逐字节不变。"""

    def test_stock_markets(self):
        cases = {
            "600519": (Market.SH, "sh600519"),
            "sh600519": (Market.SH, "sh600519"),
            "000560": (Market.SZ, "sz000560"),
            "300750": (Market.SZ, "sz300750"),
            "430047": (Market.BJ, "bj430047"),
        }
        for raw, (market, full) in cases.items():
            sym = resolve_symbol(raw)
            self.assertEqual(sym.market, market, raw)
            self.assertEqual(sym.full_code, full, raw)

    def test_stock_limits_unchanged(self):
        self.assertEqual(resolve_symbol("600519").limit_up_pct, 10.0)
        self.assertEqual(resolve_symbol("300750").limit_up_pct, 20.0)
        self.assertEqual(resolve_symbol("430047").limit_up_pct, 30.0)
        self.assertEqual(resolve_symbol("600519").limit_down_pct, -10.0)


class TestIndustrySubject(unittest.TestCase):
    """行业主体:sw + 6 位申万码。"""

    def test_industry_resolves_to_si(self):
        for raw, code in [("sw801010", "801010"), ("sw851251", "851251")]:
            sym = resolve_symbol(raw)
            self.assertEqual(sym.market, Market.SI, raw)
            self.assertEqual(sym.code, code, raw)
            self.assertEqual(sym.full_code, "sw" + code, raw)

    def test_industry_is_not_bj(self):
        """核心回归:801010 开头是 8,旧逻辑会判 BJ 并给 30% 涨跌停。"""
        sym = resolve_symbol("sw801010")
        self.assertNotEqual(sym.market, Market.BJ)
        self.assertEqual(sym.limit_up_pct, 0.0)
        self.assertEqual(sym.limit_down_pct, 0.0)

    def test_industry_case_insensitive(self):
        self.assertEqual(resolve_symbol("SW801010").market, Market.SI)
        self.assertEqual(resolve_symbol("  sw801010  ").full_code, "sw801010")

    def test_bare_code_still_bj_landmine(self):
        """记录地雷:裸申万码无前缀 → 既有 BJ 行为。调用方必须带 sw。"""
        sym = resolve_symbol("801010")
        self.assertEqual(sym.market, Market.BJ)
        self.assertEqual(sym.full_code, "bj801010")

    def test_bad_sw_forms_fall_through(self):
        """位数不符的 sw 码不按行业解析(落回既有 unknown/bj 逻辑,不静默当行业)。"""
        self.assertNotEqual(resolve_symbol("sw80101").market, Market.SI)
        self.assertNotEqual(resolve_symbol("sw8010101").market, Market.SI)


class TestMacroSubject(unittest.TestCase):
    """宏观主体(P9-5 / #13):macro:<key>。

    核心不变量:`full_code` 必须**逐字节**等于 Go 侧
    `store.NormalizeSubject` 的规范主体码 —— 主体码在 Python 与 Go 两侧
    必须是同一个字符串,否则 run_id / 产物名 / 前端展示各拿各的码。
    """

    def test_macro_resolves(self):
        for raw, key in [("macro:cn_cpi", "cn_cpi"), ("macro:cn_gdp", "cn_gdp"),
                         ("macro:cn_m2", "cn_m2"), ("macro:cn_ppi", "cn_ppi")]:
            sym = resolve_symbol(raw)
            self.assertEqual(sym.market, Market.MACRO, raw)
            self.assertEqual(sym.code, key, raw)
            # Go 规范码 = "macro:" + key(见 internal/store/research_runs.go)
            self.assertEqual(sym.full_code, "macro:" + key, raw)

    def test_full_code_matches_go_canonical(self):
        """Python full_code == Go NormalizeSubject 的规范码(逐字节)。"""
        go_canonical = {
            "macro:cn_cpi": "macro:cn_cpi",
            "MACRO:CN_CPI": "macro:cn_cpi",
            "  macro:cn_cpi  ": "macro:cn_cpi",
        }
        for raw, want in go_canonical.items():
            self.assertEqual(resolve_symbol(raw).full_code, want, raw)

    def test_macro_is_not_unknown(self):
        """回归:改造前 `resolve_symbol("macro:cn_cpi")` 产出错码 unknownmacro:cn_cpi。"""
        self.assertNotEqual(resolve_symbol("macro:cn_cpi").market, Market.UNKNOWN)
        self.assertNotEqual(resolve_symbol("macro:cn_cpi").full_code,
                            "unknownmacro:cn_cpi")

    def test_macro_has_no_limit(self):
        """宏观指标无涨跌停制度。"""
        sym = resolve_symbol("macro:cn_cpi")
        self.assertEqual(sym.limit_up_pct, 0.0)
        self.assertEqual(sym.limit_down_pct, 0.0)

    def test_case_and_whitespace_insensitive(self):
        self.assertEqual(resolve_symbol("MACRO:CN_CPI").full_code, "macro:cn_cpi")
        self.assertEqual(resolve_symbol("  macro:cn_cpi  ").full_code, "macro:cn_cpi")

    def test_key_with_inner_colon_preserved(self):
        """key 只做非空判别,不校验合法性(D-M2:维度表归 Python provider)。

        `macro:china:cpi` 会解析成 key="china:cpi" —— 形态合法但**维度表查不到**
        ⇒ provider.resolve 返回 None ⇒ collect 抛错、run 如实 failed。
        此处锁住「解析层不越权扮演 provider」这个边界。
        """
        sym = resolve_symbol("macro:china:cpi")
        self.assertEqual(sym.market, Market.MACRO)
        self.assertEqual(sym.code, "china:cpi")
        self.assertEqual(sym.full_code, "macro:china:cpi")

    def test_bare_macro_prefix_is_not_macro(self):
        """`macro:` / `macro` 空 key 不按宏观解析(不产生空 key 主体)。"""
        self.assertNotEqual(resolve_symbol("macro:").market, Market.MACRO)
        self.assertNotEqual(resolve_symbol("macro").market, Market.MACRO)

    def test_stock_and_industry_untouched(self):
        """零回归:宏观分支不改变股票/行业的既有解析。"""
        self.assertEqual(resolve_symbol("600519").full_code, "sh600519")
        self.assertEqual(resolve_symbol("sw801010").full_code, "sw801010")
        # 6 位纯数字仍是股票,不会被误当宏观 key
        self.assertEqual(resolve_symbol("000560").market, Market.SZ)


if __name__ == "__main__":
    unittest.main()
