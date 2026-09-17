"use client";

import { DOMAIN_LABEL, DOMAIN_TONE } from "@/lib/constants";
import type { ResearchDomain } from "@/lib/types";

/**
 * 三域标记（D-R5，design report-layout.md §4.3）：数据 / 计算 / 研判。
 * 「专业」的实质来源 —— 读者能区分硬数据、程序派生量与 AI 叙述。
 *
 * 域由后端 `section_manifest` **声明**（D-R8），前端不猜；颜色复用既有 `.st` 色板，
 * 不新增颜色（§4.3）。旧报告无清单时 domains 为空 → 不渲染任何标签（如实留空）。
 */
export default function DomainTag({ domains }: { domains: ResearchDomain[] }) {
  if (domains.length === 0) return null;
  return (
    <span className="inline-flex items-center gap-1">
      {domains.map((d) => (
        <span key={d} className={`domain-tag ${DOMAIN_TONE[d] ?? "st-dim"}`}>
          {DOMAIN_LABEL[d] ?? d}
        </span>
      ))}
    </span>
  );
}
