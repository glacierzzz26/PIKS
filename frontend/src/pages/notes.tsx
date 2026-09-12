"use client";

import { Suspense } from "react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { Doc } from "@/lib/types";
import { DOC_TYPE_LABEL } from "@/lib/format";
import Pagination from "@/components/ui/Pagination";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import { usePagedQuery } from "@/hooks/usePagedQuery";

const NOTE_TAG: Record<string, string> = {
  note: "t-bond",
  belief: "t-mix",
  case: "t-ev",
  mistake: "t-gold",
  "daily-review": "t-mix",
  weekly: "t-mix",
};

/** 笔记：类型筛选 + 分页 + 新建/编辑/归档（交互） */
export default function Page() {
  return (
    <Suspense fallback={<div className="panel mt-6"><LoadingBlock rows={6} /></div>}>
      <NotesInner />
    </Suspense>
  );
}

function NotesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const type = query.type ?? "";

  const docs = useData<Doc[]>({ path: ENDPOINTS.notes });
  const all = docs.data ?? [];
  const data = type ? all.filter((d) => d.type === type) : all;
  const paged = paginate(data);

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>笔记</h1>
          <div className="psub">个人判断层 · 事实（机器）与推断（自己）严格分域</div>
        </div>
        <div className="meta">
          <span className="st st-accent">共 {data.length} 篇</span>
          <Link
            to="/notes/new"
            className="inline-flex h-8 items-center gap-1.5 rounded-[9px] border border-line bg-card px-3 text-xs font-semibold text-muted no-underline hover:text-accent"
          >
            <Plus size={13} />
            新建笔记
          </Link>
        </div>
      </div>

      <div className="filter-bar">
        <button
          onClick={() => setFilter("type", "")}
          className={`chip-btn ${type === "" ? "on" : ""}`}
        >
          全部
        </button>
        {["belief", "note", "case", "mistake"].map((t) => (
          <button
            key={t}
            onClick={() => setFilter("type", t)}
            className={`chip-btn ${type === t ? "on" : ""}`}
          >
            {DOC_TYPE_LABEL[t] ?? t}
          </button>
        ))}
      </div>

      {docs.loading ? (
        <div className="panel">
          <LoadingBlock rows={6} />
        </div>
      ) : docs.error ? (
        <div className="panel">
          <ErrorState msg={docs.error} />
        </div>
      ) : data.length === 0 ? (
        <div className="panel">
          <EmptyState tip="该类型暂无笔记" />
        </div>
      ) : (
        <>
          <div className="note-grid">
            {paged.map((d) => (
              <Link key={d.id} to={`/notes/${d.id}`} className="note-card no-underline">
                <div className="nh">
                  <span className={`type-tag ${NOTE_TAG[d.type] ?? "t-gray"}`}>
                    {DOC_TYPE_LABEL[d.type] ?? d.type}
                  </span>
                  <b style={{ color: "var(--ink)" }}>{d.title}</b>
                </div>
                <p>{d.content.replace(/[#*>|-]/g, "").slice(0, 140)}</p>
                <div className="nfoot">
                  <span>更新 {d.updated_at}</span>
                  <span className="conf">阅读全文 →</span>
                </div>
              </Link>
            ))}
          </div>
          <Pagination
            page={page}
            pageSize={size}
            total={data.length}
            onPage={setPage}
            onPageSize={setSize}
          />
        </>
      )}
    </div>
  );
}
