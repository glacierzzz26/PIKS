"use client";

import { Link, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import NoteForm from "@/components/note/NoteForm";
import { useData } from "@/hooks/useData";
import { apiPut, ENDPOINTS } from "@/lib/api";
import type { NoteDetail, NoteInput } from "@/lib/types";
import { ErrorState } from "@/components/ui/States";

/** 编辑笔记：GET 详情回填表单，PUT /api/v1/notes/:id 保存 */
export default function Page() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const detail = useData<NoteDetail>({
    path: ENDPOINTS.note.replace(":id", id),
  });

  const save = async (input: NoteInput) => {
    await apiPut<{ ok: boolean }>(`/notes/${id}`, input);
    navigate(`/notes/${id}`);
  };

  if (detail.loading) {
    return (
      <div className="py-20 text-center text-[13px] text-muted">加载中…</div>
    );
  }
  if (detail.error || !detail.data) {
    return (
      <div className="mx-auto mt-6 max-w-[860px] rounded border border-line bg-card shadow-card">
        <ErrorState msg={detail.error ?? "笔记不存在"} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[860px]">
      <div className="mb-3 mt-5 flex items-center gap-3">
        <Link
          to={`/notes/${id}`}
          className="inline-flex items-center gap-1.5 text-[13px] text-muted no-underline hover:text-accent"
        >
          <ArrowLeft size={13} />
          返回笔记
        </Link>
      </div>
      <div className="rounded border border-line bg-card p-6 shadow-card">
        <h1 className="mb-5 border-b border-line pb-3 text-xl font-bold">
          编辑笔记
        </h1>
        <NoteForm initial={detail.data} submitLabel="保存修改" onSubmit={save} />
      </div>
    </div>
  );
}
