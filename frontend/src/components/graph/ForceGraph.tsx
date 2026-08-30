"use client";

import { useEffect, useRef } from "react";
import type { Entity, Relationship } from "@/lib/types";

/**
 * 原生 SVG 力导向图谱（对齐 Go 版 graph.js：Obsidian 风格）。
 * 小圆点节点 + 力模拟收敛，滚轮缩放 / 拖拽平移 / 节点拖拽 / 点选回调。
 * 渲染走 ref 直改 DOM 属性，不经 React 状态，保证 60fps。
 */

const REP = 2000, SPR = 0.02, DIST = 132, GRAV = 0.006, DAMP = 0.84;

export const NODE_COLOR: Record<string, string> = {
  company: "var(--green)",
  industry: "var(--amber)",
  concept: "var(--red)",
  person: "var(--accent)",
  region: "var(--muted)",
};

type P = { id: string; label: string; type: string; x: number; y: number; vx: number; vy: number };

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
  const svgRef = useRef<SVGSVGElement>(null);
  const gRef = useRef<SVGGElement>(null);
  const lineEls = useRef(new Map<string, SVGLineElement>());
  const nodeEls = useRef(new Map<string, SVGCircleElement>());
  const labelEls = useRef(new Map<string, SVGTextElement>());
  const sim = useRef({
    nodes: [] as P[],
    byId: new Map<string, P>(),
    edges: [] as { s: string; t: string }[],
    sel: null as P | null,
    scale: 1, tx: 0, ty: 0, alpha: 1, running: false,
    W: 820, H: 520,
  });

  /* 场景构建 + 力模拟 */
  useEffect(() => {
    const svg = svgRef.current, g = gRef.current, s = sim.current;
    if (!svg || !g) return;
    s.W = svg.clientWidth || 820;
    s.H = svg.clientHeight || 520;
    s.nodes = entities.map((e) => ({ id: e.id, label: e.name, type: e.type, x: 0, y: 0, vx: 0, vy: 0 }));
    s.byId = new Map(s.nodes.map((p) => [p.id, p]));
    s.edges = relationships
      .filter((r) => s.byId.has(r.from_id) && s.byId.has(r.to_id))
      .map((r) => ({ s: r.from_id, t: r.to_id }));
    const cx = s.W / 2, cy = s.H / 2;
    s.nodes.forEach((p, i) => {
      const a = (i / Math.max(1, s.nodes.length)) * Math.PI * 2;
      const r = Math.min(s.W, s.H) * 0.34 * (0.5 + (i % 6) / 6);
      p.x = cx + Math.cos(a) * r;
      p.y = cy + Math.sin(a) * r;
      p.vx = Math.random() - 0.5;
      p.vy = Math.random() - 0.5;
    });
    // 注意:不能再清空 lineEls/nodeEls/labelEls —— 它们由下方 ref 回调在
    // React commit 阶段填充,effect 在 commit 之后执行,clear() 会把刚填好的
    // Map 清空,导致 render() 遍历空 Map、节点永远停在 (0,0)(图谱不可点/不定位)。
    // 数据变化时由 ref 回调的 null 分支同步增删,天然对齐当前实体集。
    s.scale = 1; s.tx = 0; s.ty = 0; s.sel = null;
    restart();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entities, relationships]);

  /* 聚焦：居中 + 高亮 */
  useEffect(() => {
    const s = sim.current;
    s.sel = focusId ? s.byId.get(focusId) ?? null : null;
    if (s.sel) centerOn(s.sel.x, s.sel.y);
    render();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId]);

  function applyViewport() {
    const { scale, tx, ty } = sim.current;
    gRef.current?.setAttribute("transform", `translate(${tx},${ty}) scale(${scale})`);
  }

  function render() {
    const s = sim.current;
    applyViewport();
    for (const e of s.edges) {
      const el = lineEls.current.get(e.s + "->" + e.t);
      const a = s.byId.get(e.s), b = s.byId.get(e.t);
      if (!el || !a || !b) continue;
      el.setAttribute("x1", String(a.x));
      el.setAttribute("y1", String(a.y));
      el.setAttribute("x2", String(b.x));
      el.setAttribute("y2", String(b.y));
      const hot = s.sel && (e.s === s.sel.id || e.t === s.sel.id);
      el.style.stroke = hot ? "var(--accent)" : "";
      el.style.strokeWidth = hot ? "2" : "1";
      el.style.opacity = hot ? "1" : ".65";
    }
    const showLabel = s.scale >= 0.62;
    for (const [id, el] of nodeEls.current) {
      const p = s.byId.get(id)!;
      el.setAttribute("cx", String(p.x));
      el.setAttribute("cy", String(p.y));
      el.style.opacity = s.sel && s.sel.id !== id ? "0.28" : "1";
      if (s.sel?.id === id) {
        el.setAttribute("r", "7.5");
        el.style.strokeWidth = "2.4";
      } else {
        el.setAttribute("r", "5.5");
        el.style.strokeWidth = "1.6";
      }
    }
    for (const [id, el] of labelEls.current) {
      const p = s.byId.get(id)!;
      el.setAttribute("x", String(p.x + 9));
      el.setAttribute("y", String(p.y + 3.5));
      el.setAttribute("visibility", showLabel ? "visible" : "hidden");
    }
  }

  function step() {
    const s = sim.current, n = s.nodes.length;
    for (let i = 0; i < n; i++)
      for (let j = i + 1; j < n; j++) {
        const a = s.nodes[i], b = s.nodes[j];
        let dx = a.x - b.x, dy = a.y - b.y, d2 = dx * dx + dy * dy;
        if (d2 < 1) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 1; }
        const d = Math.sqrt(d2), f = (REP / (d2 + 4)) * s.alpha;
        a.vx += (dx / d) * f; a.vy += (dy / d) * f;
        b.vx -= (dx / d) * f; b.vy -= (dy / d) * f;
      }
    for (const e of s.edges) {
      const a = s.byId.get(e.s), b = s.byId.get(e.t);
      if (!a || !b) continue;
      const dx = b.x - a.x, dy = b.y - a.y, d = Math.sqrt(dx * dx + dy * dy) || 1;
      const f = (d - DIST) * SPR * s.alpha;
      a.vx += (dx / d) * f; a.vy += (dy / d) * f;
      b.vx -= (dx / d) * f; b.vy -= (dy / d) * f;
    }
    for (const p of s.nodes) {
      p.vx += (s.W / 2 - p.x) * GRAV * s.alpha;
      p.vy += (s.H / 2 - p.y) * GRAV * s.alpha;
      p.vx *= DAMP; p.vy *= DAMP;
      p.x += p.vx; p.y += p.vy;
    }
  }

  function loop() {
    const s = sim.current;
    if (!s.running) return;
    step();
    s.alpha *= 0.985;
    render();
    if (s.alpha < 0.03) { s.running = false; s.alpha = 0; return; }
    requestAnimationFrame(loop);
  }

  function restart() {
    const s = sim.current;
    s.alpha = 1;
    if (!s.running) { s.running = true; requestAnimationFrame(loop); }
  }

  function zoomAt(sx: number, sy: number, factor: number) {
    const s = sim.current;
    const ns = Math.max(0.2, Math.min(3.2, s.scale * factor));
    const k = ns / s.scale;
    s.tx = sx - (sx - s.tx) * k;
    s.ty = sy - (sy - s.ty) * k;
    s.scale = ns;
    render();
  }

  function centerOn(x: number, y: number) {
    const s = sim.current;
    s.tx = s.W / 2 - x * s.scale;
    s.ty = s.H / 2 - y * s.scale;
  }

  /* 交互：滚轮缩放 / 拖拽平移 / 节点拖拽 / 点选 */
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    let drag: P | null = null, pan: { x: number; y: number } | null = null, down: { x: number; y: number } | null = null;

    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = svg.getBoundingClientRect();
      zoomAt(e.clientX - r.left, e.clientY - r.top, e.deltaY < 0 ? 1.15 : 0.87);
    };
    const onDown = (e: MouseEvent) => {
      down = { x: e.clientX, y: e.clientY };
      const t = e.target as Element;
      const id = t.getAttribute?.("data-id");
      if (id) {
        const p = sim.current.byId.get(id) ?? null;
        if (p) { drag = p; sim.current.sel = p; onSelectRef.current?.(p.id); render(); }
      } else pan = { x: e.clientX, y: e.clientY };
    };
    const onMove = (e: MouseEvent) => {
      const s = sim.current, r = svg.getBoundingClientRect();
      if (drag) {
        drag.x = (e.clientX - r.left - s.tx) / s.scale;
        drag.y = (e.clientY - r.top - s.ty) / s.scale;
        drag.vx = 0; drag.vy = 0;
        render(); restart();
      } else if (pan) {
        s.tx += e.clientX - pan.x;
        s.ty += e.clientY - pan.y;
        pan = { x: e.clientX, y: e.clientY };
        applyViewport();
      }
    };
    const onUp = (e: MouseEvent) => {
      const moved = down && Math.hypot(e.clientX - down.x, e.clientY - down.y) > 5;
      if (!drag && !moved) { sim.current.sel = null; onSelectRef.current?.(undefined); }
      drag = null; pan = null; down = null;
    };
    svg.addEventListener("wheel", onWheel, { passive: false });
    svg.addEventListener("mousedown", onDown);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      svg.removeEventListener("wheel", onWheel);
      svg.removeEventListener("mousedown", onDown);
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;

  /* 暴露缩放按钮 */
  useEffect(() => {
    (window as unknown as Record<string, unknown>).__piksGraph = {
      zoom: (f: number) => zoomAt(sim.current.W / 2, sim.current.H / 2, f),
      reset: () => { sim.current.scale = 1; sim.current.tx = 0; sim.current.ty = 0; render(); },
    };
  }, []);

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
