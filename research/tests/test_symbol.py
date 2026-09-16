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


if __name__ == "__main__":
    unittest.main()
