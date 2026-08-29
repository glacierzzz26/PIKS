"use client";

import { useCallback } from "react";
import { useSearchParams, useRouter, usePathname } from "next/navigation";

/**
 * 把筛选状态持久化到 URL query（规范第 7 条：可分享）。
 * 返回 [query 对象, setParam]。
 */
export function useUrlState(): [
  Record<string, string>,
  (key: string, value: string) => void
] {
  const sp = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  const setParam = useCallback(
    (key: string, value: string) => {
      const next = new URLSearchParams(sp.toString());
      if (value) next.set(key, value);
      else next.delete(key);
      router.replace(`${pathname}?${next.toString()}`, { scroll: false });
    },
    [sp, router, pathname]
  );

  const query: Record<string, string> = {};
  sp.forEach((v, k) => {
    query[k] = v;
  });
  return [query, setParam];
}
