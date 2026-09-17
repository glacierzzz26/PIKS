"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { AbnormalVolumeDay } from "@/lib/types";

/** 量 → 万手/手（引擎落库为「手」=100 股）。 */
function fmtVolume(v: number): string {
  if (v >= 1e4) return `${(v / 1e4).toFixed(2)} 万手`;
  return `${v.toLocaleString("zh-CN")} 手`;
}

/** 成交额 → 亿/万（元）。 */
function fmtAmount(v: number): string {
  if (v >= 1e8) return `${(v / 1e8).toFixed(2)} 亿`;
  if (v >= 1e4) return `${(v / 1e4).toFixed(2)} 万`;
  return `${v.toLocaleString("zh-CN")} 元`;
}

/**
 * 「放量异常日」逐日明细（issue #3）—— 可展开表。
 *
 * 数字全部来自 `metrics.volume.abnormal_volume_detail`（引擎算好落库），
 * 此组件只做展示格式化，**不重算、不补算**。无异常日时如实空态。
 *
 * 展开态走组件内 state（非 URL）：这是一份报告内的行级详情，
 * 「可分享」的诉求由报告 URL 本身承载，不必为二级展开再写 query。
 */
export default function AbnormalVolumeDetail({
  rows,
}: {
  rows: AbnormalVolumeDay[];
}) {
  const [open, setOpen] = useState(false);
  const n = rows.length;

  if (n === 0) {
    return (
      <div className="panel panel-pad">
        <div className="mb-1 flex items-center gap-2">
          <h3 className="m-0 text-[15px] font-bold">放量异常日</h3>
          <span className="text-[11px] text-faint">确定性计算</span>
        </div>
        <p className="m-0 text-[12.5px] text-faint">
          窗口内无异常放量日（无一日成交量超过其自身阈值）。
        </p>
      </div>
    );
  }

  return (
    <div className="panel panel-pad">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 text-left"
        aria-expanded={open}
      >
        {open ? (
          <ChevronDown size={15} className="shrink-0 txt-faint" />
        ) : (
          <ChevronRight size={15} className="shrink-0 txt-faint" />
        )}
        <h3 className="m-0 text-[15px] font-bold">放量异常日</h3>
        <span className="num text-[13px] font-semibold">{n}</span>
        <span className="text-[11px] text-faint">
          {open ? "点击收起逐日明细" : "点击展开：哪几天 / 放了多少 / 放大几倍"}
        </span>
      </button>

      {open && (
        <div className="mt-3 overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th className="text-left">日期</th>
                <th>成交量</th>
                <th>成交额</th>
                <th>放大倍数</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.date} className="h-[38px]">
                  <td className="num-t text-left">{r.date}</td>
                  <td className="num-t">{fmtVolume(r.volume)}</td>
                  <td className="num-t">{fmtAmount(r.amount)}</td>
                  <td className="num-t font-semibold">
                    {r.ratio === null ? "—" : `${r.ratio.toFixed(2)}×`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-2 mb-0 text-[11px] leading-relaxed text-faint">
            判定规则：当日量 &gt; 该日阈值 = max(前 20 日均量 ×2, 前 5 日峰值)；
            放大倍数 = 当日量 ÷ 该日阈值。窗口外的更早交易日不参与扫描，故不在此列。
          </p>
        </div>
      )}
    </div>
  );
}
