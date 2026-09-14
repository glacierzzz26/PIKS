"use client";

import { Link } from "react-router-dom";
import { Scale, ChevronRight, Boxes, Share2 } from "lucide-react";

const ENTRIES = [
  {
    to: "/recon",
    icon: Scale,
    label: "对账",
    note: "快讯 → 事件 → 实体 的链路巡检",
  },
  {
    to: "/entities",
    icon: Boxes,
    label: "实体库",
    note: "公司 / 行业 / 概念 / 人物 / 地区 全量检索，按状态筛选自选",
  },
  {
    to: "/graph",
    icon: Share2,
    label: "图谱",
    note: "实体与事件的关系网络（力导图）",
  },
];

/** 数据与运维卡：底层数据浏览与巡检入口（P6-2 移出侧栏，路由保留、此处可达） */
export default function OpsCard() {
  return (
    <div className="form-card mt-4">
      <h3>数据与运维</h3>
      <p className="fnote">底层数据浏览与链路巡检（日常不常用，收在此处）</p>
      <div className="flex flex-col gap-2">
        {ENTRIES.map((e) => {
          const Icon = e.icon;
          return (
            <Link
              key={e.to}
              to={e.to}
              className="flex items-center justify-between rounded-[10px] border border-line px-3 py-2.5 text-[13px] no-underline hover:border-accent"
            >
              <span className="inline-flex items-center gap-2">
                <Icon size={15} className="text-muted" />
                <span>
                  {e.label}
                  <span className="ml-2 text-[12px] txt-faint">{e.note}</span>
                </span>
              </span>
              <ChevronRight size={14} className="txt-faint" />
            </Link>
          );
        })}
      </div>
    </div>
  );
}
