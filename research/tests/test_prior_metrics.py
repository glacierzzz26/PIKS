"""test_prior_metrics.py — issue #8:新一期把旧研报作输入,Number Lint 放开 known 集

`cli.py synthesize --prior-metrics` 把历史研报的数字并入 known 集。
不传该参数时,行为必须与改动前逐字节一致(零回归)。

这里不跑真实 LLM:直接构造产物目录(skeleton + metrics + synthesis.json),
调 run_synthesize 验证 lint 结果。
"""
import argparse
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.cli import run_synthesize


def _write_artifacts(d: str, code: str) -> None:
    """落最小产物:skeleton / metrics / synthesis.json(三段定性含历史对比数字)。"""
    # skeleton 里带 ai_synthesis 槽位,render_synthesis 才能注入。
    with open(os.path.join(d, f"{code}_skeleton.md"), "w", encoding="utf-8") as f:
        f.write("# 报告\n\n{ai_synthesis}\n")
    with open(os.path.join(d, f"{code}_metrics.json"), "w", encoding="utf-8") as f:
        json.dump({"meta": {"as_of": "2026-09-15"}, "price": {"end_price": 6.78}}, f)
    with open(os.path.join(d, f"{code}_synthesis_prompt.txt"), "w", encoding="utf-8") as f:
        f.write("提示")
    # 三段定性引用了一个「历史才有的数字」99.5(本次指标卡里没有)。
    blocks = {
        "summary": "较上次(2026-09-12)的99.5有所抬升,当前股价6.78元。",
        "trend": "趋势解读段落,足够长以满足二十字下限要求,引用6.78。",
        "conclusion": "综合结论段落,足够长以满足二十字下限要求,倾向中性。",
    }
    with open(os.path.join(d, f"{code}_synthesis.json"), "w", encoding="utf-8") as f:
        json.dump(blocks, f, ensure_ascii=False)


def _args(d: str, code: str, synth_path: str, prior_path=None) -> argparse.Namespace:
    return argparse.Namespace(
        dir=d, symbol=code, synthesis_file=synth_path, prior_metrics=prior_path
    )


class TestPriorMetrics(unittest.TestCase):
    def test_without_prior_flags_history_number(self):
        """基线:不传 --prior-metrics 时,历史数字 99.5 应被判为不可溯源(lint 未过)。"""
        with tempfile.TemporaryDirectory() as d:
            code = "600519"
            _write_artifacts(d, code)
            synth = os.path.join(d, f"{code}_synthesis.json")
            with self.assertRaises(SystemExit) as cm:
                run_synthesize(_args(d, code, synth))
            # lint 有问题 → 退 2
            self.assertEqual(cm.exception.code, 2)
            lint = json.load(open(os.path.join(d, f"{code}_lint.json"), encoding="utf-8"))
            self.assertFalse(lint["passed"])
            self.assertTrue(any(i["value"] == 99.5 for i in lint["issues"]))

    def test_with_prior_passes(self):
        """传了历史 metrics → 99.5 并入 known → lint 通过(不退 2)。"""
        with tempfile.TemporaryDirectory() as d:
            code = "600519"
            _write_artifacts(d, code)
            synth = os.path.join(d, f"{code}_synthesis.json")
            # 历史研报的 metrics 里含 99.5
            prior = os.path.join(d, f"{code}_prior_metrics.json")
            with open(prior, "w", encoding="utf-8") as f:
                json.dump([{"meta": {"as_of": "2026-09-12"}, "price": {"some_metric": 99.5}}], f)
            # 不应抛 SystemExit(即 lint 全过)
            run_synthesize(_args(d, code, synth, prior))
            lint = json.load(open(os.path.join(d, f"{code}_lint.json"), encoding="utf-8"))
            self.assertTrue(lint["passed"], lint["issues"])

    def test_fabricated_still_flagged_with_prior(self):
        """关键:并入历史数字**不放松**溯源 —— 真编造的数字仍应被拦。"""
        with tempfile.TemporaryDirectory() as d:
            code = "600519"
            _write_artifacts(d, code)
            synth = os.path.join(d, f"{code}_synthesis.json")
            prior = os.path.join(d, f"{code}_prior_metrics.json")
            with open(prior, "w", encoding="utf-8") as f:
                json.dump([{"price": {"some_metric": 99.5}}], f)
            # 把摘要改成含一个谁都没有的数字 12345.67
            blocks = {
                "summary": "较上次(2026-09-12)的99.5抬升,另有一个12345.67。",
                "trend": "趋势解读段落,足够长以满足二十字下限要求,引用6.78。",
                "conclusion": "综合结论段落,足够长以满足二十字下限要求,倾向中性。",
            }
            with open(synth, "w", encoding="utf-8") as f:
                json.dump(blocks, f, ensure_ascii=False)
            with self.assertRaises(SystemExit) as cm:
                run_synthesize(_args(d, code, synth, prior))
            self.assertEqual(cm.exception.code, 2)
            lint = json.load(open(os.path.join(d, f"{code}_lint.json"), encoding="utf-8"))
            self.assertFalse(lint["passed"])
            self.assertTrue(any(i["value"] == 12345.67 for i in lint["issues"]))

    def test_missing_prior_file_degrades(self):
        """历史文件不存在/损坏 → 如实降级(不失败),照常用本次指标卡对账。"""
        with tempfile.TemporaryDirectory() as d:
            code = "600519"
            _write_artifacts(d, code)
            synth = os.path.join(d, f"{code}_synthesis.json")
            # 指向不存在的文件 → 不应抛异常(除 lint 自身结果外)
            with self.assertRaises(SystemExit) as cm:
                run_synthesize(_args(d, code, synth, os.path.join(d, "nope.json")))
            # 仅有 99.5 不可溯源 → 退 2,说明降级后仍正常跑了 lint
            self.assertEqual(cm.exception.code, 2)
            # 损坏文件同理
            bad = os.path.join(d, f"{code}_prior_metrics.json")
            with open(bad, "w", encoding="utf-8") as f:
                f.write("{not json")
            with self.assertRaises(SystemExit) as cm2:
                run_synthesize(_args(d, code, synth, bad))
            self.assertEqual(cm2.exception.code, 2)


if __name__ == "__main__":
    unittest.main()
