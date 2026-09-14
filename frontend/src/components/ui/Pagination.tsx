"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

const PAGE_SIZES = [20, 50, 100];

/**
 * 分页（规范：默认 20/页；页码与页大小写入 URL query 可分享）。
 * 样式对齐 HTML .pager（圆角页码，当前页品牌实底）。
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
    <div className="pager">
      <span className="num mr-2 text-[12.5px] text-faint">
        共 {total} 条 · 第 {page}/{pages} 页
      </span>

      <button
        disabled={page <= 1}
        onClick={() => onPage(page - 1)}
        className="disabled:cursor-not-allowed disabled:opacity-40"
        aria-label="上一页"
      >
        <ChevronLeft size={14} />
      </button>

      {nums.map((n, i) =>
        n === -1 ? (
          <span key={`e${i}`} className="px-1 text-[12.5px] text-faint">
            …
          </span>
        ) : (
          <button key={n} className={n === page ? "cur" : ""} onClick={() => onPage(n)}>
            {n}
          </button>
        )
      )}

      <button
        disabled={page >= pages}
        onClick={() => onPage(page + 1)}
        className="disabled:cursor-not-allowed disabled:opacity-40"
        aria-label="下一页"
      >
        <ChevronRight size={14} />
      </button>

      <label className="ml-3 flex items-center gap-1.5 text-[12.5px] text-faint">
        每页
        <select
          value={pageSize}
          onChange={(e) => onPageSize(Number(e.target.value))}
          className="rounded-sm border border-line bg-card px-1.5 py-0.5 text-[12.5px] text-muted outline-none focus:border-accent"
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
