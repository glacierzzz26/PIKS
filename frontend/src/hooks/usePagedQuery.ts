"use client";

import { useMemo } from "react";
import { useUrlState } from "./useUrlState";

/**
 * 分页 + 筛选组合：page/size 持久化到 URL（规范第 7 条）。
 * defaultSize = 20（规范：分页默认 20/页）。
 *
 * ⚠️ `setFilter`/`setSize` **必须一次原子写多个键**（`setParams`）——
 * 连调两次 `setParam` 会因闭包过期互相覆盖，筛选值被静默丢弃（issue #80）。
 */
export function usePagedQuery(defaultSize = 20) {
  const [query, setParam, setParams] = useUrlState();

  const page = Math.max(1, Number(query.page) || 1);
  const size = Number(query.size) || defaultSize;

  /** 翻页 */
  const setPage = (p: number) => setParam("page", p <= 1 ? "" : String(p));
  /** 改页大小（重置到第 1 页） */
  const setSize = (s: number) => {
    setParams({ size: s === defaultSize ? "" : String(s), page: "" });
  };
  /** 改筛选（重置到第 1 页） */
  const setFilter = (k: string, v: string) => {
    setParams({ [k]: v, page: "" });
  };

  const paginate = useMemo(() => {
    return <T>(list: T[]): T[] => list.slice((page - 1) * size, page * size);
  }, [page, size]);

  return { query, setParam, setFilter, page, size, setPage, setSize, paginate };
}
