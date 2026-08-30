"use client";

import { useEffect, useState } from "react";
import { apiGet, FetchState } from "../lib/api";

type Options<P> = {
  /** REST 路径；为空表示无数据源（直接空态） */
  path: string | null;
  params?: Record<string, string | undefined>;
};

/**
 * 统一异步三态 hook（规范第 9 条）：loading / error / empty。
 * 后端不可达或接口报错时如实进入 error 态，绝不降级为演示数据。
 */
export function useData<P>(opts: Options<P>): FetchState<P> & { refresh: () => void } {
  const { path, params } = opts;
  const [state, setState] = useState<FetchState<P>>({
    data: null,
    loading: true,
    error: null,
  });
  const [tick, setTick] = useState(0);
  const key = JSON.stringify(params) + "|" + tick;

  useEffect(() => {
    if (!path) {
      setState({ data: null, loading: false, error: null });
      return;
    }
    const ac = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    apiGet<P>(path, params, ac.signal)
      .then((data) => setState({ data, loading: false, error: null }))
      .catch((err) => {
        if (err?.name === "AbortError") return;
        setState({
          data: null,
          loading: false,
          error: err instanceof Error ? err.message : String(err),
        });
      });
    return () => ac.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, key]);

  return { ...state, refresh: () => setTick((t) => t + 1) };
}
