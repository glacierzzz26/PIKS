"""test_quality_gate.py — M6 Quality Gate 机检规则单元测试

每条机检规则都必须是机器可判定的（设计文档 M6 v1.1）。
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.quality_gate import (
    run_quality_gate,
    fingerprint_report,
    extract_numbers,
)


def _full_report():
    """构造一份 6 项全过的 JSON 报告样例"""
    return {
        "meta": {"symbol": "sh600519", "as_of": "2026-09-10", "data_source": "tencent"},
        "price": {
            "period_days": 60,
            "start_price": 1240.0,
            "end_price": 1290.88,
            "period_return_pct": 4.1,
        },
        "volume": {"avg_volume_20d": 28000.0, "avg_turnover_20d": 0.25},
        "financial": {
            "latest_roe": 17.72,
            "latest_revenue_yoy": 1.47,
        },
        "events": {"total_news": 12, "earnings_mentions": 2},
        "risk": {
            "overall_level": "LOW",
            "items": [{"type": "liquidity", "level": "LOW"}],
        },
        "scorecard": {
            "overall": 1,
            "overall_label": "中性",
            "dimensions": [
                {
                    "dimension": "market_trend",
                    "score": 0,
                    "reason": "区间上涨 4.1%，趋势平稳",
                }
            ],
        },
        "evidence": [
            {"id": "ev1", "type": "fact", "section": "price", "statement": "p"},
            {"id": "ev2", "type": "fact", "section": "volume", "statement": "v"},
            {"id": "ev3", "type": "fact", "section": "financial", "statement": "f"},
            {"id": "ev4", "type": "fact", "section": "events", "statement": "e"},
            {"id": "ev5", "type": "fact", "section": "risk", "statement": "r"},
        ],
    }


SECTION_FULL = ["market", "volume", "turnover", "financial", "valuation", "events", "risk", "conclusion"]


class TestDataCompleteness(unittest.TestCase):
    def test_full_coverage(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), _full_report()["evidence"],
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "data_completeness"][0]
        self.assertTrue(c.passed)
        self.assertGreaterEqual(c.meta["coverage"], 0.8)

    def test_missing_section_fails(self):
        report = _full_report()
        report.pop("risk")
        report.pop("events")
        gate = run_quality_gate(
            SECTION_FULL, report, [], lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "data_completeness"][0]
        self.assertFalse(c.passed)
        self.assertIn("risk", c.meta["missing"])


class TestEvidenceCompleteness(unittest.TestCase):
    def test_all_sections_covered(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), _full_report()["evidence"],
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "evidence_completeness"][0]
        self.assertTrue(c.passed)

    def test_missing_section_evidence(self):
        evidence = _full_report()["evidence"][:-1]  # 去掉 risk
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), evidence,
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "evidence_completeness"][0]
        self.assertFalse(c.passed)
        self.assertIn("risk", c.meta["missing_sections"])

    def test_no_evidence_fails(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), None,
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "evidence_completeness"][0]
        self.assertFalse(c.passed)


class TestCitationCorrectness(unittest.TestCase):
    def test_reason_numbers_traceable(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), _full_report()["evidence"],
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "citation_correctness"][0]
        self.assertTrue(c.passed)

    def test_fabricated_reason_caught(self):
        report = _full_report()
        report["scorecard"]["dimensions"][0]["reason"] = "目标价 2000 元"
        gate = run_quality_gate(
            SECTION_FULL, report, report["evidence"],
            lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "citation_correctness"][0]
        self.assertFalse(c.passed)


class TestTimeBoundary(unittest.TestCase):
    def test_complete(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), [], lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "time_boundary"][0]
        self.assertTrue(c.passed)

    def test_missing_as_of(self):
        report = _full_report()
        report.pop("meta")
        gate = run_quality_gate(
            SECTION_FULL, report, [], lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "time_boundary"][0]
        self.assertFalse(c.passed)


class TestNumberLint(unittest.TestCase):
    def test_passed(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), [], lint_passed=True, lint_issue_count=0,
        )
        c = [x for x in gate.checks if x.name == "number_lint"][0]
        self.assertTrue(c.passed)

    def test_failed(self):
        gate = run_quality_gate(
            SECTION_FULL, _full_report(), [], lint_passed=False, lint_issue_count=3,
        )
        c = [x for x in gate.checks if x.name == "number_lint"][0]
        self.assertFalse(c.passed)


class TestReproducibility(unittest.TestCase):
    def test_consistent_fingerprint(self):
        report = _full_report()
        fp = fingerprint_report(report)
        gate = run_quality_gate(
            SECTION_FULL, report, [], lint_passed=True, lint_issue_count=0,
            stored_fingerprint=fp, computed_fingerprint=fp,
        )
        c = [x for x in gate.checks if x.name == "reproducibility"][0]
        self.assertTrue(c.passed)

    def test_inconsistent_fingerprint(self):
        report = _full_report()
        fp_a = fingerprint_report(report)
        report["price"]["end_price"] = 999.0
        fp_b = fingerprint_report(report)
        gate = run_quality_gate(
            SECTION_FULL, report, [], lint_passed=True, lint_issue_count=0,
            stored_fingerprint=fp_a, computed_fingerprint=fp_b,
        )
        c = [x for x in gate.checks if x.name == "reproducibility"][0]
        self.assertFalse(c.passed)

    def test_fingerprint_deterministic(self):
        report = _full_report()
        self.assertEqual(fingerprint_report(report), fingerprint_report(report))


class TestExtractNumbers(unittest.TestCase):
    def test_nested_dict(self):
        nums = extract_numbers({"a": 1.5, "b": {"c": 2}})
        self.assertIn((1.5, ""), nums)
        self.assertIn((2.0, ""), nums)

    def test_bool_excluded(self):
        nums = extract_numbers({"a": True, "b": 3})
        self.assertNotIn((1.0, ""), nums)
        self.assertIn((3.0, ""), nums)


if __name__ == "__main__":
    unittest.main()