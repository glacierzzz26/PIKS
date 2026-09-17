"use client";

import { Link } from "react-router-dom";
import { GLOSSARY_LIST } from "@/lib/glossary";

const STEPS: { to: string; title: string; body: string }[] = [
  {
    to: "/trades",
    title: "① 同步自选与持仓",
    body: "在「交易与持仓」页上传同花顺的自选 / 持仓截图。PIKS 识别后建立你的股票清单，不用手动录入。",
  },
  {
    to: "/",
    title: "② 回到「今天」",
    body: "首页列出你的自选，标出哪些有持仓、哪些还没深研过。有新消息或多日未看的票会提醒你。",
  },
  {
    to: "/research",
    title: "③ 点进一只票做研究",
    body: "在「个股分析」输入代码发起研究：选「个股分析」看今天的行情 / 量价 / 换手（日频速览），选「公司研报」看财务 / 估值 / 行业质地（季频文档）。选「宏观研报」则按维度（CPI / PPI / M2 / GDP）各出一份，数据是统计期快照。也可以从自选点进个股中心。",
  },
  {
    to: "/reports",
    title: "④ 读成篇的研报",
    body: "「研报」把公司 / 行业 / 宏观研究排成带目录的文档，章节标着数据 / 计算 / 研判三种来源 —— 你能一眼分清哪些是硬数据、哪些是 AI 的判断。",
  },
  {
    to: "/reviews",
    title: "⑤ 买入后，定期体检",
    body: "「持仓诊断」对当前持仓列出风险点与复盘点，可一键存为笔记 —— 结论会回流到那只股票的个人笔记。",
  },
];

/** 使用指南 + 词汇表（静态页，无 API）。新手第一站。 */
export default function Page() {
  return (
    <div>
      <div className="page-head">
        <div>
          <h1>使用指南</h1>
          <div className="psub">PIKS 怎么用，以及页面里那些词是什么意思</div>
        </div>
        <div className="meta">
          <Link
            to="/"
            className="inline-flex h-8 items-center rounded-[10px] border border-line bg-card px-3 text-xs text-muted no-underline hover:text-accent"
          >
            回到今天 →
          </Link>
        </div>
      </div>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>四步走通一个闭环</h2>
          <span className="hint">研究 → 决策 → 持仓 → 复盘</span>
        </div>
        <div className="note-grid">
          {STEPS.map((s) => (
            <Link key={s.to} to={s.to} className="note-card no-underline">
              <div className="nh">
                <b className="text-ink">{s.title}</b>
              </div>
              <p>{s.body}</p>
              <div className="nfoot">
                <span className="text-accent">前往 →</span>
              </div>
            </Link>
          ))}
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>名词解释</h2>
          <span className="hint">共 {GLOSSARY_LIST.length} 条 · 白话版</span>
        </div>
        <div className="panel">
          {GLOSSARY_LIST.map((g) => (
            <div
              key={g.label}
              className="flex flex-col gap-1 border-b border-line px-5 py-3.5 last:border-b-0 md:flex-row md:gap-6"
            >
              <b className="w-40 shrink-0 text-[14px]">{g.label}</b>
              <span className="text-[13px] leading-relaxed text-muted">{g.def}</span>
            </div>
          ))}
        </div>
      </section>

      <p className="mt-6 text-center text-[12px] txt-faint">
        PIKS 是知识系统，不是交易系统 —— 不预测涨跌、不给买卖建议。所有数据如实标注来源与日期。
      </p>
    </div>
  );
}
