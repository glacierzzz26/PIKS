"use client";

import { MACRO_DIMENSIONS, macroSubject } from "@/lib/constants";

/**
 * 宏观维度选择器（P9-5 / #13）。
 *
 * ⚠️ 只让用户**选**，不让用户**敲** `macro:<key>` —— key 的合法取值是 research
 * 侧维度表的知识（D-M2），前端列一份按钮就使非法 key 无从产生。与
 * `isMacroCode` 的形态校验互补：选择器防手滑，校验器防其它入口。
 */
export default function MacroDimensionPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (key: string) => void;
}) {
  return (
    <>
      <label className="text-[12.5px] font-semibold text-muted">宏观维度</label>
      <span className="inline-flex items-center gap-1.5">
        {MACRO_DIMENSIONS.map((d) => (
          <button
            key={d.key}
            type="button"
            onClick={() => onChange(d.key)}
            className={`chip-btn ${value === d.key ? "on" : ""}`}
          >
            {d.label}
          </button>
        ))}
      </span>
      <span className="num text-[12px] text-faint">{macroSubject(value)}</span>
    </>
  );
}
