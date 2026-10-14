"""test_provider_factory.py — 工厂读闸门路由（P14-1 / B2、B6）。

**零行为变更证明**：闸门关（token 空 / 白名单空）⇒ `make_*()` 返回**原 akshare 类**
⇒ 与迁移前字节级同产物。闸门开 ⇒ 惰性载入 datahub 兄弟 provider（本用例注入
`_load` 桩以隔离尚未落地的兄弟模块）。
"""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.providers import factory
from src.providers.datahub import config


def _off(monkeypatch):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", [])


def test_gate_off_returns_akshare_providers(monkeypatch):
    _off(monkeypatch)
    from src.providers.announcement.akshare_announcement import (
        AkShareAnnouncementProvider,
    )
    from src.providers.capital.akshare_lhb import AkShareLHBProvider
    from src.providers.financial.akshare_financial import AkShareFinancialProvider
    from src.providers.industry.sw_index_provider import SwIndexProvider
    from src.providers.industry.sw_provider import SwIndustryProvider
    from src.providers.macro.macro_provider import MacroProvider
    from src.providers.market.failover_provider import FailoverMarketProvider
    from src.providers.news.akshare_news import AkShareNewsProvider

    assert isinstance(factory.make_market_provider(), FailoverMarketProvider)
    assert isinstance(factory.make_financial_provider(), AkShareFinancialProvider)
    assert isinstance(factory.make_news_provider(), AkShareNewsProvider)
    assert isinstance(factory.make_announcement_provider(), AkShareAnnouncementProvider)
    assert isinstance(factory.make_capital_provider(), AkShareLHBProvider)
    assert isinstance(factory.make_industry_provider(), SwIndustryProvider)
    assert isinstance(factory.make_industry_index_provider(), SwIndexProvider)
    assert isinstance(factory.make_macro_provider(), MacroProvider)


def test_gate_on_routes_to_datahub_sibling(monkeypatch):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "tok")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", ["daily_price"])
    sentinel = object()
    # 放闸分支：工厂 `_load` 拿到兄弟 provider **类** 后 `cls(as_of=...)` 实例化。
    # 注入一个「类 → 返回 sentinel」的桩以隔离尚未接线的真实取数。
    monkeypatch.setattr(factory, "_load", lambda module, attr: (lambda **kw: sentinel))

    assert factory.make_market_provider() is sentinel       # 放闸的域走 datahub
    # 未放闸的域仍回 akshare（证明按数据集逐个灰度，不是全有全无）
    from src.providers.macro.macro_provider import MacroProvider
    assert isinstance(factory.make_macro_provider(), MacroProvider)


def test_gate_on_requires_token(monkeypatch):
    """白名单有但 token 空 → 仍回 akshare（绝不做未鉴权外呼）。"""
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", ["daily_price"])
    from src.providers.market.failover_provider import FailoverMarketProvider
    assert isinstance(factory.make_market_provider(), FailoverMarketProvider)
