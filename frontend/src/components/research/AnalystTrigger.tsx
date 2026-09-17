"use client";

import { useState } from "react";
import { AlertCircle, Loader2, Microscope } from "lucide-react";
import { RESEARCH_PROFILES } from "@/lib/constants";
import { isStockCode } from "@/lib/format";
import { useResearchTrigger } from "@/hooks/useResearchTrigger";

/**
 * 研究触发条：6 位代码输入 + 研究类型（profile）+ 触发。
 * 独立页入口 —— 不依赖实体库/持仓。
 * 校验：非 6 位数字不发请求，内联提示；触发失败内联 error 原文。
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
  const { trigger, busy, error } = useResearchTrigger();

  const valid = isStockCode(code.trim());
  const submit = () => {
    if (valid && !busy) trigger(code.trim(), profile);
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

      {(code && !valid) || error ? (
        <div className="mt-2 flex items-center gap-1.5 text-[12.5px] text-up">
          <AlertCircle size={13} />
          {error ?? "股票代码需为 6 位数字"}
        </div>
      ) : (
        <p className="mt-2 text-[12px] text-faint">
          输入 6 位 A 股代码，选类型，回车或点「开始分析」。采集+合成通常 10~60 秒。
        </p>
      )}
    </div>
  );
}
