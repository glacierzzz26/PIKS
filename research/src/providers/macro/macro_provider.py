"""宏观指标 Provider（P9-5 / issue #13，宏观研报主体）

主体是**宏观维度**（`macro:cn_cpi` 等），而非个股或行业。一维度一 run（D-M1）。

为什么用这批接口（Spike 实测，2026-09-17，akshare 1.18.94）
--------------------------------------------------------
issue #13 建议的 `macro_china_cpi_yearly` / `_m2_yearly` / `_gdp_yearly`
**不可用**：其实测末个真实值停在 2025-07/08（今天 2026-09），陈旧一年；且其
`日期` 列是**发布日期**而非统计期，与 `今值` 的统计期错位一整个月；末行常是
`今值=NaN` 的待发布占位行，直接取 `iloc[0]` 取到空值。

故一律改用 NBS 口径（经东方财富数据中心）四接口，实测当期：
  `macro_china_cpi()`          224×13  月频  2008-01 起
  `macro_china_ppi()`          248×4   月频  2006-01 起
  `macro_china_money_supply()` 224×10  月频  2008-01 起
  `macro_china_gdp()`          82×9    季频累计 2006 Q1 起

`_DIMENSIONS` 是本模块**唯一**的接口名来源，且每个名字都被
`TestNoYearlyFallback` 断言不含 `_yearly` —— 结构性杜绝回退（不是在别处
加个 if 判断）。详见 docs/phase9/design/macro-research.md §2.1。

三个口径硬事实（逐条实测，见设计文档 §2.3–§2.5）
----------------------------------------------
1. **CPI/PPI 的「当月」不是百分比**，是「上年同月=100」的指数。逐行校验
   `全国-当月 − 100 == 全国-同比增长`：CPI 偏差 0.0（224 行全精确），
   PPI 偏差 ≤0.1（248 行中 4 行源站四舍五入余数）。故本模块把该列命名为
   `level` 并靠 `MacroRef.level_label` 逐维度说明量纲，**绝不**以 % 呈现。
2. **GDP 是累计口径**（2026 Q1 334192.9 → Q1-2 695704.0 亿元）。绝对值序列
   不得直接当期数画图/排名（每年年初归零 → 假「Q2 崩塌」锯齿）；`同比增长`
   是**累计同比**，必须标注。本模块额外算 `single_quarter_level`
   （同年内 累计(to) − 累计(to−1)，披露语「由累计差分所得，非原始披露值」），
   但**不做**单季同比 —— 那需跨年两跳，且会在同表印出两个数值不同的
   「GDP 同比」。
3. **四个接口全部新→旧排序**。`analysis/industry.py` 的分位写法依赖
   `series[-1]` 是「最新」，沿用源站旧序会拿 2008 年当最新 —— 不报错但完全
   错误。故 `get_series` 一律升序化，并**断言**期号单调递增（断言失败即抛，
   不静默继续）。

发布日：这四个接口只有统计期（`REPORT_DATE`/`TIME`），**没有发布日期**。
带发布日的恰是上面那批陈旧一年的 `*_yearly`。故本模块**不产出** `released_at`，
滞后交由分析层用「报告生成日 − 统计期末」如实表达（可算的事实，非猜测）。
"""
import re
from dataclasses import dataclass, field
from datetime import date
from typing import Callable, Dict, List, Optional, Tuple

# ---------------------------------------------------------------------------
# 期号解析
# ---------------------------------------------------------------------------

# 月频："2026年08月份"（实测全部 224/248 行皆此形，无变体）
_MONTH_RE = re.compile(r"^(\d{4})年(\d{1,2})月份$")
# 季频："2026年第1季度"（单季）或 "2026年第1-2季度"（累计区间）
_QUARTER_RE = re.compile(r"^(\d{4})年第(\d)季度$")
_QUARTER_RANGE_RE = re.compile(r"^(\d{4})年第(\d)-(\d)季度$")


def _month_end(year: int, month: int) -> date:
    """统计期末 = 该月最后一天（不引 calendar 依赖，手算闰年）。"""
    if month == 12:
        return date(year, 12, 31)
    first_of_next = date(year, month + 1, 1)
    return date.fromordinal(first_of_next.toordinal() - 1)


def _quarter_end(year: int, quarter: int) -> date:
    """统计期末 = 该季度最后一天。"""
    return _month_end(year, quarter * 3)


@dataclass(frozen=True)
class PeriodParts:
    """从源站期号解析出的可对账期号（Number Lint 前提，见设计文档 §2.7）。

    ⚠️ 这几个字段**必须进指标卡**：正文一旦印出源站期号（「2026年08月份」），
    `number_lint._mask_text` 的日期掩码**匹配不到**它（实测掩码后原样返回），
    于是 `2026` / `08` 会被当数据数字扫描 —— 卡里没有即 lint 必挂。
    产出这三个字段是「引用值入 known 集、零新增 lint 代码」得以成立的前提。
    """
    label: str                          # 源站原文，如 "2026年08月份"
    year: int
    end: date                           # 统计期末
    month: Optional[int] = None         # 月频
    quarter_from: Optional[int] = None  # 季频（累计区间起点）
    quarter_to: Optional[int] = None    # 季频（结束季度）

    @property
    def cumulative_span(self) -> bool:
        """是否累计区间（如「第1-2季度」）。单季（「第1季度」）为 False。"""
        return (
            self.quarter_from is not None
            and self.quarter_to is not None
            and self.quarter_to > self.quarter_from
        )


def parse_period(label: str) -> PeriodParts:
    """源站期号 → PeriodParts。无法解析即**抛**（不静默当缺失）。

    静默跳过会让该期从序列里无声消失、分位数静默偏移 —— 宁可显式报错。
    """
    s = str(label).strip()
    m = _MONTH_RE.match(s)
    if m:
        y, mo = int(m.group(1)), int(m.group(2))
        return PeriodParts(label=s, year=y, end=_month_end(y, mo), month=mo)
    m = _QUARTER_RANGE_RE.match(s)
    if m:
        y, qf, qt = int(m.group(1)), int(m.group(2)), int(m.group(3))
        return PeriodParts(label=s, year=y, end=_quarter_end(y, qt),
                           quarter_from=qf, quarter_to=qt)
    m = _QUARTER_RE.match(s)
    if m:
        y, q = int(m.group(1)), int(m.group(2))
        return PeriodParts(label=s, year=y, end=_quarter_end(y, q),
                           quarter_from=q, quarter_to=q)
    raise ValueError(f"无法解析源站期号: {label!r}（既非「YYYY年MM月份」也非「YYYY年第N季度」）")


# ---------------------------------------------------------------------------
# 维度表（**唯一真源**，D-M2：知识表归 Python 独占，Go 侧只判形态）
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class MacroSpec:
    """一个宏观维度的取数规格。

    columns : 语义名 → akshare 返回列名。语义名只有 level/yoy/mom 三种，
              量纲差异由 `level_label` 逐维度说明（CPI 是指数、M2 是亿元）。
    """
    key: str                 # "cn_cpi" —— 与 Go 规范主体的 key 部分一致
    name: str                # 展示名（Go 侧读指标卡取名，不建表）
    unit: str                # 主口径单位说明
    cadence: str             # "month" | "quarter"
    ak_name: str             # akshare 函数名（唯一接口名来源）
    columns: Dict[str, str]
    level_label: str         # 水平列的量纲说明（**不是**百分比）
    source: str
    caliber: str             # 上屏的口径说明段


_CN_NBS_SOURCE = "国家统计局（经东方财富数据中心）"

# ⚠️ 这里出现的接口名是全部取数入口。**不得**加入任何 `*_yearly` 接口 ——
# 见模块 docstring「为什么用这批接口」。TestNoYearlyFallback 会断言此点。
_DIMENSIONS: Dict[str, MacroSpec] = {
    "cn_cpi": MacroSpec(
        key="cn_cpi",
        name="居民消费价格指数（CPI）",
        unit="同比 %｜指数（上年同月=100）",
        cadence="month",
        ak_name="macro_china_cpi",
        columns={"level": "全国-当月", "yoy": "全国-同比增长", "mom": "全国-环比增长"},
        level_label="指数（上年同月=100）",
        source=_CN_NBS_SOURCE,
        caliber=(
            "全国口径。同比/环比为百分比；「当月」为**指数（上年同月=100）**，"
            "不是百分比（100.8 表示同比 +0.8%）。"
        ),
    ),
    "cn_ppi": MacroSpec(
        key="cn_ppi",
        name="工业生产者出厂价格指数（PPI）",
        unit="同比 %｜指数（上年同月=100）",
        cadence="month",
        ak_name="macro_china_ppi",
        columns={"level": "当月", "yoy": "当月同比增长", "mom": None},
        level_label="指数（上年同月=100）",
        source=_CN_NBS_SOURCE,
        caliber=(
            "全国口径（出厂价格）。同比为百分比；「当月」为**指数（上年同月=100）**，"
            "不是百分比。⚠️ 源站该接口**不提供环比**，故环比字段为 N/A（不回退重算）。"
        ),
    ),
    "cn_m2": MacroSpec(
        key="cn_m2",
        name="货币供应量（M2）",
        unit="亿元｜同比 %｜环比 %",
        cadence="month",
        ak_name="macro_china_money_supply",
        columns={
            "level": "货币和准货币(M2)-数量(亿元)",
            "yoy": "货币和准货币(M2)-同比增长",
            "mom": "货币和准货币(M2)-环比增长",
        },
        level_label="亿元",
        source=_CN_NBS_SOURCE,
        caliber=(
            "M2（货币和准货币）口径。水平为**亿元**（原始值，展示可折万亿元）；"
            "同比/环比为百分比。同一接口另有 M1/M0，本报告只取 M2。"
        ),
    ),
    "cn_gdp": MacroSpec(
        key="cn_gdp",
        name="国内生产总值（GDP）",
        unit="亿元（累计）｜累计同比 %",
        cadence="quarter",
        ak_name="macro_china_gdp",
        columns={
            "level": "国内生产总值-绝对值",
            "yoy": "国内生产总值-同比增长",
            "mom": None,
        },
        level_label="亿元（累计）",
        source=_CN_NBS_SOURCE,
        caliber=(
            "⚠️ **累计口径**：绝对值是「年初至本季累计」，非单季值；同比是**累计同比**。"
            "本报告的「单季水平」由同年内累计差分所得，**非原始披露值**。"
            "本报告不提供单季同比（需跨年两跳，且会与累计同比并列成两个不同数值）。"
        ),
    ),
}


@dataclass(frozen=True)
class MacroRef:
    """宏观维度定位结果（**查表得来**，不靠 key 字符串推断）。"""
    key: str
    name: str
    unit: str
    cadence: str
    source: str
    caliber: str
    level_label: str


@dataclass
class MacroPoint:
    """一期宏观读数。

    缺失一律 None（不补 0）—— 「缺失」是不知道，「0」是为零，二者不可混同。
    """
    period: PeriodParts
    level: Optional[float]
    yoy: Optional[float]
    mom: Optional[float] = None
    # GDP 专用：由累计差分出的单季水平（同年内）。非季频或无法差分时为 None。
    single_quarter_level: Optional[float] = None
    # 该行是否为累计区间（GDP 的「第1-2季度」为 True，「第1季度」为 False）
    cumulative: bool = False
    total_yoy: Optional[float] = None   # GDP：各产业合计同比（= 全国 GDP 累计同比）


@dataclass
class MacroSeries:
    """一个维度的完整序列（升序）+ 定位信息。"""
    ref: MacroRef
    points: List[MacroPoint] = field(default_factory=list)

    @property
    def latest(self) -> Optional[MacroPoint]:
        return self.points[-1] if self.points else None

    @property
    def history_start(self) -> Optional[date]:
        return self.points[0].period.end if self.points else None

    @property
    def history_periods(self) -> int:
        return len(self.points)


def _opt_float(value) -> Optional[float]:
    """源站的 NaN / 空串 / '-' 一律 None —— 不补 0。"""
    if value is None:
        return None
    try:
        f = float(value)
    except (ValueError, TypeError):
        return None
    if f != f:      # NaN
        return None
    return f


def _with_retry(fn: Callable, retries: int = 3, delay: float = 0.8):
    """akshare 页面解析偶发失败，重试兜底（与 sw_index_provider 同惯例）。"""
    import time
    last: Optional[Exception] = None
    for i in range(retries):
        try:
            return fn()
        except Exception as e:      # noqa: BLE001 —— 逐次重试，最后抛出原异常
            last = e
            time.sleep(delay * (i + 1))
    raise last if last else RuntimeError("retry 未执行")


class MacroProvider:
    """宏观指标 Provider（主体 = 宏观维度，非个股/行业）。"""

    @property
    def name(self) -> str:
        return "macro"

    # ---------- 维度定位 ----------

    def resolve(self, key: str) -> Optional[MacroRef]:
        """key → 维度定位。**未知 key 返回 None，绝不臆造**。

        与 `sw_provider`「无法分类时返回 None（不脑补）」同原则。调用方
        （`engine._collect_macro`）拿到 None 即抛错、run 如实 failed —— 不降级
        出一份没有数字的空壳报告。
        """
        spec = _DIMENSIONS.get(str(key).strip().lower())
        if spec is None:
            return None
        return MacroRef(
            key=spec.key, name=spec.name, unit=spec.unit, cadence=spec.cadence,
            source=spec.source, caliber=spec.caliber, level_label=spec.level_label,
        )

    def known_keys(self) -> List[str]:
        """已支持的维度 key（供错误信息与测试用）。"""
        return sorted(_DIMENSIONS.keys())

    # ---------- 序列 ----------

    def get_series(self, ref: MacroRef) -> MacroSeries:
        """取一个维度的完整历史序列，**升序**返回。

        采不到即抛（非 optional）：序列就是宏观报告的本体，采不到就该如实失败，
        不像个股的「同业对比」那样可降级。
        """
        spec = _DIMENSIONS.get(ref.key)
        if spec is None:
            raise ValueError(f"未知宏观维度 {ref.key}（不臆造）")

        import akshare as ak
        df = _with_retry(lambda: getattr(ak, spec.ak_name)())
        if df is None or df.empty:
            raise ValueError(f"宏观维度 {ref.key}（{spec.name}）无数据")

        label_col = "季度" if spec.cadence == "quarter" else "月份"
        if label_col not in df.columns:
            raise ValueError(
                f"接口 {spec.ak_name} 缺期号列 {label_col!r}（列={list(df.columns)}）"
                "—— 源站结构调整，需重新 Spike 后再改本模块"
            )

        points: List[MacroPoint] = []
        for _, row in df.iterrows():
            period = parse_period(row[label_col])
            points.append(MacroPoint(
                period=period,
                level=_opt_float(row.get(spec.columns["level"])),
                yoy=_opt_float(row.get(spec.columns["yoy"])),
                mom=(_opt_float(row.get(spec.columns["mom"]))
                     if spec.columns.get("mom") else None),
                cumulative=(spec.cadence == "quarter"),
                total_yoy=_opt_float(row.get(spec.columns["yoy"])),
            ))

        # 升序化。⚠️ 源站是**新→旧**（实测四个接口皆然）——不排会拿 2008 年当「最新」，
        # 分位数会静默算成「2008 年水平的历史分位」：不报错，但完全错误。
        # 排序键用**期末**而非 (年, 起始季度)：后者会让 GDP 的「第1季度」与
        # 「第1-2季度」撞键（起始季都是 1），产生未定义次序。
        points.sort(key=lambda p: (p.period.year, p.period.end))

        self._assert_monotonic(ref, points)
        if spec.cadence == "quarter":
            self._fill_single_quarter(ref, points)
        return MacroSeries(ref=ref, points=points)

    @staticmethod
    def _assert_monotonic(ref: MacroRef, points: List[MacroPoint]) -> None:
        """断言期号严格递增。源站若改排序或出现重复期，此处即抛 —— 不静默继续。"""
        for prev, cur in zip(points, points[1:]):
            if cur.period.end <= prev.period.end:
                raise ValueError(
                    f"宏观维度 {ref.key} 期号非严格递增: "
                    f"{prev.period.label} → {cur.period.label}"
                    "（源站排序或去重语义已变，分位数不可信）"
                )

    @staticmethod
    def _fill_single_quarter(ref: MacroRef, points: List[MacroPoint]) -> None:
        """GDP 累计 → 单季水平差分（同年内）。

        规则：只有**累计区间**行参与差分（「第1季度」本身即单季累计，直接等于单季），
        且只在**同一年内**回看 —— 跨年回看会把上一年 Q4 累计混进来。

        实测校验（2026 年）：Q1 累计 334192.9 → 单季 334192.9；
        Q1-2 累计 695704.0 → 单季 695704.0 − 334192.9 = 361511.1（量级合理，无负值）。

        差分只在绝对值上做。**不做**单季同比 —— 那需跨年两跳，且会在同一张表里
        印出第二个数值不同的「GDP 同比」，读者必混。
        """
        by_year: Dict[int, List[MacroPoint]] = {}
        for p in points:
            by_year.setdefault(p.period.year, []).append(p)
        for year, group in by_year.items():
            # 组内已按期号升序（全序列有序 ⇒ 子序列有序）
            start_of_year: Optional[float] = None
            for p in group:
                qf, qt = p.period.quarter_from, p.period.quarter_to
                if p.level is None or qf is None or qt is None:
                    continue
                if qf == qt:
                    # 「第1季度」= 年初至 Q1，本身即单季
                    p.single_quarter_level = p.level
                    start_of_year = p.level
                elif qf == 1 and start_of_year is not None and qt >= 2:
                    # 「第1-2季度」等：减去年初至今(前一段)的累计
                    prev = next(
                        (g for g in group
                         if g.period.quarter_to == qt - 1 and g.period.quarter_from == 1),
                        None,
                    )
                    if prev is not None and prev.level is not None:
                        p.single_quarter_level = p.level - prev.level
