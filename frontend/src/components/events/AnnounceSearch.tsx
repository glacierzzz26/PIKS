"use client";

import { useState } from "react";
import { Search, X } from "lucide-react";

/** 公告标题/代码搜索框（公告 tab）。 */
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
        placeholder="搜索公告标题 / 代码…"
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
