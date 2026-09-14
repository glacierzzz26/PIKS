"""量价形态分析引擎（规则判定，可解释）

输入逐日 K 线（前复权、升序），输出：
- series：最近 60 个交易日的 {date, close, turnover, volume} —— 双轴图数据源，
  也是逐日序列首次落库的载体（随 metrics 进 research_runs.metrics JSONB，零 schema）。
- labels：规则判定的量价形态标签（地量启动 / 换手峰值见顶 / 高位放量下跌 /
  价跌量未缩 / 缩量续跌 / 放量上行 …），每条必带 evidence（日期 + 数值，可核对）。
- divergence：换手率峰值与股价高点的对齐关系（背离辅助事实）。

⚠️ 诚实边界（前端据此分区渲染）：
- 换手率仅**流通口径**（akshare/腾讯 stock_zh_a_hist_tx），拿不到同花顺自由流通口径；
- 形态是**规则判定（Inference）**，不是事实（Fact）——每条带 evidence，与数字区分区；
- 阈值随窗口**分位自适应**，不硬编码绝对值；窗口不足 MIN_BARS 不出标签。
"""
from dataclasses import dataclass, field
from datetime import date
from typing import List, Optional

import numpy as np

from ..models import Bar

# 窗口参数
SERIES_DAYS = 60      # 落库/绘图序列长度
MIN_BARS = 20         # 低于此不出形态标签（统计不显著）
RECENT = 3            # 「近期」= 最近 N 个交易日
PEAK_GAP = 2          # 换手峰值与股价高点视为「共振」的最大相隔交易日数


@dataclass
class PatternMetrics:
    """量价形态指标卡"""
    symbol: str
    as_of: date
    window_days: int
    series: List[dict] = field(default_factory=list)
    labels: List[dict] = field(default_factory=list)
    divergence: dict = field(default_factory=dict)
    note: str = ""


@dataclass
class _Win:
    """窗口聚合（供各规则函数复用，避免重复计算）"""
    dates: List[str]
    closes: List[float]
    turnovers: List[float]
    n: int
    median_t: float
    high_t: float           # 高换手阈值（分位自适应）
    low_t: float            # 低换手阈值
    peak_idx: int           # 换手率峰值位置
    high_idx: int           # 收盘价高点位置
    last_close: float
    high_close: float


def analyze_patterns(bars: List[Bar], as_of: date) -> PatternMetrics:
    """计算量价形态。bars 必须日期升序、前复权。"""
    if not bars:
        raise ValueError("bars 不能为空")

    sym = bars[0].symbol
    win_bars = bars[-SERIES_DAYS:]
    series = [
        {
            "date": b.date.isoformat(),
            "close": round(float(b.close), 2),
            "turnover": round(float(b.turnover), 2),
            "volume": int(b.volume),
        }
        for b in win_bars
    ]

    if len(win_bars) < MIN_BARS:
        return PatternMetrics(
            symbol=sym, as_of=as_of, window_days=len(win_bars), series=series,
            labels=[], divergence={},
            note=f"窗口仅 {len(win_bars)} 个交易日（不足 {MIN_BARS}），样本不足以判定形态，如实不出结论。",
        )

    w = _build_win(win_bars)
    labels: List[dict] = []
    for rule in (_rule_dizu_qidong, _rule_peak_coincide, _rule_recent):
        hit = rule(w)
        if hit:
            labels.append(hit)

    return PatternMetrics(
        symbol=sym, as_of=as_of, window_days=w.n, series=series,
        labels=labels, divergence=_divergence(w),
    )


def _build_win(bars: List[Bar]) -> _Win:
    dates = [b.date.isoformat() for b in bars]
    closes = [float(b.close) for b in bars]
    turnovers = [float(b.turnover) for b in bars]
    n = len(bars)
    median_t = float(np.median(turnovers))
    p20 = float(np.percentile(turnovers, 20))
    p80 = float(np.percentile(turnovers, 80))
    # 阈值同时受分位与中位数约束，避免极端窗口下阈值失真
    return _Win(
        dates=dates, closes=closes, turnovers=turnovers, n=n,
        median_t=median_t,
        high_t=max(p80, median_t * 1.5),
        low_t=min(p20, median_t * 0.6),
        peak_idx=int(np.argmax(turnovers)),
        high_idx=int(np.argmax(closes)),
        last_close=closes[-1],
        high_close=max(closes),
    )


def _rule_dizu_qidong(w: _Win) -> Optional[dict]:
    """地量启动：窗口内换手低点位于前段，此后股价出现显著上行（到其后的区间高点）。"""
    trough_idx = int(np.argmin(w.turnovers))
    if trough_idx > w.n * 0.6 or trough_idx > w.n - 5:
        return None  # 低点必须在窗口前段，且后面还有行情
    if w.turnovers[trough_idx] > w.low_t:
        return None
    after_high = max(w.closes[trough_idx:])
    rise = (after_high - w.closes[trough_idx]) / w.closes[trough_idx] * 100
    if rise < 10:
        return None
    return {
        "code": "dizu_qidong",
        "label": "地量启动",
        "date": w.dates[trough_idx],
        "evidence": (
            f"地量日 {w.dates[trough_idx]} 换手率 {w.turnovers[trough_idx]:.2f}%"
            f"（区间低点），此后股价自 {w.closes[trough_idx]:.2f} 元"
            f"升至区间高点 {after_high:.2f} 元（{rise:+.1f}%）"
        ),
    }


def _rule_peak_coincide(w: _Win) -> Optional[dict]:
    """换手峰值见顶：换手率峰值与股价高点在数个交易日内共振。"""
    gap = abs(w.peak_idx - w.high_idx)
    if gap > PEAK_GAP or w.turnovers[w.peak_idx] < w.high_t:
        return None
    fall = (w.last_close - w.closes[w.peak_idx]) / w.closes[w.peak_idx] * 100
    tail = (
        f"，其后回落至 {w.last_close:.2f} 元（{fall:+.1f}%）——高位放量换手，抛压迹象"
        if fall < -2 else ""
    )
    return {
        "code": "peak_coincide",
        "label": "换手峰值见顶",
        "date": w.dates[w.peak_idx],
        "evidence": (
            f"换手率峰值 {w.turnovers[w.peak_idx]:.2f}%（{w.dates[w.peak_idx]}）"
            f"与股价高点 {w.high_close:.2f} 元（{w.dates[w.high_idx]}）"
            f"相隔 {gap} 个交易日，量价同步见顶{tail}"
        ),
    }


def _rule_recent(w: _Win) -> Optional[dict]:
    """近期量价状态：价跌看抛压（价跌量未缩 / 缩量续跌），价升看量能（放量/温和/缩量上行）。"""
    recent_ret = (w.closes[-1] - w.closes[-1 - RECENT]) / w.closes[-1 - RECENT] * 100
    recent_t = float(np.mean(w.turnovers[-RECENT:]))
    ratio = recent_t / w.median_t if w.median_t > 0 else 1.0
    fall_from_high = (w.last_close - w.high_close) / w.high_close * 100
    d = w.dates[-1]

    if recent_ret < -2:
        if fall_from_high <= -8 and ratio >= 1.0:
            return {"code": "high_vol_down", "label": "高位放量下跌", "date": d,
                    "evidence": f"自区间高点 {w.high_close:.2f} 元回落 {fall_from_high:.1f}%，"
                                f"近 {RECENT} 日换手 {recent_t:.2f}% 仍处高位（中位数 {w.median_t:.2f}%）"}
        if ratio >= 1.0:
            return {"code": "fall_no_shrink", "label": "价跌量未缩", "date": d,
                    "evidence": f"近 {RECENT} 日下跌 {recent_ret:.1f}%，换手 {recent_t:.2f}% "
                                f"高于区间中位数 {w.median_t:.2f}%——抛压未减、分歧仍在"}
        if ratio < 0.9:
            return {"code": "fall_shrink", "label": "缩量续跌", "date": d,
                    "evidence": f"近 {RECENT} 日下跌 {recent_ret:.1f}%，换手 {recent_t:.2f}% "
                                f"低于区间中位数 {w.median_t:.2f}%——跌势中量能萎缩"}

    if recent_ret > 2:
        if ratio >= 1.3:
            return {"code": "vol_up", "label": "放量上行", "date": d,
                    "evidence": f"近 {RECENT} 日上涨 {recent_ret:+.1f}%，换手 {recent_t:.2f}% "
                                f"较中位数 {w.median_t:.2f}% 明显放大（{ratio:.1f}×）"}
        if ratio <= 0.7:
            return {"code": "shrink_up", "label": "缩量上行", "date": d,
                    "evidence": f"近 {RECENT} 日上涨 {recent_ret:+.1f}%，换手 {recent_t:.2f}% "
                                f"低于中位数 {w.median_t:.2f}%——涨势中量能不足"}
        return {"code": "mild_vol_up", "label": "温和放量上行", "date": d,
                "evidence": f"近 {RECENT} 日上涨 {recent_ret:+.1f}%，换手 {recent_t:.2f}% "
                            f"略高于中位数 {w.median_t:.2f}%"}
    return None


def _divergence(w: _Win) -> dict:
    """换手率峰值与股价高点的对齐关系（辅助事实，供前端做背离提示）。"""
    with np.errstate(invalid="ignore"):
        corr = float(np.corrcoef(w.closes, w.turnovers)[0, 1])
    return {
        "peak_date": w.dates[w.peak_idx],
        "peak_turnover": round(w.turnovers[w.peak_idx], 2),
        "high_date": w.dates[w.high_idx],
        "high_close": round(w.high_close, 2),
        "peak_high_gap_days": abs(w.peak_idx - w.high_idx),
        "peak_high_coincide": abs(w.peak_idx - w.high_idx) <= PEAK_GAP,
        # 换手率与收盘价的相关系数（>0 价涨量增；<0 量价背离）
        "turnover_price_corr": round(corr, 3) if not np.isnan(corr) else None,
        "after_peak_return_pct": round(
            (w.last_close - w.closes[w.peak_idx]) / w.closes[w.peak_idx] * 100, 2
        ),
    }
