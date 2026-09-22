"use client";

import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

/** 一次原子写入的多个 query 参数;值空字符串表示删除该键。 */
export type ParamPatch = Record<string, string>;

/**
 * 把筛选状态持久化到 URL query（规范第 7 条：可分享）。
 * 返回 [query 对象, setParam, setParams]。
 *
 * ⚠️ **同一事件处理器里要改多个参数，必须用 `setParams`（一次调用），
 * 绝不能连续调两次 `setParam`** —— 每次 `setParam` 都从**闭包里过期的
 * `searchParams`** 重建 URL，两次调用共享同一份旧值，**第 2 次会覆盖第 1 次**，
 * 改动静默丢失（issue #80：消息页「点筛选没反应」即此因，`setFilter`/`setSize`
 * 都曾连调两次）。
 *
 * 根治手段是**函数式更新**：`setSearchParams((prev) => …)` 基于 react-router
 * 传入的最新 params 计算，即使同一 tick 连调多次也不会互相覆盖。
 * `setParams` 是它的多键封装，`setParam` 是单键糖。
 */
export function useUrlState(): [
  Record<string, string>,
  (key: string, value: string) => void,
  (patch: ParamPatch) => void
] {
  const [searchParams, setSearchParams] = useSearchParams();

  const setParam = useCallback(
    (key: string, value: string) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value) next.set(key, value);
          else next.delete(key);
          return next;
        },
        { replace: true }
      );
    },
    [setSearchParams]
  );

  const setParams = useCallback(
    (patch: ParamPatch) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          for (const [k, v] of Object.entries(patch)) {
            if (v) next.set(k, v);
            else next.delete(k);
          }
          return next;
        },
        { replace: true }
      );
    },
    [setSearchParams]
  );

  const query: Record<string, string> = {};
  searchParams.forEach((v, k) => {
    query[k] = v;
  });
  return [query, setParam, setParams];
}
