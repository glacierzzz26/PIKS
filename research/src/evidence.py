"""Evidence Store — 证据存储与追溯

核心设计：
- 每个指标卡生成时，同时生成对应的 Evidence 记录
- Evidence 包含数据来源、原始数据快照、计算方式
- 报告中的数字必须能追溯到 Evidence ID
"""
from dataclasses import dataclass, field
from datetime import datetime
from typing import Dict, List, Optional, Any
import uuid

from .models.evidence import Evidence, EvidenceType, EvidenceTier, Source


@dataclass
class EvidenceStore:
    """内存中的 Evidence 存储（Phase 1 不下盘，Phase 1.5 进 SQLite）"""
    entries: Dict[str, Evidence] = field(default_factory=dict)

    def add(self, evidence: Evidence) -> str:
        """添加 Evidence，返回 ID"""
        self.entries[evidence.id] = evidence
        return evidence.id

    def get(self, evidence_id: str) -> Optional[Evidence]:
        return self.entries.get(evidence_id)

    def list_by_type(self, etype: EvidenceType) -> List[Evidence]:
        return [e for e in self.entries.values() if e.type == etype]

    def list_by_tier(self, tier: EvidenceTier) -> List[Evidence]:
        return [e for e in self.entries.values() if e.tier == tier]

    def to_json(self) -> List[Dict[str, Any]]:
        """导出为 JSON 序列化格式"""
        result = []
        for e in self.entries.values():
            d = {
                "id": e.id,
                "type": e.type.value,
                "tier": e.tier.value,
                "section": e.section,
                "statement": e.statement,
                "source": {
                    "provider": e.source.provider,
                    "uri": e.source.uri,
                    "title": e.source.title,
                    "published_at": e.source.published_at.isoformat() if e.source.published_at else None,
                    "retrieved_at": e.source.retrieved_at.isoformat(),
                },
                "observed_at": e.observed_at.isoformat(),
                "period": e.period,
                "confidence": e.confidence,
            }
            result.append(d)
        return result


def make_evidence_id(prefix: str = "ev") -> str:
    return f"{prefix}_{uuid.uuid4().hex[:8]}"


def evidence_from_metric(
    metric_name: str,
    value: Any,
    source_provider: str,
    period: str,
    data: Any = None,
    section: Optional[str] = None,
) -> Evidence:
    """从指标值生成 Evidence"""
    return Evidence(
        id=make_evidence_id(metric_name),
        type=EvidenceType.FACT,
        tier=EvidenceTier.STRUCTURED,
        statement=f"{metric_name} = {value}",
        source=Source(
            provider=source_provider,
            retrieved_at=datetime.now(),
        ),
        observed_at=datetime.now(),
        period=period,
        data=data,
        section=section,
    )
