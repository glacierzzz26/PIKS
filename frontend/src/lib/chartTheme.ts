import { useEffect, useState } from "react";

/**
 * ECharts 配色解析。ECharts 默认画在 canvas 上，`var(--red)` 这类 CSS 变量
 * 在 canvas 里无法解析，必须先读成具体色值，否则图表不随暗色模式切换。
 * 故此处把令牌解析一次并缓存，主题变化时由 useChartTheme 重新解析。
 */
export type ChartTheme = {
  red: string;
  green: string;
  brand: string;
  line: string;
  muted: string;
};

const FALLBACK: ChartTheme = {
  red: "#e0392b",
  green: "#1f9d57",
  brand: "#28457e",
  line: "#e7ebf2",
  muted: "#5b6678",
};

let cache: ChartTheme | null = null;
let cacheKey = "";

function read(name: string, fb: string): string {
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fb;
}

/** 解析当前主题（data-theme）下的图表色值 */
export function resolveChartTheme(): ChartTheme {
  if (typeof window === "undefined") return FALLBACK;
  const key = document.documentElement.getAttribute("data-theme") ?? "light";
  if (cache && cacheKey === key) return cache;
  cache = {
    red: read("--red", FALLBACK.red),
    green: read("--green", FALLBACK.green),
    brand: read("--accent", FALLBACK.brand),
    line: read("--line", FALLBACK.line),
    muted: read("--muted", FALLBACK.muted),
  };
  cacheKey = key;
  return cache;
}

/** 订阅主题切换，暗色/浅色切换后图表配色同步刷新 */
export function useChartTheme(): ChartTheme {
  const [theme, setTheme] = useState<ChartTheme>(FALLBACK);
  useEffect(() => {
    const sync = () => setTheme({ ...resolveChartTheme() });
    sync();
    const mo = new MutationObserver(sync);
    mo.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    return () => mo.disconnect();
  }, []);
  return theme;
}
