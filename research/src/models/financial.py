"""财务数据模型"""
from dataclasses import dataclass
from datetime import date
from typing import Optional


@dataclass(frozen=True)
class FinancialSnapshot:
    """单季度/年度财务快照"""
    symbol: str
    report_date: date
    report_type: str       # "Q1", "Q2", "Q3", "annual"

    # 盈利能力
    revenue: Optional[float] = None              # 营业收入（元）
    revenue_yoy: Optional[float] = None          # 营收同比（%）
    net_profit: Optional[float] = None           # 净利润（元）
    net_profit_yoy: Optional[float] = None       # 净利润同比（%）
    gross_margin: Optional[float] = None         # 毛利率（%）
    net_margin: Optional[float] = None           # 净利率（%）
    roe: Optional[float] = None                  # 净资产收益率（%）

    # 资产负债
    total_assets: Optional[float] = None         # 总资产（元）
    debt_ratio: Optional[float] = None           # 资产负债率（%）
    receivables: Optional[float] = None          # 应收账款（元）
    inventory: Optional[float] = None            # 存货（元）

    # 现金流
    operating_cashflow: Optional[float] = None   # 经营现金流（元）
    operating_cashflow_per_share: Optional[float] = None  # 每股经营现金流（元）

    # 估值（可能来自其他接口）
    pe_ttm: Optional[float] = None               # PE TTM
    pb: Optional[float] = None                   # PB
    ps: Optional[float] = None                   # PS


@dataclass(frozen=True)
class ValuationMetrics:
    """估值指标"""
    symbol: str
    as_of: date
    pe_ttm: Optional[float] = None
    pe_lyr: Optional[float] = None
    pb: Optional[float] = None
    ps: Optional[float] = None
    market_cap: Optional[float] = None      # 总市值（元）
    float_cap: Optional[float] = None       # 流通市值（元）
