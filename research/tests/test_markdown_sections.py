"""test_markdown_sections.py — Markdown 报告按 Profile sections 裁剪 + 表达模式回归

覆盖：sections 参数决定渲染章节；express 简版不含 full 章节；
AI synthesis 槽位保留；中文日期掩码。
"""
import os
import re
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.report.markdown import generate_markdown
from src.report.json_report import generate_json
from src.report.synthesis import render_synthesis
from src.analysis.price import PriceMetrics
from src.analysis.volume import VolumeMetrics
from src.analysis.number_lint import lint_text


def _price() -> PriceMetrics:
    return PriceMetrics(
        symbol="sh600519", as_of=date(2026, 9, 10), period_days=20,
        start_price=100.0, end_price=110.0, max_price=115.0, min_price=95.0,
        return_pct_5d=1.0, return_pct_10d=2.0, return_pct_20d=5.0,
        return_pct_60d=None, period_return_pct=10.0, max_drawdown_pct=-8.0,
        max_rise_pct=9.0, volatility_annual=25.0, limit_up_days=0, limit_down_days=0,
    )


def _volume() -> VolumeMetrics:
    return VolumeMetrics(
        symbol="sh600519", as_of=date(2026, 9, 10), period_days=20,
        total_volume=100000, avg_volume_5d=5000.0, avg_volume_20d=5000.0,
        avg_volume_60d=None, total_amount=1e8, avg_amount_5d=5e6,
        avg_amount_20d=5e6, avg_turnover_5d=0.5, avg_turnover_20d=0.4,
        avg_turnover_60d=None, max_turnover=1.2, min_turnover=0.1,
        abnormal_volume_days=0, abnormal_volume_dates=[], price_volume_corr=0.3,
    )


def _sections(md: str) -> list:
    return re.findall(r"^## [一二三四五六七八九十]+、.+$", md, re.M)


class TestSectionClipping(unittest.TestCase):
    def test_full_sections_all_rendered(self):
        md = generate_markdown(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            sections=["market", "volume", "turnover", "financial", "valuation",
                      "events", "announcements", "capital", "risk", "conclusion"],
        )
        secs = _sections(md)
        # 9 = 执行摘要(D-R6 前置,恒在) + 7 数据章 + 数据说明(D-R6 置尾,恒在)
        self.assertEqual(len(secs), 9)
        self.assertTrue(any("执行摘要" in s for s in secs))
        self.assertTrue(any("股价表现" in s for s in secs))
        self.assertTrue(any("成交量" in s for s in secs))
        self.assertTrue(any("基本面" in s for s in secs))
        self.assertTrue(any("资金面" in s for s in secs))
        self.assertTrue(any("风险" in s for s in secs))
        self.assertTrue(any("评分卡" in s for s in secs))

    def test_express_excludes_full_sections(self):
        # express：仅行情 / 量价 / 换手 / 近期事件 + 简版结论
        md = generate_markdown(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            sections=["market", "volume", "turnover", "events", "announcements", "conclusion"],
        )
        self.assertNotIn("基本面", md)
        self.assertNotIn("资金面", md)
        self.assertNotIn("风险分析", md)
        self.assertIn("股价表现", md)
        self.assertIn("成交量", md)
        self.assertIn("近期事件", md)

    def test_only_market_renders(self):
        md = generate_markdown("sh600519", date(2026, 9, 10), _price(), _volume(),
                               sections=["market"])
        secs = _sections(md)
        # 3 = 执行摘要(恒在) + 股价表现 + 数据说明(恒在)
        self.assertEqual(len(secs), 3)
        self.assertNotIn("成交量", md)

    def test_ai_synthesis_slot_preserved(self):
        md = generate_markdown("sh600519", date(2026, 9, 10), _price(), _volume(),
                               sections=["market"])
        # 双花括号（普通字符串）或 f-string 渲染后的单括号形式都应保留
        ok = "{ai_synthesis}" in md or "{{ai_synthesis}}" in md
        self.assertTrue(ok)
        out = render_synthesis(md, {"summary": "S", "trend": "T", "conclusion": "C"})
        # 摘要前置(D-R6):AI 三子段注入在首章,不再另起「## 九、AI 综合研判」。
        self.assertIn("### 执行摘要", out)
        self.assertNotIn("## 九、AI 综合研判", out)

    def test_summary_first_disclaimer_last(self):
        """D-R6:执行摘要在正文之首、免责在之尾 —— 顺序不得再颠倒。"""
        md = generate_markdown("sh600519", date(2026, 9, 10), _price(), _volume(),
                               sections=["market", "conclusion"])
        secs = _sections(md)
        self.assertIn("执行摘要", secs[0])
        self.assertIn("数据说明与免责声明", secs[-1])

    def test_conclusion_renders_scorecard_placeholder(self):
        md = generate_markdown("sh600519", date(2026, 9, 10), _price(), _volume(),
                               sections=["conclusion"], scorecard=None)
        self.assertIn("评分卡", md)


class TestSectionManifest(unittest.TestCase):
    """D-R8:章节清单是 TOC/三域标记的单一真源,须与正文 `## ` 标题一一对应。"""

    def test_manifest_matches_headings_and_order(self):
        sections = ["market", "volume", "turnover", "risk", "conclusion"]
        md = generate_markdown("sh600519", date(2026, 9, 10), _price(), _volume(),
                               sections=sections)
        js = generate_json("sh600519", date(2026, 9, 10), _price(), _volume(),
                           sections=sections)
        manifest = js["meta"]["section_manifest"]
        titles = [m["title"] for m in manifest]
        # 正文标题(去掉「一、」序号前缀) === 清单标题,且次序一致
        headings = [re.sub(r"^## [一二三四五六七八九十]+、", "", h) for h in _sections(md)]
        self.assertEqual(titles, headings)

    def test_domains_declared(self):
        js = generate_json("sh600519", date(2026, 9, 10), _price(), _volume(),
                           sections=["market", "risk"])
        by_title = {m["title"]: m["domains"] for m in js["meta"]["section_manifest"]}
        self.assertEqual(by_title["执行摘要"], ["opinion"])
        self.assertEqual(by_title["股价表现"], ["fact", "calc"])
        self.assertEqual(by_title["数据说明与免责声明"], [])

    def test_manifest_default_when_sections_omitted(self):
        js = generate_json("sh600519", date(2026, 9, 10), _price(), _volume())
        titles = [m["title"] for m in js["meta"]["section_manifest"]]
        self.assertTrue(titles[0] == "执行摘要" and titles[-1] == "数据说明与免责声明")


class TestChineseDateMasking(unittest.TestCase):
    def test_cn_date_masked(self):
        # 「9月8日」中的 9/8 是日期组成部分，不应被视为数据数值
        lint = lint_text("事件发生在9月8日，之后股价下跌。", known_values={})
        self.assertEqual(lint.issues, [])

    def test_cn_date_with_numbers_still_checked(self):
        # 日期外的真实数字仍被扫描
        lint = lint_text("9月8日调价后跌1.41%", known_values={1.41})
        self.assertEqual(lint.issues, [])

    def test_iso_date_masked(self):
        lint = lint_text("2026-09-08 事件日", known_values={})
        self.assertEqual(lint.issues, [])


if __name__ == "__main__":
    unittest.main()