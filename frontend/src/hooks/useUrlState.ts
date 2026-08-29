"use client";

import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

/**
 * 把筛选状态持久化到 URL query（规范第 7 条：可分享）。
 * 返回 [query 对象, setParam]。
 */
export function useUrlState(): [
  Record<string, string>,
  (key: string, value: string) => void
] {
  const [searchParams, setSearchParams] = useSearchParams();

  const setParam = useCallback(
    (key: string, value: string) => {
      const next = new URLSearchParams(searchParams.toString());
      if (value) next.set(key, value);
      else next.delete(key);
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams]
  );

  const query: Record<string, string> = {};
  searchParams.forEach((v, k) => {
    query[k] = v;
  });
  return [query, setParam];
}
