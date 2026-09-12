from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any
from enum import Enum


class EvidenceType(Enum):
    FACT = "fact"
    INFERENCE = "inference"
    OPINION = "opinion"


class EvidenceTier(Enum):
    STRUCTURED = "structured"   # 行情/财务结构化数据
    OFFICIAL = "official"       # 公告/交易所一手披露
    MEDIA = "media"             # 新闻媒体
    WEB = "web"                 # 网页/论坛/自媒体


@dataclass(frozen=True)
class Source:
    provider: str
    uri: Optional[str] = None
    title: Optional[str] = None
    published_at: Optional[datetime] = None
    retrieved_at: datetime = field(default_factory=datetime.now)


@dataclass(frozen=True)
class Evidence:
    id: str
    type: EvidenceType
    tier: EvidenceTier
    statement: str
    source: Source
    observed_at: datetime
    period: Optional[str] = None
    data: Any = None
    confidence: Optional[float] = None   # 0~1，media/web 时使用
    section: Optional[str] = None       # 所属报告章节：price/volume/financial/events/risk
