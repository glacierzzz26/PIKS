"use client";

import { Link, useNavigate } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import NoteForm from "@/components/note/NoteForm";
import { apiPost } from "@/lib/api";
import type { NoteInput } from "@/lib/types";

/** 新建笔记：表单提交走 POST /api/v1/notes，成功后跳转阅读页 */
export default function Page() {
  const navigate = useNavigate();

  const create = async (input: NoteInput) => {
    const res = await apiPost<{ id: string }>("/notes", input);
    navigate(`/notes/${res.id}`);
  };

  return (
    <div className="mx-auto max-w-[860px]">
      <div className="mb-3 mt-5 flex items-center gap-3">
        <Link
          to="/notes"
          className="inline-flex items-center gap-1.5 text-[13px] text-muted no-underline hover:text-accent"
        >
          <ArrowLeft size={13} />
          返回笔记列表
        </Link>
      </div>
      <div className="rounded border border-line bg-card p-6 shadow-card">
        <h1 className="mb-5 border-b border-line pb-3 text-xl font-bold">
          新建笔记
        </h1>
        <NoteForm submitLabel="创建笔记" onSubmit={create} />
      </div>
    </div>
  );
}
