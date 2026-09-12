"""Number Lint — 事后数字对账

职责（设计文档 M5/M6 质量门）：
- 任何出现在文本（报告 / LLM 定性段落）中的数字，必须能在
  「已知数值集合」（指标卡 / JSON 报告 / Evidence）中找到出处。
- 找不到出处的数字 → 标记 [待核实]，由人工或 LLM 复核。
- 核心原则：LLM 不产生新数值，只能引用已有数值。

用法：
    known = collect_numbers_from_json(json_report)
    report = lint_text(text, known_values=known)
    if not report.passed:
        for issue in report.issues: ...
"""
import re
from dataclasses import dataclass, field
from typing import Any, Iterable, List, Optional, Set, Union

# ------------- 掩码模式（先于数字扫描处理） -------------
# 日期：2026-09-10 / 2026/9/10
_DATE_RE = re.compile(r"\d{4}[-/]\d{1,2}[-/]\d{1,2}")
# 中文日期：9月8日 / 9月8号（LLM 定性段落常见写法）
_CN_DATE_RE = re.compile(r"\d{1,2}月\d{1,2}[日号]")
# 时间：12:30 / 12:30:45
_TIME_RE = re.compile(r"\d{1,2}:\d{2}(?::\d{2})?")
# 新闻来源日期 MM-DD（出现在 “(来源, 08-15)” 中）
_NEWS_DATE_RE = re.compile(r",\s*\d{1,2}-\d{1,2}\)")
# 股票代码：6 位数字（不带前后数字）
_CODE_RE = re.compile(r"(?<!\d)\d{6}(?!\d)")
# 表格分隔线与 markdown 结构
_TABLE_SEP_RE = re.compile(r"^\s*\|[-:\s|]+\|\s*$")
_HEADING_RE = re.compile(r"^\s*#{1,6}\s")
# 新闻/公告列表项（verbatim 引用内容，其中的数字是来源数据而非计算值）：
# 加粗：`1. **标题** (来源, 08-15)`；非加粗：`1. 标题 (来源, 08-15)`
_NEWS_ITEM_RE = re.compile(
    r"^\s*\d+[.、]\s+\*\*.*\([^()]*,\s*\d{1,2}-\d{1,2}\)\s*$"
    r"|^\s*\d+[.、]\s+[^*].*\([^()]*,\s*\d{1,2}-\d{1,2}\)\s*$"
    # 事件降噪行：`- **[高/中]** 标题`（标题为 verbatim 引用）
    r"|^\s*- \*\*\[[高中]\]\*\* .*$"
    # 例行事务聚合行：`**例行事务**（N 条）：标题…`（标题引用 + 计数）
    r"|^\*\*例行事务\*\*.*$"
    # 评分卡刻度说明：`> 评分说明：-2（极弱）~ +2（极强）…`（模板刻度，非数据）
    r"|^> 评分说明：.*$"
)

# 数字 token：带千分位、正负号、小数
_NUMBER_RE = re.compile(r"[-+]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?")

# 行首序数：`1.` `2、` `3)` 等列表标记，不算数据数值
_ORDINAL_RE = re.compile(r"^\s*\d+\s*[\.、)）]")

# 模板结构性常量（非数据数值，出现于报告骨架中）
# 窗口 5/10/20/60/250/520 等为分析窗口标签；3/4/8/12 等为展示条数与章节数
TEMPLATE_CONSTANTS: Set[float] = {
    3,      # 列表展示条数（重点事件/最新动态/风险项前三条）
    4,      # 财务趋势展示季度数
    5,      # 价格/量窗口；龙虎榜窗口
    8,      # 报告章节数
    10,     # 价格窗口
    12,     # 评分卡维度评分范围上限（|-12..+12|）
    20,     # 价格/量窗口
    30,     # 事件分析窗口（天）
    60,     # 价格/量窗口
    250,    # 年化波动率换算天数
    260,    # 估值窗口
    520,    # 换手率长窗口（profile）
    2,      # 评分卡刻度 ±2（-2 极弱 ~ +2 极强）
}


def _parse_number(raw: str) -> float:
    """解析数字 token → float（去掉千分位与 + 号）"""
    return float(raw.replace(",", "").lstrip("+"))


def collect_numbers_from_value(value: Any, acc: Set[float]) -> None:
    """递归收集 dict/list 中所有数值（用于构建已知集合）"""
    if isinstance(value, bool):
        return
    if isinstance(value, (int, float)):
        acc.add(float(value))
    elif isinstance(value, dict):
        for v in value.values():
            collect_numbers_from_value(v, acc)
    elif isinstance(value, (list, tuple)):
        for v in value:
            collect_numbers_from_value(v, acc)


def collect_numbers_from_json(obj: Any) -> Set[float]:
    """从 JSON 报告构建已知数值集合"""
    acc: Set[float] = set()
    collect_numbers_from_value(obj, acc)
    return acc


@dataclass
class NumberToken:
    raw: str
    value: float
    line_no: int
    col: int
    ordinal: bool = False


@dataclass
class LintIssue:
    number: str
    value: float
    line_no: int
    context: str
    reason: str
    severity: str = "warning"

    def __str__(self) -> str:
        return (
            f"[{self.severity}] L{self.line_no} 数字 {self.number!r} "
            f"{self.reason}  |  …{self.context}…"
        )


@dataclass
class NumberLintReport:
    scanned: int
    matched: int
    ignored: int
    issues: List[LintIssue] = field(default_factory=list)

    @property
    def passed(self) -> bool:
        return len(self.issues) == 0

    def summary(self) -> str:
        return (
            f"Number Lint: 扫描 {self.scanned} 个数字，"
            f"匹配 {self.matched}，忽略 {self.ignored}，"
            f"问题 {len(self.issues)}。"
            + ("✅ 通过" if self.passed else " ❌ 未通过")
        )


def _mask_text(text: str) -> str:
    """掩码日期/时间/股票代码/新闻来源日期，避免被当作数据数值"""
    text = _DATE_RE.sub("DATE", text)
    text = _CN_DATE_RE.sub("DATE", text)
    text = _TIME_RE.sub("TIME", text)
    text = _CODE_RE.sub("CODE", text)
    text = _NEWS_DATE_RE.sub(", NEWS_DATE)", text)
    return text


def _neighbor(line: str, col: int, radius: int = 25) -> str:
    """取数字附近上下文"""
    start = max(0, col - radius)
    end = min(len(line), col + radius)
    return line[start:end]


def lint_text(
    text: str,
    known_values: Optional[Set[float]] = None,
    template_constants: Optional[Set[float]] = None,
    rel_tol: float = 0.02,
    abs_tol: float = 0.06,
) -> NumberLintReport:
    """
    扫描文本中的数字，与已知集合对账。

    - known_values: 指标卡 / JSON 报告中的数值（基准）
    - template_constants: 模板骨架常量（默认 TEMPLATE_CONSTANTS）
    - rel_tol: 相对容差（容忍格式化差异，如 17.7 vs 17.70）
    - abs_tol: 最小绝对容差（容忍显示舍入，如 1.47 显示为 1.5）
    """
    known = set(known_values or set())
    known |= set(template_constants or TEMPLATE_CONSTANTS)

    def _is_known(value: float) -> bool:
        for k in known:
            # 描述性数字（“跌 5.15%”）常用绝对值，与基准符号相反时按绝对值对账
            scale = max(abs(value), abs(k), 1.0)
            if abs(abs(value) - abs(k)) <= max(rel_tol * scale, abs_tol):
                return True
        return False


    if not known:
        # 无基准时不误报，直接视为全通过（调用方应保证有基准）
        pass

    report = NumberLintReport(scanned=0, matched=0, ignored=0)
    raw_lines = text.splitlines()
    masked_lines = _mask_text(text).splitlines()

    for line_no, (line, raw_line) in enumerate(zip(masked_lines, raw_lines), start=1):
        if _TABLE_SEP_RE.match(line) or _HEADING_RE.match(line):
            continue

        # 新闻/公告引用行整体跳过（在原始行上判断，数字属于来源数据）
        if _NEWS_ITEM_RE.match(raw_line):
            report.ignored += 1
            continue


        # 行首序数跳过
        is_ordinal_line = bool(_ORDINAL_RE.match(line))
        ordinal_done = False

        for m in _NUMBER_RE.finditer(line):
            raw = m.group(0)
            value = _parse_number(raw)
            report.scanned += 1

            # 行首序数：只跳过第一个
            if is_ordinal_line and not ordinal_done:
                ordinal_done = True
                report.ignored += 1
                continue

            # 模板常量 / 已知数值
            if _is_known(value):
                report.matched += 1
                continue

            # 未匹配 → 问题
            report.issues.append(
                LintIssue(
                    number=raw,
                    value=value,
                    line_no=line_no,
                    context=_neighbor(line, m.start()),
                    reason="未在指标卡/已知数值中找到出处（可能为编造或模板残留）",
                )
            )

    return report


def lint_markdown_report(
    markdown_text: str,
    json_report: dict,
    template_constants: Optional[Set[float]] = None,
) -> NumberLintReport:
    """便捷入口：对整篇 Markdown 报告做 Number Lint"""
    known = collect_numbers_from_json(json_report)
    return lint_text(markdown_text, known_values=known, template_constants=template_constants)