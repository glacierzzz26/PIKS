"use client";

import { useMemo } from "react";
import EChart from "@/components/charts/EChart";
import { useChartTheme } from "@/lib/chartTheme";
import type { PatternPoint } from "@/lib/types";
import type { PiksChartOption } from "@/components/charts/EChart";

const UP = "#e0392b"; // 占位：实际色值由 useChartTheme 覆盖（此处仅供 tooltip 兜底）

/**
 * 量价形态双轴图：收盘价（左轴折线）+ 换手率（右轴柱）。
 * 换手率 = **A股流通股本口径**（腾讯/akshare），与同花顺逐日一致（实测偏差 ≤0.02%）；
 * 自由流通口径无免费源 —— 图注由调用方标注。
 * 柱体按当日涨跌着色（A 股习惯：涨红跌绿）；暗色模式经 useChartTheme 同步。
 */
export default function VolumePriceChart({ series }: { series: PatternPoint[] }) {
  const c = useChartTheme();
  const option = useMemo(() => build(series, c), [series, c]);
  if (series.length === 0) return null;
  return <EChart option={option} height={280} />;
}

function build(
  series: PatternPoint[],
  c: ReturnType<typeof useChartTheme>
): PiksChartOption {
  const dates = series.map((p) => p.date.slice(5)); // MM-DD，省空间
  const bars = series.map((p, i) => {
    const prev = i > 0 ? series[i - 1].close : p.close;
    return {
      value: p.turnover,
      itemStyle: { color: p.close >= prev ? c.red : c.green, opacity: 0.65 },
    };
  });
  return {
    grid: { left: 52, right: 52, top: 20, bottom: 28 },
    tooltip: {
      trigger: "axis",
      formatter: (params) => {
        const arr = Array.isArray(params) ? params : [params];
        const i = arr[0]?.dataIndex ?? 0;
        const p = series[i];
        if (!p) return "";
        const prev = i > 0 ? series[i - 1].close : p.close;
        const chg = ((p.close - prev) / prev) * 100;
        const cls = chg >= 0 ? UP : "#1f9d57";
        return (
          `<b>${p.date}</b><br/>` +
          `收盘 ${p.close.toFixed(2)} 元 ` +
          `<span style="color:${cls}">${chg >= 0 ? "+" : ""}${chg.toFixed(2)}%</span><br/>` +
          `换手 ${p.turnover.toFixed(2)}%`
        );
      },
    },
    xAxis: {
      type: "category",
      data: dates,
      axisLabel: { fontSize: 10, color: c.muted, interval: Math.max(0, Math.floor(dates.length / 8) - 1) },
      axisLine: { lineStyle: { color: c.line } },
    },
    yAxis: [
      {
        type: "value",
        name: "收盘价",
        nameTextStyle: { fontSize: 10, color: c.muted },
        scale: true,
        axisLabel: { fontSize: 10, color: c.muted },
        splitLine: { lineStyle: { color: c.line } },
      },
      {
        type: "value",
        name: "换手%",
        nameTextStyle: { fontSize: 10, color: c.muted },
        axisLabel: { fontSize: 10, color: c.muted },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: "收盘价",
        type: "bar",
        yAxisIndex: 1,
        data: bars,
        barWidth: "60%",
      },
      {
        name: "收盘价",
        type: "line",
        yAxisIndex: 0,
        data: series.map((p) => p.close),
        smooth: false,
        symbol: "none",
        lineStyle: { color: c.muted, width: 1.5 },
      },
    ],
  };
}
