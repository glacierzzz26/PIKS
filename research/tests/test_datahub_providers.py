"""test_datahub_providers.py — datahub 兄弟 provider（P14-1 / B6）。

以 mock client 断言两件事：
1. **请求契约**：发给 `DataHubClient.get` 的 `dataset_id` + 参数字典**逐字**正确
   （参数名对齐 datahub `DatasetSpec.params`，见 `app/datasets/raw.py`）。
2. **映射结果**：经 `mappers` 落回与原 akshare provider 同形的领域模型。

不触网、不触 akshare（兄弟 provider 只依赖 client + mappers）。
"""
import os
import sys
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.models import resolve_symbol  # noqa: E402
from src.providers.announcement.datahub_announcement import (  # noqa: E402
    DataHubAnnouncementProvider,
)
from src.providers.capital.datahub_lhb import DataHubLHBProvider  # noqa: E402
from src.providers.financial.datahub_financial import DataHubFinancialProvider  # noqa: E402
from src.providers.industry.datahub_index import DataHubIndexProvider  # noqa: E402
from src.providers.industry.datahub_industry import DataHubIndustryProvider  # noqa: E402
from src.providers.macro.datahub_macro import DataHubMacroProvider  # noqa: E402
from src.providers.market.datahub_market import DataHubMarketProvider  # noqa: E402
from src.providers.news.datahub_news import DataHubNewsProvider  # noqa: E402

AS_OF = date(2026, 10, 13)


class MockClient:
    """记录调用并按 dataset 返回固定行。`responses[dataset]` 可为 list 或 callable(params)。"""

    def __init__(self, responses):
        self.responses = responses
        self.calls = []

    def get(self, dataset_id, params=None):
        self.calls.append((dataset_id, dict(params or {})))
        r = self.responses.get(dataset_id, [])
        return r(params or {}) if callable(r) else r

    def last(self):
        return self.calls[-1]


def _sym(code="600519"):
    return resolve_symbol(code)


# ---------- 行情 ----------

def test_market_history_params_and_bars():
    rows = [
        {"code": "600519", "trade_date": "2026-10-10", "open": 100, "high": 105,
         "low": 99, "close": 104, "volume": 12345, "amount": 1.0e8,
         "turnover_rate": 1.5},
        {"code": "600519", "trade_date": "2026-10-11", "open": 104, "high": 110,
         "low": 103, "close": 108, "volume": 23456, "amount": 2.0e8,
         "turnover_rate": 2.0},
    ]
    client = MockClient({"daily_price": rows})
    prov = DataHubMarketProvider(as_of=AS_OF, client=client)
    bars = prov.get_history(_sym(), date(2026, 10, 1), AS_OF)

    assert client.last() == ("daily_price", {
        "codes": "600519", "start": "2026-10-01", "end": "2026-10-13"})
    assert len(bars) == 2
    # volume 已是手、turnover 已是 % —— 不得再 //100 / ×100
    assert bars[1].volume == 23456
    assert bars[1].turnover == 2.0
    # 涨跌幅由前收派生
    assert round(bars[1].pct_change, 4) == round((108 - 104) / 104 * 100, 4)


def test_market_quote_last_two():
    rows = [
        {"trade_date": "2026-10-12", "open": 100, "high": 105, "low": 99,
         "close": 104, "volume": 1, "amount": 1, "turnover_rate": 1.0},
        {"trade_date": "2026-10-13", "open": 104, "high": 110, "low": 103,
         "close": 108, "volume": 2, "amount": 2, "turnover_rate": 2.0},
    ]
    client = MockClient({"daily_price": rows})
    prov = DataHubMarketProvider(as_of=AS_OF, client=client)
    q = prov.get_quote(_sym())
    assert client.last() == ("daily_price", {
        "codes": "600519", "last": 2, "asof": "2026-10-13"})
    assert q.price == 108 and q.pre_close == 104


# ---------- 财务 / 估值 ----------

def test_financial_params_and_snapshots():
    rows = [
        {"code": "600519", "report_date": "2026-09-30", "announce_date": "2026-10-20",
         "revenue_growth": 10.0, "profit_growth": 12.0, "gross_margin": 90.0,
         "net_margin": 50.0, "roe": 30.0, "debt_ratio": 20.0},
        {"code": "600519", "report_date": "2026-06-30", "announce_date": "2026-08-01",
         "revenue_growth": 9.0, "profit_growth": 11.0, "gross_margin": 89.0,
         "net_margin": 49.0, "roe": 29.0, "debt_ratio": 21.0},
    ]
    client = MockClient({"financial_indicator": rows})
    prov = DataHubFinancialProvider(as_of=AS_OF, client=client)
    snaps = prov.get_financials(_sym(), quarters=2)
    assert client.last() == ("financial_indicator", {
        "codes": "600519", "last": 2, "as_of": "2026-10-13"})
    assert [s.report_type for s in snaps] == ["Q3", "Q2"]
    assert snaps[0].net_margin == 50.0


def test_financial_valuation_last_row():
    rows = [{"trade_date": "2026-10-13", "pe_ttm": 25.0, "pb": 8.0,
             "total_mv": 2.0e12, "float_mv": 2.0e12}]
    client = MockClient({"daily_valuation": rows})
    prov = DataHubFinancialProvider(as_of=AS_OF, client=client)
    v = prov.get_valuation(_sym())
    assert client.last() == ("daily_valuation", {
        "codes": "600519", "last": 1, "asof": "2026-10-13"})
    assert v.pe_ttm == 25.0 and v.market_cap == 2.0e12


# ---------- 龙虎榜 ----------

def test_capital_search_and_detail():
    rec_rows = [{"trade_date": "2026-10-10", "name": "贵州茅台", "close": 100.0,
                 "pct_change": 9.9, "turnover_rate": 3.0, "reasons": "日涨幅；换手率"}]
    det_rows = [{"trade_date": "2026-10-10", "dealer_name": "某营业部",
                 "buy_amount": 1.0e7, "buy_pct": 1.0, "sell_amount": 0.0,
                 "sell_pct": 0.0, "net_amount": 1.0e7, "reason": "买入"}]
    client = MockClient({"capital_flow": rec_rows, "capital_flow_detail": det_rows})
    prov = DataHubLHBProvider(as_of=AS_OF, client=client)

    recs = prov.search(_sym(), days=30, end=AS_OF)
    assert client.last() == ("capital_flow", {
        "codes": "600519", "start": "2026-09-13", "end": "2026-10-13"})
    assert recs[0].reasons == ("日涨幅", "换手率")

    dets = prov.get_detail(_sym(), date(2026, 10, 10))
    assert client.last() == ("capital_flow_detail", {
        "codes": "600519", "date": "2026-10-10"})
    assert dets[0].dealer_name == "某营业部"


# ---------- 行业（个股） ----------

def test_industry_get_industry():
    hit = [{"index_code": "851251", "stock_code": "600519"}]
    info = [{"code": "851251", "level": 3, "name": "白酒Ⅲ"}]
    cons = [
        {"index_code": "851251", "stock_code": "600519.SH", "stock_name": "贵州茅台",
         "market_cap": 20000, "pe_ttm": 25.0, "pb": 8.0, "roe": 30.0,
         "dividend_yield": 2.0, "net_profit_growth": 12.0, "revenue_growth": 10.0},
        {"index_code": "851251", "stock_code": "000858.SZ", "stock_name": "五粮液",
         "market_cap": 5000, "pe_ttm": 18.0, "pb": 4.0, "roe": 22.0,
         "dividend_yield": 3.0, "net_profit_growth": 9.0, "revenue_growth": 8.0},
    ]

    def by_codes(params):
        return cons if params.get("index_codes") else hit

    client = MockClient({"industry_sw_cons": by_codes, "industry_sw_info": info})
    prov = DataHubIndustryProvider(as_of=AS_OF, client=client)
    a = prov.get_industry(_sym())
    assert a.industry_name == "白酒Ⅲ" and a.industry_code == "851251"
    assert a.peer_count == 2
    # 按市值降序
    assert [p.symbol for p in a.peers] == ["600519", "000858"]


# ---------- 行业（指数） ----------

def test_industry_index_resolve_history_constituents():
    info_all = [
        {"code": "851251", "level": 3, "name": "白酒Ⅲ", "level_label": "申万三级",
         "parent_name": "白酒Ⅱ", "member_count": 2, "pe_static": 24.0,
         "pe_ttm": 25.0, "pb": 8.0, "dividend_yield": 2.0},
    ]
    idx_rows = [
        {"code": "851251", "trade_date": "2026-10-10", "open": 100, "high": 105,
         "low": 99, "close": 104, "pct_change": 1.0, "amplitude": 6.0,
         "chg_amount": 1.0},
        {"code": "851251", "trade_date": "2026-10-11", "open": 104, "high": 108,
         "low": 102, "close": 107, "pct_change": 2.88, "amplitude": 5.0,
         "chg_amount": 3.0},
    ]
    cons = [
        {"index_code": "851251", "stock_code": "600519.SH", "stock_name": "贵州茅台",
         "market_cap": 20000, "pe_ttm": 25.0, "pb": 8.0, "roe": 30.0,
         "dividend_yield": 2.0, "net_profit_growth": 12.0, "revenue_growth": 10.0},
    ]
    client = MockClient({
        "industry_sw_info": info_all, "industry_index": idx_rows,
        "industry_sw_cons": cons,
    })
    prov = DataHubIndexProvider(as_of=AS_OF, client=client)

    ref = prov.resolve("851251")
    assert ref is not None and ref.level == 3 and ref.level_label == "申万三级"

    bars = prov.get_history(resolve_symbol("sw851251"), date(2026, 10, 1), AS_OF)
    assert client.last() == ("industry_index", {
        "codes": "851251", "start": "2026-10-01", "end": "2026-10-13"})
    # 指数不采量价 → 恒 0
    assert bars[0].volume == 0 and bars[0].turnover == 0.0
    assert bars[0].pct_change == 1.0   # 用逐行真值，非重算

    cons_out = prov.get_constituents(ref)
    assert cons_out[0].symbol == "600519"
    # 同层级排名：仅 1 个有效 PE
    rank = prov.valuation_rank(ref)
    assert rank.rank == 1 and rank.universe == 1


# ---------- 宏观 ----------

def test_macro_series_monthly_and_quarter():
    cpi_rows = [
        {"indicator": "cn_cpi", "period_label": "2026年07月份", "period_end": "2026-07-31",
         "cadence": "month", "level": 100.5, "yoy": 0.5, "mom": 0.1},
        {"indicator": "cn_cpi", "period_label": "2026年08月份", "period_end": "2026-08-31",
         "cadence": "month", "level": 100.8, "yoy": 0.8, "mom": 0.2},
    ]
    gdp_rows = [
        {"indicator": "cn_gdp", "period_label": "2026年第1季度", "period_end": "2026-03-31",
         "cadence": "quarter", "level": 300000.0, "yoy": 5.0, "mom": None},
        {"indicator": "cn_gdp", "period_label": "2026年第1-2季度", "period_end": "2026-06-30",
         "cadence": "quarter", "level": 700000.0, "yoy": 5.2, "mom": None},
    ]

    def by_ind(params):
        return cpi_rows if params.get("indicator") == "cn_cpi" else gdp_rows

    client = MockClient({"macro": by_ind})
    prov = DataHubMacroProvider(as_of=AS_OF, client=client)

    ref = prov.resolve("cn_cpi")
    s = prov.get_series(ref)
    assert client.last() == ("macro", {"indicator": "cn_cpi"})
    assert s.latest.period.label == "2026年08月份" and s.latest.level == 100.8

    gref = prov.resolve("cn_gdp")
    gs = prov.get_series(gref)
    assert gs.points[-1].single_quarter_level == 400000.0   # 700000 − 300000


# ---------- 新闻 / 公告 ----------

def test_news_and_announcement_search_window():
    news_rows = [{"code": "600519", "title": "T", "content": "C",
                  "published_at": "2026-10-10 09:00:00", "source": "S",
                  "url": "http://x"}]
    ann_rows = [{"code": "600519", "title": "公告", "category": "A",
                 "publish_date": "2026-10-10", "url": "http://y"}]

    nclient = MockClient({"news_by_stock": news_rows})
    nprov = DataHubNewsProvider(as_of=AS_OF, client=nclient)
    items = nprov.search(_sym(), days=30)
    assert nclient.last() == ("news_by_stock", {
        "codes": "600519", "start": "2026-09-13", "end": "2026-10-13"})
    assert items[0].title == "T"

    aclient = MockClient({"announcement_by_stock": ann_rows})
    aprov = DataHubAnnouncementProvider(as_of=AS_OF, client=aclient)
    anns = aprov.search(_sym(), days=30)
    assert aclient.last() == ("announcement_by_stock", {
        "codes": "600519", "start": "2026-09-13", "end": "2026-10-13"})
    assert anns[0].title == "公告"
