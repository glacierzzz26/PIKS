"""事件降噪分级引擎（设计文档 §10「先降噪，再研究」）

流程：
1. 分类    — 确定性关键词分类器（新闻 + 公告共用）
2. 去重合并 — 同一事件多份公告/新闻联动只算一条（标题核心词 + 时间窗）
3. 重大性分级 — 分类 × 强度关键词 → 高 / 中 / 低
4. 事件 → 价格 — 高、中级事件计算事件日 ±5 交易日相对收益（§16 事件窗口）
5. 报告输出 — 「30 天无重大事件」是合法结论；低级事件聚合为一行统计

原则：LLM 不参与分类/分级，全部为确定性规则。
"""
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta
from typing import Any, Dict, List, Optional
import enum

from ..models.news import NewsItem
from ..models.announcement import Announcement
from ..models.bar import Bar

# ---------- 1. 事件分类（确定性关键词） ----------

# 每类事件的判定关键词（标题 + 内容命中即归类）
EVENT_CATEGORIES: Dict[str, List[str]] = {
    "earnings": ["年报", "年度报告", "中报", "中期报告", "半年度报告", "季报", "季度报告", "业绩预告", "业绩快报", "业绩", "净利润", "营收", "盈利", "亏损", "财报"],
    "restructure": ["重组", "并购", "收购", "资产置换", "借壳", "重大资产", "发行股份购买"],
    "shareholder": ["增持", "减持", "股权变动", "股东", "股权转让", "质押", "举牌"],
    "management": ["高管", "董事长", "总经理", "董事", "监事", "辞职", "任职", "换届"],
    "buyback": ["回购", "股份回购", "注销"],
    "contract": ["中标", "合同", "订单", "签署", "协议", "战略合作"],
    "regulatory": ["处罚", "警示函", "监管", "立案", "问询函", "违规", "立案调查"],
    "financing": ["定增", "可转债", "配股", "发债", "增发", "融资"],
    "announcement_dividend": ["分红", "派息", "送转", "每10股", "分配方案", "权益分派", "分派", "利润分配"],
    "halt": ["停牌", "复牌", "暂停上市"],
    "risk": ["退市", "风险警示", "ST"],
    "routine": ["公告", "提示", "更正", "补充", "进展", "会议", "通知"],  # 例行
}

# ---------- 2. 重大性强度关键词（叠加分类基础分） ----------

HIGH_KEYWORDS = [
    "重组", "并购", "收购", "重大资产", "借壳", "退市", "立案", "立案调查",
    "业绩预告", "业绩快报", "大幅", "预增", "预减", "预亏", "亏损",
    "减持", "增持", "质押", "举牌", "回购", "定增", "停牌",
    "中标", "合同", "订单",
]

MEDIUM_KEYWORDS = [
    "分红", "派息", "送转", "分配方案", "股权激励", "可转债",
    "高管", "董事长", "总经理", "辞职", "换届", "问询函", "警示函",
]

# 分类基础等级：0=例行, 1=中, 2=高
CATEGORY_BASE_LEVEL: Dict[str, int] = {
    "routine": 0,
    "earnings": 1,
    "announcement_dividend": 1,
    "management": 1,
    "contract": 1,
    "shareholder": 2,
    "restructure": 2,
    "buyback": 2,
    "regulatory": 2,
    "financing": 2,
    "halt": 2,
    "risk": 2,
}

# 标题中出现的强信号词，直接判定为高
DIRECT_HIGH = ["资产重组", "借壳", "退市风险", "立案调查", "业绩预亏", "大幅减持", "终止重组", "中止", "被实施"]


# ---------- 数据结构 ----------

class EventSeverity(enum.Enum):
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


@dataclass
class DenoisedEvent:
    """降噪后的一条独立事件"""
    id: str
    category: str
    severity: EventSeverity
    title: str
    source: str                    # 新闻 / 公告
    source_items: List[str]        # 合并进来的原始 ID 列表
    published_at: date
    keyword_hits: List[str] = field(default_factory=list)
    price_window: Optional[Dict[str, Any]] = None   # 事件 → 价格关联结果

    def to_dict(self) -> Dict[str, Any]:
        return {
            "id": self.id,
            "category": self.category,
            "severity": self.severity.value,
            "title": self.title,
            "source": self.source,
            "source_items": self.source_items,
            "published_at": self.published_at.isoformat(),
            "keyword_hits": self.keyword_hits,
            "price_window": self.price_window,
        }


@dataclass
class EventFilterResult:
    """事件降噪分级输出"""
    symbol: str
    as_of: date
    total_raw: int                 # 原始新闻+公告条数
    total_events: int              # 去重后事件数
    high_events: List[DenoisedEvent]
    medium_events: List[DenoisedEvent]
    low_events: List[DenoisedEvent]
    dedup_merged: int              # 被合并掉的重复条数
    category_distribution: Dict[str, int]
    no_major_event: bool           # 30 天无高/中级事件（合法结论）

    @property
    def major_events(self) -> List[DenoisedEvent]:
        return self.high_events + self.medium_events

    def to_dict(self) -> Dict[str, Any]:
        return {
            "symbol": self.symbol,
            "as_of": self.as_of.isoformat(),
            "total_raw": self.total_raw,
            "total_events": self.total_events,
            "dedup_merged": self.dedup_merged,
            "high_events": [e.to_dict() for e in self.high_events],
            "medium_events": [e.to_dict() for e in self.medium_events],
            "low_events": [e.to_dict() for e in self.low_events],
            "category_distribution": self.category_distribution,
            "no_major_event": self.no_major_event,
        }


# ---------- 3. 分类与分级实现 ----------

def classify_event(title: str, content: str = "") -> tuple[str, List[str]]:
    """确定性分类：返回 (category, keyword_hits)"""
    text = title + " " + content

    # 例行会议/通知类先降噪（避免「股东大会」误中 shareholder 高风险）
    routine_patterns = ["股东大会", "会议通知", "临时股东大会", "董事会决议", "监事会决议"]
    has_major = any(k in text for k in ["增持", "减持", "重组", "并购", "收购", "退市", "立案"])
    if not has_major and any(p in text for p in routine_patterns):
        return "routine", ["例行会议"]

    # 直接高优先词
    for kw in DIRECT_HIGH:
        if kw in text:
            return "restructure" if ("重组" in kw or "借壳" in kw) else _direct_high_category(kw), [kw]

    hits: Dict[str, List[str]] = {}
    for category, keywords in EVENT_CATEGORIES.items():
        for kw in keywords:
            if kw in text:
                hits.setdefault(category, []).append(kw)

    # 多个分类命中时取最高基础级
    if not hits:
        return "routine", ["例行"]

    def base(cat: str) -> int:
        return CATEGORY_BASE_LEVEL.get(cat, 0)

    best = max(hits, key=lambda c: (base(c), len(hits[c])))
    return best, hits[best]


def _direct_high_category(kw: str) -> str:
    if "退市" in kw or "ST" in kw:
        return "risk"
    if "处罚" in kw or "立案" in kw:
        return "regulatory"
    if "减持" in kw or "增持" in kw:
        return "shareholder"
    if "重组" in kw or "并购" in kw:
        return "restructure"
    if "回购" in kw:
        return "buyback"
    return "earnings" if ("业绩" in kw or "预" in kw) else "routine"


def grade_severity(category: str, title: str, content: str = "") -> EventSeverity:
    """重大性分级：高 / 中 / 低"""
    text = title + " " + content
    for kw in DIRECT_HIGH:
        if kw in text:
            return EventSeverity.HIGH

    base = CATEGORY_BASE_LEVEL.get(category, 0)
    if any(k in text for k in HIGH_KEYWORDS):
        return EventSeverity.HIGH
    if any(k in text for k in MEDIUM_KEYWORDS):
        return EventSeverity.MEDIUM
    if base == 2:
        return EventSeverity.MEDIUM
    if base == 1:
        return EventSeverity.MEDIUM
    return EventSeverity.LOW


# ---------- 4. 去重合并 ----------

STOPWORDS = {"公司", "公告", "关于", "的", "有限", "股份", "集团", "提示", "暨", "与", "及"}


def normalize_title(title: str) -> str:
    """提取标题核心标识（用于合并判定）"""
    s = title
    for w in ["（", "）", "(", ")", "：", ":", "·", " ", "　"]:
        s = s.replace(w, "")
    return s


def _merge_key(title: str, category: str) -> str:
    """合并键：分类 + 标题归一（去括号内容 / 年份 / 期次 / 数字）"""
    import re
    t = title
    t = re.sub(r"[（(][^）)]*[）)]", "", t)   # 去括号及其内容
    t = normalize_title(t)
    t = re.sub(r"(20\d{2}|19\d{2})", "", t)      # 去年份
    t = re.sub(r"[一二三四五六七八九十]+", "N", t)  # 期次归一
    t = re.sub(r"\d+", "N", t)                    # 数字归一
    return f"{category}|{t}"


def _dedup(
    items: List[Any],
    kind: str,
) -> tuple[List[DenoisedEvent], int]:
    """对统一来源（新闻或公告）去重合并，返回 (事件列表, 合并条数)"""
    groups: Dict[str, List[Any]] = {}
    for item in items:
        category, hits = classify_event(item.title, getattr(item, "content", ""))
        key = _merge_key(item.title, category)
        groups.setdefault(key, []).append(item)

    events: List[DenoisedEvent] = []
    merged = 0
    for key, group in groups.items():
        group.sort(key=lambda x: getattr(x, "published_at", getattr(x, "publish_date", date.today())))
        primary = group[0]
        category = key.split("|")[0]
        severity = grade_severity(category, primary.title, getattr(primary, "content", ""))
        pub = getattr(primary, "published_at", None)
        if pub is None:
            pub = getattr(primary, "publish_date", date.today())
        pub_date = pub.date() if isinstance(pub, datetime) else pub
        events.append(DenoisedEvent(
            id=f"ev_{kind}_{primary.id}",
            category=category,
            severity=severity,
            title=primary.title,
            source=kind,
            source_items=[getattr(x, "id", "") for x in group],
            published_at=pub_date,
            keyword_hits=[],
            price_window=None,
        ))
        merged += len(group) - 1
    return events, merged


# ---------- 5. 事件 → 价格关联（§16 事件窗口） ----------

def _event_price_correlation(
    event: DenoisedEvent,
    bars: List[Bar],
) -> Optional[Dict[str, Any]]:
    """事件日 ±5 交易日窗口相对收益（对照区间 [0,+5]）"""
    if not bars:
        return None

    sorted_bars = sorted(bars, key=lambda b: b.date)
    target = event.published_at
    if isinstance(target, datetime):
        target = target.date()
    # 找事件日当天或之前的最后一个交易日
    idx = None
    for i, b in enumerate(sorted_bars):
        if b.date <= target:
            idx = i
        else:
            break
    if idx is None:
        return None

    # 事件日 → +5 交易日
    window_end = min(idx + 5, len(sorted_bars) - 1)
    if window_end <= idx:
        return None

    ret_t0 = sorted_bars[idx].close
    ret_t5 = sorted_bars[window_end].close
    window_return = (ret_t5 - ret_t0) / ret_t0 * 100 if ret_t0 else None

    # 相对指数近似：用当日涨跌幅累计（若 bars 无指数则跳过）
    cum_chg = sum(b.pct_change for b in sorted_bars[idx + 1 : window_end + 1])

    return {
        "event_date": sorted_bars[idx].date.isoformat(),
        "t0_close": round(ret_t0, 2),
        "t5_close": round(ret_t5, 2),
        "window_return_pct": round(window_return, 2) if window_return is not None else None,
        "window_days": window_end - idx,
        "cum_pct_change": round(cum_chg, 2),
        "window_complete": (window_end - idx) == 5,  # 事件后 5 交易日数据是否齐备
    }


# ---------- 6. 主入口 ----------

def filter_events(
    symbol: str,
    as_of: date,
    news: Optional[List[NewsItem]] = None,
    announcements: Optional[List[Announcement]] = None,
    bars: Optional[List[Bar]] = None,
    window_days: int = 30,
) -> EventFilterResult:
    """事件降噪分级主入口

    Args:
        news          : 原始新闻列表
        announcements : 原始公告列表
        bars          : K线（用于事件→价格关联）
        window_days   : 降噪窗口（天）
    """
    news = news or []
    announcements = announcements or []

    # 时间窗口过滤
    from datetime import datetime
    cutoff = datetime.combine(as_of - timedelta(days=window_days), datetime.min.time())
    news_in = [n for n in news if n.published_at >= cutoff]

    cutoff_date = as_of - timedelta(days=window_days)
    anns_in = [a for a in announcements if a.publish_date >= cutoff_date]

    total_raw = len(news_in) + len(anns_in)

    # 新闻去重
    news_events, news_merged = _dedup(news_in, "news")
    ann_events, ann_merged = _dedup(anns_in, "ann")

    # 跨源合并：新闻与公告同一 merge_key 合并（公告优先保留标题）
    all_events: Dict[str, DenoisedEvent] = {}
    merged_across = 0
    for ev in ann_events + news_events:
        key = ev.category + "|" + normalize_title(ev.title)
        if key in all_events:
            existing = all_events[key]
            existing.source_items.extend(ev.source_items)
            merged_across += 1
        else:
            all_events[key] = ev

    events = list(all_events.values())
    dedup_merged = news_merged + ann_merged + merged_across

    # 事件 → 价格关联（高、中级）
    if bars:
        for ev in events:
            if ev.severity in (EventSeverity.HIGH, EventSeverity.MEDIUM):
                ev.price_window = _event_price_correlation(ev, bars)

    # 分类统计
    distribution: Dict[str, int] = {}
    for ev in events:
        distribution[ev.category] = distribution.get(ev.category, 0) + 1

    high = [e for e in events if e.severity == EventSeverity.HIGH]
    medium = [e for e in events if e.severity == EventSeverity.MEDIUM]
    low = [e for e in events if e.severity == EventSeverity.LOW]

    return EventFilterResult(
        symbol=symbol,
        as_of=as_of,
        total_raw=total_raw,
        total_events=len(events),
        high_events=high,
        medium_events=medium,
        low_events=low,
        dedup_merged=dedup_merged,
        category_distribution=distribution,
        no_major_event=len(high) + len(medium) == 0,
    )