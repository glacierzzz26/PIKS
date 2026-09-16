from dataclasses import dataclass
from enum import Enum


class Market(Enum):
    SH = "sh"           # 上海证券交易所（主板）
    SZ = "sz"           # 深圳证券交易所（主板/创业板）
    BJ = "bj"           # 北京证券交易所
    SI = "sw"           # 申万行业指数（研报主体，非股票）
    UNKNOWN = "unknown"

    @property
    def display_name(self) -> str:
        return {
            Market.SH: "上海证券交易所",
            Market.SZ: "深圳证券交易所",
            Market.BJ: "北京证券交易所",
            Market.SI: "申万行业指数",
            Market.UNKNOWN: "未知",
        }[self]


@dataclass(frozen=True)
class Symbol:
    """标准化股票代码"""
    code: str           # 纯数字代码，如 "600519"
    market: Market

    def __post_init__(self):
        object.__setattr__(self, "code", self.code.strip())

    @property
    def full_code(self) -> str:
        """带市场前缀的代码，如 sh600519"""
        return f"{self.market.value}{self.code}"

    @property
    def limit_up_pct(self) -> float:
        """涨停幅度（%）"""
        # 申万行业指数无涨跌停制度（P9）：返回 0 而非按「8 开头」误判为北交所 30%。
        if self.market == Market.SI:
            return 0.0
        if self.market == Market.BJ:
            return 30.0
        if self.code.startswith(("30", "68")):
            return 20.0
        if self.code.startswith("8") or self.code.startswith("4"):
            return 30.0  # 北交所/新三板
        # ST 股判断需要额外数据，这里先返回主板标准
        return 10.0

    @property
    def limit_down_pct(self) -> float:
        """跌停幅度（%）"""
        if self.market == Market.SI:
            return 0.0
        if self.market == Market.BJ:
            return -30.0
        if self.code.startswith(("30", "68")):
            return -20.0
        if self.code.startswith("8") or self.code.startswith("4"):
            return -30.0
        return -10.0

    def __str__(self) -> str:
        return self.full_code


def resolve_symbol(raw: str) -> Symbol:
    """
    解析用户输入的代码。
    支持格式：600519, sh600519（股票）, sw801010（申万行业指数，研报主体）。
    """
    raw = raw.strip().lower()

    # 申万行业指数（P9 主体轴泛化）：sw + 6 位申万代码，如 sw801010 农林牧渔。
    # 必须在股票分支**之前**判断：否则 801010 会落到下方「8 开头 → BJ」，
    # 被当北交所股票并套用 30% 涨跌停（实测错误行为）。
    if raw.startswith("sw") and len(raw) == 8 and raw[2:].isdigit():
        return Symbol(code=raw[2:], market=Market.SI)

    # 去除市场前缀
    if raw.startswith("sh"):
        return Symbol(code=raw[2:], market=Market.SH)
    if raw.startswith("sz"):
        return Symbol(code=raw[2:], market=Market.SZ)
    if raw.startswith("bj"):
        return Symbol(code=raw[2:], market=Market.BJ)

    # 纯数字代码，按规则推断市场
    code = raw
    if code.startswith(("600", "601", "603", "605", "688", "689")):
        market = Market.SH
    elif code.startswith(("000", "001", "002", "003", "300", "301")):
        market = Market.SZ
    elif code.startswith(("8", "43", "83", "87", "88")):
        market = Market.BJ
    else:
        market = Market.UNKNOWN

    return Symbol(code=code, market=market)
