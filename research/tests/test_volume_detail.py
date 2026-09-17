"""test_volume_detail.py — 放量异常日逐日明细回归（issue #3）

覆盖：明细字段与含义（date/volume/amount/threshold/ratio）；阈值取该日自身
max(前20日均量×2, 前5日峰值)；倍数 = 当日量 / 该日阈值；无异常日如实空；
前 20 日无足够历史不参与（不臆造）；明细与 abnormal_volume_dates 逐条对齐。
"""
import os
import sys
import unittest
from datetime import date, timedelta

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.volume import analyze_volume
from src.models import Bar


def _bars(volumes, start=date(2026, 6, 1)) -> list:
    """按成交量构造升序 bars（其余字段占位，量能模块只看 volume/amount）。"""
    out = []
    for i, v in enumerate(volumes):
        d = start + timedelta(days=i)
        out.append(Bar(
            symbol="sh600519", date=d, open=10.0, high=10.0, low=10.0, close=10.0,
            volume=int(v), amount=float(v) * 10.0, amplitude=0.0,
            pct_change=0.0, chg_amount=0.0, turnover=1.0,
        ))
    return out


class TestAbnormalVolumeDetail(unittest.TestCase):
    def test_no_abnormal_day_yields_empty_detail(self):
        """全平量 → 无异常日：明细空，不臆造。"""
        vm = analyze_volume(_bars([100] * 30), date(2026, 9, 10))
        self.assertEqual(vm.abnormal_volume_days, 0)
        self.assertEqual(vm.abnormal_volume_dates, [])
        self.assertEqual(vm.abnormal_volume_detail, [])

    def test_detail_fields_and_alignment(self):
        """有异常日 → 明细字段齐、且与 abnormal_volume_dates 逐条对齐。"""
        vols = [100] * 25 + [1000]  # 第 26 日放量 10 倍
        vm = analyze_volume(_bars(vols), date(2026, 9, 10))
        self.assertEqual(vm.abnormal_volume_days, 1)
        self.assertEqual(len(vm.abnormal_volume_detail), 1)
        d = vm.abnormal_volume_detail[0]
        self.assertEqual(set(d.keys()), {"date", "volume", "amount", "threshold", "ratio"})
        # 与 dates 数组同源同序
        self.assertEqual(d["date"], vm.abnormal_volume_dates[0].isoformat())
        self.assertEqual(d["volume"], 1000)

    def test_threshold_and_ratio_are_self_relative(self):
        """阈值 = max(前20日均量×2, 前5日峰值)；倍数 = 当日量 / 该阈值。"""
        vols = [100] * 25 + [1000]
        vm = analyze_volume(_bars(vols), date(2026, 9, 10))
        d = vm.abnormal_volume_detail[0]
        # 前 20 日均量 = 100 → ×2 = 200；前 5 日峰值 = 100 → 阈值 = 200
        self.assertAlmostEqual(d["threshold"], 200.0, places=6)
        # 倍数 = 1000 / 200 = 5.0（>1 即异常）
        self.assertAlmostEqual(d["ratio"], 5.0, places=6)
        self.assertGreater(d["ratio"], 1.0)

    def test_first_20_days_not_scanned(self):
        """前 20 个交易日无足够历史 → 不参与扫描：即便天量也不计入。"""
        vols = [9999] + [100] * 29  # 首日天量，但 i<20 被跳过
        vm = analyze_volume(_bars(vols), date(2026, 9, 10))
        self.assertEqual(vm.abnormal_volume_days, 0)
        self.assertNotIn("2026-06-01", [d["date"] for d in vm.abnormal_volume_detail])

    def test_multiple_abnormal_days_all_recorded(self):
        """多个异常日 → 全部逐日记录，非只留天数。"""
        vols = [100] * 25 + [1000, 100, 2000]
        vm = analyze_volume(_bars(vols), date(2026, 9, 10))
        self.assertEqual(vm.abnormal_volume_days, 2)
        self.assertEqual(len(vm.abnormal_volume_detail), 2)
        self.assertEqual([d["volume"] for d in vm.abnormal_volume_detail], [1000, 2000])


if __name__ == "__main__":
    unittest.main()
