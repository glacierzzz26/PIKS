"""行业分析 Provider（Phase 1 降级：分类 + 同业财务对比）

数据源：申万行业指数（akshare sw_index_* 接口，未受限）
流程：
1. 行业分类：sw_index_third_info 拉全量三级行业 → 逐行业查成分股（按名称启发式排序优先）
   找到含目标股票的三级行业 → 该行业名称即分类
2. 同业列表：该行业全部成分股
3. 核心财务对比：成分表自带 ROE / 市盈率 / 市净率 / 股息率 / 净利润增速 / 营收增速 / 市值

缓存：行业→成分股列表落盘（首次慢，之后秒回）
设计原则：不写死行业代码；无法分类时返回 None（不脑补）。
"""
import json
import os
import time
from dataclasses import dataclass, field
from typing import Dict, List, Optional

import akshare as ak

from ...models import Symbol

CACHE_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "cache",
    "sw_industry_cons.json",
)
INDUSTRY_LIST_CACHE = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "cache",
    "sw_industry_third_info.json",
)

# 常见行业三级代码白名单（行业列表接口不可用时兜底）
FALLBACK_INDUSTRIES: List[Dict[str, str]] = [
    {"行业代码": "851251.SI", "行业名称": "白酒Ⅲ"},
]


def _with_retry(fn, retries: int = 3, delay: float = 1.0):
    """简单重试，容忍 akshare 页面解析偶发失败"""
    last = None
    for i in range(retries):
        try:
            return fn()
        except Exception as e:
            last = e
            time.sleep(delay * (i + 1))
    raise last


@dataclass
class PeerMetric:
    """同业公司单项财务对比"""
    symbol: str          # 600519
    name: str            # 贵州茅台
    price: Optional[float]
    pe_ttm: Optional[float]
    pb: Optional[float]
    roe: Optional[float]
    dividend_yield: Optional[float]
    market_cap: Optional[float]   # 亿
    net_profit_growth: Optional[float]
    revenue_growth: Optional[float]

    def to_dict(self) -> dict:
        return {
            "symbol": self.symbol,
            "name": self.name,
            "price": self.price,
            "pe_ttm": self.pe_ttm,
            "pb": self.pb,
            "roe": self.roe,
            "dividend_yield": self.dividend_yield,
            "market_cap": self.market_cap,
            "net_profit_growth": self.net_profit_growth,
            "revenue_growth": self.revenue_growth,
        }


@dataclass
class IndustryAnalysis:
    """行业分类 + 同业对比结果"""
    symbol: str
    industry_name: Optional[str]      # 申万三级行业名（如 白酒Ⅲ）
    industry_code: Optional[str]
    peer_count: int
    peers: List[PeerMetric] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "symbol": self.symbol,
            "industry_name": self.industry_name,
            "industry_code": self.industry_code,
            "peer_count": self.peer_count,
            "peers": [p.to_dict() for p in self.peers],
        }


class SwIndustryProvider:
    """申万行业 Provider"""

    @property
    def name(self) -> str:
        return "sw_industry"

    def get_industry(self, symbol: Symbol) -> IndustryAnalysis:
        """获取目标股票的行业分类与同业对比

        symbol.code: 6 位代码（如 600519）
        """
        code = symbol.code
        cons_cache = self._load_cache()

        # 0. 缓存优先（离线可用）：找含目标股的已缓存行业
        for sw_code, members in cons_cache.items():
            if members and code in members:
                name = self._industry_name(sw_code)
                return self._build_analysis(symbol, sw_code, name, members)

        # 1. 全量三级行业列表（重试 → 缓存 → 白名单兜底）
        df3 = self._load_industry_list()
        if df3 is None:
            return IndustryAnalysis(symbol=code, industry_name=None, industry_code=None, peer_count=0)

        # 名称启发式排序：目标股可能是酒/饮料/银行等，把常见行业名排前（命中更快）
        rows = df3[["行业代码", "行业名称"]].to_dict("records")
        rows.sort(key=lambda r: _industry_rank(r["行业名称"]))

        for row in rows:
            sw_code = row["行业代码"]
            name = row["行业名称"]
            members = cons_cache.get(sw_code)
            if members is None:
                members = self._fetch_members(sw_code)
                # 失败（空）不缓存，避免永久 miss
                if members:
                    cons_cache[sw_code] = members
                    self._save_cache(cons_cache)

            # 命中：目标股在该行业成分中
            if members and code in members:
                return self._build_analysis(symbol, sw_code, name, members)

        return IndustryAnalysis(symbol=code, industry_name=None, industry_code=None, peer_count=0)

    # ---------- 内部 ----------

    def _industry_name(self, sw_code: str) -> str:
        """行业代码 → 名称：白名单映射优先，列表缓存其次"""
        for row in FALLBACK_INDUSTRIES:
            if row["行业代码"] == sw_code:
                return row["行业名称"]
        # 尝试行业列表缓存
        if os.path.exists(INDUSTRY_LIST_CACHE):
            try:
                with open(INDUSTRY_LIST_CACHE, "r", encoding="utf-8") as f:
                    rows = json.load(f)
                for r in rows:
                    if r.get("行业代码") == sw_code:
                        return r.get("行业名称", sw_code)
            except Exception:
                pass
        return sw_code

    def _build_analysis(self, symbol: Symbol, sw_code: str, name: str, members: list) -> IndustryAnalysis:
        """拉成分股财务详情并构建分析（成分表缓存按 code 存 json，含财务字段）"""
        detail_cache_path = os.path.join(
            os.path.dirname(CACHE_PATH), f"sw_cons_{sw_code.split('.')[0]}.json"
        )
        df = None
        # 1. 读成分详情缓存（离线可用 + 快）
        if os.path.exists(detail_cache_path):
            try:
                import pandas as pd
                with open(detail_cache_path, "r", encoding="utf-8") as f:
                    df = pd.DataFrame(json.load(f))
            except Exception:
                df = None
        # 2. 在线拉取并缓存（单次尝试，失败走下方保底名单）
        if df is None:
            try:
                df = _with_retry(lambda: ak.sw_index_third_cons(symbol=sw_code), retries=1)
                os.makedirs(os.path.dirname(detail_cache_path), exist_ok=True)
                with open(detail_cache_path, "w", encoding="utf-8") as f:
                    json.dump(df.to_dict("records"), f, ensure_ascii=False, default=str)
            except Exception:
                df = None

        peers = []
        if df is not None and not df.empty:
            code2row = {}
            for _, r in df.iterrows():
                raw = str(r["股票代码"])
                short = raw.split(".")[0]
                code2row.setdefault(short, r)

            drop_raw = {"NaN", "nan", "", "-"}
            for short in members:
                row = code2row.get(short)
                if row is None:
                    continue
                peers.append(PeerMetric(
                    symbol=short,
                    name=str(row.get("股票简称", "")),
                    price=_to_float(row.get("价格"), drop_raw),
                    pe_ttm=_to_float(row.get("市盈率ttm"), drop_raw),
                    pb=_to_float(row.get("市净率"), drop_raw),
                    roe=_to_float(row.get("ROE(%)"), drop_raw),
                    dividend_yield=_to_float(row.get("股息率"), drop_raw),
                    market_cap=_to_float(row.get("市值"), drop_raw),
                    net_profit_growth=_to_float(row.get("净利润增速(%)"), drop_raw),
                    revenue_growth=_to_float(row.get("营收增速(%)"), drop_raw),
                ))
        else:
            # 详情不可得：用成员代码表保底（行业分类 + 同业名单仍有效，财务 N/A）
            for short in members:
                peers.append(PeerMetric(
                    symbol=short, name=short,
                    price=None, pe_ttm=None, pb=None, roe=None,
                    dividend_yield=None, market_cap=None,
                    net_profit_growth=None, revenue_growth=None,
                ))

        peers.sort(key=lambda p: (p.market_cap is None, -(p.market_cap or 0)))
        return IndustryAnalysis(
            symbol=symbol.code, industry_name=name, industry_code=sw_code,
            peer_count=len(peers), peers=peers,
        )

    def _fetch_members(self, sw_code: str) -> list:
        """拉取行业成分股（6 位代码列表）"""
        try:
            df = _with_retry(lambda: ak.sw_index_third_cons(symbol=sw_code))
            return [str(c).split(".")[0] for c in df["股票代码"].tolist()]
        except Exception:
            return []

    # ---------- 行业列表（重试 → 缓存 → 兜底） ----------

    def _load_industry_list(self):
        """三级行业列表：缓存文件 → 在线重试 → 白名单兜底"""
        import pandas as pd
        # 1. 缓存优先（离线可用）
        if os.path.exists(INDUSTRY_LIST_CACHE):
            try:
                with open(INDUSTRY_LIST_CACHE, "r", encoding="utf-8") as f:
                    return pd.DataFrame(json.load(f))
            except Exception:
                pass
        # 2. 白名单兜底优先于在线（避免接口不可用时重试延迟；白名单覆盖目标则跳过在线）
        #    在线仅当白名单查不到目标时才尝试（见 get_industry 的二次查询逻辑）
        if FALLBACK_INDUSTRIES:
            return pd.DataFrame(FALLBACK_INDUSTRIES)
        return None

    # ---------- 缓存 ----------

    _cache: Optional[Dict[str, list]] = None

    def _load_cache(self) -> Dict[str, list]:
        if self._cache is not None:
            return self._cache
        if os.path.exists(CACHE_PATH):
            try:
                with open(CACHE_PATH, "r", encoding="utf-8") as f:
                    self._cache = json.load(f)
                    return self._cache
            except Exception:
                pass
        self._cache = {}
        return self._cache

    def _save_cache(self, cache: Dict[str, list]) -> None:
        os.makedirs(os.path.dirname(CACHE_PATH), exist_ok=True)
        with open(CACHE_PATH, "w", encoding="utf-8") as f:
            json.dump(cache, f, ensure_ascii=False)


def _industry_rank(name: str) -> int:
    """行业名启发式排序：常见板块优先（减少遍历次数）"""
    for i, kw in enumerate(["酒", "饮料", "食品", "银行", "证券", "保险",
                            "汽车", "医药", "半导体", "电子", "房地产", "电力"]):
        if kw in name:
            return i
    return 99


def _to_float(value, drop_raw) -> Optional[float]:
    s = str(value).strip()
    if s in drop_raw:
        return None
    try:
        return float(s)
    except (ValueError, TypeError):
        return None