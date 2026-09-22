"use client";

import { Link } from "react-router-dom";
import { Camera, PenLine, MessageSquare, ArrowRight } from "lucide-react";

/**
 * 快速开始（设计 phase6/ux-ia.md §2 区块5）。
 * 三步都指向现有页面，不新造交互：同步截图 → 录交易 → 问 AI。
 */
const STEPS = [
  {
    to: "/trades",
    icon: Camera,
    title: "① 同步自选 / 持仓截图",
    body: "自选每天自动从同花顺同步（自动失灵时可在「交易与持仓」用截图兜底）；持仓截图上传后 PIKS 识别建立记录，不用手动录入。",
  },
  {
    to: "/trades",
    icon: PenLine,
    title: "② 录一笔交易",
    body: "买入或卖出后记一笔。以后能回答「我为什么买它」——若关联了当时的报告或笔记。",
  },
  {
    to: "/chat",
    icon: MessageSquare,
    title: "③ 问 AI 一只票",
    body: "对某只票有疑问？在「问 AI」里问，答案带知识库引用，可溯源。",
  },
];

export default function QuickStart() {
  return (
    <section className="section">
      <div className="section-head">
        <span className="bar" />
        <h2>快速开始</h2>
        <span className="hint">三步走通一个闭环</span>
      </div>
      <div className="note-grid">
        {STEPS.map((s) => {
          const Icon = s.icon;
          return (
            <Link key={s.title} to={s.to} className="note-card no-underline">
              <div className="nh">
                <Icon size={17} className="text-accent" />
                <b className="text-ink">{s.title}</b>
              </div>
              <p>{s.body}</p>
              <div className="nfoot">
                <span className="inline-flex items-center gap-1 text-accent">
                  前往 <ArrowRight size={12} />
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </section>
  );
}
