"use client";

import { useState } from "react";
import { AlertCircle, Loader2, Microscope } from "lucide-react";
import { RESEARCH_PROFILES, MACRO_DIMENSIONS, macroSubject } from "@/lib/constants";
import { isStockCode, isMacroCode } from "@/lib/format";
import { useResearchTrigger } from "@/hooks/useResearchTrigger";
import MacroDimensionPicker from "./MacroDimensionPicker";

/**
 * 研究触发条：主体输入 + 研究类型（profile）+ 触发。
 * 独立页入口 —— 不依赖实体库/持仓。
 *
 * 两种输入模式，**校验规则相反**（P9-5 / #13 起）：
 * - 个股档案：6 位数字（`isStockCode`），输入框剥非数字。
 * - 宏观档案：`macro:<key>`，code 由**维度按钮**产出，无自由输入。
 * 二者刻意不共用输入框 —— 剥非数字会把 `cn_cpi` 一起吃掉；且放开非 6 位
 * **绝不等于**让任意字符串进后端（issue #2 的脏 code 防线仍在此）。
 *
 * profile 选择受控（父级写入 URL query，规则 7）。
 */
export default function AnalystTrigger({
  profile,
  onProfile,
}: {
  profile: string;
  onProfile: (p: string) => void;
}) {
  const [code, setCode] = useState("");
  const [dim, setDim] = useState(MACRO_DIMENSIONS[0].key);
  const { trigger, busy, error } = useResearchTrigger();

  const isMacro = profile === "macro";
  // 提交目标是**主体码**：个股档案走输入框，宏观档案由维度按钮拼出。
  // 宏观仍过 `isMacroCode` 形态校验 —— 按钮是唯一来源，正常恒真；这里防的是
  // 维度表与选择器**漂移**（按钮列了后端不认的 key）时把脏码送进后端。
  const target = isMacro ? macroSubject(dim) : code.trim();
  const valid = isMacro ? isMacroCode(target) : isStockCode(code.trim());

  const submit = () => {
    if (valid && !busy) trigger(target, profile);
  };

  return (
    <div className="panel panel-pad">
      <form
        className="filter-bar mb-0"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {isMacro ? (
          <MacroDimensionPicker value={dim} onChange={setDim} />
        ) : (
          <>
            <label className="text-[12.5px] font-semibold text-muted">股票代码</label>
            <div className="f-search num" style={{ maxWidth: 220 }}>
              <input
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                placeholder="如 000560"
                inputMode="numeric"
                autoFocus
                aria-label="股票代码"
              />
            </div>
          </>
        )}

        <span className="inline-flex items-center gap-1.5">
          {RESEARCH_PROFILES.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => onProfile(p.key)}
              className={`chip-btn ${profile === p.key ? "on" : ""}`}
            >
              {p.label}
            </button>
          ))}
          {/*
            宏观研报 chip：与上面三个同属「选类型」，故并列一排。
            ⚠️ 主体输入随之切换为维度按钮 —— 它是主体选择器，不只是档案选择器。
          */}
          <button
            type="button"
            onClick={() => onProfile("macro")}
            className={`chip-btn ${isMacro ? "on" : ""}`}
          >
            宏观研报
          </button>
        </span>

        <button
          type="submit"
          disabled={!valid || busy}
          className="btn-save ml-auto disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span className="inline-flex items-center gap-1.5">
            {busy ? (
              <Loader2 size={13} className="animate-spin" />
            ) : (
              <Microscope size={13} />
            )}
            {busy ? "提交中…" : "开始分析"}
          </span>
        </button>
      </form>

      <Hint isMacro={isMacro} code={code} valid={valid} error={error} />
    </div>
  );
}

/** 底部提示/报错行。与主体模式耦合，故拆出以免 JSX 超 80 行（规则 5）。 */
function Hint({
  isMacro,
  code,
  valid,
  error,
}: {
  isMacro: boolean;
  code: string;
  valid: boolean;
  error: string | null;
}) {
  // 个股：输入了却不成 6 位即报。宏观：码由按钮产出，不合法只可能是维度表漂移。
  const showError = error ?? (!valid && (isMacro || code) ? INVALID_HINT(isMacro) : null);
  if (showError) {
    return (
      <div className="mt-2 flex items-center gap-1.5 text-[12.5px] text-up">
        <AlertCircle size={13} />
        {showError}
      </div>
    );
  }
  return (
    <p className="mt-2 text-[12px] text-faint">
      {isMacro
        ? "选宏观维度，回车或点「开始分析」。宏观研报一维度一份，数据为统计期快照。"
        : "输入 6 位 A 股代码，选类型，回车或点「开始分析」。采集+合成通常 10~60 秒。"}
    </p>
  );
}

const INVALID_HINT = (isMacro: boolean) =>
  isMacro ? "该维度暂不可用，请换一个" : "股票代码需为 6 位数字";
