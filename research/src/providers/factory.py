"""Provider 工厂 —— 读闸门决定走 datahub 兄弟 provider 还是原 akshare provider（P14-1 / B6）。

**唯一门控 + 导入点**：`engine._execute_collect` 只把硬编码构造换成这里的 `make_*()`
—— `Task.provider` 字符串 / `provider_calls` 键 / evidence 标签**一概不动**（Go 契约
零改动）。akshare 类保留为回退。

**零行为变更**：`config.read_enabled(dataset)` 为 False（token 空 或 不在白名单）⇒
返回**原 akshare provider** ⇒ 字节级同产物。datahub 兄弟 provider 在其分支内
**惰性导入** —— 闸门关时永不触碰（也永不 import 尚未落地的兄弟模块）。
"""
from .datahub import config


def _load(module: str, attr: str):
    """惰性按相对包导入兄弟 provider 类（仅在放闸时执行）。"""
    import importlib

    return getattr(importlib.import_module(f".{module}", __package__), attr)


def _resolve(dataset: str, module: str, attr: str, fallback):
    if config.read_enabled(dataset):
        return _load(module, attr)
    return fallback()


def make_market_provider():
    from .market.failover_provider import FailoverMarketProvider

    return _resolve("daily_price", "market.datahub_market", "DataHubMarketProvider",
                    FailoverMarketProvider)


def make_financial_provider():
    from .financial.akshare_financial import AkShareFinancialProvider

    return _resolve("financial_indicator", "financial.datahub_financial",
                    "DataHubFinancialProvider", AkShareFinancialProvider)


def make_news_provider():
    from .news.akshare_news import AkShareNewsProvider

    return _resolve("news_by_stock", "news.datahub_news", "DataHubNewsProvider",
                    AkShareNewsProvider)


def make_announcement_provider():
    from .announcement.akshare_announcement import AkShareAnnouncementProvider

    return _resolve("announcement_by_stock", "announcement.datahub_announcement",
                    "DataHubAnnouncementProvider", AkShareAnnouncementProvider)


def make_capital_provider():
    from .capital.akshare_lhb import AkShareLHBProvider

    return _resolve("capital_flow", "capital.datahub_lhb", "DataHubLHBProvider",
                    AkShareLHBProvider)


def make_industry_provider():
    from .industry.sw_provider import SwIndustryProvider

    return _resolve("industry_sw_info", "industry.datahub_industry",
                    "DataHubIndustryProvider", SwIndustryProvider)


def make_industry_index_provider():
    from .industry.sw_index_provider import SwIndexProvider

    return _resolve("industry_index", "industry.datahub_index",
                    "DataHubIndexProvider", SwIndexProvider)


def make_macro_provider():
    from .macro.macro_provider import MacroProvider

    return _resolve("macro", "macro.datahub_macro", "DataHubMacroProvider", MacroProvider)
