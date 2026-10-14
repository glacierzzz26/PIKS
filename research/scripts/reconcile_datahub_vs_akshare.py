#!/usr/bin/env python3
"""reconcile_datahub_vs_akshare.py — 新链路 vs 旧 akshare 跨源对账（P14-1 / 验证 2）。

**用途**：放闸前，对**同一 (主体, 区间, as_of)** 在**同进程内**建两个 provider
（旧 = akshare，新 = datahub 兄弟 provider），单位归一后逐键逐列比对，输出 5 桶：

- `accepted`      两端都有、共享列全在容差内
- `drifted`       两端都有、有列超容差（列出漂移列）
- `missing_local` 旧链路有、datahub 缺
- `extra_datahub` datahub 有、旧链路缺
- `rejected`      键非法/不可比（如缺日期）—— 不静默丢

**放行门**：`drifted` 落在**硬列**（`close`/`volume`）⇒ exit 1。

⚠️ 需要 datahub 可达（`DATAHUB_BASE_URL`/`DATAHUB_TOKEN`）。**在 datahub 采集开之后**跑，
且对账期间**读闸门保持关**（两个 provider 显式直构，不依赖 env 闸门）。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import date, datetime

sys.path.insert(0, os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

# 容差：键 → (模式, 阈值)。abs = 绝对差；rel = 相对差。
# close/volume 为**硬列**（超容差即 exit 1）；其余为软列。
_TOL = {
    "open": ("rel", 0.001), "high": ("rel", 0.001),
    "low": ("rel", 0.001), "close": ("rel", 0.001),
    "volume": ("abs", 0.0), "amount": ("rel", 0.005),
    "turnover_rate": ("abs", 0.01), "turnover": ("abs", 0.01),
    "pct_change": ("abs", 0.05), "amplitude": ("abs", 0.05),
    "chg_amount": ("rel", 0.005),
    "revenue_growth": ("abs", 0.05), "profit_growth": ("abs", 0.05),
    "net_profit_yoy": ("abs", 0.05), "gross_margin": ("abs", 0.05),
    "net_margin": ("abs", 0.05), "roe": ("abs", 0.05), "debt_ratio": ("abs", 0.05),
    "level": ("rel", 0.001), "yoy": ("abs", 0.05), "mom": ("abs", 0.05),
    "pe_ttm": ("rel", 0.01), "pb": ("rel", 0.01),
}
_HARD_COLS = frozenset({"close", "volume"})


def _num(v):
    if v is None or isinstance(v, bool):
        return None
    if isinstance(v, (int, float)):
        return float(v)
    try:
        return float(str(v).strip())
    except (TypeError, ValueError):
        return None


def _within(a, b, col):
    """a、b 在 col 的容差内？缺失（None/NaN）按**相等**处理（两侧都缺 = 一致）。"""
    if a is None and b is None:
        return True
    if a is None or b is None:
        return False
    mode, thr = _TOL.get(col, ("rel", 0.005))
    if mode == "abs":
        return abs(a - b) <= thr
    denom = max(abs(a), abs(b), 1e-12)
    return abs(a - b) / denom <= thr


def classify_row(legacy: dict, new: dict, columns) -> tuple[str, list[str]]:
    """同键两行逐列比对 → (status, drifted_cols)。纯函数，供单测直接调。"""
    drifted = [c for c in columns
               if not _within(_num(legacy.get(c)), _num(new.get(c)), c)]
    return ("accepted" if not drifted else "drifted"), drifted


def reconcile(legacy_rows, new_rows, key_col, columns, hard_cols=_HARD_COLS) -> dict:
    """按 key_col 比对两批行 → 5 桶 + 硬列漂移标志。"""
    lmap = {r.get(key_col): r for r in legacy_rows if r.get(key_col) is not None}
    nmap = {r.get(key_col): r for r in new_rows if r.get(key_col) is not None}
    n_rejected = len(legacy_rows) - len(lmap) + len(new_rows) - len(nmap)

    buckets = {"accepted": [], "drifted": [], "missing_local": [],
               "extra_datahub": [], "rejected": []}
    if n_rejected:
        buckets["rejected"] = [{"count": n_rejected, "reason": "键缺失/不可比"}]

    hard_drift = False
    for k in sorted(set(lmap) | set(nmap), key=str):
        if k not in nmap:
            buckets["missing_local"].append({"key": str(k)})
        elif k not in lmap:
            buckets["extra_datahub"].append({"key": str(k)})
        else:
            status, cols = classify_row(lmap[k], nmap[k], columns)
            if status == "accepted":
                buckets["accepted"].append({"key": str(k)})
            else:
                buckets["drifted"].append({"key": str(k), "columns": cols})
                if set(cols) & set(hard_cols):
                    hard_drift = True
    return {"buckets": buckets, "hard_drift": hard_drift}


# ---------- 各数据集：建 provider + 抽规范化行 ----------

def _d(v):
    if isinstance(v, (date, datetime)):
        return v.strftime("%Y-%m-%d") if isinstance(v, date) else v.date().isoformat()
    return str(v)


def _spec_daily_price(code, start, end, as_of):
    from src.providers.market.datahub_market import DataHubMarketProvider
    from src.providers.market.failover_provider import FailoverMarketProvider
    from src.models import resolve_symbol

    sym = resolve_symbol(code)
    cols = ["open", "high", "low", "close", "volume", "amount", "turnover"]

    def extract(p):
        return [{"trade_date": _d(b.date), "open": b.open, "high": b.high, "low": b.low,
                 "close": b.close, "volume": b.volume, "amount": b.amount,
                 "turnover": b.turnover}
                for b in p.get_history(sym, start, end)]

    return FailoverMarketProvider(), DataHubMarketProvider(as_of=as_of), "trade_date", cols, extract


def _spec_financial(code, start, end, as_of):
    from src.providers.financial.datahub_financial import DataHubFinancialProvider
    from src.providers.financial.akshare_financial import AkShareFinancialProvider
    from src.models import resolve_symbol

    sym = resolve_symbol(code)
    cols = ["revenue_yoy", "net_profit_yoy", "gross_margin", "net_margin", "roe", "debt_ratio"]

    def extract(p):
        return [{"report_date": _d(s.report_date), "revenue_yoy": s.revenue_yoy,
                 "net_profit_yoy": s.net_profit_yoy, "gross_margin": s.gross_margin,
                 "net_margin": s.net_margin, "roe": s.roe, "debt_ratio": s.debt_ratio}
                for s in p.get_financials(sym, quarters=8)]

    return AkShareFinancialProvider(), DataHubFinancialProvider(as_of=as_of), \
        "report_date", cols, extract


def _spec_capital_flow(code, start, end, as_of):
    from src.providers.capital.datahub_lhb import DataHubLHBProvider
    from src.providers.capital.akshare_lhb import AkShareLHBProvider
    from src.models import resolve_symbol

    sym = resolve_symbol(code)
    days = (end - start).days
    cols = ["close_price", "pct_change", "turnover_pct"]

    def extract(p):
        return [{"trade_date": _d(r.trade_date), "close_price": r.close_price,
                 "pct_change": r.pct_change, "turnover_pct": r.turnover_pct}
                for r in p.search(sym, days=days, end=end)]

    return AkShareLHBProvider(), DataHubLHBProvider(as_of=as_of), "trade_date", cols, extract


def _spec_industry_index(code, start, end, as_of):
    from src.providers.industry.datahub_index import DataHubIndexProvider
    from src.providers.industry.sw_index_provider import SwIndexProvider
    from src.models import resolve_symbol

    sym = resolve_symbol(code if code.startswith("sw") else f"sw{code}")
    cols = ["open", "high", "low", "close", "pct_change", "amplitude", "chg_amount"]

    def extract(p):
        return [{"trade_date": _d(b.date), "open": b.open, "high": b.high, "low": b.low,
                 "close": b.close, "pct_change": b.pct_change,
                 "amplitude": b.amplitude, "chg_amount": b.chg_amount}
                for b in p.get_history(sym, start, end)]

    return SwIndexProvider(), DataHubIndexProvider(as_of=as_of), "trade_date", cols, extract


def _spec_macro(key, start, end, as_of):
    from src.providers.macro.datahub_macro import DataHubMacroProvider
    from src.providers.macro.macro_provider import MacroProvider

    cols = ["level", "yoy", "mom"]

    def extract(p):
        ref = p.resolve(key)
        if ref is None:
            return []
        return [{"period": pt.period.label, "level": pt.level, "yoy": pt.yoy, "mom": pt.mom}
                for pt in p.get_series(ref).points]

    return MacroProvider(), DataHubMacroProvider(as_of=as_of), "period", cols, extract


# dataset → spec_builder(code, start, end, as_of) → (legacy_p, new_p, key_col, cols, extract)
_SPECS = {
    "daily_price": _spec_daily_price,
    "financial_indicator": _spec_financial,
    "capital_flow": _spec_capital_flow,
    "industry_index": _spec_industry_index,
    "macro": _spec_macro,
}


def run(dataset: str, code: str, start: date, end: date, as_of: date) -> dict:
    if dataset not in _SPECS:
        raise SystemExit(f"未知数据集 {dataset!r}；支持：{', '.join(_SPECS)}")
    legacy_p, new_p, key_col, cols, extract = _SPECS[dataset](code, start, end, as_of)
    return reconcile(extract(legacy_p), extract(new_p), key_col, cols)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="P14-1 datahub vs akshare 跨源对账")
    ap.add_argument("--dataset", required=True, choices=sorted(_SPECS))
    ap.add_argument("--code", required=True, help="主体码（个股 600519 / 行业 sw851251 / 宏观 cn_cpi）")
    ap.add_argument("--start", required=True)
    ap.add_argument("--end", required=True)
    ap.add_argument("--as-of", dest="as_of", default=None)
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args(argv)

    start = datetime.strptime(args.start, "%Y-%m-%d").date()
    end = datetime.strptime(args.end, "%Y-%m-%d").date()
    as_of = datetime.strptime(args.as_of, "%Y-%m-%d").date() if args.as_of else end

    result = run(args.dataset, args.code, start, end, as_of)
    buckets = result["buckets"]
    summary = {k: len(v) for k, v in buckets.items()}

    if args.json:
        print(json.dumps({"dataset": args.dataset, "code": args.code,
                          "summary": summary, "hard_drift": result["hard_drift"],
                          "buckets": buckets}, ensure_ascii=False, indent=2))
    else:
        print(f"[{args.dataset}] {args.code} {start}..{end} (as_of={as_of})")
        for k in ("accepted", "drifted", "missing_local", "extra_datahub", "rejected"):
            print(f"  {k:14s}: {len(buckets[k])}")
        for row in buckets["drifted"][:20]:
            print(f"    drift {row['key']}: {row['columns']}")
        if result["hard_drift"]:
            print("  ✗ 硬列(close/volume)漂移 —— 放行门未过")

    return 1 if result["hard_drift"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
