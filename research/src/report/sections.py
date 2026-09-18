"""报告章节清单 —— 版面装配与 TOC/三域标记的**单一真源**(P9 #12 / D-R8)。

为什么要有这个模块
------------------
改造前,章节顺序散落在 `markdown.py` 的 if 链里,章节序号由 `add_section` 按出现
顺序自增;前端 TOC 只能靠解析 markdown 的 `## ` 标题 —— 而 `MarkdownBody` 是裸
`react-markdown`(无 heading id、无锚点),解析不可靠且必然与正文错位。
本模块把「有哪些章、什么顺序、属于哪个域」收敛成一份声明,由它同时驱动:

  1. `markdown.py` 的装配顺序(正文)
  2. `json_report.py` 的 `metrics.meta.section_manifest`(前端 TOC + 三域标签)

两者同源 ⇒ 章节增删不会再错位(design report-layout.md §1.1 问题 1)。

三域(D-R5)
----------
  fact    数据:可溯源到当前指标卡的硬数据
  calc    计算:程序确定性算出的派生量(分位/回撤/排名/中位)
  opinion 研判:AI 生成的定性叙述

⚠️ 域是**声明**,不是从标题正文猜的(§4.3)—— 前端不得自行推断。
新增章节时在此登记,并在 `markdown.py` 的 builder 表里补同名 key 的渲染函数。
"""
from dataclasses import dataclass
from typing import List, Sequence, Tuple

# 域 key → 前端 .domain-tag 的中文文案(§4.3;颜色复用既有 .st 色板,不新增颜色)
DOMAIN_LABEL = {"fact": "数据", "calc": "计算", "opinion": "研判"}

# 首章与末章恒定(D-R6 摘要前置 / 免责置尾),不参与 triggers 判定。
EXEC_SUMMARY_TITLE = "执行摘要"
DISCLAIMER_TITLE = "数据说明与免责声明"

# 首章容器:AI 三段的槽位占位符。`render_synthesis` 在 cli.py 的 synthesize 步
# 把它替换成三个 `### ` 子槽(执行摘要/趋势解读/综合结论)。
AI_SYNTHESIS_SLOT = "{{ai_synthesis}}"


@dataclass(frozen=True)
class Chapter:
    """一个数据章节的声明。

    key      : 稳定标识(不展示);`markdown.py` 的 builder 表按它取渲染函数
    title    : 正文与 TOC **共用**的章节标题(必须与 builder 产出的 `## ` 标题一致)
    domains  : 三域标记,顺序即展示顺序
    triggers : `profile.sections` 中任一命中即渲染本章
    """

    key: str
    title: str
    domains: Tuple[str, ...]
    triggers: Tuple[str, ...]


# 章节顺序 = 正文渲染顺序 = TOC 顺序。与改造前 markdown.py 的 if 链逐一对应
# (标题字符串逐字节保留,故个股报告既有的章节文案零变化)。
CHAPTERS: Tuple[Chapter, ...] = (
    # 触发只看 `market`(P9-4,issue #11):原先还含 `company`,但 `company` 节
    # 本身不渲染任何章节(它只是机检的 meta 键)—— 这个耦合会让**任何含 company
    # 的 profile 凭空多出一个量价章**。公司研报(只财务、不采行情)正踩此坑。
    # 零回归:现有所有含 company 的 profile(complete-stock/short-term/prebuy)
    # 同时含 market,故 market 触发照常命中。
    Chapter("price", "股价表现", ("fact", "calc"), ("market",)),
    Chapter("volume", "成交量与换手率", ("fact", "calc"), ("volume", "turnover")),
    # 量价形态(P7,规则判定)。master 线在 volume 之后、financial 之前渲染;
    # 域为 数据+计算 —— 标签与共振结论皆由确定性规则算出,无 AI 叙述。
    Chapter("patterns", "量价形态", ("fact", "calc"), ("patterns",)),
    Chapter("financial", "基本面分析", ("fact", "calc"), ("financial", "valuation")),
    Chapter("events", "近期事件与新闻", ("fact",), ("events", "announcements")),
    # 个股主体的「同业横比」章(与下面的行业**本体**三章互斥:profile 决定谁出现)
    Chapter("industry", "行业对比", ("fact", "calc"), ("industry",)),
    Chapter("industry_index", "行业行情", ("fact", "calc"), ("industry_index",)),
    Chapter("industry_valuation", "估值定位", ("fact",), ("industry_valuation",)),
    Chapter("industry_structure", "成分结构", ("fact", "calc"), ("industry_structure",)),
    # 宏观维度主体(P9-5 / #13):两节同源于 macro 指标卡。域为 数据+计算 ——
    # 读数/分位/窗口统计/连续同向期数**全部**由确定性规则算出,无 AI 叙述
    # (AI 定性统一收在首章「执行摘要」)。
    Chapter("macro_level", "宏观指标读数", ("fact", "calc"), ("macro_level",)),
    Chapter("macro_position", "历史定位与趋势", ("fact", "calc"), ("macro_position",)),
    Chapter("capital", "资金面分析（龙虎榜）", ("fact",), ("capital",)),
    # 风险章是**确定性规则**算出来的(等级 + 逐条依据),不是 AI 叙述 ——
    # 故域为 数据+计算。design §4.4 的示意表把它标成「研判」,那是 AI 三段并入
    # 首章(见下方 user 决策)之前的旧口径:AI 定性现已全部收在第一章「执行摘要」,
    # 本章不再含任何 opinion 内容,再标「研判」即为倒挂。
    Chapter("risk", "风险分析", ("fact", "calc"), ("risk",)),
    Chapter("conclusion", "综合评分卡", ("calc",), ("conclusion",)),
)

# 未显式传 sections 时的兜底(与 markdown.py 的历史默认一致,兼容旧调用)。
DEFAULT_SECTIONS: Tuple[str, ...] = (
    "market", "volume", "turnover", "financial", "valuation",
    "events", "announcements", "capital", "risk", "conclusion",
)


def active_chapters(sections: Sequence[str]) -> List[Chapter]:
    """按声明顺序返回本次实际渲染的数据章节(首章/末章不在此列)。"""
    return [c for c in CHAPTERS if any(t in sections for t in c.triggers)]


def build_manifest(sections: Sequence[str]) -> List[dict]:
    """产出 `metrics.meta.section_manifest`(前端 TOC + 三域标签的数据源,§4.8)。

    结构:[{"title": <章节标题>, "domains": [<域>...]}, ...]
    首章恒为「执行摘要」(AI 三段容器),末章恒为「数据说明与免责声明」,
    中间章节由 profile 的 sections 决定 —— 与 `markdown.py` 渲染的 `## ` 标题
    一一对应、次序一致。
    """
    manifest = [{"title": EXEC_SUMMARY_TITLE, "domains": ["opinion"]}]
    manifest += [{"title": c.title, "domains": list(c.domains)} for c in active_chapters(sections)]
    manifest.append({"title": DISCLAIMER_TITLE, "domains": []})
    return manifest
