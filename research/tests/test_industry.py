"""test_industry.py — 行业分析 Provider 单元测试

覆盖：行业分类命中（含缓存优先路径）、同业名单、财务字段填充与 N/A 降级、
markdown 行业章节渲染、JSON 序列化。
"""
import os
import sys
import unittest
from datetime import date
from unittest.mock import patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.models import Symbol, Market
from src.providers.industry.sw_provider import (
    SwIndustryProvider,
    PeerMetric,
    IndustryAnalysis,
    FALLBACK_INDUSTRIES,
)
from src.report.markdown import generate_markdown
from src.report.json_report import generate_json
from src.analysis.price import PriceMetrics
from src.analysis.volume import VolumeMetrics


def _price() -> PriceMetrics:
    return PriceMetrics(
        symbol="sh600519", as_of=date(2026, 9, 10), period_days=60,
        start_price=1240.0, end_price=1290.88, max_price=1363.35, min_price=1151.01,
        return_pct_5d=-0.51, return_pct_10d=-0.91, return_pct_20d=-3.88,
        return_pct_60d=4.1, period_return_pct=4.1, max_drawdown_pct=6.53,
        max_rise_pct=9.0, volatility_annual=26.28, limit_up_days=0, limit_down_days=0,
    )


def _volume() -> VolumeMetrics:
    return VolumeMetrics(
        symbol="sh600519", as_of=date(2026, 9, 10), period_days=60,
        total_volume=27634, avg_volume_5d=5000.0, avg_volume_20d=5000.0,
        avg_volume_60d=None, total_amount=1e8, avg_amount_5d=5e6,
        avg_amount_20d=5e6, avg_turnover_5d=0.3, avg_turnover_20d=0.25,
        avg_turnover_60d=None, max_turnover=1.2, min_turnover=0.1,
        abnormal_volume_days=0, abnormal_volume_dates=[], price_volume_corr=0.3,
    )


def _industry_with_peers() -> IndustryAnalysis:
    peers = [
        PeerMetric(symbol="600519", name="贵州茅台", price=1290.88, pe_ttm=19.82,
                   pb=6.42, roe=17.95, dividend_yield=4.03, market_cap=16137.05,
                   net_profit_growth=1.5, revenue_growth=1.47),
        PeerMetric(symbol="000858", name="五粮液", price=71.16, pe_ttm=21.11,
                   pb=2.33, roe=7.34, dividend_yield=7.25, market_cap=2762.15,
                   net_profit_growth=89.3, revenue_growth=20.87),
        PeerMetric(symbol="603589", name="口子窖", price=18.65, pe_ttm=34.52,
                   pb=1.07, roe=3.5, dividend_yield=2.67, market_cap=111.55,
                   net_profit_growth=-48.97, revenue_growth=-22.89),
    ]
    return IndustryAnalysis(
        symbol="600519", industry_name="白酒Ⅲ", industry_code="851251.SI",
        peer_count=3, peers=peers,
    )


class TestIndustryAnalysisStructure(unittest.TestCase):
    def test_to_dict(self):
        ia = _industry_with_peers()
        d = ia.to_dict()
        self.assertEqual(d["industry_name"], "白酒Ⅲ")
        self.assertEqual(d["peer_count"], 3)
        self.assertEqual(d["peers"][0]["roe"], 17.95)

    def test_no_industry_is_valid(self):
        ia = IndustryAnalysis(symbol="600519", industry_name=None,
                              industry_code=None, peer_count=0)
        d = ia.to_dict()
        self.assertIsNone(d["industry_code"])
        self.assertEqual(d["peer_count"], 0)


class TestProviderCachePath(unittest.TestCase):
    @patch("src.providers.industry.sw_provider.SwIndustryProvider._build_analysis")
    @patch("src.providers.industry.sw_provider.SwIndustryProvider._load_cache")
    def test_cache_hit_returns_analysis(self, mock_cache, mock_build):
        mock_cache.return_value = {"851251.SI": ["600519", "000858"]}
        mock_build.return_value = _industry_with_peers()
        p = SwIndustryProvider()
        r = p.get_industry(Symbol(code="600519", market=Market.SH))
        self.assertEqual(r.industry_name, "白酒Ⅲ")
        mock_build.assert_called_once()

    @patch("src.providers.industry.sw_provider.SwIndustryProvider._load_industry_list")
    @patch("src.providers.industry.sw_provider.SwIndustryProvider._load_cache")
    def test_no_cache_no_list_returns_empty(self, mock_cache, mock_list):
        mock_cache.return_value = {}
        mock_list.return_value = None
        p = SwIndustryProvider()
        r = p.get_industry(Symbol(code="600519", market=Market.SH))
        self.assertIsNone(r.industry_code)
        self.assertEqual(r.peer_count, 0)


class TestMarkdownIndustrySection(unittest.TestCase):
    def test_renders_peers_table(self):
        md = generate_markdown(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            industry=_industry_with_peers(),
            sections=["market", "industry"],
        )
        self.assertIn("白酒Ⅲ", md)
        self.assertIn("贵州茅台", md)
        self.assertIn("ROE", md)
        self.assertIn("16137", md)  # 市值

    def test_empty_industry_placeholder(self):
        md = generate_markdown(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            industry=None, sections=["market", "industry"],
        )
        self.assertIn("行业数据暂不可得", md)

    def test_no_industry_section_when_clipped(self):
        md = generate_markdown(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            industry=_industry_with_peers(),
            sections=["market"],  # express 无 industry
        )
        self.assertNotIn("行业对比", md)


class TestJsonIndustry(unittest.TestCase):
    def test_industry_in_json(self):
        js = generate_json(
            "sh600519", date(2026, 9, 10), _price(), _volume(),
            industry=_industry_with_peers(),
        )
        self.assertIn("industry", js)
        self.assertEqual(js["industry"]["industry_name"], "白酒Ⅲ")

    def test_no_industry_absent(self):
        js = generate_json("sh600519", date(2026, 9, 10), _price(), _volume())
        self.assertNotIn("industry", js)


class TestFallbackIndustries(unittest.TestCase):
    def test_bai_jiu_in_fallback(self):
        codes = {r["行业代码"]: r["行业名称"] for r in FALLBACK_INDUSTRIES}
        self.assertEqual(codes.get("851251.SI"), "白酒Ⅲ")


if __name__ == "__main__":
    unittest.main()