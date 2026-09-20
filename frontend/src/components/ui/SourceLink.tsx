"use client";

import { ExternalLink } from "lucide-react";

/**
 * 来源外链（issue #38）：有原文 URL 时渲染为可点外链（新标签页），
 * 无 URL 时退化为纯文本 —— 绝不渲染空 href / 死链，也不显示误导性外链图标。
 *
 * `stopPropagation`：来源常位于可点击行内（事件表格行打开抽屉、快讯行内）,
 * 不拦住冒泡的话点外链会同时触发行点击,外链与抽屉二选一被吞。
 */
export function SourceLink({
  source,
  url,
  showIcon = false,
}: {
  source: string;
  url?: string;
  /** 是否在文字后追加外链图标（事件详情用；无 url 时不显示，避免假链接暗示） */
  showIcon?: boolean;
}) {
  if (!url) return <span>{source}</span>;
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      onClick={(e) => e.stopPropagation()}
      className="inline-flex items-center gap-1 text-accent no-underline hover:underline"
      title="在新标签页打开原文"
    >
      {source}
      {showIcon && <ExternalLink size={12} />}
    </a>
  );
}
