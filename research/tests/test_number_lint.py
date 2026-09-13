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

    def test_real_data_value_traceable(self):
        rep = lint_text("最新收盘价 6.78 元。", known_values={6.78})
        self.assertTrue(rep.passed, [str(i) for i in rep.issues])


if __name__ == "__main__":
    unittest.main()
