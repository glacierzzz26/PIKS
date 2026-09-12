"""test_event_denoise.py — 事件降噪分级引擎单元测试

覆盖设计文档 §10：分类 / 去重合并 / 重大性分级 / 事件→价格窗口 / 「30 天无重大事件」合法结论。
"""
import os
import sys
import unittest
from datetime import date, datetime, timedelta

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.analysis.event_denoise import (
    filter_events,
    classify_event,
    grade_severity,
    EventSeverity,
    _merge_key,
)
from src.analysis.events import analyze_events
from src.models.news import NewsItem
from src.models.announcement import Announcement
from src.models.bar import Bar


AS_OF = date(2026, 9, 10)


def _bars(days: int = 60) -> list:
    return [
        Bar(f"sh_test", AS_OF - timedelta(days=59 - i), 100, 101, 99,
            100.0 + i * 0.1, 10000, 1000000, 2.0, 0.5, 0.5, 0.2)
        for i in range(days)
    ]


def _news(title: str, days_ago: int, nid: str = "") -> NewsItem:
    return NewsItem(
        id=nid or f"n_{title[:8]}_{days_ago}",
        symbol="sh_test",
        title=title,
        content="",
        published_at=datetime.combine(AS_OF - timedelta(days=days_ago), datetime.min.time()),
        source="test",
        url=None,
    )


def _ann(title: str, days_ago: int, aid: str = "") -> Announcement:
    return Announcement(
        id=aid or f"a_{title[:8]}_{days_ago}",
        symbol="sh_test",
        title=title,
        category="公告",
        publish_date=AS_OF - timedelta(days=days_ago),
        url=None,
    )


class TestClassifyEvent(unittest.TestCase):
    def test_earnings(self):
        cat, hits = classify_event("贵州茅台2025年年度报告")
        self.assertEqual(cat, "earnings")
        self.assertTrue(hits)

    def test_restructure_high(self):
        cat, _ = classify_event("关于重大资产重组的进展公告")
        self.assertEqual(cat, "restructure")

    def test_shareholder(self):
        cat, _ = classify_event("控股股东拟减持公司股份")
        self.assertEqual(cat, "shareholder")

    def test_routine_meeting_not_shareholder(self):
        cat, _ = classify_event("2026年第一次临时股东大会通知")
        self.assertEqual(cat, "routine")

    def test_buyback(self):
        cat, _ = classify_event("关于回购公司股份的公告")
        self.assertEqual(cat, "buyback")

    def test_regulatory(self):
        cat, _ = classify_event("证监会立案调查通知")
        self.assertEqual(cat, "regulatory")

    def test_dividend(self):
        cat, _ = classify_event("2025年半年度权益分派实施公告")
        self.assertEqual(cat, "announcement_dividend")


class TestGradeSeverity(unittest.TestCase):
    def test_restructure_is_high(self):
        self.assertEqual(grade_severity("restructure", "重大资产重组公告"), EventSeverity.HIGH)

    def test_shareholder_is_high(self):
        self.assertEqual(grade_severity("shareholder", "控股股东拟减持不超过2%"), EventSeverity.HIGH)

    def test_earnings_without_hard_word_is_medium(self):
        self.assertEqual(grade_severity("earnings", "年度报告公告"), EventSeverity.MEDIUM)

    def test_routine_is_low(self):
        self.assertEqual(grade_severity("routine", "关于变更注册地址的公告"), EventSeverity.LOW)

    def test_direct_high_keyword(self):
        self.assertEqual(grade_severity("routine", "重大资产重组"), EventSeverity.HIGH)


class TestMergeKey(unittest.TestCase):
    def test_merge_progress_updates(self):
        k1 = _merge_key("关于重大资产重组进展的公告", "restructure")
        k2 = _merge_key("关于重大资产重组进展的公告（二）", "restructure")
        k3 = _merge_key("关于重大资产重组进展的公告（三）", "restructure")
        self.assertEqual(k1, k2)
        self.assertEqual(k1, k3)

    def test_merge_year_normalized(self):
        k1 = _merge_key("2025年年度报告", "earnings")
        k2 = _merge_key("2026年年度报告", "earnings")
        self.assertEqual(k1, k2)

    def test_different_events_not_merged(self):
        k1 = _merge_key("关于重大资产重组进展的公告", "restructure")
        k2 = _merge_key("关于变更注册地址的公告", "routine")
        self.assertNotEqual(k1, k2)


class TestFilterEvents(unittest.TestCase):
    def setUp(self):
        self.anns = [
            _ann("关于重大资产重组进展的公告", 2, "a1"),
            _ann("关于重大资产重组进展的公告（二）", 1, "a2"),
            _ann("关于重大资产重组进展的公告（三）", 0, "a3"),
            _ann("2026年第一次临时股东大会通知", 3, "a4"),
        ]
        self.news = [
            _news("控股股东拟增持公司股份", 4, "n1"),
            _news("控股股东拟增持公司股份", 3, "n2"),
        ]

    def test_dedup_merge(self):
        r = filter_events("sh_test", AS_OF, news=self.news, announcements=self.anns)
        self.assertEqual(r.total_raw, 6)
        self.assertEqual(r.dedup_merged, 3)  # 重组3合1出2条冗余 + 增持2合1出1条冗余
        self.assertEqual(r.total_events, 3)  # 重组 + 增持 + 股东大会

    def test_no_major_event_is_valid_conclusion(self):
        r = filter_events("sh_test", AS_OF, news=[_news("公司日常经营正常", 2)],
                          announcements=[_ann("关于变更注册地址的公告", 3)])
        self.assertTrue(r.no_major_event)
        self.assertEqual(r.high_events, [])
        self.assertEqual(r.medium_events, [])

    def test_price_window_attached_to_major(self):
        r = filter_events("sh_test", AS_OF, news=self.news, announcements=self.anns, bars=_bars())
        major = r.major_events
        self.assertTrue(major)
        for ev in major:
            self.assertIsNotNone(ev.price_window)
            self.assertIn("window_return_pct", ev.price_window)

    def test_low_events_aggregated(self):
        r = filter_events("sh_test", AS_OF, announcements=[_ann("关于变更注册地址的公告", 3)],
                          bars=_bars())
        self.assertEqual(len(r.low_events), 1)
        self.assertEqual(r.low_events[0].category, "routine")

    def test_window_outside_bars_skipped(self):
        # 事件日早于 bars 起点（但仍在窗口内）→ 无价格窗口（不报错）
        # bars 起点为 AS_OF-59 天；用 90 天窗口让 100 天前事件进入但无 bar 可关联
        r = filter_events(
            "sh_test", AS_OF,
            announcements=[_ann("重大资产重组", 100)],
            bars=_bars(),
            window_days=120,
        )
        self.assertGreaterEqual(len(r.high_events), 1)
        self.assertIsNone(r.high_events[0].price_window)


class TestAnalyzeEventsCompat(unittest.TestCase):
    def test_denoised_field_present(self):
        em = analyze_events(
            [_news("控股股东拟增持公司股份", 2)],
            AS_OF,
            announcements=[_ann("重大资产重组", 1)],
            bars=_bars(),
        )
        self.assertIsNotNone(em.denoised)
        self.assertEqual(em.total_news, 1)
        self.assertGreaterEqual(em.earnings_mentions, 0)

    def test_to_dict_hierarchy(self):
        em = analyze_events([_news("控股股东拟增持公司股份", 2)], AS_OF)
        d = em.to_dict()
        self.assertIn("denoised", d)
        self.assertIn("total_news", d)


if __name__ == "__main__":
    unittest.main()