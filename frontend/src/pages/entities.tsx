"use client";

import { useMemo, Suspense, useState } from "react";
import { Search, X } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { ENTITY_TYPES } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import { ENTITY_TYPE_LABEL } from "@/lib/format";
import Pagination from "@/components/ui/Pagination";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import DeepResearchButton from "@/components/research/DeepResearchButton";
import type { Entity } from "@/lib/types";

const TYPE_ST: Record<string, string> = {
  company: "st-accent",
  industry: "t-idx",
  concept: "t-ev",
  person: "st-up",
  region: "st-dim",
};

/** 实体库：类型/关键词筛选 + 卡片网格 + 详情（关系结构见「图谱」页） */
export default function Page() {
  return (
    <Suspense fallback={<div className="panel mt-6"><LoadingBlock rows={8} /></div>}>
      <EntitiesInner />
    </Suspense>
  );
}

function EntitiesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const [kw, setKw] = useState(query.q ?? "");
  const selectedId = query.id ?? "";

  const entities = useData<Entity[]>({
    path: ENDPOINTS.entities,
    params: { type: query.type, q: query.q },
  });
  const data = entities.data ?? [];
  const paged = paginate(data);
  const selected = useMemo(
    () => data.find((e) => e.id === selectedId) ?? null,
    [data, selectedId]
  );

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>实体库</h1>
          <div className="psub">
            从事件中构建的投资对象档案 · 公司 / 行业 / 概念 / 人物 / 地域
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">
            共 {entities.loading ? "…" : data.length} 个
          </span>
        </div>
      </div>

      <div className="filter-bar">
        {ENTITY_TYPES.map((t) => (
          <button
            key={t.key}
            onClick={() => setFilter("type", t.key)}
            className={`chip-btn ${(query.type ?? "") === t.key ? "on" : ""}`}
          >
            {t.label}
          </button>
        ))}
        <form
          className="f-search"
          onSubmit={(e) => {
            e.preventDefault();
            setFilter("q", kw.trim());
          }}
        >
          <Search size={15} className="text-faint" strokeWidth={2} />
          <input
            value={kw}
            onChange={(e) => setKw(e.target.value)}
            placeholder="搜索实体名称 / 别名…"
          />
          {query.q && (
            <button
              type="button"
              onClick={() => {
                setKw("");
                setFilter("q", "");
              }}
              className="text-faint hover:text-up"
            >
              <X size={13} />
            </button>
          )}
        </form>
      </div>

      {entities.loading ? (
        <div className="panel">
          <LoadingBlock rows={8} />
        </div>
      ) : entities.error ? (
        <div className="panel">
          <ErrorState msg={entities.error} />
        </div>
      ) : data.length === 0 ? (
        <div className="panel">
          <EmptyState tip="没有匹配的实体" />
        </div>
      ) : (
        <>
          <div className="ent-grid">
            {paged.map((e) => (
              <div
                key={e.id}
                role="button"
                tabIndex={0}
                onClick={() => setFilter("id", e.id)}
                onKeyDown={(ev) => {
                  if (ev.key === "Enter" || ev.key === " ") {
                    ev.preventDefault();
                    setFilter("id", e.id);
                  }
                }}
                className={`ent-card text-left ${
                  selectedId === e.id ? "border-accent" : ""
                }`}
              >
                <div className="eh">
                  <b>{e.name}</b>
                  <span className={`st ${TYPE_ST[e.type] ?? "st-dim"}`}>
                    {ENTITY_TYPE_LABEL[e.type]}
                  </span>
                  {e.status === "watch" && <span className="st st-amber">关注</span>}
                </div>
                <div className="aliases">
                  别名：{e.aliases.join(" · ") || "—"}
                </div>
                <div className="desc">{e.description}</div>
                <div className="efoot">
                  <span>更新 <b>{e.updated_at}</b></span>
                  {/* 仅带股票代码的公司实体提供深研入口（detail.code 归一） */}
                  {e.code && (
                    <span onClick={(ev) => ev.stopPropagation()}>
                      <DeepResearchButton code={e.code} />
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
          <Pagination
            page={page}
            pageSize={size}
            total={data.length}
            onPage={setPage}
            onPageSize={setSize}
          />
        </>
      )}

      {selected && (
        <div className="mt-4">
          <div className="panel panel-pad">
            <div className="flex items-center gap-2.5">
              <h3 className="m-0 text-lg font-bold">{selected.name}</h3>
              <span className={`st ${TYPE_ST[selected.type] ?? "st-dim"}`}>
                {ENTITY_TYPE_LABEL[selected.type]}
              </span>
              {selected.status === "watch" && <span className="st st-amber">观察</span>}
            </div>
            {selected.aliases.length > 0 && (
              <div className="mt-1 text-xs text-faint">
                别名：{selected.aliases.join(" / ")}
              </div>
            )}
            <p className="mt-3 text-sm leading-relaxed text-muted">
              {selected.description}
            </p>
            <div className="num mt-3 text-left text-xs text-faint">
              更新于 {selected.updated_at}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
