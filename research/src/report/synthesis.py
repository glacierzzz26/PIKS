"""AI Synthesis — LLM 定性分析层

设计原则（§38）：LLM 只写定性段落，不产生新数值。
- 所有数值来自指标卡 / JSON 报告（程序确定性计算）
- LLM 可以参考、引用、解释这些数值，但不得编造新数字
- Number Lint 事后扫描 LLM 文本，把不能追溯到指标卡的数字标记出来

职责拆分：
- build_synthesis_prompt   : 把指标卡摘要 + 约束 + 输出格式组装成给 LLM 的提示
- parse_synthesis          : 解析 LLM 输出（容错 JSON）
- render_synthesis         : 把 LLM 段落注入 Markdown 模板槽位
"""
import json
import re
from typing import Any, Dict, Optional

from .json_report import generate_json

# Markdown 模板中的 AI Synthesis 槽位
SYNTHESIS_SLOTS = ["summary", "trend", "conclusion"]

SLOT_FALLBACK = {
    "summary": "_（待 AI 综合研判：执行摘要）_",
    "trend": "_（待 AI 综合研判：趋势解读）_",
    "conclusion": "_（待 AI 综合研判：综合结论）_",
}


def build_synthesis_prompt(
    symbol: str,
    json_report: Dict[str, Any],
    prompt_extras: Optional[Dict[str, str]] = None,
) -> str:
    """
    生成 AI 合成提示。

    输入：结构化 JSON 报告（含全部指标）
    输出：给 LLM 的提示文本（含指标摘要 + 硬约束 + 输出格式）
    """
    # 指标摘要：只保留数值字段，排除列表型长数据
    summary: Dict[str, Any] = {}
    for section, data in json_report.items():
        if section == "meta" or section == "evidence":
            continue
        if isinstance(data, dict):
            summary[section] = {
                k: v
                for k, v in data.items()
                if isinstance(v, (int, float, str)) and not isinstance(v, bool)
            }

    lines = [
        f"你是一名 A 股研究分析师，正在撰写 {symbol} 的个股研究报告。",
        "以下是该股票近期研究的数据指标卡（全部由确定性计算引擎生成）：",
        "",
        "```json",
        json.dumps(summary, ensure_ascii=False, indent=2, default=str),
        "```",
        "",
        "请根据以上数据，撰写三段**定性叙述**（不要生成表格，用纯段落文字）：",
        "",
        "1. **summary**（执行摘要，2-4 句）：概括当前股价表现、基本面与整体多空格局。",
        "2. **trend**（趋势解读，3-5 句）：解读近期量价与基本面趋势，解释主要驱动与隐忧。",
        "3. **conclusion**（综合结论，3-5 句）：基于评分卡给出综合判断与关注要点，明确是正面/中性/负面倾向。",
        "",
        "**硬性约束：**",
        "- 只能引用上述指标卡中出现的数字；不得编造任何新数字、金额、百分比、日期。",
        "- 引用数字时保持原值，允许四舍五入到 1-2 位小数，允许添加 %、元、手、条等单位后缀。",
        "- 不得声称「数据缺失但给出估计」，缺失字段直接不讨论。",
        "- 不得进行财务预测、目标价预估、收益承诺。",
        "",
        "**输出格式：**严格输出以下 JSON（不要 markdown 代码块包裹，不要其他文字）：",
        '{"summary": "...", "trend": "...", "conclusion": "..."}',
        "",
    ]

    extras = prompt_extras or {}
    for key, text in extras.items():
        lines.append(f"[{key}] {text}")

    return "\n".join(lines)


def parse_synthesis(text: str) -> Dict[str, str]:
    """
    解析 LLM 输出的合成结果。

    容错策略：
    - 剥掉 markdown 代码块围栏
    - 找到第一个 { 到最后一个 } 之间的内容当 JSON 解析
    - 解析失败时逐槽位尝试独立提取
    """
    text = (text or "").strip()
    if not text:
        return {}

    # 剥 markdown 围栏
    if text.startswith("```"):
        text = re.sub(r"^```[a-zA-Z]*\s*", "", text)
        text = re.sub(r"\s*```$", "", text)

    # 尝试整体 JSON
    start, end = text.find("{"), text.rfind("}")
    if start != -1 and end > start:
        candidate = text[start : end + 1]
        try:
            data = json.loads(candidate)
            result = {}
            for slot in SYNTHESIS_SLOTS:
                v = data.get(slot)
                if isinstance(v, str) and v.strip():
                    result[slot] = v.strip()
            return result
        except json.JSONDecodeError:
            pass

    # 逐槽位容错提取：`"summary": "..."` 或 `summary: ...`
    result = {}
    for slot in SYNTHESIS_SLOTS:
        m = re.search(rf'["\']?{slot}["\']?\s*[:：]\s*["\'](.+?)["\']', text, re.S)
        if m:
            result[slot] = m.group(1).strip()
    return result


def validate_synthesis(
    blocks: Dict[str, str],
    json_report: Dict[str, Any],
) -> Dict[str, str]:
    """
    校验 LLM 定性段落。

    返回 {slot: error_message}，error 为空表示通过。
    校验规则：
    1. 槽位完整性：三个槽位都必须有内容
    2. 长度下限：每段至少 20 字
    （数字对账交给 Number Lint 事后扫描）
    """
    errors: Dict[str, str] = {}
    for slot in SYNTHESIS_SLOTS:
        text = blocks.get(slot, "").strip()
        if not text:
            errors[slot] = "槽位为空"
        elif len(text) < 20:
            errors[slot] = f"内容过短（{len(text)} 字，要求 ≥20）"
    return errors


def render_synthesis(
    markdown: str,
    blocks: Optional[Dict[str, str]],
    include_section: bool = True,
) -> str:
    """
    把 LLM 定性段落渲染进 Markdown 模板。

    - 有内容：注入槽位
    - 无内容：用占位符（_待 AI 综合研判_）
    """
    blocks = blocks or {}

    slot_lines = []
    if include_section:
        slot_lines.append("\n\n---\n\n## 九、AI 综合研判\n")

    slot_lines.append("\n### 执行摘要\n")
    slot_lines.append(blocks.get("summary", SLOT_FALLBACK["summary"]))

    slot_lines.append("\n### 趋势解读\n")
    slot_lines.append(blocks.get("trend", SLOT_FALLBACK["trend"]))

    slot_lines.append("\n### 综合结论\n")
    slot_lines.append(blocks.get("conclusion", SLOT_FALLBACK["conclusion"]))

    synthesized = "\n".join(slot_lines)

    # 替换模板中的槽位标记（兼容 f-string 渲染后的 `{ai_synthesis}` 与原始 `{{ai_synthesis}}`）
    import re
    if re.search(r"\{\{?\s*ai_synthesis\s*\}\}?", markdown):
        return re.sub(r"\{\{?\s*ai_synthesis\s*\}\}?", lambda _m: synthesized, markdown)
    return markdown + synthesized