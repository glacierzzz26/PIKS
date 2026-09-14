"use client";

import { useEffect, useRef } from "react";
import * as echarts from "echarts/core";
import { BarChart } from "echarts/charts";
import type { BarSeriesOption } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  type GridComponentOption,
  type TooltipComponentOption,
} from "echarts/components";
import { LabelLayout } from "echarts/features";
import { CanvasRenderer } from "echarts/renderers";
import type { ComposeOption } from "echarts/core";

// 只注册本应用实际用到的图表与组件（柱状图 + 直角坐标系 + 提示框），
// 其余 echarts 模块由 tree-shaking 剔除，显著缩小体积。
echarts.use([BarChart, GridComponent, TooltipComponent, LabelLayout, CanvasRenderer]);

export type PiksChartOption = ComposeOption<
  BarSeriesOption | GridComponentOption | TooltipComponentOption
>;

/**
 * ECharts 轻封装：声明式 option，容器自适应。
 * 涨红跌绿配色由调用方按 A 股习惯传入（规范第 1 条）。
 */
export default function EChart({
  option,
  height = 300,
}: {
  option: PiksChartOption;
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
