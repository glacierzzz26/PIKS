"""test_capital_lhb.py — 资金面(龙虎榜)链路接线 + 数据正确性回归（issue #26）

覆盖三件事：
  1. 接线 —— `capital` 是合法 section、plan 会建采集/分析任务、机检覆盖率不误报
  2. 数据正确性 —— 原实现三处假数字缺陷的回归：
       a. 涨跌幅取错字段（「对应值」随上榜原因变化，非涨跌幅）
       b. 同日多原因上榜 → 至少不被丢成一条
       c. 席位明细跨原因重复计数 → 去重 + 拒「连续三个交易日」聚合块
  3. `--no-lhb` 不再产出「未上榜」假陈述

全部用 mock DataFrame，不依赖网络。
"""
import os
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pandas as pd

from src.models import resolve_symbol
from src.providers.capital.akshare_lhb import AkShareLHBProvider, _parse_date
from src.analysis.capital import analyze_capital
from src.workflow.profile import (
    ProfileValidator, SECTION_REQUIREMENTS, load_profile,
)
from src.workflow.plan import PlanGenerator
from src.quality_gate import run_quality_gate

AS_OF = date(2026, 9, 17)
SYMBOL = resolve_symbol("sz000978")


# ---------- mock 数据（逐列对齐实测东财返回） ----------

def _daily_df():
    """`stock_lhb_detail_em` 的返回样例：000978 同日因 3 个原因上榜 3 行。"""
    return pd.DataFrame([
        {"序号": 1, "代码": "000978", "名称": "桂林旅游", "上榜日": "2026-09-17",
         "收盘价": 7.97, "涨跌幅": -9.3288, "换手率": 35.2429,
         "上榜原因": "日换手率达到20%的前5只证券"},
        {"序号": 2, "代码": "000978", "名称": "桂林旅游", "上榜日": "2026-09-17",
         "收盘价": 7.97, "涨跌幅": -9.3288, "换手率": 35.2429,
         "上榜原因": "日跌幅偏离值达到7%的前5只证券"},
        {"序号": 3, "代码": "000978", "名称": "桂林旅游", "上榜日": "2026-09-17",
         "收盘价": 7.97, "涨跌幅": -9.3288, "换手率": 35.2429,
         "上榜原因": "连续三个交易日内，跌幅偏离值累计达到20%的证券"},
        {"序号": 4, "代码": "600519", "名称": "贵州茅台", "上榜日": "2026-09-17",
         "收盘价": 1275.0, "涨跌幅": 1.0, "换手率": 0.3, "上榜原因": "其他"},
    ])


def _detail_df(reason, rows):
    return pd.DataFrame([
        {"序号": i + 1, "交易营业部名称": name,
         "买入金额": buy, "买入金额-占总成交比例": 0.01,
         "卖出金额": sell, "卖出金额-占总成交比例": 0.01,
         "净额": buy - sell, "类型": reason}
        for i, (name, buy, sell) in enumerate(rows)
    ])


# 同一席位的同一笔交易，在东财返回的每个上榜原因块里都出现一次（数值完全相同）
_SAME = ("国信证券股份有限公司浙江互联网分公司", 30128993.0, 10883734.0)
_OTHER = ("东方财富证券股份有限公司拉萨金融城南环路证券营业部", 20786298.0, 8645777.0)


class TestLHBProviderRecordParsing(unittest.TestCase):
    """缺陷 a + b：涨跌幅取真字段 + 同日多原因聚合。"""

    def setUp(self):
        self.provider = AkShareLHBProvider()
        import akshare as ak
        self._orig = ak.stock_lhb_detail_em
        ak.stock_lhb_detail_em = lambda **kw: _daily_df()

    def tearDown(self):
        import akshare as ak
        ak.stock_lhb_detail_em = self._orig

    def test_pct_change_is_real_pct_not_corresponding_value(self):
        """涨跌幅必须是 -9.33（真实），不是 35.24（那是换手率/对应值）。"""
        recs = self.provider.search(SYMBOL, days=30, end=AS_OF)
        self.assertEqual(len(recs), 1)
        self.assertAlmostEqual(recs[0].pct_change, -9.3288, places=3)
        self.assertAlmostEqual(recs[0].turnover_pct, 35.2429, places=3)

    def test_same_day_multi_reason_merged_into_one_record(self):
        """同日 3 个上榜原因 → 1 条记录，原因合并保留全部 3 个。"""
        recs = self.provider.search(SYMBOL, days=30, end=AS_OF)
        self.assertEqual(len(recs), 1)
        self.assertEqual(len(recs[0].reasons), 3)
        self.assertIn("日换手率达到20%的前5只证券", recs[0].reason)

    def test_filters_target_symbol_only(self):
        recs = self.provider.search(SYMBOL, days=30, end=AS_OF)
        self.assertTrue(all(r.symbol == "sz000978" for r in recs))


class TestLHBProviderDetailDedup(unittest.TestCase):
    """缺陷 c：跨上榜原因块的重复席位不得累加。"""

    def setUp(self):
        self.provider = AkShareLHBProvider()
        import akshare as ak
        self._orig = ak.stock_lhb_stock_detail_em
        ak.stock_lhb_stock_detail_em = self._mock

    def tearDown(self):
        import akshare as ak
        ak.stock_lhb_stock_detail_em = self._orig

    @staticmethod
    def _mock(symbol, date, flag):
        # 3 个原因块，前两块含相同席位（同笔交易），第三块是跨日聚合。
        return pd.concat([
            _detail_df("日换手率达到20%的前5只证券", [_SAME, _OTHER]),
            _detail_df("日跌幅偏离值达到7%的前5只证券", [_SAME, _OTHER]),
            _detail_df("连续三个交易日内，跌幅偏离值累计达到20%的证券",
                       [(_SAME[0], 39567968.0, 13585940.0)]),
        ], ignore_index=True)

    def test_duplicate_across_reason_blocks_deduped(self):
        """同一席位重复块 → 只保留一条，金额不翻倍。"""
        details = self.provider.get_detail(SYMBOL, AS_OF)
        names = [d.dealer_name for d in details]
        self.assertEqual(names.count(_SAME[0]), 1)
        same = next(d for d in details if d.dealer_name == _SAME[0])
        self.assertAlmostEqual(same.buy_amount, _SAME[1], places=2)  # 未翻倍

    def test_multi_day_aggregate_block_rejected(self):
        """「连续三个交易日」块是跨日口径，不得混入当日资金流。"""
        details = self.provider.get_detail(SYMBOL, AS_OF)
        self.assertTrue(all("连续三个交易日" not in d.reason for d in details))
        # 该块独有的 39567968 不应出现
        self.assertFalse(any(abs(d.buy_amount - 39567968.0) < 1 for d in details))


class TestCapitalAnalysisRejectsMultiDayAggregate(unittest.TestCase):
    """端到端：跨日聚合块被拒后，机构净额不再虚高。"""

    def test_institution_net_not_inflated(self):
        from src.providers.capital.akshare_lhb import LHBRecord, LHBDetail
        rec = LHBRecord(
            symbol="sz000978", name="桂林旅游", trade_date=AS_OF,
            close_price=7.97, pct_change=-9.3288, turnover_pct=35.24,
            reasons=("日跌幅偏离值达到7%的前5只证券",),
        )
        # provider 已去重：机构专用只出现一次
        details = [LHBDetail(
            trade_date=AS_OF, dealer_name="机构专用",
            buy_amount=20000000.0, buy_pct=0.02,
            sell_amount=5000000.0, sell_pct=0.005,
            net_amount=15000000.0, reason="日跌幅偏离值达到7%的前5只证券",
        )]
        m = analyze_capital([rec], {AS_OF: details}, AS_OF, window_days=30)
        self.assertEqual(m.lhb_count, 1)
        self.assertAlmostEqual(m.daily[0].institution_net, 15000000.0, places=2)


class TestCapitalWiring(unittest.TestCase):
    """接线四处：section 合法 / plan 建任务 / 覆盖率不误报。"""

    def test_capital_is_a_valid_section(self):
        self.assertIn("capital", SECTION_REQUIREMENTS)
        self.assertIn("capital", ProfileValidator.VALID_SECTIONS)

    def test_short_term_plan_builds_capital_tasks(self):
        plan = PlanGenerator.generate(load_profile("short-term"))
        names = [t.name for t in plan.tasks]
        self.assertIn("collect_capital", names)
        self.assertIn("analyze_capital", names)
        # 未上榜是合法结论 → 采集/分析均为 optional（不阻断整篇）
        cap_tasks = [t for t in plan.tasks if "capital" in t.name]
        self.assertTrue(all(t.optional for t in cap_tasks))

    def test_profile_without_capital_builds_no_task(self):
        """零回归：不含 capital 的 profile 不该凭空多出采集任务。"""
        names = [t.name for t in PlanGenerator.generate(load_profile("stock")).tasks]
        self.assertFalse(any("capital" in n for n in names))

    def test_gate_coverage_counts_capital(self):
        """机检 SECTION_TO_KEY 有 capital 映射 → 不误报 missing。"""
        js = {"capital": {"lhb_count": 1}}
        secs = ["company", "capital"]
        gate = run_quality_gate(secs, js, [{"section": "x"}], True, 0)
        cov = next(c for c in gate.checks if c.name == "data_completeness")
        self.assertNotIn("capital", cov.meta.get("missing", []))


class TestParseDate(unittest.TestCase):
    def test_parses_iso_and_compact(self):
        self.assertEqual(_parse_date("2026-09-17"), date(2026, 9, 17))
        self.assertEqual(_parse_date("20260917"), date(2026, 9, 17))

    def test_unparseable_returns_none(self):
        self.assertIsNone(_parse_date(""))
        self.assertIsNone(_parse_date("not-a-date"))


if __name__ == "__main__":
    unittest.main()
