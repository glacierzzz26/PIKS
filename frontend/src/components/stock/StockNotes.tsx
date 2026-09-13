"use client";

import { Link } from "react-router-dom";
import { NotebookPen } from "lucide-react";
import { DOC_TYPE_LABEL } from "@/lib/format";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import type { StockNote } from "@/lib/types";

/** 个股中心「我的笔记」区块：引用了该公司实体的笔记。 */
export default function StockNotes({ notes }: { notes: StockNote[] }) {
  if (notes.length === 0) {
    return (
      <StockSectionEmpty
        tip="暂无引用该股的笔记"
        action={{ to: "/notes/new", label: "写一条笔记", icon: NotebookPen }}
      />
    );
  }

  return (
    <div className="flex flex-col divide-y divide-line">
      {notes.map((n) => (
        <Link
          key={n.id}
          to={`/notes/${n.id}`}
          className="flex items-center gap-3 px-1 py-2.5 hover:bg-hover"
        >
          <span className="st st-dim">{DOC_TYPE_LABEL[n.type] ?? n.type}</span>
          <span className="min-w-0 flex-1 truncate font-medium">
            {n.title || "（无标题）"}
          </span>
          <span className="num text-[12px] text-faint">{n.updated_at.slice(0, 10)}</span>
        </Link>
      ))}
    </div>
  );
}
