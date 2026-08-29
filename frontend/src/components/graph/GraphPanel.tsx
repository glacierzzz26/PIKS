"use client";

import { useMemo } from "react";
import { Link } from "react-router-dom";
import { ZoomIn, ZoomOut, Maximize2 } from "lucide-react";
import { ENTITY_TYPE_LABEL } from "@/lib/format";
import { Chip } from "@/components/ui/Num";
import type { Entity, Relationship } from "@/lib/types";

/** 图谱右侧详情面板（对齐 .panel）：元信息 + 相关关系 + 跳转实体卡 */
export default function GraphPanel({
  entity,
  relationships,
  entities,
  onClose,
  onFocus,
}: {
  entity: Entity;
  relationships: Relationship[];
  entities: Entity[];
  onClose: () => void;
  onFocus: (id: string) => void;
}) {
  const rels = useMemo(
    () =>
      relationships.filter((r) => r.from_id === entity.id || r.to_id === entity.id),
    [relationships, entity.id]
  );

  return (
    <div className="absolute bottom-3 right-3 top-3 w-[340px] max-w-[80%] overflow-auto rounded border border-line bg-card p-4 shadow-pop">
      <div className="flex items-start justify-between">
        <h3 className="m-0 text-base font-bold leading-snug">{entity.name}</h3>
        <button
          onClick={onClose}
          className="text-xs text-muted hover:text-up"
        >
          关闭
        </button>
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        <Chip tone="accent">{ENTITY_TYPE_LABEL[entity.type]}</Chip>
        {entity.aliases.slice(0, 3).map((a) => (
          <Chip key={a} tone="dim">
            {a}
          </Chip>
        ))}
      </div>
      {entity.description && (
        <p className="mt-3 text-[13px] leading-relaxed">{entity.description}</p>
      )}

      <h4 className="mb-1.5 mt-3 border-t border-line pt-3 text-xs font-semibold text-muted">
        关系（{rels.length}）
      </h4>
      {rels.length === 0 && <p className="text-xs italic text-muted">暂无</p>}
      {rels.map((r) => {
        const otherId = r.from_id === entity.id ? r.to_id : r.from_id;
        const other = entities.find((e) => e.id === otherId);
        return (
          <button
            key={r.id}
            onClick={() => onFocus(otherId)}
            className="mb-1 block w-full truncate text-left text-[13px] hover:text-accent"
          >
            <span className="text-muted">{r.rel_type}</span> {other?.name ?? otherId}
          </button>
        );
      })}

      <Link
        to={`/entities?id=${entity.id}`}
        className="mt-2 block text-[13px] text-accent no-underline hover:underline"
      >
        打开完整实体卡 →
      </Link>
    </div>
  );
}

/** 图谱工具栏右侧按钮：缩放 / 重置 / 全屏 */
export function GraphActions() {
  const call = (fn: string, arg?: number) => {
    const g = (window as unknown as Record<string, { zoom?: (f: number) => void; reset?: () => void }>).__piksGraph;
    if (fn === "zoom") g?.zoom?.(arg!);
    if (fn === "reset") g?.reset?.();
  };
  return (
    <div className="flex flex-col gap-1.5">
      {[
        { icon: ZoomIn, fn: () => call("zoom", 1.25), title: "放大" },
        { icon: ZoomOut, fn: () => call("zoom", 0.8), title: "缩小" },
        { icon: Maximize2, fn: () => call("reset"), title: "重置视图" },
      ].map(({ icon: Icon, fn, title }, i) => (
        <button
          key={i}
          title={title}
          onClick={fn}
          className="flex h-8 w-8 items-center justify-center rounded-sm border border-line bg-card text-muted hover:border-accent hover:text-accent"
        >
          <Icon size={14} />
        </button>
      ))}
    </div>
  );
}
