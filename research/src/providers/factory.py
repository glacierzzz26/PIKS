"""Provider 工厂 —— 读闸门决定走 datahub 兄弟 provider 还是原 akshare provider（P14-1 / B6）。

**唯一门控 + 导入点**：`engine._execute_collect`（及 `_collect_*` 三个 helper）只把硬编码
构造换成这里的 `make_*()` —— `Task.provider` 字符串 / `provider_calls` 键 / evidence
标签**一概不动**（Go 契约零改动）。akshare 类保留为回退，且**只**在回退分支导入。

**零行为变更**：`config.read_enabled(dataset)` 为 False（token 空 或 不在白名单）⇒
返回**原 akshare provider 实例** ⇒ 字节级同产物。datahub 兄弟 provider 在其分支内
**惰性导入** —— 闸门关时永不触碰（也永不 import 尚未落地的兄弟模块）。

**`as_of` 透传**：datahub 兄弟 provider 需 `as_of` 做防未来函数（B5：截断在 datahub SQL）。
akshare 回退类不接受该参（其语义靠方法参数/`date.today()`），故 `as_of` 只喂给 datahub 分支。
"""
from .datahub import config


def _load(module: str, attr: str):
    """惰性按相对包导入兄弟 provider **类**（仅在放闸时执行）。"""
    import importlib

    return getattr(importlib.import_module(f".{module}", __package__), attr)


def _resolve(dataset: str, module: str, attr: str, fallback, as_of=None):
    """放闸 ⇒ 构造 datahub 兄弟 provider（实例）；否则 ⇒ 原 akshare provider（实例）。"""
    if config.read_enabled(dataset):
        return _load(module, attr)(as_of=as_of)
    return fallback()


def make_market_provider(as_of=None):
    from .market.failover_provider import FailoverMarketProvider

    return _resolve("daily_price", "market.datahub_market", "DataHubMarketProvider",
                    FailoverMarketProvider, as_of)


def make_financial_provider(as_of=None):
    from .financial.akshare_financial import AkShareFinancialProvider

    return _resolve("financial_indicator", "financial.datahub_financial",
                    "DataHubFinancialProvider", AkShareFinancialProvider, as_of)


def make_news_provider(as_of=None):
    from .news.akshare_news import AkShareNewsProvider

    return _resolve("news_by_stock", "news.datahub_news", "DataHubNewsProvider",
                    AkShareNewsProvider, as_of)


def make_announcement_provider(as_of=None):
    from .announcement.akshare_announcement import AkShareAnnouncementProvider

    return _resolve("announcement_by_stock", "announcement.datahub_announcement",
                    "DataHubAnnouncementProvider", AkShareAnnouncementProvider, as_of)


def make_capital_provider(as_of=None):
    from .capital.akshare_lhb import AkShareLHBProvider

    return _resolve("capital_flow", "capital.datahub_lhb", "DataHubLHBProvider",
                    AkShareLHBProvider, as_of)


def make_industry_provider(as_of=None):
    from .industry.sw_provider import SwIndustryProvider

    return _resolve("industry_sw_info", "industry.datahub_industry",
                    "DataHubIndustryProvider", SwIndustryProvider, as_of)


def make_industry_index_provider(as_of=None):
    from .industry.sw_index_provider import SwIndexProvider

    return _resolve("industry_index", "industry.datahub_index",
                    "DataHubIndexProvider", SwIndexProvider, as_of)


def make_macro_provider(as_of=None):
    from .macro.macro_provider import MacroProvider

    return _resolve("macro", "macro.datahub_macro", "DataHubMacroProvider", MacroProvider,
                    as_of)
