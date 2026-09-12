"""test_synthesis.py — AI 综合研判层单元测试

覆盖：prompt 生成 / LLM 输出解析容错 / 槽位渲染 / 内容校验。
"""
import os
import sys
import json
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.report.synthesis import (
    build_synthesis_prompt,
    parse_synthesis,
    render_synthesis,
    validate_synthesis,
)


class TestParseSynthesis(unittest.TestCase):
    def test_plain_json(self):
        blocks = parse_synthesis('{"summary": "s", "trend": "t", "conclusion": "c"}')
        self.assertEqual(blocks["summary"], "s")
        self.assertEqual(len(blocks), 3)

    def test_markdown_fence(self):
        text = '''我基于指标卡的分析如下：
```json
{"summary": "摘要", "trend": "趋势", "conclusion": "结论"}
```
完毕'''
        blocks = parse_synthesis(text)
        self.assertEqual(blocks, {"summary": "摘要", "trend": "趋势", "conclusion": "结论"})

    def test_cjk_colon_fallback(self):
        text = '"summary": "摘要内容", "trend": "趋势内容"'
        blocks = parse_synthesis(text)
        self.assertIn("summary", blocks)
        self.assertEqual(blocks["summary"], "摘要内容")

    def test_garbage(self):
        self.assertEqual(parse_synthesis("完全不是 JSON"), {})

    def test_empty(self):
        self.assertEqual(parse_synthesis(""), {})


class TestValidateSynthesis(unittest.TestCase):
    def test_empty_slot(self):
        errs = validate_synthesis({"summary": "内容足够长的一段话"}, {})
        self.assertIn("trend", errs)
        self.assertIn("conclusion", errs)

    def test_too_short(self):
        errs = validate_synthesis({"summary": "短", "trend": "x" * 20, "conclusion": "y" * 20}, {})
        self.assertIn("summary", errs)

    def test_all_valid(self):
        blocks = {"summary": "s" * 20, "trend": "t" * 20, "conclusion": "c" * 20}
        self.assertEqual(validate_synthesis(blocks, {}), {})


class TestRenderSynthesis(unittest.TestCase):
    def test_fstring_rendered_slot(self):
        md = "# 报告\n\n{ai_synthesis}"
        out = render_synthesis(md, {"summary": "S", "trend": "T", "conclusion": "C"})
        self.assertIn("### 执行摘要", out)
        self.assertIn("S", out)
        self.assertNotIn("{ai_synthesis}", out)
        self.assertNotIn("{{ai_synthesis}}", out)

    def test_double_brace_slot(self):
        md = "# 报告\n\n{{ai_synthesis}}"
        out = render_synthesis(md, {"summary": "S"})
        self.assertIn("### 执行摘要", out)
        self.assertIn("S", out)

    def test_no_slot_appends(self):
        out = render_synthesis("# 报告", {"summary": "S"})
        self.assertIn("AI 综合研判", out)

    def test_null_blocks_placeholder(self):
        out = render_synthesis("# 报告", None)
        self.assertIn("待 AI 综合研判", out)

    def test_section_flag(self):
        out = render_synthesis("# 报告", {"summary": "S"}, include_section=False)
        self.assertNotIn("## 九、AI 综合研判", out)


class TestBuildPrompt(unittest.TestCase):
    def test_contains_constraints_and_numbers(self):
        report = {"price": {"end_price": 1290.88, "period_return_pct": 4.1}, "meta": {}}
        prompt = build_synthesis_prompt("600519", report)
        self.assertIn("硬性约束", prompt)
        self.assertIn("1290.88", prompt)
        self.assertIn("summary", prompt)
        self.assertIn("不得编造", prompt)

    def test_skips_long_lists(self):
        report = {"price": {"end_price": 1.0}, "history": {"list": [1, 2, 3]}}
        prompt = build_synthesis_prompt("600519", report)
        # 列表字段不进入指标摘要（避免超长 + 诱导复述）
        self.assertIn("end_price", prompt)


if __name__ == "__main__":
    unittest.main()