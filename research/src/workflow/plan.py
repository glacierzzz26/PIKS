"""Research Plan 生成器

根据 Profile 生成执行计划（Task 列表）。
"""
from dataclasses import dataclass, field
from enum import Enum
from typing import Dict, List, Optional, Set

from .profile import ResearchProfile, SECTION_REQUIREMENTS


class TaskType(Enum):
    """任务类型"""
    RESOLVE = "resolve"
    COLLECT = "collect"
    ANALYZE = "analyze"
    REPORT = "report"


@dataclass
class Task:
    """执行任务"""
    name: str
    task_type: TaskType
    # 对于 COLLECT: provider 名称
    provider: Optional[str] = None
    # 对于 ANALYZE: analysis 模块名称
    analysis: Optional[str] = None
    # 对于 COLLECT: 参数（如 days, quarters）
    params: Dict = field(default_factory=dict)
    # 依赖的其他 task name
    depends_on: List[str] = field(default_factory=list)
    # 是否可选（失败不阻断流程）
    optional: bool = False


@dataclass
class ResearchPlan:
    """研究执行计划"""
    profile_name: str
    mode: str
    tasks: List[Task]
    sections: List[str]


def _get_display_days(profile: ResearchProfile, key: str, default: int) -> int:
    """从 Profile 解析 display 窗口天数"""
    raw = profile.period.display.get(key, f"{default}d")
    return int(raw.rstrip("d"))


def _get_display_quarters(profile: ResearchProfile, key: str, default: int) -> int:
    """从 Profile 解析 display 窗口季度数"""
    raw = profile.period.display.get(key, f"{default}q")
    return int(raw.rstrip("q"))


class PlanGenerator:
    """计划生成器"""

    @classmethod
    def generate(cls, profile: ResearchProfile) -> ResearchPlan:
        tasks: List[Task] = []

        # 1. 股票解析（必须）
        tasks.append(Task(name="resolve", task_type=TaskType.RESOLVE))

        # 2. 数据采集阶段
        needed_providers: Set[str] = set()
        for sec in profile.sections:
            req = SECTION_REQUIREMENTS.get(sec, {})
            needed_providers.update(req.get("providers", []))

        if "market" in needed_providers:
            days = _get_display_days(profile, "price", 60)
            tasks.append(Task(
                name="collect_market",
                task_type=TaskType.COLLECT,
                provider="market",
                params={"days": days},
            ))

        if "financial" in needed_providers:
            quarters = _get_display_quarters(profile, "financial", 4)
            tasks.append(Task(
                name="collect_financial",
                task_type=TaskType.COLLECT,
                provider="financial",
                params={"quarters": quarters},
                optional=True,
            ))

        if "news" in needed_providers:
            days = _get_display_days(profile, "news", 30)
            tasks.append(Task(
                name="collect_news",
                task_type=TaskType.COLLECT,
                provider="news",
                params={"days": days},
                optional=True,
            ))

        if "announcement" in needed_providers:
            days = _get_display_days(profile, "announcement", 30)
            tasks.append(Task(
                name="collect_announcement",
                task_type=TaskType.COLLECT,
                provider="announcement",
                params={"days": days},
                optional=True,
            ))

        if "industry" in needed_providers:
            tasks.append(Task(
                name="collect_industry",
                task_type=TaskType.COLLECT,
                provider="industry",
                params={},
                optional=True,
            ))

        # 3. 分析阶段
        needed_analysis: Set[str] = set()
        for sec in profile.sections:
            req = SECTION_REQUIREMENTS.get(sec, {})
            needed_analysis.update(req.get("analysis", []))

        # price / volume 分析依赖 market 数据
        if "price" in needed_analysis:
            tasks.append(Task(
                name="analyze_price",
                task_type=TaskType.ANALYZE,
                analysis="price",
                depends_on=["collect_market"],
            ))

        if "volume" in needed_analysis:
            tasks.append(Task(
                name="analyze_volume",
                task_type=TaskType.ANALYZE,
                analysis="volume",
                depends_on=["collect_market"],
            ))

        if "patterns" in needed_analysis:
            tasks.append(Task(
                name="analyze_patterns",
                task_type=TaskType.ANALYZE,
                analysis="patterns",
                depends_on=["collect_market"],
            ))

        if "financial" in needed_analysis:
            tasks.append(Task(
                name="analyze_financial",
                task_type=TaskType.ANALYZE,
                analysis="financial",
                depends_on=["collect_financial"],
                optional=True,
            ))

        if "events" in needed_analysis:
            tasks.append(Task(
                name="analyze_events",
                task_type=TaskType.ANALYZE,
                analysis="events",
                depends_on=["collect_news"],
                optional=True,
            ))

        if "risk" in needed_analysis:
            # risk 依赖前面所有分析结果
            deps = []
            if "analyze_price" in [t.name for t in tasks]:
                deps.append("analyze_price")
            if "analyze_volume" in [t.name for t in tasks]:
                deps.append("analyze_volume")
            if "analyze_financial" in [t.name for t in tasks]:
                deps.append("analyze_financial")
            deps.append("collect_announcement")
            tasks.append(Task(
                name="analyze_risk",
                task_type=TaskType.ANALYZE,
                analysis="risk",
                depends_on=deps,
            ))

        if "scorecard" in needed_analysis:
            deps = []
            if "analyze_price" in [t.name for t in tasks]:
                deps.append("analyze_price")
            if "analyze_volume" in [t.name for t in tasks]:
                deps.append("analyze_volume")
            if "analyze_financial" in [t.name for t in tasks]:
                deps.append("analyze_financial")
            if "analyze_events" in [t.name for t in tasks]:
                deps.append("analyze_events")
            if "analyze_risk" in [t.name for t in tasks]:
                deps.append("analyze_risk")
            tasks.append(Task(
                name="analyze_scorecard",
                task_type=TaskType.ANALYZE,
                analysis="scorecard",
                depends_on=deps,
            ))

        # 4. 报告阶段
        tasks.append(Task(
            name="generate_report",
            task_type=TaskType.REPORT,
            depends_on=[t.name for t in tasks if t.task_type in (TaskType.ANALYZE, TaskType.COLLECT)],
        ))

        return ResearchPlan(
            profile_name=profile.name,
            mode=profile.mode,
            tasks=tasks,
            sections=profile.sections,
        )
