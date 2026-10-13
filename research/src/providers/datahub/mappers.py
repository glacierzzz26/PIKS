"""datahub 行 → research 领域模型映射（P14-1 / B4：单位约定，最高频 bug 源）。

逐条对齐 datahub `app/datasets/raw.py` 冻结的列名，**不臆造列**。

单位约定（**务必逐条核对，这是 100× 量级 bug 的温床**）
--------------------------------------------------------
1. `daily_price.volume` **已经是手**（datahub 采集时未做 akshare 腾讯源的 `//100`）。
   适配层**不得**再 `// 100`。
2. `daily_price.turnover_rate` **已经是 %**（未做腾讯源的 `× 100`）。适配层**不得**再 `× 100`。
3. `close` 用**不复权**价（datahub 存的是不复权收盘），**不乘** `adj_factor`。
4. 派生字段 `pct_change / amplitude / chg_amount / is_limit_up / is_limit_down`
   **留适配层** —— 依赖 `Symbol.limit_up_pct` 的 PIKS 涨跌停规则，datahub 不得内嵌。
5. 申万指数 `industry_index`：量纲与 A 股不兼容 ⇒ `volume/amount/turnover` **恒 0**，
   `is_limit_up/down` 恒 False（指数无涨跌停制度）；`pct/amplitude/chg_amount`
   直接用 datahub 采到的**逐行真值**（非按窗口重算）。

缺失一律 `None`（不补 0）—— 「缺失」是不知道，「0」是为零（`_f` vs `_f0`）。
"""
from __future__ import annotations

import math
from datetime import date, datetime

from ...models import Bar, Quote, Symbol
from ...models.announcement import Announcement
from ...models.financial import FinancialSnapshot, ValuationMetrics
from ...models.news import NewsItem
from ..capital.akshare_lhb import LHBDetail, LHBRecord
from ..industry.sw_index_provider import Constituent, IndustryRef
from ..industry.sw_provider import PeerMetric
from ..macro.macro_provider import MacroPoint, parse_period

# 季末日 (月*100+日) → report_type（与 akshare financial provider 同判定）。
_REPORT_TYPE = {331: "Q1", 630: "Q2", 930: "Q3", 1231: "annual"}

# 申万层级 → PIKS 展示标签（**沿用 sw_index_provider 的「申万N级」**，
# 不用 datahub 存的中性「N级」——两侧展示串必须一致）。
_SW_LEVEL_LABEL = {1: "申万一级", 2: "申万二级", 3: "申万三级"}

_MISSING = {"", "-", "nan", "NaN", "None", "null", "N/A", "NoneType", "--"}


# ---------- 基础取值 ----------

def _f(v):
    """→ float 或 None（缺失/NaN/占位符一律 None，不补 0）。"""
    if v is None:
        return None
    if isinstance(v, float) and math.isnan(v):
        return None
    if isinstance(v, (int, float)):
        return float(v)
    s = str(v).strip()
    if s in _MISSING:
        return None
    try:
        return float(s)
    except (TypeError, ValueError):
        return None


def _f0(v) -> float:
    """→ float，缺失补 0.0（用于 DDL NOT NULL 的金额字段）。"""
    x = _f(v)
    return 0.0 if x is None else x


def _i(v):
    x = _f(v)
    return None if x is None else int(x)


def _i0(v) -> int:
    x = _f(v)
    return 0 if x is None else int(x)


def _date(v):
    if v is None:
        return None
    if isinstance(v, datetime):
        return v.date()
    if isinstance(v, date):
        return v
    s = str(v).strip()
    for fmt in ("%Y-%m-%d", "%Y/%m/%d", "%Y%m%d"):
        try:
            return datetime.strptime(s, fmt).date()
        except ValueError:
            continue
    return None


def _datetime(v):
    if v is None:
        return None
    if isinstance(v, datetime):
        return v
    s = str(v).strip()
    for fmt in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M", "%Y-%m-%d"):
        try:
            return datetime.strptime(s, fmt)
        except ValueError:
            continue
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


def _sorted_by(rows, key):
    """按某日期列升序；缺日期者排最后（不丢行，调用方可再筛）。"""
    def _k(r):
        d = _date(r.get(key))
        return (d is None, d or date.min)
    return sorted(rows, key=_k)


# ---------- 行情 ----------

def to_bars(symbol: Symbol, rows) -> list[Bar]:
    """`daily_price` 行 → Bar 列表（升序）。

    volume/turnover_rate **原样取用**（datahub 已是手/%）；close 不复权；
    pct/amplitude/chg 由前收派生（首行 0），涨跌停用 `Symbol.limit_up_pct` 判。
    """
    bars: list[Bar] = []
    prev_close = None
    for r in _sorted_by(rows, "trade_date"):
        d = _date(r.get("trade_date"))
        if d is None:
            continue
        close = _f0(r.get("close"))
        high, low = _f0(r.get("high")), _f0(r.get("low"))
        if prev_close and prev_close > 0:
            pct = (close - prev_close) / prev_close * 100
            amplitude = (high - low) / prev_close * 100
            chg = close - prev_close
        else:
            pct = amplitude = chg = 0.0
        bars.append(Bar(
            symbol=symbol.full_code,
            date=d,
            open=_f0(r.get("open")),
            high=high,
            low=low,
            close=close,
            volume=_i0(r.get("volume")),        # 已是手 —— 勿再 //100
            amount=_f0(r.get("amount")),
            amplitude=amplitude,
            pct_change=pct,
            chg_amount=chg,
            turnover=_f0(r.get("turnover_rate")),  # 已是 % —— 勿再 ×100
            is_limit_up=pct >= symbol.limit_up_pct * 0.99,
            is_limit_down=pct <= symbol.limit_down_pct * 0.99,
            is_halted=False,
        ))
        prev_close = close
    return bars


def to_index_bars(symbol: Symbol, rows) -> list[Bar]:
    """`industry_index` 行 → Bar 列表（升序；量价恒 0，非涨停标记）。

    pct/amplitude/chg_amount 用 datahub 采到的逐行真值（非按返回窗口重算）。
    """
    bars: list[Bar] = []
    for r in _sorted_by(rows, "trade_date"):
        d = _date(r.get("trade_date"))
        if d is None:
            continue
        bars.append(Bar(
            symbol=symbol.full_code,
            date=d,
            open=_f0(r.get("open")),
            high=_f0(r.get("high")),
            low=_f0(r.get("low")),
            close=_f0(r.get("close")),
            volume=0,           # 申万量纲与 A 股不兼容 —— 恒 0
            amount=0.0,
            amplitude=_f0(r.get("amplitude")),
            pct_change=_f0(r.get("pct_change")),
            chg_amount=_f0(r.get("chg_amount")),
            turnover=0.0,
            is_limit_up=False,
            is_limit_down=False,
            is_halted=False,
        ))
    return bars


def to_quote(symbol: Symbol, rows) -> Quote | None:
    """`daily_price` 行 → 最新快照（取末两行）。不足两行 → None（由调用方处置）。"""
    ordered = _sorted_by(rows, "trade_date")
    if len(ordered) < 2:
        return None
    today, prev = ordered[-1], ordered[-2]
    close, prev_close = _f0(today.get("close")), _f0(prev.get("close"))
    pct = (close - prev_close) / prev_close * 100 if prev_close else 0.0
    return Quote(
        symbol=symbol.full_code,
        name=symbol.code,
        price=close,
        pre_close=prev_close,
        open=_f0(today.get("open")),
        high=_f0(today.get("high")),
        low=_f0(today.get("low")),
        volume=_i0(today.get("volume")),
        amount=_f0(today.get("amount")),
        pct_change=pct,
        turnover=_f0(today.get("turnover_rate")),
        pe_ttm=0.0,
        pb=0.0,
        market_cap=0.0,
        float_cap=0.0,
        timestamp=datetime.now(),
    )


# ---------- 财务 / 估值 ----------

def to_financial_snapshots(symbol: Symbol, rows) -> list[FinancialSnapshot]:
    """`financial_indicator` 行 → FinancialSnapshot 列表（保持 datahub 返回顺序）。

    只取财报日（季末/年末）行，按 report_date 去重（与 akshare provider 一致）。
    `net_margin` 来自 datahub 侧表 `financial_indicator_ext`（未采集时为 None）。
    """
    out: list[FinancialSnapshot] = []
    seen: set = set()
    for r in rows:
        d = _date(r.get("report_date"))
        if d is None:
            continue
        rtype = _REPORT_TYPE.get(d.month * 100 + d.day)
        if rtype is None:
            continue                       # 非季末行跳过
        if d in seen:
            continue
        seen.add(d)
        out.append(FinancialSnapshot(
            symbol=symbol.full_code,
            report_date=d,
            report_type=rtype,
            revenue_yoy=_f(r.get("revenue_growth")),
            net_profit_yoy=_f(r.get("profit_growth")),
            gross_margin=_f(r.get("gross_margin")),
            net_margin=_f(r.get("net_margin")),
            roe=_f(r.get("roe")),
            debt_ratio=_f(r.get("debt_ratio")),
        ))
    return out


def to_valuation(symbol: Symbol, row) -> ValuationMetrics | None:
    """`daily_valuation` 末行 → ValuationMetrics（无行 → None）。"""
    if not row:
        return None
    return ValuationMetrics(
        symbol=symbol.full_code,
        as_of=_date(row.get("trade_date")) or date.today(),
        pe_ttm=_f(row.get("pe_ttm")),
        pb=_f(row.get("pb")),
        market_cap=_f(row.get("total_mv")),
        float_cap=_f(row.get("float_mv")),
    )


# ---------- 龙虎榜 ----------

def to_lhb_records(symbol: Symbol, rows) -> list[LHBRecord]:
    """`capital_flow` 行 → LHBRecord 列表（reasons 串按「；」还原为 tuple）。"""
    out: list[LHBRecord] = []
    for r in rows:
        d = _date(r.get("trade_date"))
        if d is None:
            continue
        reasons = tuple(x for x in str(r.get("reasons") or "").split("；") if x)
        out.append(LHBRecord(
            symbol=symbol.full_code,
            name=str(r.get("name") or ""),
            trade_date=d,
            close_price=_f0(r.get("close")),
            pct_change=_f0(r.get("pct_change")),
            turnover_pct=_f0(r.get("turnover_rate")),
            reasons=reasons,
        ))
    return out


def to_lhb_details(rows) -> list[LHBDetail]:
    """`capital_flow_detail` 行 → LHBDetail 列表（金额缺失补 0.0，镜像原 _to_float）。"""
    out: list[LHBDetail] = []
    for r in rows:
        out.append(LHBDetail(
            trade_date=_date(r.get("trade_date")),
            dealer_name=str(r.get("dealer_name") or ""),
            buy_amount=_f0(r.get("buy_amount")),
            buy_pct=_f0(r.get("buy_pct")),
            sell_amount=_f0(r.get("sell_amount")),
            sell_pct=_f0(r.get("sell_pct")),
            net_amount=_f0(r.get("net_amount")),
            reason=str(r.get("reason") or ""),
        ))
    return out


# ---------- 行业 ----------

def to_industry_ref(row) -> IndustryRef | None:
    """`industry_sw_info` 单行 → IndustryRef（level_label 由 level 派生「申万N级」）。"""
    if not row:
        return None
    level = _i(row.get("level")) or 0
    parent = row.get("parent_name")
    return IndustryRef(
        code=str(row.get("code") or ""),
        name=str(row.get("name") or ""),
        level=level,
        level_label=_SW_LEVEL_LABEL.get(level, str(row.get("level_label") or "")),
        parent=(str(parent) if parent else None),
        member_count=_i(row.get("member_count")),
        pe_static=_f(row.get("pe_static")),
        pe_ttm=_f(row.get("pe_ttm")),
        pb=_f(row.get("pb")),
        dividend_yield=_f(row.get("dividend_yield")),
    )


def to_constituents(rows) -> list[Constituent]:
    """`industry_sw_cons` 行 → Constituent 列表（成分股代码去 `.SH/.SZ`）。"""
    out: list[Constituent] = []
    for r in rows:
        code = str(r.get("stock_code") or "").strip().split(".")[0]
        if not code:
            continue
        out.append(Constituent(
            symbol=code,
            name=str(r.get("stock_name") or ""),
            market_cap=_f(r.get("market_cap")),
            pe_ttm=_f(r.get("pe_ttm")),
            pb=_f(r.get("pb")),
            roe=_f(r.get("roe")),
            dividend_yield=_f(r.get("dividend_yield")),
            net_profit_growth=_f(r.get("net_profit_growth")),
            revenue_growth=_f(r.get("revenue_growth")),
        ))
    return out


def to_peer_metrics(rows) -> list[PeerMetric]:
    """`industry_sw_cons` 行 → PeerMetric 列表（供个股行业对比；datahub 无价格列 → None）。"""
    out: list[PeerMetric] = []
    for r in rows:
        code = str(r.get("stock_code") or "").strip().split(".")[0]
        if not code:
            continue
        out.append(PeerMetric(
            symbol=code,
            name=str(r.get("stock_name") or ""),
            price=None,                      # datahub 未冻结价格列
            pe_ttm=_f(r.get("pe_ttm")),
            pb=_f(r.get("pb")),
            roe=_f(r.get("roe")),
            dividend_yield=_f(r.get("dividend_yield")),
            market_cap=_f(r.get("market_cap")),
            net_profit_growth=_f(r.get("net_profit_growth")),
            revenue_growth=_f(r.get("revenue_growth")),
        ))
    return out


# ---------- 宏观 ----------

def to_macro_points(rows) -> list[MacroPoint]:
    """`macro` 行 → MacroPoint 列表（保持顺序；升序化/差分由调用方沿用 MacroProvider）。"""
    out: list[MacroPoint] = []
    for r in rows:
        label = r.get("period_label")
        if not label:
            continue
        period = parse_period(str(label))    # 非法期号抛错（不静默丢期）
        out.append(MacroPoint(
            period=period,
            level=_f(r.get("level")),
            yoy=_f(r.get("yoy")),
            mom=_f(r.get("mom")),
            cumulative=(str(r.get("cadence") or "") == "quarter"),
            total_yoy=_f(r.get("yoy")),
        ))
    return out


# ---------- 新闻 / 公告 ----------

def to_news_items(symbol: Symbol, rows) -> list[NewsItem]:
    """`news_by_stock` 行 → NewsItem 列表（时间范围已在 datahub 侧过滤，此处不重滤）。"""
    out: list[NewsItem] = []
    for r in rows:
        pub = _datetime(r.get("published_at"))
        if pub is None:
            continue
        url = r.get("url")
        out.append(NewsItem(
            id=f"news_{symbol.code}_{pub.strftime('%Y%m%d%H%M%S')}",
            symbol=symbol.full_code,
            title=str(r.get("title") or ""),
            content=str(r.get("content") or ""),
            published_at=pub,
            source=str(r.get("source") or ""),
            url=(str(url) if url else None),
        ))
    return out


def to_announcements(symbol: Symbol, rows) -> list[Announcement]:
    """`announcement_by_stock` 行 → Announcement 列表（空标题已在 datahub 侧丢弃）。"""
    out: list[Announcement] = []
    for i, r in enumerate(rows):
        d = _date(r.get("publish_date"))
        if d is None:
            continue
        url = r.get("url")
        out.append(Announcement(
            id=f"ann_{symbol.code}_{d.strftime('%Y%m%d')}_{i}",
            symbol=symbol.full_code,
            title=str(r.get("title") or ""),
            category=str(r.get("category") or ""),
            publish_date=d,
            url=(str(url) if url else None),
        ))
    return out
