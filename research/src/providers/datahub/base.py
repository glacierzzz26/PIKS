"""datahub 兄弟 provider 的公共骨架（P14-1 / B6）。

各域兄弟 provider 与**原 akshare 类同方法签名**，只把取数换成经 `DataHubClient`
读 datahub 数据集、再经 `mappers` 落回同一批领域模型。此处只收敛两件共性：

- **客户端惰性建 + 可注入**：`client` 缺省惰性 `DataHubClient()`；单测注入 mock。
- **`as_of` 防未来函数锚点**（B5）：截断在 datahub SQL，适配层只负责把锚点传下去。
  `as_of` 为 None 时 `_anchor()` 回落 `date.today()`（独立 CLI/无 as_of 场景）。

⚠️ 兄弟 provider **不**接受工厂以外的构造参数；工厂按读闸门逐数据集构造（见
`providers/factory.py`）。
"""
from __future__ import annotations

from datetime import date


class DataHubBackedProvider:
    """datahub 数据集读取 provider 的公共基类（混入，不单独实例化）。"""

    def __init__(self, as_of: date | None = None, client=None) -> None:
        self.as_of = as_of
        self._client = client

    @property
    def client(self):
        """DataHubClient（惰性建，可注入 mock）。"""
        if self._client is None:
            from .client import DataHubClient

            self._client = DataHubClient()
        return self._client

    def _anchor(self) -> date:
        """取数锚点：显式 `as_of` 优先，否则 `date.today()`。"""
        return self.as_of or date.today()

    @staticmethod
    def _iso(d: date | None) -> str | None:
        """date → ISO 串；None 原样返回（由 `DataHubClient.get` 丢弃空参）。"""
        return d.isoformat() if d is not None else None
