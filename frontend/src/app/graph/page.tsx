"use client";

import { useMemo, useRef, useState } from "react";
import { useData } from "@/hooks/useData";
import { getEntities, getRelationships } from "@/lib/mockService";
import { ENTITY_TYPES } from "@/lib/mock/entities";
import { ENDPOINTS } from "@/lib/api";
import { ENTITY_TYPE_LABEL } from "@/lib/format";
import ForceGraph, { NODE_COLOR } from "@/components/graph/ForceGraph";
import GraphPanel, { GraphActions } from "@/components/graph/GraphPanel";
import { Chip } from "@/components/ui/Num";
import { EmptyState } from "@/components/ui/States";

/** 图谱页（对齐 Go 版 /graph）：原生 SVG 力导向图 + 搜索 + 类型筛选 + 详情面板 */
export default function Page() {
  const entities = useData({ path: ENDPOINTS.entities, fallback: () => getEntities({}) });
  const rels = useData({ path: ENDPOINTS.relationships, fallback: getRelationships });

  const [type, setType] = useState("");
  const [focusId, setFocusId] = useState<string | undefined>();
  const [kw, setKw] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);

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
      <div className="mb-1 mt-5">
        <h1 className="mb-0 text-2xl font-bold tracking-wide">图谱</h1>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        {ENTITY_TYPES.map((t) => (
          <button
            key={t.key}
            onClick={() => {
              setType(t.key);
              setFocusId(undefined);
            }}
            className={`chip ${type === t.key ? "chip-accent" : "chip-dim"}`}
          >
            {t.label}
          </button>
        ))}
        <form
          className="ml-auto flex h-8 w-[200px] items-center gap-2 rounded-sm border border-line bg-card px-2.5 focus-within:border-accent"
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
            className="w-full bg-transparent text-xs outline-none placeholder:text-muted"
          />
        </form>
      </div>

      <div className="relative mt-3 h-[calc(100vh-240px)] min-h-[440px] overflow-hidden rounded border border-line bg-card p-0 shadow-card">
        {entities.data && rels.data ? (
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
                className="inline-block h-2 w-2 rounded-full"
                style={{ background: NODE_COLOR[k] ?? "var(--muted)" }}
              />
              {label}
            </span>
          ))}
        </div>
        <span className="pointer-events-none absolute bottom-3 left-4 rounded-full border border-line bg-card px-2.5 py-0.5 text-xs text-muted">
          拖拽平移 · 滚轮缩放 · 拖动节点 · 点击查看
        </span>
        <div className="absolute right-3 top-12">
          <GraphActions />
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
