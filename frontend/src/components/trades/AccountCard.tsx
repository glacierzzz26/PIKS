"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { AccountRow } from "@/lib/types";

/**
 * 账户资金卡（issue #19）：同花顺「持仓」页顶部四项汇总。
 *
 * 口径（勿混，详见 design trades.md §账户口径）：
 * - 总资产   含现金（持仓市值 + 可用资金）；唯一含现金的口径
 * - 总市值   不含现金
 * - 浮动盈亏 持仓**累计**浮盈（不含现金）
 * - 当日参考盈亏 按**当日**价格变动的浮盈（与累计是两个维度）
 *
 * 四项均可为 null = 截图没这个数 → 显示「—」，**不显示 0**（0 是「确实为零」）。
 */
export default function AccountCard({
  account,
  className = "",
}: {
  /** 已有数据时传入（/trades 的响应已含 account，避免重复请求）；/reviews 不传则自取。 */
  account?: AccountRow | null;
  className?: string;
}) {
  const { data } = useData<{ account: AccountRow | null }>({
    // 外部已给数据（含 null = 确无快照）→ path=null 直接空态，不重复拉取。
    path: account !== undefined ? null : ENDPOINTS.account,
  });
  const a = account !== undefined ? account : data?.account;
  // 无快照：如实说明「还没采到」，不摆一排空数字假装有界面。
  if (!a) {
    return (
      <div className={`panel ${className}`}>
        <div className="flex h-12 items-center border-b border-line px-5">
          <h2 className="mb-0 text-[15px] font-bold tracking-wide">账户资金</h2>
        </div>
        <p className="px-5 py-4 text-xs text-faint">
          还没有账户资金快照 · 到「截图导入 → 持仓」上传含顶部汇总的同花顺持仓截图
        </p>
      </div>
    );
  }
  return (
    <div className={`panel ${className}`}>
      <div className="flex h-12 items-center gap-2 border-b border-line px-5">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">账户资金</h2>
        <span className="text-xs text-faint">{a.date} 快照</span>
      </div>
      <div className="grid grid-cols-2 gap-x-6 gap-y-4 px-5 py-4 md:grid-cols-4">
        <Metric label="总资产" value={a.total_asset} unit="元" />
        <Metric label="总市值" value={a.total_mv} unit="元" />
        <Metric label="浮动盈亏" value={a.float_pl} unit="元" colored />
        <Metric label="当日参考盈亏" value={a.daily_pl} unit="元" colored />
      </div>
    </div>
  );
}

/** 单项：值 null → 「—」（截图没这个数），不折成 0。涨红跌绿（A 股习惯）。 */
function Metric({
  label,
  value,
  unit,
  colored = false,
}: {
  label: string;
  value: number | null;
  unit: string;
  colored?: boolean;
}) {
  const color =
    colored && value !== null && value !== 0
      ? value > 0
        ? "var(--red)"
        : "var(--green)"
      : undefined;
  return (
    <div>
      <div className="text-xs text-muted">{label}</div>
      <div className="num mt-1 text-right text-[17px] font-semibold" style={{ color }}>
        {value === null ? (
          <span className="text-faint">—</span>
        ) : (
          <>
            {colored && value > 0 ? "+" : ""}
            {value.toLocaleString("zh-CN", { maximumFractionDigits: 2 })}
            <span className="ml-1 text-xs text-faint">{unit}</span>
          </>
        )}
      </div>
    </div>
  );
}
