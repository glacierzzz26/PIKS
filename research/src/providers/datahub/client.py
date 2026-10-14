"""datahub HTTP 客户端（P14-1 / D4：research→datahub 走 HTTP）。

**传输可换**：HTTP 藏在 `DataHubClient.get()` 后 —— 将来若改走 MCP，换 `_fetch`
不动适配层。契约逐字对齐 datahub `app/http_api.py`：

- `GET {base}/v1/datasets/{id}?<params>` + `Authorization: Bearer {token}`
- 信封 `{code,message,data,meta}`：`data` = 行 dict 列表，`meta` = {stale,cached,rows}
- 错误语义：404 未知数据集 / 400 参数非法 / 401 未鉴权 / 503 源不可用

**重试只对瞬时错**（连接、超时、429、5xx），≤3 次指数退避；400/401/403/404 立即抛
（重试无意义）。**进程内 60s TTL 缓存**，键 `(dataset_id, canonical(params))`；
`meta.stale=True` 记 WARNING —— 不把陈旧数据当新鲜发出。
"""
from __future__ import annotations

import json
import logging
import time
import urllib.error
import urllib.parse
import urllib.request

from . import config

logger = logging.getLogger("datahub.client")

# 可重试的 HTTP 状态：限流 + 网关/上游瞬时故障。
_TRANSIENT_STATUS = frozenset({429, 500, 502, 503, 504})
_MAX_ATTEMPTS = 3
_BACKOFF_BASE = 0.5   # 秒；第 i 次失败后 sleep base * 2**i（0.5 / 1.0）


class DataHubError(RuntimeError):
    """datahub 取数失败（非瞬时错，或瞬时错重试仍失败）。调用方按「取数失败」处置。"""


def _canonical(params: dict) -> str:
    """参数的规范串（作缓存键的一部分）：排序 + ensure_ascii，避免 {a,b} 与 {b,a} 撞键。"""
    return json.dumps(params, sort_keys=True, ensure_ascii=False, default=str)


def _snippet(body: bytes | str, n: int = 200) -> str:
    if isinstance(body, (bytes, bytearray)):
        body = body.decode("utf-8", "replace")
    text = str(body).strip()
    return text if len(text) <= n else text[:n] + "…"


def _default_transport(url: str, headers: dict, timeout: float) -> tuple[int, bytes]:
    """(url, headers, timeout) → (status, body_bytes)。

    HTTPError（4xx/5xx）也返回其 body（不抛）——交由上层按状态判定重试/抛出；
    仅连接层异常（URLError/timeout）向上抛，由 `_fetch` 视为瞬时错。
    """
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


class DataHubClient:
    """datahub 数据集读取客户端。每个 `WorkflowEngine` run 一个实例（进程内缓存随之）。"""

    def __init__(
        self,
        base_url: str | None = None,
        token: str | None = None,
        timeout: float | None = None,
        cache_ttl: float | None = None,
        transport=None,
        sleep=time.sleep,
        monotonic=time.monotonic,
    ) -> None:
        self._base = (base_url if base_url is not None else config.DATAHUB_BASE_URL).rstrip("/")
        self._token = token if token is not None else config.DATAHUB_TOKEN
        self._timeout = config.DATAHUB_TIMEOUT if timeout is None else timeout
        self._ttl = config.DATAHUB_CACHE_TTL if cache_ttl is None else cache_ttl
        self._transport = transport or _default_transport
        self._sleep = sleep
        self._monotonic = monotonic
        self._cache: dict[tuple, tuple[float, list]] = {}

    def get(self, dataset_id: str, params: dict | None = None) -> list[dict]:
        """读一个数据集，返回行 dict 列表（升序/顺序由 datahub 侧决定，此处不改）。

        - `params` 丢弃 None/空串（避免把空参传给 datahub 的枚举校验）
        - 命中 TTL 缓存直接返回（同 run 内同参不重复外呼）
        """
        clean = {k: str(v) for k, v in (params or {}).items() if v is not None and v != ""}
        key = (dataset_id, _canonical(clean))
        hit = self._cache.get(key)
        if hit is not None and (self._monotonic() - hit[0]) < self._ttl:
            return hit[1]
        if not self._token:
            # 读闸门保证有 token；此处兜底：绝不做未鉴权外呼。
            raise DataHubError("未配置 DATAHUB_TOKEN，拒绝未鉴权外呼")
        rows = self._fetch(dataset_id, clean)
        self._cache[key] = (self._monotonic(), rows)
        return rows

    # ---------- 内部 ----------

    def _fetch(self, dataset_id: str, params: dict) -> list[dict]:
        qs = urllib.parse.urlencode(params)
        url = f"{self._base}/v1/datasets/{dataset_id}" + (f"?{qs}" if qs else "")
        headers = {"Authorization": f"Bearer {self._token}", "Accept": "application/json"}

        last = ""
        for attempt in range(_MAX_ATTEMPTS):
            try:
                status, body = self._transport(url, headers, self._timeout)
            except Exception as e:  # noqa: BLE001 —— 任何传输层异常均视为瞬时错，逐次重试
                last = f"{type(e).__name__}: {e}"
                self._backoff(attempt)
                continue
            if status == 200:
                return self._parse(dataset_id, body)
            if status in _TRANSIENT_STATUS:
                last = f"HTTP {status} {_snippet(body)}"
                self._backoff(attempt)
                continue
            # 400/401/403/404：参数/鉴权/资源错，重试无意义
            raise DataHubError(
                f"datahub {dataset_id} 取数失败：HTTP {status} {_snippet(body)}"
            )
        raise DataHubError(
            f"datahub {dataset_id} 取数失败（重试 {_MAX_ATTEMPTS} 次仍失败）：{last}"
        )

    def _backoff(self, attempt: int) -> None:
        if attempt + 1 < _MAX_ATTEMPTS:
            self._sleep(_BACKOFF_BASE * (2 ** attempt))

    @staticmethod
    def _parse(dataset_id: str, body: bytes) -> list[dict]:
        try:
            obj = json.loads(body)
        except (ValueError, TypeError):
            raise DataHubError(f"datahub {dataset_id} 响应非 JSON：{_snippet(body)}") from None
        if not isinstance(obj, dict):
            raise DataHubError(f"datahub {dataset_id} 响应信封非法：{_snippet(body)}")
        if obj.get("code", 0) != 0:
            raise DataHubError(
                f"datahub {dataset_id} 返回错误码 {obj.get('code')}：{obj.get('message')}"
            )
        meta = obj.get("meta") or {}
        if meta.get("stale"):
            logger.warning("datahub %s 返回 stale 数据（上游失败回落缓存）：%s", dataset_id, meta)
        data = obj.get("data")
        if data is None:
            return []
        if not isinstance(data, list):
            raise DataHubError(f"datahub {dataset_id} data 非列表：{_snippet(body)}")
        return data
