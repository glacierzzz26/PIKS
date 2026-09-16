"""申万行业指数 Provider(P9 / issue #12,行业研报主体)

与 `sw_provider.py` 的分工:
- `sw_provider.SwIndustryProvider`:主体是**个股**,回答「这只票属于哪个行业、同业怎么样」
- 本模块 `SwIndexProvider`          :主体是**行业**(sw+6 位申万码),回答「这个行业本身怎么样」

实测事实(2026-09-16,akshare 1.18.94,见 docs/phase9/design/p9-3-datasource-findings.md):
- `index_hist_sw`    : 指数日线,1999-12-30 起、6456 行(一级/二级/三级均可用,但三级
                       新设码历史短 —— 851251 白酒Ⅲ 仅 2021-12-13 起 1148 行)
- `sw_index_*_info`  : 三级行业表的**当期快照**(行业名/成份数/静态 PE/TTM PE/PB/股息率)。
                       层级归属**必须查表**:801010 是农林牧渔,不是食品饮料。
- `sw_index_third_cons`: **只有三级有**成分股接口,带 ROE/净利增速/营收增速/市值。

**不采集成交量/成交额**:实测白酒Ⅲ 指数行 成交额=45.5、成交量=0.81,而同日成分股合计市值
23629 亿 —— 申万指数该字段的量纲与 A 股个股(手/元)不一致,写进报告即编造量纲。
故 Bar 的 volume/amount/turnover 一律置 0,且 industry profile **不含 volume/turnover 章节**。
"""
import json
import os
import time
from dataclasses import dataclass, field
from datetime import date, datetime
from typing import Dict, List, Optional

import pandas as pd

from ...models import Bar, Symbol

# 申万行业表缓存目录(与 sw_provider.py 同惯例:首次慢,之后秒回)。
# ⚠️ 必须有磁盘缓存:实测申万官网 swsresearch.com 在被频繁请求后返回 **508**,
# 此时 three 张 info 表全部解析失败(akshare 抛 `'NoneType' object has no attribute
# 'find_all'`)。缓存让「行业码→名称/层级」在官网抖动时仍可解析。
_CACHE_DIR = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "cache",
)

# 申万层级 → 表名/中文标签。键为层级序号,顺序即查表顺序(先一级)。
_LEVEL_TABLES = {
    1: ("sw_index_first_info", "申万一级"),
    2: ("sw_index_second_info", "申万二级"),
    3: ("sw_index_third_info", "申万三级"),
}

# 无涨跌停制度:指数不适用 A 股涨跌停(个股侧靠 symbol.limit_up_pct,对 SI 返回 0,
# 若沿用 `pct >= limit_up_pct*0.99` 会把**每个非下跌日**都判成涨停 —— 故显式置 False)。
_NO_LIMIT = False

# 成分接口连续失败阈值:达到即停止再试(见 _cons_of_third)。
# 取 3 —— 偶发抖动(单次超时)不会熔断,官网整体不可用则很快短路。
_CONS_FAIL_THRESHOLD = 3


def _with_retry(fn, retries: int = 3, delay: float = 0.8):
    """akshare 页面解析偶发失败(实测 sw_index_first_info 会抛 soup None),重试兜底。"""
    last = None
    for i in range(retries):
        try:
            return fn()
        except Exception as e:
            last = e
            time.sleep(delay * (i + 1))
    raise last


def _short(code: str) -> str:
    """'801010.SI' → '801010'"""
    return str(code).split(".")[0].strip()


def _opt_float(value) -> Optional[float]:
    """申万快照里的 N/A / 空串 / '-' 一律 None —— 不补 0(0 是「为零」,缺失是「不知道」)。"""
    if value is None:
        return None
    s = str(value).strip()
    if s in ("", "-", "nan", "NaN", "None", "null"):
        return None
    try:
        f = float(s)
    except (ValueError, TypeError):
        return None
    return f


@dataclass(frozen=True)
class IndustryRef:
    """行业主体定位结果(查表得来,不靠代码前缀推断)。"""
    code: str                      # "801010"
    name: str                      # "农林牧渔"
    level: int                     # 1 / 2 / 3
    level_label: str               # "申万一级"
    parent: Optional[str] = None   # 上级行业名(一级为 None)
    member_count: Optional[int] = None
    pe_static: Optional[float] = None
    pe_ttm: Optional[float] = None
    pb: Optional[float] = None
    dividend_yield: Optional[float] = None


@dataclass
class ValuationRank:
    """估值横截面排名(D-R7:行业估值**只有当期快照**,禁止历史分位表述)。

    排名在**同层级**内进行:一级=31 个行业、二级=131、三级=335。
    仅对 TTM PE 有效(>0)的行业排名 —— 亏损行业(PE<0)不进分母,否则位次无意义。
    """
    rank: Optional[int]            # 1-based 升序位次(PE 低者靠前)
    universe: int                  # 参与排名的同层级行业数
    level_label: str
    value: Optional[float]         # 本行业的 TTM PE

    @property
    def text(self) -> str:
        if self.rank is None or self.value is None:
            return "N/A"
        return f"{self.value:.2f}（{self.level_label} {self.universe} 个行业中第 {self.rank}）"


@dataclass
class Constituent:
    """成分股(仅三级行业可直接取;一级/二级由三级成分聚合而来)。"""
    symbol: str
    name: str
    market_cap: Optional[float] = None       # 亿
    pe_ttm: Optional[float] = None
    pb: Optional[float] = None
    roe: Optional[float] = None
    dividend_yield: Optional[float] = None
    net_profit_growth: Optional[float] = None
    revenue_growth: Optional[float] = None


@dataclass
class IndustrySeries:
    """行业行情序列 + 成分。估值快照在 ref 里,排名另给。"""
    ref: IndustryRef
    bars: List[Bar] = field(default_factory=list)
    constituents: List[Constituent] = field(default_factory=list)
    # 聚合成分数 vs 行业表挂牌成份数:不一致时如实暴露,不假装相等。
    declared_count: Optional[int] = None


class SwIndexProvider:
    """申万行业指数 Provider(主体 = 行业,非个股)。"""

    def __init__(self) -> None:
        # 进程内记忆:一次 run 只拉一次全量表,避免 31/131/335 反复抓。
        self._info_cache: Dict[int, pd.DataFrame] = {}
        self._cons_cache: Dict[str, List[Constituent]] = {}
        # 成分接口连续失败计数(见 _cons_of_third 的熔断说明)。
        self._cons_fail_count = 0

    @property
    def name(self) -> str:
        return "sw_index"

    # ---------- 行业定位 ----------

    def _info(self, level: int) -> Optional[pd.DataFrame]:
        """行业表(进程内 → 在线 → 磁盘缓存兜底)。

        ⚠️ **必须在线优先**:这三张表是**当期快照**(行业名/成份数/PE/PB/股息率),
        磁盘缓存永久生效等于把估值冻死在首次抓取的那一天。缓存只在官网失败时兜底
        (实测 swsresearch.com 被频繁请求后返回 508,三张表全部解析失败)。
        故顺序为「进程内 → 在线(重试) → 磁盘」,而非反过来。
        """
        if level in self._info_cache:
            return self._info_cache[level]
        table = _LEVEL_TABLES[level][0]
        cache_path = os.path.join(_CACHE_DIR, f"{table}.json")

        # 1. 在线(重试)。成功即落缓存并返回。
        df = None
        try:
            import akshare as ak
            df = _with_retry(getattr(ak, table))
        except Exception:
            df = None

        if df is not None and not df.empty:
            try:
                os.makedirs(_CACHE_DIR, exist_ok=True)
                with open(cache_path, "w", encoding="utf-8") as f:
                    json.dump(df.to_dict("records"), f, ensure_ascii=False, default=str)
            except Exception:
                pass
            self._info_cache[level] = df
            return df

        # 2. 官网失败 → 磁盘缓存兜底(可能是上次成功抓取的快照,已过期但好过没有)。
        if os.path.exists(cache_path):
            try:
                with open(cache_path, "r", encoding="utf-8") as f:
                    df = pd.DataFrame(json.load(f))
                if not df.empty:
                    self._info_cache[level] = df
                    return df
            except Exception:
                pass

        # 3. 都没有 → None。不臆造行业表。
        return None

    def resolve(self, code: str) -> Optional[IndustryRef]:
        """6 位申万码 → 行业定位。逐级查表(二级/三级表**不含**一级代码)。"""
        code = _short(code)
        for level in (1, 2, 3):
            df = self._info(level)
            if df is None or df.empty:
                continue
            hit = df[df["行业代码"].map(_short) == code]
            if hit.empty:
                continue
            row = hit.iloc[0]
            return IndustryRef(
                code=code,
                name=str(row["行业名称"]),
                level=level,
                level_label=_LEVEL_TABLES[level][1],
                parent=(str(row["上级行业"]) if "上级行业" in df.columns else None),
                member_count=(_opt_int(row.get("成份个数"))),
                pe_static=_opt_float(row.get("静态市盈率")),
                pe_ttm=_opt_float(row.get("TTM(滚动)市盈率")),
                pb=_opt_float(row.get("市净率")),
                dividend_yield=_opt_float(row.get("静态股息率")),
            )
        return None

    def valuation_rank(self, ref: IndustryRef) -> ValuationRank:
        """同层级 TTM PE 升序排名(横截面快照,D-R7)。"""
        df = self._info(ref.level)
        label = ref.level_label
        if df is None or df.empty:
            return ValuationRank(rank=None, universe=0, level_label=label, value=ref.pe_ttm)
        vals = [_opt_float(v) for v in df["TTM(滚动)市盈率"]]
        # 只对有效且为正的 PE 排名:亏损行业(PE<0)不进分母,否则位次无意义。
        valid = sorted(v for v in vals if v is not None and v > 0)
        if ref.pe_ttm is None or ref.pe_ttm <= 0 or not valid:
            return ValuationRank(rank=None, universe=len(valid), level_label=label, value=ref.pe_ttm)
        # 位次用「≤ 本值的个数」,同值并列不虚增位次。
        rank = sum(1 for v in valid if v < ref.pe_ttm) + 1
        return ValuationRank(rank=rank, universe=len(valid), level_label=label, value=ref.pe_ttm)

    # ---------- 行情序列 ----------

    def get_history(self, symbol: Symbol, start_date: date, end_date: date) -> List[Bar]:
        """申万指数日线 → Bar 列表。

        单位说明:指数无成交额/换手率口径(见模块 docstring),volume/amount/turnover 置 0,
        **不猜测量纲**;涨跌停标记显式 False(指数无涨跌停制度)。
        """
        import akshare as ak

        df = _with_retry(lambda: ak.index_hist_sw(symbol=symbol.code, period="day"))
        if df is None or df.empty:
            return []

        df = df.copy()
        df["日期"] = pd.to_datetime(df["日期"])
        mask = (df["日期"].dt.date >= start_date) & (df["日期"].dt.date <= end_date)
        df = df.loc[mask].sort_values("日期").reset_index(drop=True)
        if df.empty:
            return []

        bars: List[Bar] = []
        prev_close: Optional[float] = None
        for _, row in df.iterrows():
            close = float(row["收盘"])
            high, low, open_ = float(row["最高"]), float(row["最低"]), float(row["开盘"])
            if prev_close and prev_close > 0:
                pct = (close - prev_close) / prev_close * 100
                amplitude = (high - low) / prev_close * 100
                chg = close - prev_close
            else:
                pct = amplitude = chg = 0.0
            bars.append(Bar(
                symbol=symbol.full_code,
                date=pd.to_datetime(row["日期"]).date(),
                open=open_, high=high, low=low, close=close,
                volume=0,        # 量纲不可靠,不采集(见模块 docstring)
                amount=0.0,
                amplitude=amplitude,
                pct_change=pct,
                chg_amount=chg,
                turnover=0.0,
                is_limit_up=_NO_LIMIT,
                is_limit_down=_NO_LIMIT,
                is_halted=False,
            ))
            prev_close = close
        return bars

    # ---------- 成分 ----------

    def third_level_children(self, parent_name: str) -> List[str]:
        """某二级行业名 → 其下三级行业代码列表。"""
        df = self._info(3)
        if df is None or df.empty:
            return []
        return [_short(c) for c in df[df["上级行业"] == parent_name]["行业代码"]]

    def second_level_of(self, level1_name: str) -> List[str]:
        """某一级行业名 → 其下二级行业名列表。"""
        df = self._info(2)
        if df is None or df.empty:
            return []
        return [str(n) for n in df[df["上级行业"] == level1_name]["行业名称"]]

    def _cons_of_third(self, third_code: str) -> List[Constituent]:
        """三级行业成分股(唯一有 cons 接口的层级)。

        ⚠️ 一级/二级成分是把**上百个**三级行业的 cons 逐个聚合出来的
        (基础化工 412 个成分 ≈ 几十个三级)。官网 508 期间每次都卡 ~5s,
        若逐个硬试就是分钟级空转 —— 故加**熔断**:连续失败达阈值即停止尝试,
        余下三级直接记空。熔断只在单次 run 内有效(Provider 随 run 销毁),
        不会把一次官网抖动固化成长期事实。
        """
        if third_code in self._cons_cache:
            return self._cons_cache[third_code]
        if self._cons_fail_count >= _CONS_FAIL_THRESHOLD:
            self._cons_cache[third_code] = []
            return []
        import akshare as ak
        try:
            df = _with_retry(lambda: ak.sw_index_third_cons(symbol=f"{third_code}.SI"), retries=2)
        except Exception:
            self._cons_fail_count += 1
            self._cons_cache[third_code] = []   # 失败也记空,避免反复重试
            return []
        self._cons_fail_count = 0               # 成功即复位:偶发抖动不应触发熔断
        out: List[Constituent] = []
        if df is not None and not df.empty:
            for _, r in df.iterrows():
                out.append(Constituent(
                    symbol=_short(r.get("股票代码")),
                    name=str(r.get("股票简称", "")),
                    market_cap=_opt_float(r.get("市值")),
                    pe_ttm=_opt_float(r.get("市盈率ttm")),
                    pb=_opt_float(r.get("市净率")),
                    roe=_opt_float(r.get("ROE(%)")),
                    dividend_yield=_opt_float(r.get("股息率")),
                    net_profit_growth=_opt_float(r.get("净利润增速(%)")),
                    revenue_growth=_opt_float(r.get("营收增速(%)")),
                ))
        self._cons_cache[third_code] = out
        return out

    def get_constituents(self, ref: IndustryRef) -> List[Constituent]:
        """成分股。三级直接取;二级/一级由三级成分**聚合**(申万只有三级 cons 接口)。

        聚合正确性已实测:一级 基础化工 L3 成份数合计 412 == 挂牌 412
        (电力设备 379==379、机械设备 542==542)。按股票代码去重。
        """
        if ref.level == 3:
            thirds = [ref.code]
        elif ref.level == 2:
            thirds = self.third_level_children(ref.name)
        else:
            thirds = []
            for l2 in self.second_level_of(ref.name):
                thirds.extend(self.third_level_children(l2))

        seen: set = set()
        out: List[Constituent] = []
        for t in thirds:
            for c in self._cons_of_third(t):
                if c.symbol and c.symbol not in seen:
                    seen.add(c.symbol)
                    out.append(c)
        out.sort(key=lambda c: (c.market_cap is None, -(c.market_cap or 0)))
        return out


def _opt_int(value) -> Optional[int]:
    f = _opt_float(value)
    return int(f) if f is not None else None
