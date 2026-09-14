"use client";

import type { Entity, Relationship } from "@/lib/types";
import { NODE_COLOR, useForceSim } from "./useForceSim";

export { NODE_COLOR };

/**
 * 原生 SVG 力导向图谱（对齐 Go 版 graph.js：Obsidian 风格）。
 * 小圆点节点 + 力模拟收敛，滚轮缩放 / 拖拽平移 / 节点拖拽 / 点选回调。
 * 模拟与交互逻辑抽在 useForceSim（本组件只做视图）。
 */
export default function ForceGraph({
  entities,
  relationships,
  focusId,
  onSelect,
}: {
  entities: Entity[];
  relationships: Relationship[];
  focusId?: string;
  onSelect: (id: string | undefined) => void;
}) {
  const { svgRef, gRef, lineEls, nodeEls, labelEls } = useForceSim({
    entities,
    relationships,
    focusId,
    onSelect,
  });

  return (
    <svg ref={svgRef} className="h-full w-full cursor-grab active:cursor-grabbing">
      <g ref={gRef}>
        {relationships.map((r) => (
          <line
            key={r.id}
            ref={(el) => { if (el) lineEls.current.set(r.from_id + "->" + r.to_id, el); else lineEls.current.delete(r.from_id + "->" + r.to_id); }}
            className="g-edge"
          />
        ))}
        {entities.map((e) => (
          <g key={e.id}>
            <circle
              ref={(el) => { if (el) nodeEls.current.set(e.id, el); else nodeEls.current.delete(e.id); }}
              data-id={e.id}
              className="g-node cursor-pointer"
              style={{ fill: NODE_COLOR[e.type] ?? "var(--muted)", stroke: "var(--card)" }}
            />
            <text
              ref={(el) => { if (el) labelEls.current.set(e.id, el); else labelEls.current.delete(e.id); }}
              className="pointer-events-none fill-[var(--muted)] text-[11.5px]"
            >
              {e.name.length > 16 ? e.name.slice(0, 15) + "…" : e.name}
            </text>
          </g>
        ))}
      </g>
    </svg>
  );
}
