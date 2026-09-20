"""M6 Quality Gate — 机检规则（每条都必须可判定，非形容词）

规则清单（设计文档 §M6 v1.1）：
1. data_completeness    : JSON 报告 section 覆盖 Profile 所需 section（覆盖率阈值）
2. evidence_completeness: 核心指标有 Evidence 支撑（按 section 匹配 evidence.type）
3. citation_correctness : JSON 报告数值 ⊆ Evidence 数值集合（报告引用与证据一致）
4. time_boundary        : 每个 section 有 as_of / period 时间边界
5. number_lint          : Number Lint 通过（模板槽位数字 100% 可溯源）
6. reproducibility      : report_fingerprint 与 JSON 重算指纹一致（同数据同代码可复现）
7. no_hallucination     : LLM 合成段落数字未超出现有数值集合（由 Number Lint 事后扫描保障）

输出：QualityGateReport（passed / issues / checks）
"""
import hashlib
import json
import re
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Set, Tuple

SECTION_COVERAGE_MIN = 0.8  # Profile section 覆盖率下限


@dataclass
class GateCheck:
    """单项机检结果"""
    name: str
    passed: bool
    detail: str = ""
    meta: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        return {
            "name": self.name,
            "passed": self.passed,
            "detail": self.detail,
            "meta": self.meta,
        }


@dataclass
class QualityGateReport:
    """质量门禁总报告"""
    passed: bool
    checks: List[GateCheck] = field(default_factory=list)
    issues: List[str] = field(default_factory=list)

    def add(self, check: GateCheck) -> None:
        self.checks.append(check)
        if not check.passed:
            self.issues.append(f"[{check.name}] {check.detail}")

    def summary(self) -> str:
        n_pass = sum(1 for c in self.checks if c.passed)
        n_all = len(self.checks)
        status = "✅ 通过" if self.passed else "❌ 未通过"
        return f"M6 Quality Gate: {status}（{n_pass}/{n_all} 项机检）"

    def to_dict(self) -> Dict[str, Any]:
        return {
            "passed": self.passed,
            "checks": [c.to_dict() for c in self.checks],
            "issues": self.issues,
        }


# ---------- 机器可判定数值提取 ----------

def extract_numbers(value: Any) -> Set[Tuple[float, str]]:
    """递归提取 dict/list 中的数值（带字段名），用于对账"""
    nums: Set[Tuple[float, str]] = set()
    if isinstance(value, dict):
        for k, v in value.items():
            for t in extract_numbers(v):
                nums.add(t)
    elif isinstance(value, list):
        for v in value:
            for t in extract_numbers(v):
                nums.add(t)
    elif isinstance(value, (int, float)) and not isinstance(value, bool):
        # 用字段名路径标识：此处简化，仅保留纯数值，后续按字段匹配
        nums.add((float(value), ""))
    return nums


def _flatten_metrics(data: Dict[str, Any], prefix: str = "") -> Dict[str, Any]:
    """扁平化指标字段 → {field_path: value}，仅保留标量数值/字符串"""
    out: Dict[str, Any] = {}
    for k, v in data.items():
        path = f"{prefix}.{k}" if prefix else k
        if isinstance(v, dict):
            # 列表型字典（financial_snapshots）跳过
            if any(isinstance(x, dict) for x in v.values() if isinstance(v, dict)):
                continue
            out.update(_flatten_metrics(v, path))
        elif isinstance(v, (int, float, str)) and not isinstance(v, bool):
            out[path] = v
    return out


def fingerprint_report(report_json: dict) -> str:
    """生成报告指纹（规范化 JSON 的 hash）——同数据同代码重跑可比对一致性"""
    canonical = json.dumps(report_json, sort_keys=True, ensure_ascii=False, default=str)
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:16]


def run_quality_gate(
    profile_sections: List[str],
    json_report: Dict[str, Any],
    evidence: Optional[List[Dict[str, Any]]],
    lint_passed: bool,
    lint_issue_count: int,
    stored_fingerprint: Optional[str] = None,
    computed_fingerprint: Optional[str] = None,
) -> QualityGateReport:
    """执行 M6 六项机检

    Args:
        profile_sections : Profile 要求的 sections（如 ["company","market",...]）
        json_report      : JSON 报告
        evidence         : Evidence 列表（JSON 序列化）
        lint_passed      : Number Lint 是否通过
        lint_issue_count : Lint 问题数
        stored_fingerprint : 归档时存的 report_fingerprint
        computed_fingerprint: 当前重算的指纹
    """
    gate = QualityGateReport(passed=True)

    # ---- 1. Data Completeness：section 覆盖率 ----
    # Profile section → JSON 顶层 key 的映射
    SECTION_TO_KEY = {
        "company": "meta",
        "market": "price",
        "volume": "volume",
        "turnover": "volume",
        "financial": "financial",
        "valuation": "financial",
        "events": "events",
        "announcements": "events",
        "industry": "industry",
        # 行业主体(P9 #12):三节同源于 industry_index 指标卡。
        "industry_index": "industry_index",
        "industry_valuation": "industry_index",
        "industry_structure": "industry_index",
        # 宏观维度主体(P9-5 / #13):两节同源于 macro 指标卡。
        "macro_level": "macro",
        "macro_position": "macro",
        # 资金面/龙虎榜(issue #26):JSON 顶层 key 是 `capital`。
        "capital": "capital",
        "risk": "risk",
        "conclusion": "scorecard",
    }
    covered = 0
    missing = []
    for sec in profile_sections:
        key = SECTION_TO_KEY.get(sec)
        if key in json_report:
            covered += 1
        else:
            missing.append(sec)
    coverage = covered / len(profile_sections) if profile_sections else 1.0
    gate.add(
        GateCheck(
            name="data_completeness",
            passed=coverage >= SECTION_COVERAGE_MIN,
            detail=f"section 覆盖率 {coverage:.0%}（要求 ≥{SECTION_COVERAGE_MIN:.0%}）",  # noqa: E501
            meta={"coverage": round(coverage, 3), "missing": missing},
        )
    )

    # ---- 2. Evidence Completeness：核心 section 有 Evidence 支撑 ----
    evidence_sections: Set[str] = set()
    if evidence:
        evidence_sections = {e.get("section", "") for e in evidence if e.get("section")}
    # 核心 section → 需要的 Evidence section
    SECTION_FOR_SECTION = {
        "price": "price",
        "volume": "volume",
        "turnover": "volume",
        "financial": "financial",
        "valuation": "financial",
        "events": "events",
        "risk": "risk",
        "conclusion": "risk",
        # 行业主体:三节的 Evidence 由 add_industry_evidence 按此 section 登记。
        "industry_index": "industry_index",
        "industry_valuation": "industry_valuation",
        "industry_structure": "industry_structure",
        # 宏观主体:两节的 Evidence 由 add_macro_evidence 按此 section 登记。
        "macro_level": "macro_level",
        "macro_position": "macro_position",
    }
    missing_ev = []
    for sec in profile_sections:
        need = SECTION_FOR_SECTION.get(sec)
        if need and need not in evidence_sections:
            missing_ev.append(sec)
    ev_check_passed = len(missing_ev) == 0 and bool(evidence)
    gate.add(
        GateCheck(
            name="evidence_completeness",
            passed=ev_check_passed,
            detail=(
                f"{len(evidence or [])} 条 Evidence，覆盖 sections={sorted(evidence_sections)}"
                if evidence
                else "无 Evidence"
            ),
            meta={"evidence_count": len(evidence or []), "missing_sections": missing_ev},
        )
    )

    # ---- 3. Citation Correctness：报告数值 ⊆ 证据数值 ----
    # 简化机检：scorecard 维度 reason 中的数字应能在 JSON 中找到
    citation_issues = 0
    for dim in json_report.get("scorecard", {}).get("dimensions", []):
        reason = dim.get("reason", "")
        if not reason:
            continue
        for m in re.finditer(r"-?\d+(?:\.\d+)?", reason):
            num = float(m.group())
            if not _number_in_report(num, json_report):
                citation_issues += 1
    gate.add(
        GateCheck(
            name="citation_correctness",
            passed=citation_issues == 0,
            detail=f"评分卡 reason 中有 {citation_issues} 个数字无法在报告中追溯",
            meta={"unresolved": citation_issues},
        )
    )

    # ---- 4. Time Boundary：as_of + period 存在 ----
    # 行业主体无 price 节(行业指数不是个股行情),时间边界落在 industry_index.price。
    # 公司研报(P9-4)同样无 price —— 它不采行情,时间边界落在**财报报告期**
    # (financial_snapshots[].report_date)。宏观研报(P9-5)三处皆无,边界落在
    # macro.period(统计期 + 滞后天数)。四态各自取对应来源,判定的是
    # 「有没有时间边界」,不是「边界一定长在 price 上」。
    meta = json_report.get("meta", {})
    has_as_of = bool(meta.get("as_of"))
    period_src = json_report.get("price") or (
        (json_report.get("industry_index") or {}).get("price") or {}
    )
    has_period = bool(period_src.get("period_days"))
    # 基本面路径兜底:有财报快照即视为有明确时间边界(最新报告期)。
    if not has_period:
        snaps = json_report.get("financial_snapshots") or []
        has_period = any(s.get("report_date") for s in snaps)
    # 宏观路径兜底:有 period 块(统计期末 + 期号)即视为有明确时间边界。
    # ⚠️ 判的是 `period_end` 而非 `label` —— label 是展示原文,period_end 才是
    # 机器可比的边界值。缺 period 块的宏观报告仍须 FAIL(反向守卫见测试)。
    if not has_period:
        macro_period = (json_report.get("macro") or {}).get("period") or {}
        has_period = bool(macro_period.get("period_end"))
    gate.add(
        GateCheck(
            name="time_boundary",
            passed=has_as_of and has_period,
            detail=f"as_of={'✓' if has_as_of else '✗'}, period_days={'✓' if has_period else '✗'}",
            meta={"has_as_of": has_as_of, "has_period": has_period},
        )
    )

    # ---- 5. Number Lint ----
    gate.add(
        GateCheck(
            name="number_lint",
            passed=lint_passed,
            detail=f"扫描完成，{lint_issue_count} 个问题" + ("" if lint_passed else "（未通过）"),
            meta={"issue_count": lint_issue_count},
        )
    )

    # ---- 6. Reproducibility：fingerprint 一致 ----
    if stored_fingerprint is not None and computed_fingerprint is not None:
        fp_ok = stored_fingerprint == computed_fingerprint
        gate.add(
            GateCheck(
                name="reproducibility",
                passed=fp_ok,
                detail=(
                    f"指纹一致（{computed_fingerprint[:12]}…）"
                    if fp_ok
                    else f"指纹不一致: 存库 {stored_fingerprint[:12]}… ≠ 重算 {computed_fingerprint[:12]}…"
                ),
                meta={"stored": stored_fingerprint, "computed": computed_fingerprint},
            )
        )
    else:
        gate.add(
            GateCheck(
                name="reproducibility",
                passed=True,
                detail="（跳过：无对照指纹）",
                meta={},
            )
        )

    # 汇总
    gate.passed = all(c.passed for c in gate.checks)
    return gate


def _flatten_numeric_fields(data: Any, out: Optional[Set[float]] = None) -> Set[float]:
    """递归收集纯数值字段（排除字符串字段里嵌的数字，避免 reason 自引用）"""
    if out is None:
        out = set()
    if isinstance(data, dict):
        for v in data.values():
            _flatten_numeric_fields(v, out)
    elif isinstance(data, list):
        for v in data:
            _flatten_numeric_fields(v, out)
    elif isinstance(data, (int, float)) and not isinstance(data, bool):
        out.add(float(data))
    return out


def _number_in_report(num: float, report: Dict[str, Any], tol: float = 0.06) -> bool:
    """判断数值是否对应报告数值字段之一（显示舍入对账）

    与 Number Lint 同一容差口径：rel 2% + abs 0.06。
    reason 中的描述性数字是绝对值（如“下降 5.1pct”），按 abs 匹配。
    """
    known = _flatten_numeric_fields(report)
    for val in known:
        if abs(abs(val) - abs(num)) <= max(abs(num) * 0.02, tol):
            return True
    return False