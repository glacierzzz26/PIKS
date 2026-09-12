"""公告数据模型"""
from dataclasses import dataclass
from datetime import date
from typing import Optional


@dataclass(frozen=True)
class Announcement:
    """单条公告"""
    id: str
    symbol: str
    title: str
    category: str           # 公告类型
    publish_date: date
    url: Optional[str]

    @property
    def is_major(self) -> bool:
        """是否重大公告"""
        major_types = [
            "年度报告", "半年度报告", "季度报告",
            "业绩预告", "业绩快报",
            "重大资产重组", "并购重组",
            "股权变动", "增持", "减持",
            "回购", "股权激励",
            "停牌", "复牌",
            "退市风险",
        ]
        return any(t in self.category for t in major_types) or any(t in self.title for t in major_types)
