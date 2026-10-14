"""test_datahub_mappers.py — datahub 行 → 领域模型映射单元测试（P14-1 / B4）。

**重点锁口径红线**（单位约定 = 最高频 100× bug 源）：
- `daily_price.volume` 已是手、`turnover_rate` 已是 % —— 适配层**不得**再 //100 / ×100
- close 不复权；pct/amplitude/chg 由前收派生；涨跌停用 PIKS 规则
- 申万指数量价恒 0、非涨停标记；pct 用 datahub 逐行真值
- 缺失一律 None（不补 0），仅 DDL NOT NULL 金额补 0.0
"""
import os
import sys
from datetime import date, datetime

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.models import Market, Symbol
from src.providers.datahub import mappers

_SH = Symbol(code="600519", market=Market.SH)          # limit ±10%
_SW = Symbol(code="801010", market=Market.SI)


# ---------- 行情（单位红线）----------

def test_to_bars_does_not_rescale_volume_or_turnover():
    rows = [
        {"trade_date": "2026-01-02", "open": 10.0, "high": 10.5, "low": 9.8,
         "close": 10.0, "volume": 12345, "amount": 1.0e8, "turnover_rate": 3.2},
    ]
    bars = mappers.to_bars(_SH, rows)
    assert bars[0].volume == 12345        # 已是手 —— 不 //100
    assert bars[0].turnover == 3.2        # 已是 % —— 不 ×100


def test_to_bars_derives_pct_amplitude_chg_and_limit():
    rows = [
        {"trade_date": "2026-01-02", "open": 10.0, "high": 10.5, "low": 9.8,
         "close": 10.0, "volume": 1, "amount": 1.0, "turnover_rate": 1.0},
        {"trade_date": "2026-01-03", "open": 10.0, "high": 11.0, "low": 10.0,
         "close": 11.0, "volume": 1, "amount": 1.0, "turnover_rate": 1.0},
    ]
    bars = mappers.to_bars(_SH, rows)
    assert bars[0].pct_change == 0.0     # 首行无前收
    assert bars[1].pct_change == pytest.approx(10.0)
    assert bars[1].amplitude == pytest.approx((11.0 - 10.0) / 10.0 * 100)
    assert bars[1].chg_amount == pytest.approx(1.0)
    assert bars[1].is_limit_up is True    # 主板 10%
    assert bars[1].is_limit_down is False


def test_to_bars_sorts_ascending():
    rows = [
        {"trade_date": "2026-01-03", "close": 11.0, "high": 11.0, "low": 11.0,
         "open": 11.0, "volume": 1, "amount": 1.0, "turnover_rate": 1.0},
        {"trade_date": "2026-01-02", "close": 10.0, "high": 10.0, "low": 10.0,
         "open": 10.0, "volume": 1, "amount": 1.0, "turnover_rate": 1.0},
    ]
    bars = mappers.to_bars(_SH, rows)
    assert [b.date for b in bars] == [date(2026, 1, 2), date(2026, 1, 3)]


def test_to_index_bars_zero_volume_and_stored_pct():
    rows = [{"trade_date": "2026-01-02", "open": 3000.0, "high": 3050.0,
             "low": 2990.0, "close": 3040.0, "pct_change": 1.33,
             "amplitude": 2.0, "chg_amount": 40.0}]
    bars = mappers.to_index_bars(_SW, rows)
    assert bars[0].volume == 0 and bars[0].amount == 0.0 and bars[0].turnover == 0.0
    assert bars[0].pct_change == 1.33      # datahub 逐行真值
    assert bars[0].is_limit_up is False and bars[0].is_limit_down is False


# ---------- 财务 / 估值 ----------

def test_to_financial_snapshots_classifies_and_dedups():
    rows = [
        {"report_date": "2026-03-31", "revenue_growth": 12.0, "profit_growth": 15.0,
         "gross_margin": 50.0, "net_margin": 25.0, "roe": 20.0, "debt_ratio": 40.0},
        {"report_date": "2026-03-31", "revenue_growth": 12.0},   # 重复 → 丢
        {"report_date": "2026-06-30", "revenue_growth": 13.0},
        {"report_date": "2026-02-15", "revenue_growth": 1.0},    # 非季末 → 丢
    ]
    snaps = mappers.to_financial_snapshots(_SH, rows)
    assert [s.report_type for s in snaps] == ["Q1", "Q2"]
    assert snaps[0].net_margin == 25.0
    assert snaps[0].revenue_yoy == 12.0


def test_to_valuation_maps_market_cap():
    v = mappers.to_valuation(_SH, {"trade_date": "2026-01-03", "total_mv": 1.0e12,
                                   "float_mv": 5.0e11, "pe_ttm": 30.0, "pb": 9.0})
    assert v.as_of == date(2026, 1, 3)
    assert v.market_cap == 1.0e12 and v.float_cap == 5.0e11 and v.pb == 9.0


def test_to_valuation_none_when_no_row():
    assert mappers.to_valuation(_SH, None) is None


# ---------- 龙虎榜 ----------

def test_to_lhb_records_splits_reasons():
    rows = [{"trade_date": "2026-09-17", "name": "贵州茅台", "close": 1700.0,
             "pct_change": 9.98, "turnover_rate": 3.2,
             "reasons": "日涨幅偏离值达7%；连续三个交易日"}]
    recs = mappers.to_lhb_records(_SH, rows)
    assert recs[0].reasons == ("日涨幅偏离值达7%", "连续三个交易日")
    assert recs[0].reason == "日涨幅偏离值达7%；连续三个交易日"
    assert recs[0].pct_change == 9.98


def test_to_lhb_details_missing_amount_defaults_zero():
    rows = [{"trade_date": "2026-09-17", "dealer_name": "机构专用", "buy_amount": None,
             "buy_pct": 5.0, "sell_amount": 0.0, "sell_pct": 0.0,
             "net_amount": 100.0, "reason": "买入"}]
    d = mappers.to_lhb_details(rows)
    assert d[0].buy_amount == 0.0
    assert d[0].dealer_name == "机构专用"


# ---------- 宏观 ----------

def test_to_macro_points_parses_period_and_cadence():
    month = mappers.to_macro_points([{"period_label": "2026年08月份", "cadence": "month",
                                      "level": 100.8, "yoy": 0.8, "mom": 0.3}])
    assert month[0].period.end == date(2026, 8, 31)
    assert month[0].level == 100.8 and month[0].cumulative is False

    quarter = mappers.to_macro_points([{"period_label": "2026年第1季度", "cadence": "quarter",
                                        "level": 334192.9, "yoy": 5.0}])
    assert quarter[0].cumulative is True


def test_to_macro_points_bad_period_raises():
    with pytest.raises(ValueError):
        mappers.to_macro_points([{"period_label": "2026/08", "cadence": "month", "level": 1.0}])


# ---------- 新闻 / 公告 ----------

def test_to_news_items_parses_time_and_null_url():
    rows = [{"title": "t", "content": "c", "published_at": "2026-01-02 09:30:00",
             "source": "证券时报", "url": None}]
    items = mappers.to_news_items(_SH, rows)
    assert items[0].published_at == datetime(2026, 1, 2, 9, 30)
    assert items[0].url is None
    assert items[0].id == "news_600519_20260102093000"


def test_to_announcements_maps_fields():
    rows = [{"title": "2025年年度报告", "category": "年度报告",
             "publish_date": "2026-01-02", "url": "http://x/1"}]
    ann = mappers.to_announcements(_SH, rows)
    assert ann[0].publish_date == date(2026, 1, 2)
    assert ann[0].url == "http://x/1"


# ---------- 行业 ----------

def test_to_industry_ref_derives_sw_level_label():
    ref = mappers.to_industry_ref({"code": "801010", "level": 1, "name": "农林牧渔",
                                   "level_label": "一级", "parent_name": None,
                                   "member_count": 100, "pe_static": 20.0,
                                   "pe_ttm": 22.0, "pb": 2.5, "dividend_yield": 1.0})
    assert ref.level_label == "申万一级"      # 用 PIKS 展示串，非 datahub 中性串
    assert ref.member_count == 100 and ref.parent is None


def test_to_constituents_strips_exchange_suffix():
    rows = [{"index_code": "801012", "stock_code": "600519.SH", "stock_name": "贵州茅台",
             "market_cap": 20000.0, "pe_ttm": 30.0, "pb": 9.0, "roe": 25.0,
             "dividend_yield": 2.0, "net_profit_growth": 15.0, "revenue_growth": 12.0}]
    cons = mappers.to_constituents(rows)
    assert cons[0].symbol == "600519" and cons[0].roe == 25.0


def test_to_peer_metrics_price_is_none():
    rows = [{"stock_code": "600519.SH", "stock_name": "贵州茅台", "roe": 25.0}]
    peers = mappers.to_peer_metrics(rows)
    assert peers[0].symbol == "600519" and peers[0].price is None
