"use client";

import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Pencil, Archive } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiDelete, ENDPOINTS } from "@/lib/api";
import MarkdownBody from "@/components/md/MarkdownBody";
import { Chip } from "@/components/ui/Num";
import { EmptyState, ErrorState } from "@/components/ui/States";
import { DOC_TYPE_LABEL } from "@/lib/format";
import type { NoteDetail } from "@/lib/types";

const NOTE_TONE: Record<string, "accent" | "amber" | "up" | "down"> = {
  note: "down",
  belief: "accent",
  case: "amber",
  mistake: "up",
};

const STATUS_LABEL: Record<string, string> = {
  hypothesis: "假设",
  active: "活跃",
  confirmed: "已确认",
  questioned: "存疑",
  rejected: "否定",
  archived: "归档",
};

/** 笔记阅读页（Markdown 渲染）+ 编辑 / 归档（周报经此路径只读回退） */
export default function Page() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [archiving, setArchiving] = useState(false);
  const docs = useData<NoteDetail>({
    path: id ? ENDPOINTS.note.replace(":id", id) : null,
  });

  const archive = async () => {
    if (!window.confirm("归档后将从列表隐藏，确定归档这篇笔记？")) return;
    setArchiving(true);
    try {
      await apiDelete<{ ok: boolean }>(`/notes/${id}`);
      navigate("/notes");
    } catch (e) {
      window.alert(e instanceof Error ? e.message : String(e));
      setArchiving(false);
    }
  };

  if (docs.loading) {
    return <div className="py-20 text-center text-[13px] text-muted">加载中…</div>;
  }
  if (docs.error) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <ErrorState msg={docs.error} />
      </div>
    );
  }
  const doc = docs.data;
  if (!doc) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <EmptyState tip="文档不存在或已被归档" />
      </div>
    );
  }
  const isNote = doc.sel_events !== undefined;

  return (
    <div className="mx-auto max-w-[820px]">
      <div className="mb-3 mt-5 flex items-center gap-3">
        <Link
          to="/notes"
          className="inline-flex items-center gap-1.5 text-[13px] text-muted no-underline hover:text-accent"
        >
          <ArrowLeft size={13} />
          返回笔记列表
        </Link>
        {isNote && (
          <span className="ml-auto flex items-center gap-2">
            <Link
              to={`/notes/${doc.id}/edit`}
              className="inline-flex h-8 items-center gap-1.5 rounded-sm border border-line bg-card px-3 text-xs text-muted no-underline hover:text-accent"
            >
              <Pencil size={12} />
              编辑
            </Link>
            <button
              onClick={archive}
              disabled={archiving}
              className="inline-flex h-8 items-center gap-1.5 rounded-sm border border-line bg-card px-3 text-xs text-muted hover:text-up disabled:opacity-50"
            >
              <Archive size={12} />
              {archiving ? "归档中…" : "归档"}
            </button>
          </span>
        )}
      </div>

      <article className="rounded border border-line bg-card px-7 py-6 shadow-card">
        <div className="flex items-center gap-2.5">
          <Chip tone={NOTE_TONE[doc.type] ?? "accent"}>
            {DOC_TYPE_LABEL[doc.type] ?? doc.type}
          </Chip>
          {isNote && doc.status && (
            <Chip tone={doc.status === "archived" ? "amber" : "dim"}>
              {STATUS_LABEL[doc.status] ?? doc.status}
            </Chip>
          )}
          <span className="num text-xs text-muted">更新于 {doc.updated_at}</span>
        </div>
        <h1 className="mb-4 mt-3 border-b border-line pb-3 text-2xl font-bold leading-snug">
          {doc.title}
        </h1>
        <MarkdownBody content={doc.content} />
      </article>
    </div>
  );
}
