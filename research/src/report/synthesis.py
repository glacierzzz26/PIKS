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

    # 主体感知(P9 #12 行业 / P9-5 宏观):主体不是个股。
    # ⚠️ 个股分支的文案 "A 股研究分析师" 是 internal/ai/mock.go 的关键词,
    # 改动它会让 mock provider 在 dev 下不回话 —— 故个股分支**逐字节不变**。
    is_industry = "industry_index" in json_report
    is_macro = "macro" in json_report

    if is_macro:
        ref = (json_report.get("macro") or {}).get("ref") or {}
        period = (json_report.get("macro") or {}).get("period") or {}
        name = ref.get("name") or symbol
        # 主体句带上统计期:宏观报告必须让 LLM 知道「这是哪一期的读数」,
        # 否则它会写出无时间锚点的定性段落。
        label = period.get("label")
        subj = f"{name}（{label}）" if label else name
        lines = [
            f"你是一名宏观研究分析师，正在撰写 {subj} 的宏观研究报告。",
            "以下是该宏观维度近期的数据指标卡（全部由确定性计算引擎生成）：",
        ]
    elif is_industry:
        ref = (json_report.get("industry_index") or {}).get("ref") or {}
        # ⚠️ 不要 .strip("（）"):它会把刚拼上的右括号也剥掉("农林牧渔（申万一级")。
        name, label = ref.get("name"), ref.get("level_label")
        subj = f"{name}（{label}）" if name and label else (name or symbol)
        lines = [
            f"你是一名 A 股行业研究分析师，正在撰写 {subj} 的行业研究报告。",
            "以下是该行业近期研究的数据指标卡（全部由确定性计算引擎生成）：",
        ]
    else:
        lines = [
            f"你是一名 A 股研究分析师，正在撰写 {symbol} 的个股研究报告。",
            "以下是该股票近期研究的数据指标卡（全部由确定性计算引擎生成）：",
        ]

    lines += [
        "",
        "```json",
        json.dumps(summary, ensure_ascii=False, indent=2, default=str),
        "```",
        "",
        "请根据以上数据，撰写三段**定性叙述**（不要生成表格，用纯段落文字）：",
        "",
    ]

    if is_macro:
        lines += [
            "1. **summary**（执行摘要，2-4 句）：概括该宏观指标最新读数、同比/环比方向与整体所处位置。",
            "2. **trend**（趋势解读，3-5 句）：解读近期序列走势与历史分位变化，说明主要驱动与隐忧。",
            "3. **conclusion**（综合结论，3-5 句）：基于读数、历史定位与规则判定的风险等级给出综合判断，明确是正面/中性/负面倾向。",
        ]
    elif is_industry:
        lines += [
            "1. **summary**（执行摘要，2-4 句）：概括行业指数近期表现、估值位置与成分结构特征。",
            "2. **trend**（趋势解读，3-5 句）：解读行业行情趋势与成分盈利分化，解释主要驱动与隐忧。",
            "3. **conclusion**（综合结论，3-5 句）：基于行情、估值横截面位次与成分结构给出行业综合判断，明确是正面/中性/负面倾向。",
        ]
    else:
        lines += [
            "1. **summary**（执行摘要，2-4 句）：概括当前股价表现、基本面与整体多空格局。",
            "2. **trend**（趋势解读，3-5 句）：解读近期量价与基本面趋势，解释主要驱动与隐忧。",
            "3. **conclusion**（综合结论，3-5 句）：基于评分卡给出综合判断与关注要点，明确是正面/中性/负面倾向。",
        ]

    lines += [
        "",
        "**硬性约束：**",
        "- 只能引用上述指标卡中出现的数字；不得编造任何新数字、金额、百分比、日期。",
        "- 引用数字时保持原值，允许四舍五入到 1-2 位小数，允许添加 %、元、手、条等单位后缀。",
        "- 不得声称「数据缺失但给出估计」，缺失字段直接不讨论。",
        "- 不得进行财务预测、目标价预估、收益承诺。",
    ]
    if is_macro:
        # 水平列的量纲**逐维度不同**(CPI/PPI 是「上年同月=100」的指数、M2/GDP 是亿元),
        # 提示词不得写死某一种 —— 从卡里的 `ref.level_label` 取(知识表单一真源在
        # provider,此处只读)。取不到才退回泛述。
        level_label = ref.get("level_label") or "指数或亿元"
        lines += [
            "- 不得对宏观指标作任何预测（不得写未来值、目标位、「预计下月」、「政策将…」）。",
            "- 不得表述「数据将于 X 月公布」—— 数据源不提供发布日历。",
            "- 不得跨维度作因果或领先滞后推断（不得写「M2 领先 CPI」这类关系）。",
            "- 不得与政策目标对比（如「CPI 低于 3% 目标」）—— 目标值不在数据源内。",
            "- 不得给出资产配置建议。",
            f"- ⚠️ 最新读数/序列读数那几列的**水平量纲是「{level_label}」，不是百分比**；"
            "只有同比/环比是百分比，不得混读。",
        ]
    elif is_industry:
        # D-R7(design report-layout.md):申万行业估值只有当期快照,无历史序列。
        lines.append("- 不得表述行业估值的历史分位；估值只可作横截面比较（同层级行业间的位次）。")

    lines += [
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
    include_section: bool = False,
) -> str:
    """
    把 LLM 定性段落渲染进 Markdown 模板。

    - 有内容：注入槽位（三个 `### ` 子段）
    - 无内容：用占位符（_待 AI 综合研判_）

    include_section 默认 **False**(P9 D-R6 摘要前置):骨架已含「## 一、执行摘要」
    章标题,此处只需注入三子段。置 True 会另起「## 九、AI 综合研判」把 AI 段落又
    搬到文末 —— 正是改造前的硬伤,仅作兼容保留。
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