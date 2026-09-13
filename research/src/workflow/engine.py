"""Workflow 执行引擎"""
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta
from typing import Any, Dict, List, Optional

from ..models import Symbol, resolve_symbol
from ..providers.market import FailoverMarketProvider
from ..providers.financial import AkShareFinancialProvider
from ..providers.news import AkShareNewsProvider
from ..providers.announcement import AkShareAnnouncementProvider
from ..providers.capital import AkShareLHBProvider
from ..analysis.engine import run_analysis as run_analysis_engine
from ..analysis.capital import analyze_capital
from ..analysis.price import analyze_price
from ..analysis.volume import analyze_volume
from ..analysis.financial import analyze_financial
from ..analysis.events import analyze_events
from ..analysis.risk import analyze_risk
from ..analysis.scorecard import analyze_scorecard
from ..analysis.number_lint import lint_markdown_report, NumberLintReport
from ..report.markdown import generate_markdown
from ..report.json_report import generate_json
from ..evidence import EvidenceStore
from .profile import ResearchProfile, ThresholdConfig
from .plan import ResearchPlan, Task, TaskType


@dataclass
class WorkflowResult:
    """工作流执行结果"""
    success: bool
    outputs: Dict[str, Any] = field(default_factory=dict)
    errors: Dict[str, str] = field(default_factory=dict)
    evidence_store: Optional[EvidenceStore] = None
    markdown_report: Optional[str] = None
    json_report: Optional[dict] = None
    lint_report: Optional[NumberLintReport] = None
    # 运行元信息(供上层写 run_meta.json / 归档 PG;不再落 SQLite)
    run_id: Optional[str] = None
    sections: List[str] = field(default_factory=list)
    provider_calls: Dict[str, int] = field(default_factory=dict)


class WorkflowEngine:
    """研究工作流引擎

    按 Plan 拓扑排序执行，管理数据流和错误。
    """

    def __init__(
        self,
        symbol_code: str,
        profile: ResearchProfile,
        plan: ResearchPlan,
        as_of: Optional[date] = None,
        skip_capital: bool = False,
        run_id: Optional[str] = None,
    ):
        self.symbol = resolve_symbol(symbol_code)
        self.profile = profile
        self.plan = plan
        self.as_of = as_of or date.today()
        self.skip_capital = skip_capital
        # 上层(Go cmd/research-run)可传入稳定 run_id 作为幂等键;
        # 不传则按"代码_档案_日期_时刻"生成(独立 CLI 用)。
        self.run_id_override = run_id
        self.context: Dict[str, Any] = {}
        self.errors: Dict[str, str] = {}

    def run(self) -> WorkflowResult:
        """执行工作流"""
        run_id = self.run_id_override or (
            f"{self.symbol.full_code}_{self.profile.name}_{self.as_of.isoformat()}"
            f"_{datetime.now().strftime('%H%M%S')}"
        )
        provider_calls: Dict[str, int] = {}

        # 归档层已改为 PG(见 cmd/research-run);此处只产出 run_id 供上层落盘。
        # 2. 拓扑排序执行
        executed = set()
        for task in self.plan.tasks:
            if not self._can_run(task, executed):
                continue
            self._execute(task)
            executed.add(task.name)
            if task.task_type == TaskType.COLLECT and task.provider:
                provider_calls[task.provider] = provider_calls.get(task.provider, 0) + 1

        # 3. 组装报告
        md_report = None
        json_report = None
        if "generate_report" in executed or self._can_run(
            next((t for t in self.plan.tasks if t.name == "generate_report"), None), executed
        ):
            md_report, json_report = self._generate_reports()

        # 3.5 Number Lint（不阻断）：报告生成后扫描数字对账
        lint_report = None
        if md_report and json_report:
            lint_report = lint_markdown_report(md_report, json_report)

        evidence_store = self.context.get("evidence_store")

        return WorkflowResult(
            success=len(self.errors) == 0 or all(
                t.optional for t in self.plan.tasks if t.name in self.errors
            ),
            outputs=dict(self.context),
            errors=dict(self.errors),
            evidence_store=evidence_store,
            markdown_report=md_report,
            json_report=json_report,
            lint_report=lint_report,
            run_id=run_id,
            sections=list(self.plan.sections),
            provider_calls=provider_calls,
        )

    def _can_run(self, task: Optional[Task], executed: set) -> bool:
        if task is None:
            return False
        for dep in task.depends_on:
            if dep in self.errors:
                if not task.optional:
                    return False
        return True

    def _execute(self, task: Task) -> None:
        try:
            if task.task_type == TaskType.RESOLVE:
                self.context["symbol"] = self.symbol
            elif task.task_type == TaskType.COLLECT:
                self._execute_collect(task)
            elif task.task_type == TaskType.ANALYZE:
                self._execute_analyze(task)
            elif task.task_type == TaskType.REPORT:
                pass  # 报告在外部组装
        except Exception as e:
            self.errors[task.name] = str(e)
            if not task.optional:
                raise

    def _execute_collect(self, task: Task) -> None:
        provider_name = task.provider
        params = task.params

        if provider_name == "market":
            days = params.get("days", 60)
            provider = FailoverMarketProvider()
            start = self.as_of - timedelta(days=days * 2)
            bars = provider.get_history(self.symbol, start, self.as_of)
            if len(bars) > days:
                bars = bars[-days:]
            self.context["bars"] = bars

        elif provider_name == "financial":
            quarters = params.get("quarters", 4)
            provider = AkShareFinancialProvider()
            snapshots = provider.get_financials(self.symbol, quarters=quarters)
            valuation = provider.get_valuation(self.symbol)
            self.context["financial_snapshots"] = snapshots
            self.context["valuation"] = valuation

        elif provider_name == "news":
            days = params.get("days", 30)
            provider = AkShareNewsProvider()
            news = provider.search(self.symbol, days=days)
            self.context["news"] = news

        elif provider_name == "announcement":
            days = params.get("days", 30)
            provider = AkShareAnnouncementProvider()
            announcements = provider.search(self.symbol, days=days)
            self.context["announcements"] = announcements

        elif provider_name == "industry":
            from ..providers.industry.sw_provider import SwIndustryProvider
            provider = SwIndustryProvider()
            industry = provider.get_industry(self.symbol)
            self.context["industry"] = industry

    def _execute_analyze(self, task: Task) -> None:
        analysis_name = task.analysis
        bars = self.context.get("bars", [])
        snapshots = self.context.get("financial_snapshots")
        valuation = self.context.get("valuation")
        news = self.context.get("news", [])
        announcements = self.context.get("announcements", [])

        if analysis_name == "price":
            if not bars:
                raise ValueError("无行情数据，无法计算价格指标")
            self.context["price_metrics"] = analyze_price(bars, self.as_of)

        elif analysis_name == "volume":
            if not bars:
                raise ValueError("无行情数据，无法计算成交量指标")
            self.context["volume_metrics"] = analyze_volume(bars, self.as_of)

        elif analysis_name == "financial":
            if snapshots:
                self.context["financial_metrics"] = analyze_financial(snapshots, valuation, self.as_of)
            else:
                self.context["financial_metrics"] = None

        elif analysis_name == "events":
            # 降噪分级：新闻 + 公告 + K线（事件→价格关联）
            if news or announcements:
                self.context["event_metrics"] = analyze_events(
                    news, self.as_of, announcements=announcements, bars=bars
                )
            else:
                self.context["event_metrics"] = None

        elif analysis_name == "risk":
            price = self.context.get("price_metrics")
            volume = self.context.get("volume_metrics")
            financial = self.context.get("financial_metrics")
            if price and volume:
                self.context["risk_metrics"] = analyze_risk(
                    price, volume, financial, announcements
                )
            else:
                self.context["risk_metrics"] = None

        elif analysis_name == "scorecard":
            price = self.context.get("price_metrics")
            volume = self.context.get("volume_metrics")
            financial = self.context.get("financial_metrics")
            events = self.context.get("event_metrics")
            risk = self.context.get("risk_metrics")
            self.context["scorecard"] = analyze_scorecard(
                price, volume, financial, events, risk, self.as_of,
                dimensions=self.profile.scorecard.dimensions,
            )

        # 统一分析引擎（生成 Evidence）
        if analysis_name in ("price", "volume", "financial", "events", "risk"):
            # 当所有核心分析完成后再跑统一引擎
            # 简化：每次分析后都尝试更新 evidence_store
            self._refresh_evidence()

    def _refresh_evidence(self) -> None:
        """刷新 Evidence Store（基于当前已完成的分析结果）"""
        price = self.context.get("price_metrics")
        volume = self.context.get("volume_metrics")
        financial = self.context.get("financial_metrics")
        events = self.context.get("event_metrics")
        news = self.context.get("news", [])
        announcements = self.context.get("announcements", [])

        # 只有当有 bars 时才跑统一引擎
        bars = self.context.get("bars", [])
        if not bars:
            return

        try:
            result = run_analysis_engine(
                bars,
                self.context.get("financial_snapshots"),
                self.context.get("valuation"),
                news,
                announcements,
                self.as_of,
            )
            self.context["analysis_result"] = result
            self.context["evidence_store"] = result.evidence_store
        except Exception:
            # Evidence 生成失败不阻断流程
            pass

    def _generate_reports(self):
        """生成 Markdown + JSON 报告"""
        price = self.context.get("price_metrics")
        volume = self.context.get("volume_metrics")
        financial = self.context.get("financial_metrics")
        snapshots = self.context.get("financial_snapshots")
        events = self.context.get("event_metrics")
        risk = self.context.get("risk_metrics")
        capital = self.context.get("capital_metrics")

        scorecard = self.context.get("scorecard")

        md = generate_markdown(
            self.symbol.full_code,
            self.as_of,
            price,
            volume,
            financial,
            snapshots,
            events,
            risk,
            capital,
            scorecard,
            industry=self.context.get("industry"),
            sections=self.plan.sections,
        )

        js = generate_json(
            self.symbol.full_code,
            self.as_of,
            price,
            volume,
            financial,
            snapshots,
            events,
            risk,
            capital,
            scorecard,
            industry=self.context.get("industry"),
        )

        evidence = self.context.get("evidence_store")
        if evidence:
            js["evidence"] = evidence.to_json()

        return md, js
