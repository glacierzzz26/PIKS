"use client";

import { CheckCircle2, CircleDashed, CircleSlash, TriangleAlert, XCircle } from "lucide-react";
import type { TaskRun } from "@/lib/types";

/** 内部命令 → 白话任务名（P6-2 隐藏管线内部：不再把 research-run:gather 这类命令名暴露给用户） */
const TASK_LABEL: Record<string, string> = {
  "quote-collector": "行情采集",
  "market-state": "市场情绪",
  "daily-review": "每日复盘",
  "entity-build": "股票档案",
  "research-run:gather": "研究报告 · 取数",
  "research-run:synth": "研究报告 · 生成",
  "research-run:verify": "研究报告 · 校核",
};

function label(cmd: string) {
  return TASK_LABEL[cmd] ?? "数据更新";
}

const TONE = {
  ok: { icon: CheckCircle2, color: "text-down", text: "已完成" },
  partial: { icon: TriangleAlert, color: "text-amber", text: "部分未入库" },
  running: { icon: CircleDashed, color: "text-amber", text: "进行中" },
  skipped: { icon: CircleSlash, color: "txt-faint", text: "已跳过" },
  failed: { icon: XCircle, color: "text-up", text: "失败（下次自动重试）" },
} as const;

/**
 * 数据更新状态：把最近几次后台任务按「白话名 + 结果」列出。
 * 按索引作 key —— 同一任务可能在一段时间内多次出现（重试），命令名并不唯一。
 *
 * issue #64：后端此前把失败条数塞进 meta 却无人读取 ⇒ 修好记账后仍看不见。
 * 现 `failed > 0` 显式上屏「N 条未入库」，不再让绿色对勾盖住丢数据的事实。
 */
export function Pipeline({ runs }: { runs: TaskRun[] }) {
  if (runs.length === 0) {
    return <p className="px-5 pb-4 text-[13px] txt-faint">暂无更新记录。</p>;
  }
  return (
    <ul className="flex flex-col divide-y divide-line">
      {runs.map((t, i) => {
        const tone = TONE[t.status] ?? TONE.running;
        const Icon = tone.icon;
        return (
          <li key={`${t.command}-${i}`} className="flex items-center gap-2.5 px-5 py-2.5 text-[13px]">
            <Icon size={15} className={tone.color} strokeWidth={2} />
            <b className="font-semibold">{label(t.command)}</b>
            <span className="text-muted">{tone.text}</span>
            {t.failed > 0 && <span className="num text-amber">{t.failed} 条未入库</span>}
            {t.note && <span className="truncate txt-faint">· {t.note}</span>}
            <span className="num ml-auto text-[12px] txt-faint">{t.time}</span>
          </li>
        );
      })}
    </ul>
  );
}
