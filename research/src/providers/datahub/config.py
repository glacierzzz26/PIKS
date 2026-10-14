"""datahub 读取配置与读闸门（P14-1 / phase14 §4）。

与 datahub 侧 `collect_config` 同惯例：env → 模块常量（import 时读一次），
测试 monkeypatch 模块属性；生产在进程启动前设 env，故 import 时读正确。

**零行为变更证明（D5 闸门灰度 + 单步回滚）**：`DATAHUB_READ_DATASETS` 默认空 ⇒
`read_enabled()` 恒 False ⇒ 工厂返回**原 akshare provider** ⇒ 与迁移前字节级同产物。
"""
import os

# datahub 服务基址。生产在私有网络内 `http://datahub:8100`（见 compose / D9）。
DATAHUB_BASE_URL = os.getenv("DATAHUB_BASE_URL", "http://datahub:8100").rstrip("/")


def _list_env(name: str) -> list[str]:
    return [s.strip() for s in os.getenv(name, "").split(",") if s.strip()]


def _float_env(name: str, default: float) -> float:
    try:
        return float(os.getenv(name, "") or default)
    except ValueError:
        return float(default)


# Bearer token（与 datahub 的 DATAHUB_TOKEN 一致）。空 = 未配置 ⇒ 读闸门恒 False。
DATAHUB_TOKEN = os.getenv("DATAHUB_TOKEN", "")
# 读闸门白名单：**只有**列此处的数据集才走 datahub，否则回退本地 akshare。
DATAHUB_READ_DATASETS = _list_env("DATAHUB_READ_DATASETS")
# 单请求超时（秒）与进程内短 TTL 缓存（秒）。
DATAHUB_TIMEOUT = _float_env("DATAHUB_TIMEOUT", 10.0)
DATAHUB_CACHE_TTL = _float_env("DATAHUB_CACHE_TTL", 60.0)


def read_enabled(dataset: str) -> bool:
    """该数据集是否放闸走 datahub。

    token 未配置或数据集不在白名单 ⇒ False（回退 akshare）。**防止未鉴权外呼**：
    token 为空时即便数据集在名单里也不走 datahub。
    """
    return bool(DATAHUB_TOKEN) and dataset in DATAHUB_READ_DATASETS
