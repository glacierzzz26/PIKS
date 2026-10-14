"""test_read_gate_off.py — 闸门关 = 字节级回退（P14-1 / 验证 4）。

**CI 安全**：不触网、不触 akshare 实网。核心不变量：

1. 闸门关（token 空 / 白名单空 / 数据集未列）⇒ `factory.make_*()` 返回**原 akshare
   provider 实例**（构造路径与迁移前逐字一致）。
2. 闸门关 ⇒ **绝不构造 / 外呼 `DataHubClient`**（把 `__init__` 换成会炸的桩；若回退
   路径误接 datahub，构造即 AssertionError）。

字节级同产物由「返回同类实例 + 无 datahub 外呼」两条共同保证（真产物 diff 需实网，
由 `scripts/reconcile_datahub_vs_akshare.py` 在放闸前跑）。
"""
import os
import sys
from datetime import date

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.providers import factory  # noqa: E402
from src.providers.datahub import client as dh_client  # noqa: E402
from src.providers.datahub import config  # noqa: E402

# 每个工厂函数 → 闸门关时应返回的 akshare 类名（旧链路）。
_AKSHARE_TYPE = {
    "make_market_provider": "FailoverMarketProvider",
    "make_financial_provider": "AkShareFinancialProvider",
    "make_news_provider": "AkShareNewsProvider",
    "make_announcement_provider": "AkShareAnnouncementProvider",
    "make_capital_provider": "AkShareLHBProvider",
    "make_industry_provider": "SwIndustryProvider",
    "make_industry_index_provider": "SwIndexProvider",
    "make_macro_provider": "MacroProvider",
}

AS_OF = date(2026, 10, 13)


def _forbid_datahub(monkeypatch):
    """任何 DataHubClient 构造即失败 —— 证明回退路径不碰 datahub。"""

    def _boom(self, *a, **k):
        raise AssertionError("闸门关时不得构造 DataHubClient")

    monkeypatch.setattr(dh_client.DataHubClient, "__init__", _boom)


@pytest.mark.parametrize("fn,expected", sorted(_AKSHARE_TYPE.items()))
def test_token_empty_falls_back_to_akshare(monkeypatch, fn, expected):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", ["daily_price"])
    _forbid_datahub(monkeypatch)
    assert type(getattr(factory, fn)(AS_OF)).__name__ == expected


@pytest.mark.parametrize("fn,expected", sorted(_AKSHARE_TYPE.items()))
def test_empty_whitelist_falls_back_to_akshare(monkeypatch, fn, expected):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "tok")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", [])
    _forbid_datahub(monkeypatch)
    assert type(getattr(factory, fn)(AS_OF)).__name__ == expected


@pytest.mark.parametrize("fn,expected", sorted(_AKSHARE_TYPE.items()))
def test_other_dataset_whitelisted_still_falls_back(monkeypatch, fn, expected):
    """按数据集逐个灰度：白名单只放行了别的数据集 ⇒ 本域仍回退。"""
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "tok")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", ["some_other_dataset"])
    _forbid_datahub(monkeypatch)
    assert type(getattr(factory, fn)(AS_OF)).__name__ == expected
