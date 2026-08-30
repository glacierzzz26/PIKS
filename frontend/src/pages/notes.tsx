"use client";

import { Suspense } from "react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { Doc } from "@/lib/types";
import { DOC_TYPE_LABEL } from "@/lib/format";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import { usePagedQuery } from "@/hooks/usePagedQuery";

const NOTE_TONES: Record<string, "accent" | "amber" | "up" | "down"> = {
  note: "down",
  belief: "accent",
  case: "amber",
  mistake: "up",
  "daily-review": "accent",
  weekly: "accent",
};

/** 笔记：类型筛选 + 分页 + 新建/编辑/归档（交互） */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6 h-40 animate-pulse rounded bg-card" />}>
      <NotesInner />
    </Suspense>
  );
}

function NotesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const type = query.type ?? "";

  const docs = useData<Doc[]>({
    path: ENDPOINTS.notes,
  });
  const all = docs.data ?? [];
  const data = type ? all.filter((d) => d.type === type) : all;
  const paged = paginate(data);

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">笔记</h1>
          <span className="num text-[13px] text-muted">{data.length} 篇</span>
          <Link
            to="/notes/new"
            className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-sm border border-line bg-card px-3 text-xs text-muted no-underline hover:text-accent"
          >
            <Plus size={13} />
            新建笔记
          </Link>
        </div>
        <div className="mt-3 flex flex-wrap gap-1.5">
          <button
            onClick={() => setFilter("type", "")}
            className={`chip ${type === "" ? "chip-accent" : "chip-dim"}`}
          >
            全部
          </button>
          {["daily-review", "note", "weekly", "mistake"].map((t) => (
            <button
              key={t}
              onClick={() => setFilter("type", t)}
              className={`chip ${type === t ? "chip-accent" : "chip-dim"}`}
            >
              {DOC_TYPE_LABEL[t]}
            </button>
          ))}
        </div>
      </div>

      {docs.loading ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <LoadingBlock rows={6} />
        </div>
      ) : docs.error ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <ErrorState msg={docs.error} />
        </div>
      ) : data.length === 0 ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <EmptyState tip="该类型暂无笔记" />
        </div>
      ) : (
        <>
          <div className="mt-4 flex flex-col gap-2.5">
            {paged.map((d) => (
              <Link
                key={d.id}
                to={`/notes/${d.id}`}
                className="block rounded border border-line bg-card p-4 shadow-card no-underline hover:border-accent"
              >
                <div className="flex items-center gap-2.5">
                  <span className="text-base font-semibold text-ink">
                    {d.title}
                  </span>
                  <Chip tone={NOTE_TONES[d.type] ?? "dim"}>
                    {DOC_TYPE_LABEL[d.type]}
                  </Chip>
                  <span className="num ml-auto text-xs text-muted">
                    {d.updated_at}
                  </span>
                </div>
                <p className="mb-0 mt-1.5 line-clamp-2 text-[13px] text-muted">
                  {d.content.replace(/[#*>|-]/g, "").slice(0, 120)}
                </p>
              </Link>
            ))}
          </div>
          <div className="mt-3 rounded border border-line bg-card shadow-card">
            <Pagination
              page={page}
              pageSize={size}
              total={data.length}
              onPage={setPage}
              onPageSize={setSize}
            />
          </div>
        </>
      )}
    </div>
  );
}
