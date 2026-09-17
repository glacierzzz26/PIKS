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
from ..analysis.patterns import analyze_patterns
from ..analysis.number_lint import lint_markdown_report, NumberLintReport
from ..report.markdown import generate_markdown
from ..report.json_report import generate_json
from ..evidence import EvidenceStore
from .profile import ResearchProfile, ThresholdConfig
from .plan import ResearchPlan, Task, TaskType


def _display_days(profile: ResearchProfile, key: str, default: int) -> int:
    """Profile 的展示窗口天数(如 "250d" → 250)。缺省用 default。"""
    raw = profile.period.display.get(key, "")
    if isinstance(raw, str) and raw.endswith("d"):
        try:
            return int(raw.rstrip("d"))
        except ValueError:
            pass
    return default


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

        elif provider_name == "industry_index":
            # 行业**本体**(P9 #12):主体是申万行业指数。需要全历史(点位分位,见下),
            # 故采集窗口取 profile.compute.price(6000d ≈ 全量 6456 交易日),展示窗口另裁。
            self._collect_industry_index()

    def _collect_industry_index(self) -> None:
        """行业本体采集(P9 #12)。失败即抛(非 optional)—— 行业报告的行情是主章节,
        采不到就该如实失败,不像「个股的同业对比」那样可降级。"""
        from ..providers.industry.sw_index_provider import SwIndexProvider

        provider = SwIndexProvider()
        ref = provider.resolve(self.symbol.code)
        if ref is None:
            raise ValueError(
                f"申万行业码 {self.symbol.code} 不在申万一级/二级/三级行业表中"
                f"(行业码↔名称必须查表,不按前缀推断)"
            )
        # 全历史:index_hist_sw 实测 6456 行(1999-12-30 起)。天数上限即全量。
        history_days = _display_days(self.profile, "price", 250)
        compute_days = 6000
        raw = self.profile.period.compute.get("price", "")
        if raw.endswith("d"):
            try:
                compute_days = int(raw.rstrip("d"))
            except ValueError:
                pass
        history_start = self.as_of - timedelta(days=compute_days * 2)
        history_bars = provider.get_history(self.symbol, history_start, self.as_of)
        if not history_bars:
            raise ValueError(f"申万行业 {ref.code} {ref.name} 无行情序列")

        display_bars = history_bars[-history_days:] if len(history_bars) > history_days else history_bars
        constituents = provider.get_constituents(ref)
        rank = provider.valuation_rank(ref)

        self.context["industry_ref"] = ref
        self.context["industry_history_bars"] = history_bars
        self.context["industry_display_bars"] = display_bars
        self.context["industry_constituents"] = constituents
        self.context["industry_valuation_rank"] = rank
        # 个股的 Evidence 由 _refresh_evidence 建(它要求 bars,对指数不适用),
        # 行业路径在此自建空 store,供后续 add_industry_evidence 填。
        if self.context.get("evidence_store") is None:
            self.context["evidence_store"] = EvidenceStore()

    def _execute_analyze(self, task: Task) -> None:
        analysis_name = task.analysis
        bars = self.context.get("bars", [])
        snapshots = self.context.get("financial_snapshots")
        valuation = self.context.get("valuation")
        news = self.context.get("news", [])
        announcements = self.context.get("announcements", [])

        if analysis_name == "industry_index":
            from ..analysis.industry import analyze_industry
            ref = self.context.get("industry_ref")
            if ref is None:
                raise ValueError("无行业主体,无法计算行业指标")
            self.context["industry_metrics"] = analyze_industry(
                ref,
                self.context.get("industry_display_bars", []),
                self.context.get("industry_history_bars", []),
                self.context.get("industry_constituents", []),
                self.context.get("industry_valuation_rank"),
                self.as_of,
            )
            return

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
            # 行业主体(P9 #12)走独立风险引擎:个股风险依据「换手率+成交额」,
            # 而申万指数这两个字段量纲不可靠 —— 套用会产出基于假数字的结论。
            im = self.context.get("industry_metrics")
            if im is not None:
                from ..analysis.industry import analyze_industry_risk
                self.context["industry_risk_metrics"] = analyze_industry_risk(im, self.as_of)
                # 行业 Evidence(个股的 _refresh_evidence 依赖 bars,对指数不适用)
                from ..analysis.engine import add_industry_evidence
                store = self.context.get("evidence_store")
                if store is not None:
                    add_industry_evidence(
                        store, im, self.context["industry_risk_metrics"]
                    )
            elif price or volume or financial or announcements:
                # 量价可选(P9-4,issue #11):公司研报不采行情,靠基本面规则出风险章。
                self.context["risk_metrics"] = analyze_risk(
                    self.symbol.code, self.as_of,
                    price, volume, financial, announcements,
                )
            else:
                self.context["risk_metrics"] = None

            # 无行情主体(公司研报)的 Evidence:_refresh_evidence 被 `if not bars`
            # 挡住,不在此补登记则机检 evidence_completeness 必失败。与行业路径
            # (上面的 add_industry_evidence)对称 —— 只在 bars 为空时走这条。
            if im is None and not bars:
                from ..analysis.engine import add_fundamental_evidence
                store = self.context.get("evidence_store")
                if store is None:
                    store = EvidenceStore()
                    self.context["evidence_store"] = store
                add_fundamental_evidence(
                    store,
                    self.context.get("financial_metrics"),
                    self.context.get("risk_metrics"),
                    self.context.get("event_metrics"),
                )

        elif analysis_name == "patterns":
            # 量价形态（规则判定）；无 bars 时留空，不阻断
            if bars:
                self.context["pattern_metrics"] = analyze_patterns(bars, self.as_of)
            else:
                self.context["pattern_metrics"] = None

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
        if analysis_name in ("price", "volume", "financial", "events", "risk", "patterns"):
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
        industry_metrics = self.context.get("industry_metrics")
        industry_risk = self.context.get("industry_risk_metrics")

        # 行业主体:风险章节用行业风险引擎的产物(个股 risk_metrics 为 None)。
        if industry_metrics is not None and industry_risk is not None:
            risk = industry_risk

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
            patterns=self.context.get("pattern_metrics"),
            industry_metrics=industry_metrics,
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
            patterns=self.context.get("pattern_metrics"),
            industry_metrics=industry_metrics,
            # 章节清单与 markdown 同源(D-R8):前端 TOC/三域标记据此渲染。
            sections=self.plan.sections,
        )

        evidence = self.context.get("evidence_store")
        if evidence:
            js["evidence"] = evidence.to_json()

        return md, js
