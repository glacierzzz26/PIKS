"use client";

import { useEffect, useRef, useState } from "react";
import { apiGet, FetchState } from "../lib/api";
import { incDemo, decDemo } from "../lib/demoSignal";

type Options<P> = {
  /** REST 路径；为空表示该模块后端尚未提供，直接用演示数据 */
  path: string | null;
  params?: Record<string, string | undefined>;
  /** 后端不可达时的降级数据源 */
  fallback: () => P;
};

/**
 * 统一异步三态 hook（规范第 9 条）：loading / error / empty。
 * 后端不可达时自动降级为演示数据（demo=true，界面标注），不阻塞浏览。
 */
export function useData<P>(opts: Options<P>): FetchState<P> & { refresh: () => void } {
  const { path, params, fallback } = opts;
  const [state, setState] = useState<FetchState<P>>({
    data: null,
    loading: true,
    error: null,
    demo: false,
  });
  const [tick, setTick] = useState(0);
  const key = JSON.stringify(params) + "|" + tick;

  // 降级计数：进入演示态 inc，退出 dec，卸载时归零；驱动 TopNav 徽章。
  const demoRef = useRef<boolean | null>(null);
  const applyDemo = (d: boolean) => {
    const prev = demoRef.current;
    if (prev === d) return;
    demoRef.current = d;
    if (d) incDemo();
    else if (prev === true) decDemo();
  };

  useEffect(() => {
    if (!path) {
      applyDemo(true);
      setState({ data: fallback(), loading: false, error: null, demo: true });
      return;
    }
    const ac = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    apiGet<P>(path, params, ac.signal)
      .then((data) => {
        applyDemo(false);
        setState({ data, loading: false, error: null, demo: false });
      })
      .catch((err) => {
        if (err?.name === "AbortError") return;
        // 降级：演示数据保持界面可用
        applyDemo(true);
        setState({ data: fallback(), loading: false, error: null, demo: true });
      });
    return () => {
      ac.abort();
      if (demoRef.current === true) decDemo();
      demoRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, key]);

  return { ...state, refresh: () => setTick((t) => t + 1) };
}
