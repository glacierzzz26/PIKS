"""test_number_lint.py — Number Lint 单位归一化 + 模板常量精确匹配

回归两个真实缺陷（生产 601118 complete-stock 因之降级为骨架）：
1. LLM 把「1861653.75 手」写成「186.17万手」——正确换算却被判「编造」。
2. 模板结构常量带 2% 相对容差 —— 262.01 会误命中「260」。
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.number_lint import lint_text


class TestUnitNormalization(unittest.TestCase):
    def test_wan_restatement_matches(self):
        # 卡片 1861653.75 手 → 文本「186.17万手」应可溯源
        rep = lint_text("20日均量约186.17万手。", known_values={1861653.75})
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])

    def test_yi_restatement_matches(self):
        # 卡片 2438783390 元 → 文本「24.39亿元」应可溯源
        rep = lint_text("区间成交额约24.39亿元。", known_values={2438783390.0})
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])

    def test_fabricated_number_still_flagged(self):
        rep = lint_text("区间涨幅 37.5%。", known_values={10.0, 20.0})
        self.assertFalse(rep.passed)
        self.assertEqual(rep.issues[0].value, 37.5)

    def test_unit_scaled_fabrication_still_flagged(self):
        # 37.5万 = 375000 仍不在基准里 → 不得因单位还原而放行
        rep = lint_text("成交 37.5万手。", known_values={10.0})
        self.assertFalse(rep.passed)


class TestTemplateConstants(unittest.TestCase):
    def test_exact_window_constant_passes(self):
        rep = lint_text("近 20 个交易日。", known_values=set())
        self.assertTrue(rep.passed)

    def test_near_constant_not_silently_passed(self):
        # 262.01 曾落进「260」的 2% 容差带被误放行 → 现应判为不可溯源
        rep = lint_text("收益率 262.01%。", known_values={1.0})
        self.assertFalse(rep.passed)

class TestRealDataValueTraceable(unittest.TestCase):
    def test_real_data_value_traceable(self):
        rep = lint_text("最新收盘价 6.78 元。", known_values={6.78})
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])


class TestCodeMaskingDoesNotEatDecimals(unittest.TestCase):
    """股票代码掩码不得吞掉小数的整数部分（生产 macro:cn_gdp 实测缺陷）。

    `_CODE_RE` 原为 `(?<!\\d)\\d{6}(?!\\d)`，`695704.0` 的整数部分形如 6 位码，
    被整段掩成 `CODE.0`，残留的 `.0` 被当数据数字扫描 → GDP/亿元级读数必挂。
    生产实证：`macro:cn_gdp` 报告 5 个问题全部形如「水平为CODE.0亿元」。
    """

    def test_six_digit_decimal_is_not_masked(self):
        # GDP 亿元级水平：6 位整数 + 小数 —— 修复前扫成 '0' 判不可溯源
        rep = lint_text(
            "最新累计水平为695704.0亿元。", known_values={695704.0}
        )
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])
        self.assertEqual(rep.scanned, 1)

    def test_seven_digit_decimal_still_matches(self):
        rep = lint_text("累计水平1401879.2亿元。", known_values={1401879.2})
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])

    def test_stock_code_still_masked(self):
        # 回归守卫：真·6 位代码仍须掩掉，不得因放宽而把它当数据扫描
        rep = lint_text("标的 000560 今日放量。", known_values=set())
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])
        self.assertEqual(rep.scanned, 0)

    def test_stock_code_with_exchange_prefix_still_masked(self):
        rep = lint_text("标的 sz000560 今日放量。", known_values=set())
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])


if __name__ == "__main__":
    unittest.main()
