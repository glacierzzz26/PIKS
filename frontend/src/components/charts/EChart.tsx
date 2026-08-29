"use client";

import { useEffect, useRef } from "react";
import * as echarts from "echarts";

/**
 * ECharts 轻封装：声明式 option，容器自适应。
 * 涨红跌绿配色由调用方按 A 股习惯传入（规范第 1 条）。
 */
export default function EChart({
  option,
  height = 300,
}: {
  option: echarts.EChartsOption;
  height?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const chartRef = useRef<echarts.ECharts | null>(null);

  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current);
    chartRef.current = chart;
    const onResize = () => chart.resize();
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("resize", onResize);
      chart.dispose();
    };
  }, []);

  useEffect(() => {
    chartRef.current?.setOption(option, true);
  }, [option]);

  return <div ref={ref} style={{ width: "100%", height }} />;
}
