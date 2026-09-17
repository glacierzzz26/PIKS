"""test_patterns.py — 量价形态规则回归

覆盖：序列裁剪与形状；地量启动；换手峰值见顶（量价共振+回落尾巴）；近期量价四态
（价跌量未缩 / 缩量续跌 / 放量上行 / 缩量上行）；样本不足如实不出结论。
"""
import os
import sys
import unittest
from datetime import date, timedelta

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.patterns import analyze_patterns, MIN_BARS, SERIES_DAYS
from src.models import Bar


def _bars(closes, turnovers, start=date(2026, 6, 1)) -> list:
    """按收盘价 + 换手率构造升序 bars（其余字段占位，模式模块不依赖）。"""
    out = []
    for i, (c, t) in enumerate(zip(closes, turnovers)):
        d = start + timedelta(days=i)
        out.append(Bar(
            symbol="sh600519", date=d, open=c, high=c, low=c, close=c,
            volume=int(t * 10000), amount=c * t * 10000, amplitude=0.0,
            pct_change=0.0, chg_amount=0.0, turnover=t,
        ))
    return out


def _labels(pm) -> dict:
    return {l["code"]: l for l in pm.labels}


class TestSeries(unittest.TestCase):
    def test_series_shape_and_cap(self):
        bars = _bars([10 + i * 0.1 for i in range(80)], [1.0] * 80)
        pm = analyze_patterns(bars, date(2026, 9, 10))
        self.assertEqual(len(pm.series), SERIES_DAYS)  # 截到 60
        s = pm.series[-1]
        self.assertEqual(set(s.keys()), {"date", "close", "turnover", "volume"})
        self.assertEqual(s["date"], bars[-1].date.isoformat())

    def test_insufficient_window_no_labels(self):
        bars = _bars([10 + i for i in range(MIN_BARS - 1)], [1.0] * (MIN_BARS - 1))
        pm = analyze_patterns(bars, date(2026, 9, 10))
        self.assertEqual(pm.labels, [])
        self.assertTrue(pm.note)  # 如实说明样本不足


class TestDizuQidong(unittest.TestCase):
    def test_detects_low_turnover_then_rally(self):
        # 前段 5 个交易日地量（0.3%），随后 25 日放量上行到 +30%
        n_low, n_up = 5, 25
        closes = [10.0] * n_low + [10.0 + i * 0.12 for i in range(1, n_up + 1)]
        turnovers = [0.3] * n_low + [1.0 + i * 0.02 for i in range(n_up)]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        lb = _labels(pm)
        self.assertIn("dizu_qidong", lb)
        self.assertEqual(lb["dizu_qidong"]["label"], "地量启动")
        self.assertIn("地量日", lb["dizu_qidong"]["evidence"])


class TestPeakCoincide(unittest.TestCase):
    def test_detects_peak_turnover_at_price_high(self):
        # 缓涨 → 第 25 日换手率峰值 8% 且股价高点 → 之后放量下跌
        closes = [10.0 + i * 0.1 for i in range(25)] + [12.4 - i * 0.15 for i in range(1, 16)]
        turnovers = [1.0] * 24 + [8.0] + [4.0] * 15
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        lb = _labels(pm)
        self.assertIn("peak_coincide", lb)
        self.assertEqual(lb["peak_coincide"]["label"], "换手峰值见顶")
        self.assertIn("高位放量换手", lb["peak_coincide"]["evidence"])
        self.assertTrue(pm.divergence["peak_high_coincide"])


class TestRecentState(unittest.TestCase):
    def test_fall_no_shrink(self):
        # 前段横盘，近 3 日下跌但换手仍在高位（≥中位数），且回撤未达高位放量下跌阈值
        closes = [11.0] * 17 + [10.8, 10.6, 10.4]
        turnovers = [3.0] * 17 + [3.2, 3.1, 3.0]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        self.assertEqual(_labels(pm)["fall_no_shrink"]["label"], "价跌量未缩")

    def test_high_vol_down(self):
        # 自区间高点深幅回落（>8%）+ 换手仍高于中位数 → 高位放量下跌
        closes = [12.0 - i * 0.05 for i in range(17)] + [11.4, 11.1, 10.8]
        turnovers = [3.0] * 17 + [3.2, 3.1, 3.0]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        self.assertEqual(_labels(pm)["high_vol_down"]["label"], "高位放量下跌")

    def test_fall_shrink(self):
        closes = [12.0 - i * 0.05 for i in range(17)] + [11.4, 11.1, 10.8]
        turnovers = [3.0] * 17 + [0.5, 0.4, 0.4]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        self.assertEqual(_labels(pm)["fall_shrink"]["label"], "缩量续跌")

    def test_vol_up(self):
        closes = [10.0 + i * 0.05 for i in range(17)] + [11.0, 11.3, 11.6]
        turnovers = [1.0] * 17 + [2.5, 2.6, 2.7]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        self.assertEqual(_labels(pm)["vol_up"]["label"], "放量上行")

    def test_shrink_up(self):
        closes = [10.0 + i * 0.05 for i in range(17)] + [11.0, 11.3, 11.6]
        turnovers = [2.0] * 17 + [0.4, 0.4, 0.4]
        pm = analyze_patterns(_bars(closes, turnovers), date(2026, 9, 10))
        self.assertEqual(_labels(pm)["shrink_up"]["label"], "缩量上行")


class TestDivergence(unittest.TestCase):
    def test_divergence_fields(self):
        bars = _bars([10 + i * 0.1 for i in range(60)], [1.0 + i * 0.01 for i in range(60)])
        d = analyze_patterns(bars, date(2026, 9, 10)).divergence
        for k in ("peak_date", "peak_turnover", "high_close", "peak_high_coincide",
                  "turnover_price_corr", "after_peak_return_pct"):
            self.assertIn(k, d)


if __name__ == "__main__":
    unittest.main()
