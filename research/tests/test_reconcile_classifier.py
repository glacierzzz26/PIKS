"""test_reconcile_classifier.py — 对账比较器的纯函数单测（P14-1 / 验证 2）。

CI 安全：只测 `scripts/reconcile_datahub_vs_akshare.py` 的纯判定核心，
不建 provider、不触网、不触 akshare。

覆盖：5 桶归类（accepted/drifted/missing_local/extra_datahub/rejected）+
容差形态（相对/绝对）+ 缺失即相等 + **硬列漂移 ⇒ hard_drift**（放行门）。
"""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "scripts"))

import importlib.util  # noqa: E402


def _load_mod():
    path = os.path.join(os.path.dirname(__file__), "..", "scripts",
                        "reconcile_datahub_vs_akshare.py")
    spec = importlib.util.spec_from_file_location("recon_mod", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


rec = _load_mod()
COLS = ["close", "volume", "turnover"]


def test_accepted_within_tolerance():
    a = {"k": "2026-10-10", "close": 100.0, "volume": 1000, "turnover": 1.0}
    b = {"k": "2026-10-10", "close": 100.05, "volume": 1000, "turnover": 1.005}
    status, cols = rec.classify_row(a, b, COLS)   # close rel 0.001 → 0.05% 内
    assert status == "accepted" and cols == []


def test_drifted_on_soft_column():
    a = {"k": "x", "close": 100.0, "volume": 1000, "turnover": 1.0}
    b = {"k": "x", "close": 100.0, "volume": 1000, "turnover": 5.0}   # 换手差 4pp
    status, cols = rec.classify_row(a, b, COLS)
    assert status == "drifted" and cols == ["turnover"]


def test_missing_both_sides_is_equal():
    a = {"k": "x", "close": 100.0, "volume": 1000, "turnover": None}
    b = {"k": "x", "close": 100.0, "volume": 1000, "turnover": None}
    assert rec.classify_row(a, b, COLS) == ("accepted", [])


def test_one_side_missing_is_drift():
    a = {"k": "x", "close": 100.0, "volume": 1000, "turnover": None}
    b = {"k": "x", "close": 100.0, "volume": 1000, "turnover": 1.0}
    status, cols = rec.classify_row(a, b, COLS)
    assert status == "drifted" and cols == ["turnover"]


def test_reconcile_five_buckets():
    legacy = [
        {"k": "d1", "close": 10.0, "volume": 1, "turnover": 1.0},   # accepted
        {"k": "d2", "close": 20.0, "volume": 2, "turnover": 2.0},   # drifted (volume)
        {"k": "d3", "close": 30.0, "volume": 3, "turnover": 3.0},   # missing_local
        {"k": None, "close": 40.0, "volume": 4, "turnover": 4.0},   # rejected
    ]
    new = [
        {"k": "d1", "close": 10.0, "volume": 1, "turnover": 1.0},
        {"k": "d2", "close": 20.0, "volume": 9, "turnover": 2.0},
        {"k": "d4", "close": 50.0, "volume": 5, "turnover": 5.0},   # extra_datahub
    ]
    out = rec.reconcile(legacy, new, "k", COLS)
    b = out["buckets"]
    assert [r["key"] for r in b["accepted"]] == ["d1"]
    assert [r["key"] for r in b["drifted"]] == ["d2"]
    assert b["drifted"][0]["columns"] == ["volume"]
    assert [r["key"] for r in b["missing_local"]] == ["d3"]
    assert [r["key"] for r in b["extra_datahub"]] == ["d4"]
    assert b["rejected"] and b["rejected"][0]["count"] == 1
    # volume 是硬列 ⇒ 闸门不过
    assert out["hard_drift"] is True


def test_hard_drift_false_on_soft_only():
    legacy = [{"k": "d1", "close": 10.0, "volume": 1, "turnover": 1.0}]
    new = [{"k": "d1", "close": 10.0, "volume": 1, "turnover": 9.0}]
    out = rec.reconcile(legacy, new, "k", COLS)
    assert out["hard_drift"] is False


def test_valuation_dataset_registered():
    """估值虽随 financial 闸门开，但须有独立 spec 覆盖（否则对账漏一域）。"""
    assert rec._SPECS["daily_valuation"] is rec._spec_valuation
    assert {"market_cap", "float_cap", "pe_ttm", "pb"} <= set(rec._TOL)


def test_drifted_carries_both_side_values():
    """drifted 行须带双侧数值 —— 供分辨「1 手级厂商差」vs「半量级真 bug」。"""
    legacy = [{"k": "d1", "close": 10.0, "volume": 100, "turnover": 1.0}]
    new = [{"k": "d1", "close": 10.0, "volume": 300, "turnover": 1.0}]
    out = rec.reconcile(legacy, new, "k", COLS)
    row = out["buckets"]["drifted"][0]
    assert row["columns"] == ["volume"]
    assert row["values"]["volume"] == {"legacy": 100.0, "new": 300.0}
