"use client";

import { useMemo, useRef, useState } from "react";
import { useData } from "@/hooks/useData";
import { ENTITY_TYPES } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import { ENTITY_TYPE_LABEL } from "@/lib/format";
import type { Entity, Relationship } from "@/lib/types";
import ForceGraph, { NODE_COLOR } from "@/components/graph/ForceGraph";
import GraphPanel, { GraphActions } from "@/components/graph/GraphPanel";
import { EmptyState, ErrorState } from "@/components/ui/States";

/**
 * 图谱页：原生 SVG 力导向图 + 类型筛选 + 搜索 + 详情面板。
 * 注意：图谱内容/交互逻辑保持原状，仅外层容器与工具栏换新风格。
 */
export default function Page() {
  const entities = useData<Entity[]>({ path: ENDPOINTS.entities });
  const rels = useData<Relationship[]>({ path: ENDPOINTS.relationships });

  const [type, setType] = useState("");
  const [focusId, setFocusId] = useState<string | undefined>();
  const [kw, setKw] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const all = entities.data ?? [];
  const relsAll = rels.data ?? [];

  const { nodes, edges } = useMemo(() => {
    const ids =
      type === "" ? null : new Set(all.filter((e) => e.type === type).map((e) => e.id));
    const nodes = ids ? all.filter((e) => ids.has(e.id)) : all;
    const idset = new Set(nodes.map((e) => e.id));
    const edges = relsAll.filter((r) => idset.has(r.from_id) && idset.has(r.to_id));
    return { nodes, edges };
  }, [all, relsAll, type]);

  const focus = focusId ? all.find((e) => e.id === focusId) : undefined;

  const doSearch = () => {
    const q = kw.trim().toLowerCase();
    if (!q) return;
    const hit = nodes.find((e) => e.name.toLowerCase().includes(q));
    if (hit) setFocusId(hit.id);
  };

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>图谱</h1>
          <div className="psub">实体关系力导向图 · 拖拽平移 / 滚轮缩放 / 点击查看</div>
        </div>
        <div className="meta">
          <span className="st st-accent">节点 {nodes.length}</span>
          <span className="st st-dim">关系 {edges.length}</span>
        </div>
      </div>

      <div className="filter-bar">
        {ENTITY_TYPES.map((t) => (
          <button
            key={t.key}
            onClick={() => {
              setType(t.key);
              setFocusId(undefined);
            }}
            className={`chip-btn ${type === t.key ? "on" : ""}`}
          >
            {t.label}
          </button>
        ))}
        <form
          className="f-search"
          onSubmit={(e) => {
            e.preventDefault();
            doSearch();
          }}
        >
          <input
            ref={searchRef}
            value={kw}
            onChange={(e) => setKw(e.target.value)}
            placeholder="搜索节点，回车聚焦…"
          />
        </form>
      </div>

      <div
        ref={containerRef}
        className="graph-canvas panel relative h-[calc(100vh-190px)] min-h-[460px] overflow-hidden"
      >
        {entities.error || rels.error ? (
          <ErrorState msg={entities.error ?? rels.error ?? ""} />
        ) : entities.data && rels.data ? (
          <ForceGraph
            entities={nodes}
            relationships={edges}
            focusId={focusId}
            onSelect={setFocusId}
          />
        ) : (
          <EmptyState tip="加载中…" />
        )}

        {/* 图例 */}
        <div className="pointer-events-none absolute left-4 top-3 flex flex-col gap-1 text-xs text-muted">
          {Object.entries(ENTITY_TYPE_LABEL).map(([k, label]) => (
            <span key={k} className="flex items-center gap-1.5">
              <i
                className="inline-block h-2.5 w-2.5 rounded-[4px]"
                style={{ background: NODE_COLOR[k] ?? "var(--muted)" }}
              />
              {label}
            </span>
          ))}
        </div>
        <span className="pointer-events-none absolute bottom-3 left-4 rounded-full border border-line bg-card px-2.5 py-0.5 text-xs text-faint">
          拖拽平移 · 滚轮缩放 · 拖动节点 · 点击查看
        </span>
        <div className="absolute right-3 top-12">
          <GraphActions fullscreenRef={containerRef} />
        </div>

        {focus && (
          <GraphPanel
            entity={focus}
            relationships={edges}
            entities={all}
            onClose={() => setFocusId(undefined)}
            onFocus={setFocusId}
          />
        )}
      </div>
    </div>
  );
}
