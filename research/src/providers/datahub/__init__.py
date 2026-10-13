"""datahub 读取适配层（P14-1 / phase14 §4）。

research→datahub 走 **HTTP**（真实契约，D4）；MCP 面留给 Go 侧。

- `config`  ：env + 读闸门 `read_enabled(dataset)`
- `client`  ：`DataHubClient`（HTTP 传输，藏在 `get()` 后，将来切 MCP 不动适配层）
- `mappers` ：datahub 行 → 领域模型 + **单位约定**（最高频 bug 源）

本包只依赖 config / client；`mappers` 需领域模型故单独 import（避免只读闸门时
连带拉起 akshare）。
"""
from . import config
from .client import DataHubClient, DataHubError

__all__ = ["DataHubClient", "DataHubError", "config"]
