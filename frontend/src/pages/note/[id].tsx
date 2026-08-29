"use client";

import { Link, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { useData } from "@/hooks/useData";
import { getDocs, getDoc } from "@/lib/mockService";
import { ENDPOINTS } from "@/lib/api";
import MarkdownBody from "@/components/md/MarkdownBody";
import { Chip } from "@/components/ui/Num";
import { EmptyState } from "@/components/ui/States";
import { DOC_TYPE_LABEL } from "@/lib/format";

/** 笔记阅读页（只读；Markdown 渲染） */
export default function Page() {
  const { id = "" } = useParams();
  const docs = useData({
    path: id ? ENDPOINTS.note.replace(":id", id) : null,
    fallback: () => getDoc(id) ?? getDocs()[0],
  });
  const doc = docs.data;

  if (docs.loading) {
    return (
      <div className="py-20 text-center text-[13px] text-muted">加载中…</div>
    );
  }
  if (!doc) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <EmptyState tip="文档不存在或已被归档" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[820px]">
      <div className="mb-3 mt-5">
        <Link
          to="/notes"
          className="inline-flex items-center gap-1.5 text-[13px] text-muted no-underline hover:text-accent"
        >
          <ArrowLeft size={13} />
          返回笔记列表
        </Link>
      </div>

      <article className="rounded border border-line bg-card px-7 py-6 shadow-card">
        <div className="flex items-center gap-2.5">
          <Chip tone="accent">{DOC_TYPE_LABEL[doc.type]}</Chip>
          <span className="num text-xs text-muted">更新于 {doc.updated_at}</span>
          <span className="ml-auto text-xs text-muted">
            编辑请前往 Go 端 /notes
          </span>
        </div>
        <h1 className="mb-4 mt-3 border-b border-line pb-3 text-2xl font-bold leading-snug">
          {doc.title}
        </h1>
        <MarkdownBody content={doc.content} />
      </article>
    </div>
  );
}
