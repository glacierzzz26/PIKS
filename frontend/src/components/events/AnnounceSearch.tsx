"use client";

import { useState } from "react";
import { Search, X } from "lucide-react";

/** 公告标题 / 代码 / 简称搜索框（公告 tab）。占位符须与后端实际搜索字段一致
 *  —— `handleAPIAnnouncements` 的 strSub 口径为 title + sec_code + sec_name（issue #80）。 */
export function AnnounceSearch({
  q,
  onSearch,
}: {
  q: string;
  onSearch: (v: string) => void;
}) {
  const [local, setLocal] = useState(q);
  return (
    <form
      className="f-search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(local.trim());
      }}
    >
      <Search size={15} className="txt-faint" strokeWidth={2} />
      <input
        value={local}
        onChange={(e) => setLocal(e.target.value)}
        placeholder="搜索公告标题 / 代码 / 简称…"
      />
      {q ? (
        <button
          type="button"
          onClick={() => {
            setLocal("");
            onSearch("");
          }}
          className="txt-faint hover:text-up"
        >
          <X size={13} />
        </button>
      ) : null}
    </form>
  );
}
