"""datahub 个股行业 provider（P14-1 / B6；替 `SwIndustryProvider`）。

主体 = **个股**，回答「这只票属于哪个行业、同业怎么样」。经 datahub：
1. `industry_sw_cons`（`codes=<股票>`）→ 命中的申万三级行业码；
2. `industry_sw_info`（`codes=<行业码>`）→ 行业名；
3. `industry_sw_cons`（`index_codes=<行业码>`）→ 全部成分股 → 同业对比。

⚠️ 个股理论上可挂在多个三级行业下。原 `SwIndustryProvider` 用启发式排序取**首个**
命中；此处取**行业码升序第一个**（确定性，不引启发式）。正常一票一行业，二者一致。
⚠️ datahub `industry_sw_cons` 未冻结价格列 → `PeerMetric.price` 恒 None（照原样）。
"""
from ...models import Symbol
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider
from .sw_provider import IndustryAnalysis


class DataHubIndustryProvider(DataHubBackedProvider):
    """经 datahub 取个股行业分类 + 同业对比（与 `SwIndustryProvider` 同签名）。"""

    @property
    def name(self) -> str:
        return "datahub_industry"

    def get_industry(self, symbol: Symbol) -> IndustryAnalysis:
        code = symbol.code
        hit = self.client.get("industry_sw_cons", {"codes": code})
        index_codes = sorted(
            {str(r.get("index_code")).strip() for r in hit if r.get("index_code")}
        )
        if not index_codes:
            return IndustryAnalysis(symbol=code, industry_name=None,
                                    industry_code=None, peer_count=0)
        sw_code = index_codes[0]

        info = self.client.get("industry_sw_info", {"codes": sw_code, "levels": 3})
        name = str(info[0].get("name")) if info and info[0].get("name") else sw_code

        members = self.client.get("industry_sw_cons", {"index_codes": sw_code})
        peers = M.to_peer_metrics(members)
        peers.sort(key=lambda p: (p.market_cap is None, -(p.market_cap or 0)))
        return IndustryAnalysis(symbol=code, industry_name=name, industry_code=sw_code,
                                peer_count=len(peers), peers=peers)
