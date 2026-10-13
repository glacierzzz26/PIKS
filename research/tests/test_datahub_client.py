"""test_datahub_client.py — DataHubClient / 读闸门单元测试（P14-1 / Part B2–B3）。

**不联网**：transport 注入桩函数，全部走进程内。锁住：
1. 读闸门 `read_enabled`（token + 白名单双条件）
2. 信封解析（code!=0 抛；data 列表）
3. **只对瞬时错重试**（4xx 立即抛、5xx/连接错重试 ≤3）
4. 进程内 TTL 缓存（同参不重复外呼）
5. `meta.stale=True` 记 WARNING
6. 无 token 拒绝外呼
"""
import json
import logging
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.providers.datahub import config
from src.providers.datahub.client import DataHubClient, DataHubError


def _envelope(rows, stale=False):
    return json.dumps({
        "code": 0, "message": "ok", "data": rows,
        "meta": {"stale": stale, "cached": False, "rows": len(rows)},
    }).encode()


class _Recorder:
    """记录 transport 调用 + 按脚本返回 (status, body)。"""

    def __init__(self, script):
        self.script = list(script)
        self.calls = []

    def __call__(self, url, headers, timeout):
        self.calls.append((url, headers, timeout))
        item = self.script.pop(0)
        if isinstance(item, Exception):
            raise item
        return item


def _client(monkeypatch, transport, **kw):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "tok")
    kw.setdefault("sleep", lambda _s: None)
    kw.setdefault("monotonic", lambda: 1000.0)
    return DataHubClient(base_url="http://dh:8100", transport=transport, **kw)


# ---------- 读闸门 ----------

def test_read_enabled_requires_token_and_whitelist(monkeypatch):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "")
    monkeypatch.setattr(config, "DATAHUB_READ_DATASETS", ["daily_price"])
    assert config.read_enabled("daily_price") is False       # 无 token → 关

    monkeypatch.setattr(config, "DATAHUB_TOKEN", "tok")
    assert config.read_enabled("daily_price") is True
    assert config.read_enabled("macro") is False             # 不在白名单 → 关


# ---------- 成功 / 信封 ----------

def test_get_parses_envelope(monkeypatch):
    rec = _Recorder([(200, _envelope([{"code": "600519", "trade_date": "2026-01-02"}]))])
    rows = _client(monkeypatch, rec).get("daily_price", {"codes": "600519"})
    assert rows == [{"code": "600519", "trade_date": "2026-01-02"}]
    url, headers, _ = rec.calls[0]
    assert url == "http://dh:8100/v1/datasets/daily_price?codes=600519"
    assert headers["Authorization"] == "Bearer tok"


def test_get_business_error_code_raises(monkeypatch):
    body = json.dumps({"code": 12, "message": "boom", "data": None}).encode()
    with pytest.raises(DataHubError):
        _client(monkeypatch, _Recorder([(200, body)])).get("macro")


def test_get_data_null_is_empty(monkeypatch):
    body = json.dumps({"code": 0, "message": "ok", "data": None}).encode()
    assert _client(monkeypatch, _Recorder([(200, body)])).get("macro") == []


# ---------- 重试策略 ----------

def test_get_4xx_no_retry(monkeypatch):
    rec = _Recorder([(404, b"unknown")])
    with pytest.raises(DataHubError):
        _client(monkeypatch, rec).get("nope")
    assert len(rec.calls) == 1          # 4xx 不重试


def test_get_transient_then_success(monkeypatch):
    rec = _Recorder([(503, b"down"), (200, _envelope([{"a": 1}]))])
    assert _client(monkeypatch, rec).get("daily_price") == [{"a": 1}]
    assert len(rec.calls) == 2


def test_get_transient_exhausts(monkeypatch):
    rec = _Recorder([(503, b"down"), (503, b"down"), (503, b"down")])
    with pytest.raises(DataHubError):
        _client(monkeypatch, rec).get("daily_price")
    assert len(rec.calls) == 3          # ≤3 次


def test_get_connection_error_retries(monkeypatch):
    rec = _Recorder([ConnectionError("refused"), ConnectionError("refused"),
                     (200, _envelope([]))])
    assert _client(monkeypatch, rec).get("macro") == []
    assert len(rec.calls) == 3


# ---------- TTL 缓存 ----------

def test_get_caches_within_ttl(monkeypatch):
    clock = {"t": 1000.0}
    rec = _Recorder([(200, _envelope([{"a": 1}]))])
    c = _client(monkeypatch, rec, monotonic=lambda: clock["t"], cache_ttl=60.0)
    c.get("daily_price", {"codes": "600519"})
    clock["t"] = 1030.0                 # ttl 内
    c.get("daily_price", {"codes": "600519"})
    assert len(rec.calls) == 1


def test_get_cache_expires_and_refetches(monkeypatch):
    clock = {"t": 1000.0}
    rec = _Recorder([(200, _envelope([{"a": 1}])), (200, _envelope([{"a": 2}]))])
    c = _client(monkeypatch, rec, monotonic=lambda: clock["t"], cache_ttl=60.0)
    assert c.get("daily_price", {"codes": "600519"}) == [{"a": 1}]
    clock["t"] = 1100.0                 # 超 ttl
    assert c.get("daily_price", {"codes": "600519"}) == [{"a": 2}]
    assert len(rec.calls) == 2


def test_get_cache_key_includes_params(monkeypatch):
    rec = _Recorder([(200, _envelope([{"a": 1}])), (200, _envelope([{"a": 2}]))])
    c = _client(monkeypatch, rec)
    assert c.get("daily_price", {"codes": "600519"}) == [{"a": 1}]
    assert c.get("daily_price", {"codes": "000001"}) == [{"a": 2}]
    assert len(rec.calls) == 2


# ---------- 参数清洗 / 安全 ----------

def test_get_drops_none_and_empty_params(monkeypatch):
    rec = _Recorder([(200, _envelope([]))])
    _client(monkeypatch, rec).get("daily_price", {"codes": "600519", "start": None, "end": ""})
    url, _, _ = rec.calls[0]
    assert url == "http://dh:8100/v1/datasets/daily_price?codes=600519"


def test_get_without_token_refuses(monkeypatch):
    monkeypatch.setattr(config, "DATAHUB_TOKEN", "")
    c = DataHubClient(base_url="http://dh", transport=lambda *a: (200, b"{}"))
    with pytest.raises(DataHubError):
        c.get("daily_price")


def test_stale_meta_logs_warning(monkeypatch, caplog):
    rec = _Recorder([(200, _envelope([{"a": 1}], stale=True))])
    with caplog.at_level(logging.WARNING, logger="datahub.client"):
        _client(monkeypatch, rec).get("daily_price")
    assert any("stale" in r.getMessage() for r in caplog.records)
