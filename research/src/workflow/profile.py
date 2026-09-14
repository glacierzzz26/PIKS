"""Research Profile 加载与验证"""
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List, Optional

import yaml


@dataclass
class PeriodConfig:
    """时间窗口配置"""
    display: Dict[str, str] = field(default_factory=dict)
    compute: Dict[str, str] = field(default_factory=dict)
    unit: Dict[str, str] = field(default_factory=dict)


@dataclass
class ScorecardConfig:
    """评分卡配置"""
    dimensions: List[str] = field(default_factory=list)
    overall_rule: str = "simple_sum"
    scale: List[int] = field(default_factory=lambda: [-2, -1, 0, 1, 2])


@dataclass
class ThresholdConfig:
    """异常阈值配置"""
    abnormal_volume_multiplier: float = 2.0
    abnormal_volume_lookback: int = 20
    turnover_percentile_window: int = 520


@dataclass
class ResearchProfile:
    """研究 Profile"""
    name: str
    version: str
    description: str
    mode: str  # full / express
    period: PeriodConfig
    sections: List[str]
    scorecard: ScorecardConfig
    thresholds: ThresholdConfig
    source_path: Optional[str] = None


SECTION_REQUIREMENTS = {
    "company": {"providers": [], "analysis": []},
    "market": {"providers": ["market"], "analysis": ["price"]},
    "volume": {"providers": ["market"], "analysis": ["volume"]},
    "turnover": {"providers": ["market"], "analysis": ["volume"]},
    "financial": {"providers": ["financial"], "analysis": ["financial"]},
    "valuation": {"providers": ["financial"], "analysis": ["financial"]},
    "events": {"providers": ["news"], "analysis": ["events"]},
    "announcements": {"providers": ["announcement"], "analysis": []},
    "industry": {"providers": ["industry"], "analysis": []},
    "risk": {"providers": [], "analysis": ["risk"]},
    "patterns": {"providers": ["market"], "analysis": ["patterns"]},
    "conclusion": {"providers": [], "analysis": ["scorecard"]},
}


class ProfileValidator:
    """Profile 校验器"""

    VALID_SECTIONS = set(SECTION_REQUIREMENTS.keys())
    VALID_MODES = {"full", "express"}
    VALID_UNITS = {"trading", "calendar"}

    @classmethod
    def validate(cls, profile: ResearchProfile) -> List[str]:
        errors = []

        if profile.mode not in cls.VALID_MODES:
            errors.append(f"mode 必须是 {cls.VALID_MODES} 之一，当前: {profile.mode}")

        invalid_sections = set(profile.sections) - cls.VALID_SECTIONS
        if invalid_sections:
            errors.append(f"非法 section: {invalid_sections}")

        for key, unit in profile.period.unit.items():
            if unit not in cls.VALID_UNITS:
                errors.append(f"period.unit[{key}] 非法: {unit}")

        # 校验每个 section 所需数据是否可被满足（Phase 1：只做存在性检查）
        for sec in profile.sections:
            req = SECTION_REQUIREMENTS.get(sec, {})
            # industry 需要 industry provider 做同业对比
            if sec == "industry":
                if "industry" not in req.get("providers", []):
                    errors.append(f"section '{sec}' 声明了不存在的 provider 依赖")

        return errors


def _parse_period(raw: dict) -> PeriodConfig:
    return PeriodConfig(
        display=raw.get("display", {}),
        compute=raw.get("compute", {}),
        unit=raw.get("unit", {}),
    )


def _parse_scorecard(raw: dict) -> ScorecardConfig:
    return ScorecardConfig(
        dimensions=raw.get("dimensions", []),
        overall_rule=raw.get("overall_rule", "simple_sum"),
        scale=raw.get("scale", [-2, -1, 0, 1, 2]),
    )


def _parse_thresholds(raw: dict) -> ThresholdConfig:
    return ThresholdConfig(
        abnormal_volume_multiplier=raw.get("abnormal_volume_multiplier", 2.0),
        abnormal_volume_lookback=raw.get("abnormal_volume_lookback", 20),
        turnover_percentile_window=raw.get("turnover_percentile_window", 520),
    )


def load_profile(name_or_path: str) -> ResearchProfile:
    """加载 Profile。

    如果传入的是文件路径（含 / 或 .yaml），直接加载；
    否则从 profiles/ 目录按名称加载内置 Profile。
    """
    if "/" in name_or_path or name_or_path.endswith(".yaml"):
        path = Path(name_or_path)
    else:
        # src/workflow/profile.py → workflow → src → research(仓库内 research/ 根)
        base = Path(__file__).parent.parent.parent
        path = base / "profiles" / f"{name_or_path}.yaml"

    if not path.exists():
        raise FileNotFoundError(f"Profile 不存在: {path}")

    with open(path, "r", encoding="utf-8") as f:
        raw = yaml.safe_load(f)

    profile = ResearchProfile(
        name=raw.get("name", "unknown"),
        version=raw.get("version", "0.0.0"),
        description=raw.get("description", ""),
        mode=raw.get("mode", "full"),
        period=_parse_period(raw.get("period", {})),
        sections=raw.get("sections", []),
        scorecard=_parse_scorecard(raw.get("scorecard", {})),
        thresholds=_parse_thresholds(raw.get("thresholds", {})),
        source_path=str(path),
    )

    errors = ProfileValidator.validate(profile)
    if errors:
        raise ValueError(f"Profile 校验失败 ({path}): " + "; ".join(errors))

    return profile
