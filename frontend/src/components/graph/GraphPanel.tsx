"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ZoomIn, ZoomOut, Maximize2, Minimize2, RotateCcw } from "lucide-react";
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

/** 图谱工具栏右侧按钮：缩放 / 重置 / 真全屏（请求浏览器全屏，而非仅重置视图） */
export function GraphActions({
  fullscreenRef,
}: {
  /** 要全屏的目标元素（图谱画布容器）；不传则按钮不可用 */
  fullscreenRef?: React.RefObject<HTMLDivElement>;
}) {
  const call = (fn: string, arg?: number) => {
    const g = (window as unknown as Record<string, { zoom?: (f: number) => void; reset?: () => void }>).__piksGraph;
    if (fn === "zoom") g?.zoom?.(arg!);
    if (fn === "reset") g?.reset?.();
  };
  const [isFs, setIsFs] = useState(false);

  // 跟随全屏状态切换图标/文案
  useEffect(() => {
    const onFs = () => setIsFs(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  const toggleFs = () => {
    const el = fullscreenRef?.current;
    if (!el) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      el.requestFullscreen?.().catch(() => {});
    }
  };

  const btn =
    "flex h-8 w-8 items-center justify-center rounded-sm border border-line bg-card text-muted hover:border-accent hover:text-accent";
  return (
    <div className="flex flex-col gap-1.5">
      <button title="放大" onClick={() => call("zoom", 1.25)} className={btn}>
        <ZoomIn size={14} />
      </button>
      <button title="缩小" onClick={() => call("zoom", 0.8)} className={btn}>
        <ZoomOut size={14} />
      </button>
      <button title="重置视图" onClick={() => call("reset")} className={btn}>
        <RotateCcw size={14} />
      </button>
      <button title={isFs ? "退出全屏" : "全屏"} onClick={toggleFs} className={btn}>
        {isFs ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
      </button>
    </div>
  );
}
