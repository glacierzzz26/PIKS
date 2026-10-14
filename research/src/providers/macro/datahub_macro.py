"""datahub 宏观指标 provider（P14-1 / B6；替 `MacroProvider`）。

**语义表留 Python（R2）**：`MacroProvider._DIMENSIONS`（维度 ↔ 接口/量纲/口径）仍在
`macro_provider.py` 独占 —— 本类**继承** `MacroProvider` 复用 `resolve`/`known_keys`/
期号解析/单调断言/季度差分，只把 `get_series` 换成读 datahub `macro` 表。

datahub 采集器内嵌**最小** `indicator→(level,yoy,mom)` 抽取映射；此处按 `ref.cadence`
决定是否做 GDP 单季差分（与 akshare 路径一致）。
"""
from .macro_provider import MacroProvider, MacroSeries
from ..datahub import mappers as M
from ..datahub.base import DataHubBackedProvider


class DataHubMacroProvider(DataHubBackedProvider, MacroProvider):
    """经 datahub 取宏观序列（与 `MacroProvider` 同方法签名）。"""

    @property
    def name(self) -> str:
        return "datahub_macro"

    def get_series(self, ref) -> MacroSeries:
        rows = self.client.get("macro", {"indicator": ref.key})
        points = M.to_macro_points(rows)
        # ⚠️ 与 akshare 路径同：源站顺序不可信，一律按**期末**升序（见 MacroProvider）。
        points.sort(key=lambda p: (p.period.year, p.period.end))
        self._assert_monotonic(ref, points)
        if ref.cadence == "quarter":
            self._fill_single_quarter(ref, points)
        return MacroSeries(ref=ref, points=points)
