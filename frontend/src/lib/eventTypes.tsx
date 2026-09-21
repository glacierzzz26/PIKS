"use client";

/**
 * 事件类型枚举的前端消费层（issue #61）。
 *
 * **类型文案的唯一真源是后端**（`model.EventTypes` → `GET /api/v1/event-types`）。
 * 前端**不存**本地类型表 —— 曾经 `lib/constants.ts` / `lib/format.ts` /
 * `EventTable.tsx` 各存一份 8 值枚举,与后端 9 值权威枚举漂移:
 * 92% 事件在表格里露英文原值、类型下拉 8 项里 6 项永远筛不出东西。
 * 只下发 key 不够 —— 前端仍要留一张 label 表,后端加类型照样回落英文,漂移只修一半。
 * 故 key **与** label 都下发。
 *
 * 在 App 根部挂一次 `EventTypesProvider`,各消费方用 `useEventTypes()` 取。
 * 三态(loading/error)在这里统一处理,消费方不必各写一遍。
 */

import {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";

export type EventType = { key: string; label: string };

type EventTypesState = {
  /** 后端下发的类型清单；未就绪时为空数组（调用方按 loading 分支处理） */
  types: EventType[];
  loading: boolean;
  error: string | null;
  /** key → 中文 label；未命中的 key 返回 undefined（调用方决定兜底文案） */
  labelOf: (key: string) => string | undefined;
  /** key → `.type-tag` 配色类；按固定 5 色循环，不新增色（见下） */
  toneOf: (key: string) => string;
  /** 筛选下拉选项：首项「全部类型」+ 全部后端类型 */
  filterOptions: EventType[];
};

const Ctx = createContext<EventTypesState | null>(null);

/**
 * 配色：**沿用既有 5 个 tag 色，不新增**（issue #61 决策）。
 * 9 类复用 5 色足够可辨；新增色要动浅/暗两套令牌，且颜色过多反而弱化扫读。
 * 按**后端下发的顺序**循环 —— 后端加类型时自动拿到一个色，前端零改动。
 */
const TONES = ["t-mix", "t-idx", "t-ev", "t-bond", "t-gray"];

export function EventTypesProvider({ children }: { children: ReactNode }) {
  const { data, loading, error } = useData<EventType[]>({
    path: ENDPOINTS.eventTypes,
  });
  const types = data ?? [];

  const value = useMemo<EventTypesState>(() => {
    const byKey = new Map(types.map((t) => [t.key, t]));
    const toneByKey = new Map(
      types.map((t, i) => [t.key, TONES[i % TONES.length]])
    );
    return {
      types,
      loading,
      error,
      labelOf: (key) => byKey.get(key)?.label,
      toneOf: (key) => toneByKey.get(key) ?? "t-gray",
      filterOptions: [{ key: "", label: "全部类型" }, ...types],
    };
  }, [types, loading, error]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

/** 取事件类型枚举。必须在 `EventTypesProvider` 内使用。 */
export function useEventTypes(): EventTypesState {
  const v = useContext(Ctx);
  if (!v) {
    throw new Error("useEventTypes 必须在 EventTypesProvider 内使用");
  }
  return v;
}
