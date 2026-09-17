"""宏观分析引擎（P9-5 / issue #13）

输入是**宏观维度的时间序列**（`providers/macro` 的产物），输出确定性指标卡。
与个股/行业分析的关键差异：

1. **无 OHLC、无量** —— 宏观序列只有「一期一个读数」，故本模块**不**算技术形态、
   均线、量价共振（那些字段结构上不存在，见 MacroMetrics）
2. **无年化波动率** —— 月频统计量年化是双重计数（CPI 同比本身已是变化率）。
   沿用行业做法：**不产出该字段**，而非产出后再靠 lint 拦
3. **不做任何预测** —— 无未来值、无目标位、无政策预判
4. **不跨维度** —— 一份报告一个维度，故无因果/领先滞后/相关系数

Number Lint 口径：本模块产出的每个数字都会进 metrics.json，故均可溯源。
未算出的字段一律 None（不补 0、"N/A" 由渲染层负责）。

`as_of` 语义（D-M3）：`MacroPeriod.as_of` 是**报告生成日**（与全站一致）；
**数据边界**另由 `period_end` / `source_lag_days` 表达 —— 二者刻意分开，
因为宏观数据的统计期总落后于生成日，混成一个字段会让人误读「数据到今天」。
⚠️ **不写 `released_at`**：数据源只有统计期，没有发布日期（见 provider docstring），
写发布日期即编造。
"""
from dataclasses import dataclass, field
from datetime import date, timedelta
from typing import List, Optional

import numpy as np

from ..providers.macro.macro_provider import MacroPoint, MacroRef, MacroSeries


@dataclass
class MacroPeriod:
    """数据边界（D-M3）。与 `as_of`（报告生成日）刻意分开。

    期号字段（year/month/quarter_*）是 **int**，不是 str —— 正文印出源站期号
    （「2026年08月份」）时，`number_lint._mask_text` 的日期掩码**匹配不到**它，
    于是 `2026`/`08` 会被当数据数字扫描。转成 int 才能被
    `collect_numbers_from_json` 的递归收集命中（str 会被跳过）。
    这是「引用值入 known、零新增 lint 代码」得以成立的前提（设计文档 §2.7）。
    """
    label: str                        # 源站原文，如 "2026年08月份"
    period_end: date                  # 统计期末（不是发布日）
    period_year: int
    period_month: Optional[int] = None
    period_quarter_from: Optional[int] = None
    period_quarter_to: Optional[int] = None
    cumulative: bool = False          # 季频累计区间（如「第1-2季度」）
    source_lag_days: int = 0          # 报告生成日 − 统计期末（可算的事实）
    history_start: Optional[date] = None
    history_periods: int = 0
    window_periods: int = 0           # 展示窗口期数


@dataclass
class MacroReading:
    """一期读数（用于序列展示）。缺失一律 None。

    期号字段（year/month/quarter_*）是 **int** 且**必须随读数一起下传**：正文
    表格会逐行印源站期号（「2026年08月份」/「2025年第1-4季度」），而
    `number_lint._mask_text` 的日期掩码**匹配不到**这两种形态 —— 其中的
    2026/08/1/4 都会被当数据数字扫描。只有把它们作为 int 放进指标卡，才能被
    `collect_numbers_from_json` 命中（str 会被跳过）。
    """
    period_label: str
    period_year: int
    level: Optional[float] = None
    yoy: Optional[float] = None
    mom: Optional[float] = None
    single_quarter_level: Optional[float] = None
    period_month: Optional[int] = None
    period_quarter_from: Optional[int] = None
    period_quarter_to: Optional[int] = None


@dataclass
class MacroMetrics:
    """宏观研报的完整指标卡（= Fact 唯一源）。

    ⚠️ **结构性禁止清单**（设计文档 §4.2）：本数据类**没有** `volatility_annual`、
    没有 OHLC/量/换手/涨跌停字段、没有预测字段、没有跨维度字段。不支持的
    口径靠「不产出该字段」杜绝 —— 不是在别处加 if 判断，也不是靠 lint 事后拦。
    """
    ref: MacroRef
    as_of: date

    # ---- 最新读数（fact：源站已发布值，不由序列自行重算） ----
    latest_level: Optional[float]
    latest_yoy: Optional[float]
    latest_mom: Optional[float]
    # GDP 专用：由累计差分出的单季水平（披露语「非原始披露值」）。
    latest_single_quarter_level: Optional[float] = None

    # ---- 上期与变化（calc） ----
    prev_label: Optional[str] = None
    prev_level: Optional[float] = None
    prev_yoy: Optional[float] = None
    delta_level: Optional[float] = None       # 最新水平 − 上期水平
    delta_yoy: Optional[float] = None         # 最新同比 − 上期同比（pct）

    # ---- 历史定位（calc）：水平分位与同比分位**分别命名**，混用即误导 ----
    percentile_level: Optional[float] = None  # 最新水平在全历史中的分位（0~100）
    percentile_yoy: Optional[float] = None    # 最新同比在全历史中的分位

    # ---- 近 N 期窗口统计（calc）。不做均值（承行业「中位抗极值」先例） ----
    window_median_level: Optional[float] = None
    window_min_level: Optional[float] = None
    window_max_level: Optional[float] = None

    # ---- 状态描述（calc，非预测）：同比连续同向期数 ----
    direction_run: int = 0                    # >0 连续回升，<0 连续回落
    direction_from_label: Optional[str] = None

    # ---- 序列窗口（fact，供正文表格） ----
    readings: List[MacroReading] = field(default_factory=list)

    period: Optional[MacroPeriod] = None


def _percentile(series: List[float], value: float) -> Optional[float]:
    """`value` 在 `series` 中的分位（0~100）。

    与 `analysis/industry.py` 的 `point_percentile` 同口径：用「≤ 本值的个数」
    作分子，并列不虚增。分母是**全部可用历史**（非展示窗口）。
    ⚠️ 依赖序列升序 —— provider 已保证（并断言单调），此处不再重排。
    """
    if not series:
        return None
    arr = np.array(series, dtype=float)
    return round(float((arr <= value).sum() / len(arr) * 100), 1)


def _direction_run(yoys: List[float]) -> int:
    """同比连续同向期数。>0 = 连续回升，<0 = 连续回落，0 = 不足两期或持平。

    纯**状态描述**（「同比已连续 3 期回落」是可从序列直接数出的事实），
    不含任何外推。不足两期返回 0 —— 不猜方向。
    """
    if len(yoys) < 2:
        return 0
    diffs = np.diff(np.array(yoys, dtype=float))
    sign = 0 if diffs[-1] == 0 else (1 if diffs[-1] > 0 else -1)
    if sign == 0:
        return 0
    run = 0
    for d in reversed(diffs):
        s = 0 if d == 0 else (1 if d > 0 else -1)
        if s != sign:
            break
        run += 1
    return sign * run


def analyze_macro(
    ref: MacroRef,
    series: MacroSeries,
    as_of: date,
    window_periods: int = 36,
) -> MacroMetrics:
    """算宏观指标卡。

    series         : **全历史**序列（升序，provider 保证）—— 分位数基于全历史
    window_periods : 展示窗口期数（正文表格 + 窗口统计）；分位数**不受其影响**
    """
    points: List[MacroPoint] = list(series.points)
    if not points:
        raise ValueError(f"宏观维度 {ref.key}（{ref.name}）序列为空，无法计算指标")

    # 展示窗口是**尾部**切片（序列升序 ⇒ 尾部即最近）
    window = points[-window_periods:] if len(points) > window_periods else points
    latest, prev = points[-1], (points[-2] if len(points) >= 2 else None)

    # 分位数基于全历史。水平序列与同比序列**分开**算 —— 二者刻度不同，
    # 混用会得出「水平处于 88% 分位」这类被同比误导的结论。
    levels_all = [p.level for p in points if p.level is not None]
    yoys_all = [p.yoy for p in points if p.yoy is not None]

    pct_level = _percentile(levels_all, latest.level) if latest.level is not None else None
    pct_yoy = _percentile(yoys_all, latest.yoy) if latest.yoy is not None else None

    win_levels = [p.level for p in window if p.level is not None]

    yoys_window = [p.yoy for p in points if p.yoy is not None]

    period = MacroPeriod(
        label=latest.period.label,
        period_end=latest.period.end,
        period_year=latest.period.year,
        period_month=latest.period.month,
        period_quarter_from=latest.period.quarter_from,
        period_quarter_to=latest.period.quarter_to,
        cumulative=latest.cumulative,
        # 报告生成日 − 统计期末。为负（生成日早于期末）时取 0 —— 那是本地时钟
        # 与源站不一致，不是「数据来自未来」，取 0 比印负数诚实。
        source_lag_days=max(0, (as_of - latest.period.end).days),
        history_start=series.history_start,
        history_periods=series.history_periods,
        window_periods=len(window),
    )

    # 累计序列**一律不出 `delta_level`**：
    #   - 同年内：「本期累计 − 上期累计」**恒等于** `single_quarter_level`，是重复
    #     呈现（GDP Q1-2 − Q1 = 695704.0 − 334192.9 = 单季 361511.1）。该增量已有
    #     专属字段 + 披露语（§2.4c 只允许以「单季水平」这一种形式面世）。
    #   - 跨年：「2026 Q1 − 2025 Q1-4」是**两个不同跨度累计总量之差**，不是任何
    #     真实变化（实测必为负）—— 属 §4.2「数据源不支持 = 不产出字段」。
    # 一条规则覆盖两种情形，胜过「产出了再在渲染层藏着」。
    is_cumulative = bool(latest.cumulative)
    # 同比跨年同样不可比（累计同比在年边界处跨度不同）；非累计口径跨年无碍
    # （CPI 同比 0.8% 与十年前的 0.8% 是同一件事）。
    delta_yoy_ok = (
        prev is not None
        and not (is_cumulative and prev.period.year != latest.period.year)
    )

    return MacroMetrics(
        ref=ref,
        as_of=as_of,
        latest_level=latest.level,
        latest_yoy=latest.yoy,
        latest_mom=latest.mom,
        latest_single_quarter_level=latest.single_quarter_level,
        # 累计口径一律不出 delta_level（见上方 is_cumulative 说明）：增量只经
        # `latest_single_quarter_level` 一种形式面世。prev_* 照常保留 —— 它们本身
        # 是可对账的事实，只是不与之作差。
        prev_label=(prev.period.label if prev else None),
        prev_level=(prev.level if prev else None),
        prev_yoy=(prev.yoy if prev else None),
        delta_level=(
            round(latest.level - prev.level, 4)
            if not is_cumulative and latest.level is not None
            and prev is not None and prev.level is not None
            else None
        ),
        delta_yoy=(
            round(latest.yoy - prev.yoy, 4)
            if delta_yoy_ok and latest.yoy is not None and prev.yoy is not None
            else None
        ),
        percentile_level=pct_level,
        percentile_yoy=pct_yoy,
        window_median_level=(round(float(np.median(win_levels)), 4) if win_levels else None),
        window_min_level=(round(float(min(win_levels)), 4) if win_levels else None),
        window_max_level=(round(float(max(win_levels)), 4) if win_levels else None),
        direction_run=_direction_run(yoys_window),
        direction_from_label=(points[0].period.label if len(points) >= 2 else None),
        readings=[
            MacroReading(
                period_label=p.period.label,
                period_year=p.period.year,
                period_month=p.period.month,
                period_quarter_from=p.period.quarter_from,
                period_quarter_to=p.period.quarter_to,
                level=p.level, yoy=p.yoy, mom=p.mom,
                single_quarter_level=p.single_quarter_level,
            )
            for p in window
        ],
        period=period,
    )


@dataclass
class MacroRiskItem:
    """宏观风险项。与行业风险项同构，但**不臆造**不适用类别。"""
    category: str
    level: str          # low / medium / high
    evidence: str
    description: str


@dataclass
class MacroRiskMetrics:
    symbol: str
    as_of: date
    overall_level: str
    items: List[MacroRiskItem] = field(default_factory=list)


def analyze_macro_risk(m: MacroMetrics, as_of: date) -> MacroRiskMetrics:
    """基于规则的宏观风险。只标记**可从本指标卡溯源**的风险，不做预测。

    ⚠️ 这是本报告封面结论 chip 的**唯一来源** —— 宏观无评分卡（D-M5），
    前端取 `scorecard.overall_label ?? risk.overall_level`。故本函数必须
    总给出一个 `overall_level`（无风险项时即 "low"）。
    """
    items: List[MacroRiskItem] = []

    # 1. **水平**的历史极端位置 —— **仅对水平分位有信息量的维度启用**。
    #
    #    实测发现必须逐维度区分（不是所有宏观指标都适用）：
    #    - CPI/PPI 水平是「上年同月=100」的指数，长期在 100 附近震荡 ⇒ 分位是
    #      真实的位置信息（实测 CPI 25.4%，有意义）
    #    - M2/GDP 是**名义总量**，随经济增长长期单调上行 ⇒ 分位恒为 ~100%
    #      （实测 M2 = 100.0%）—— 报「处于历史 100% 分位」是噪声，且会诱导出
    #      「均值回归」这类对货币供应量/经济体量**不成立**的推断。
    #
    #    故此处按 `ref.level_percentile_meaningful` 分派（知识表在 provider，
    #    指标卡里 `percentile_level` 照常产出 —— 它是事实，只是不拿它当信号）。
    if m.ref.level_percentile_meaningful and m.percentile_level is not None:
        if m.percentile_level >= 90:
            items.append(MacroRiskItem(
                "宏观位置", "medium",
                f"最新水平处于全历史 {m.percentile_level:.1f}% 分位"
                f"（{m.period.history_periods} 期）",
                "读数位于历史区间上沿，需关注均值回归压力"))
        elif m.percentile_level <= 10:
            items.append(MacroRiskItem(
                "宏观位置", "medium",
                f"最新水平处于全历史 {m.percentile_level:.1f}% 分位"
                f"（{m.period.history_periods} 期）",
                "读数位于历史区间下沿，需关注其持续性与方向"))

    # 2. 同比的分位。同比是可平稳比较的读数（CPI 同比 0.8% 与十年前的 0.8%
    #    是同一件事），故**所有**维度都适用 —— 名义总量维度上这是唯一的定位依据。
    if m.percentile_yoy is not None:
        if m.percentile_yoy >= 90 and not m.ref.level_percentile_meaningful:
            items.append(MacroRiskItem(
                "同比位置", "medium",
                f"最新同比处于全历史 {m.percentile_yoy:.1f}% 分位",
                "同比读数位于历史区间上沿，需关注其持续性"))
        elif m.percentile_yoy <= 10:
            items.append(MacroRiskItem(
                "同比位置", "medium",
                f"最新同比处于全历史 {m.percentile_yoy:.1f}% 分位",
                "同比读数接近历史低位，需关注通缩/收缩压力"))

    # 3. 连续同向：状态描述，非预测。
    if m.direction_run <= -3:
        items.append(MacroRiskItem(
            "趋势", "medium",
            f"同比已连续 {abs(m.direction_run)} 期回落",
            "同比读数连续回落，趋势偏弱"))
    elif m.direction_run >= 3:
        items.append(MacroRiskItem(
            "趋势", "low",
            f"同比已连续 {m.direction_run} 期回升",
            "同比读数连续回升，趋势偏强"))

    if any(i.level == "high" for i in items):
        overall = "high"
    elif any(i.level == "medium" for i in items):
        overall = "medium"
    else:
        overall = "low"

    return MacroRiskMetrics(symbol=m.ref.key, as_of=as_of,
                            overall_level=overall, items=items)


# --- 供渲染层复用的披露语句（保持正文与口径说明同源） ---

GDP_CUMULATIVE_NOTE = (
    "单季水平由同年内累计差分所得，**非原始披露值**；"
    "本报告不提供单季同比（需跨年两跳，且会与累计同比并列成两个不同数值）。"
)

LEVEL_IS_INDEX_NOTE = (
    "「当月」列为**指数（上年同月=100）**，不是百分比；"
    "同比/环比为百分比。两者量纲不同，不可混读。"
)
