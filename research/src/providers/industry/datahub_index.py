"""datahub 申万行业指数 provider（P14-1 / B6；替 `SwIndexProvider`）。

主体 = **行业**（`sw` + 6 位申万码）。读三张 datahub 表：
`industry_index`（日线）、`industry_sw_info`（一/二/三级分类 + 当期估值快照）、
`industry_sw_cons`（仅三级有成分股）。

与原 `SwIndexProvider` 的行为对齐（见其模块 docstring）：
- 指数**不采成交量/成交额**（申万量纲与 A 股不兼容）→ `mappers.to_index_bars` 恒 0；
- 一级/二级成分由三级成分**聚合**（申万只有三级 cons）；
- `valuation_rank` 只对同层级内 TTM PE > 0 的行业排名（亏损行业不进分母）。
"""
from datetime import date
from typing import List, Optional

from ...models import Bar, Symbol
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider
from .sw_index_provider import Constituent, IndustryRef, ValuationRank


class DataHubIndexProvider(DataHubBackedProvider):
    """经 datahub 取申万行业指数（与 `SwIndexProvider` 同方法签名）。"""

    @property
    def name(self) -> str:
        return "datahub_index"

    # ---------- 行业定位 ----------

    def resolve(self, code: str) -> Optional[IndustryRef]:
        """6 位申万码 → 行业定位（查 `industry_sw_info`，不按前缀推断）。"""
        short = str(code).split(".")[0].strip()
        rows = self.client.get("industry_sw_info", {"codes": short})
        if not rows:
            return None
        return M.to_industry_ref(rows[0])

    def valuation_rank(self, ref: IndustryRef) -> ValuationRank:
        """同层级 TTM PE 升序排名（横截面快照，D-R7）。"""
        rows = self.client.get("industry_sw_info", {"levels": ref.level})
        vals = [M._f(r.get("pe_ttm")) for r in rows]
        valid = sorted(v for v in vals if v is not None and v > 0)
        if ref.pe_ttm is None or ref.pe_ttm <= 0 or not valid:
            return ValuationRank(rank=None, universe=len(valid),
                                 level_label=ref.level_label, value=ref.pe_ttm)
        rank = sum(1 for v in valid if v < ref.pe_ttm) + 1
        return ValuationRank(rank=rank, universe=len(valid),
                             level_label=ref.level_label, value=ref.pe_ttm)

    # ---------- 行情序列 ----------

    def get_history(self, symbol: Symbol, start_date: date, end_date: date) -> List[Bar]:
        rows = self.client.get("industry_index", {
            "codes": symbol.code,
            "start": self._iso(start_date),
            "end": self._iso(end_date),
        })
        return M.to_index_bars(symbol, rows)

    # ---------- 成分 ----------

    def third_level_children(self, parent_name: str) -> List[str]:
        """某二级行业名 → 其下三级行业代码列表。"""
        rows = self.client.get("industry_sw_info", {"levels": 3, "parent": parent_name})
        return [str(r.get("code")).split(".")[0].strip() for r in rows if r.get("code")]

    def second_level_of(self, level1_name: str) -> List[str]:
        """某一级行业名 → 其下二级行业名列表。"""
        rows = self.client.get("industry_sw_info", {"levels": 2, "parent": level1_name})
        return [str(r.get("name")) for r in rows if r.get("name")]

    def get_constituents(self, ref: IndustryRef) -> List[Constituent]:
        """成分股。三级直接取；二级/一级由三级成分**聚合**（按代码去重）。"""
        if ref.level == 3:
            thirds = [ref.code]
        elif ref.level == 2:
            thirds = self.third_level_children(ref.name)
        else:
            thirds = []
            for l2 in self.second_level_of(ref.name):
                thirds.extend(self.third_level_children(l2))
        thirds = [t for t in dict.fromkeys(thirds) if t]
        if not thirds:
            return []
        rows = self.client.get("industry_sw_cons", {"index_codes": ",".join(thirds)})
        seen: set = set()
        out: List[Constituent] = []
        for c in M.to_constituents(rows):
            if c.symbol and c.symbol not in seen:
                seen.add(c.symbol)
                out.append(c)
        out.sort(key=lambda c: (c.market_cap is None, -(c.market_cap or 0)))
        return out
