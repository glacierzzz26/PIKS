"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

const PAGE_SIZES = [20, 50, 100];

/**
 * 分页（规范：默认 20/页；页码与页大小写入 URL query 可分享）。
 * 用法：page/size 由页面的 useUrlState 提供，翻页时调用 onPage 重置筛选后的页码。
 */
export default function Pagination({
  page,
  pageSize,
  total,
  onPage,
  onPageSize,
}: {
  page: number;
  pageSize: number;
  total: number;
  onPage: (p: number) => void;
  onPageSize: (s: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (total === 0) return null;

  const nums = pageNumbers(page, pages);

  return (
    <div className="flex flex-wrap items-center gap-3 border-t border-line px-4 py-3">
      <span className="num text-xs text-muted">
        共 {total} 条 · 第 {page}/{pages} 页
      </span>

      <div className="ml-auto flex items-center gap-1">
        <PagerBtn disabled={page <= 1} onClick={() => onPage(page - 1)}>
          <ChevronLeft size={14} />
        </PagerBtn>

        {nums.map((n, i) =>
          n === -1 ? (
            <span key={`e${i}`} className="px-1 text-xs text-muted">
              …
            </span>
          ) : (
            <PagerBtn key={n} active={n === page} onClick={() => onPage(n)}>
              {n}
            </PagerBtn>
          )
        )}

        <PagerBtn disabled={page >= pages} onClick={() => onPage(page + 1)}>
          <ChevronRight size={14} />
        </PagerBtn>
      </div>

      <label className="flex items-center gap-1.5 text-xs text-muted">
        每页
        <select
          value={pageSize}
          onChange={(e) => onPageSize(Number(e.target.value))}
          className="h-7 rounded-sm border border-line bg-card-soft px-1.5 text-xs outline-none focus:border-accent"
        >
          {PAGE_SIZES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        条
      </label>
    </div>
  );
}

function PagerBtn({
  children,
  onClick,
  disabled,
  active,
}: {
  children: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={`flex h-7 min-w-7 items-center justify-center rounded-sm border px-1.5 text-xs transition-colors ${
        active
          ? "border-accent bg-accent-soft font-semibold text-accent"
          : "border-line bg-card text-muted hover:border-accent hover:text-accent"
      } disabled:cursor-not-allowed disabled:opacity-40`}
    >
      {children}
    </button>
  );
}

/** 页码序列：首尾保留，中间窗口，超出用 -1 表示省略号 */
function pageNumbers(page: number, pages: number): number[] {
  if (pages <= 7) return Array.from({ length: pages }, (_, i) => i + 1);
  const set = new Set([1, 2, page - 1, page, page + 1, pages - 1, pages]);
  const list = [...set].filter((n) => n >= 1 && n <= pages).sort((a, b) => a - b);
  const out: number[] = [];
  let prev = 0;
  for (const n of list) {
    if (n - prev > 1) out.push(-1);
    out.push(n);
    prev = n;
  }
  return out;
}
