"use client";

import { useMemo, Suspense, useState } from "react";
import { Search } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { ENTITY_TYPES } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import { ENTITY_TYPE_LABEL } from "@/lib/format";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Entity } from "@/lib/types";

const TYPE_TONE: Record<string, "accent" | "down" | "amber" | "up"> = {
  company: "accent",
  industry: "down",
  concept: "amber",
  person: "up",
};

/** 实体库：类型/关键词筛选 + 分页列表 + 详情（图谱见「图谱」页） */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6"><LoadingBlock rows={8} /></div>}>
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
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">实体库</h1>
          <span className="num text-[13px] text-muted">
            {entities.loading ? "" : `${data.length} 个实体`}
          </span>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          {ENTITY_TYPES.map((t) => (
            <button
              key={t.key}
              onClick={() => setFilter("type", t.key)}
              className={`chip ${
                (query.type ?? "") === t.key ? "chip-accent" : "chip-dim"
              }`}
            >
              {t.label}
            </button>
          ))}
          <form
            className="ml-auto flex h-8 w-[220px] items-center gap-2 rounded-sm border border-line bg-card px-2.5 focus-within:border-accent"
            onSubmit={(e) => {
              e.preventDefault();
              setFilter("q", kw.trim());
            }}
          >
            <Search size={13} className="shrink-0 text-muted" />
            <input
              value={kw}
              onChange={(e) => setKw(e.target.value)}
              placeholder="搜索实体 / 别名…"
              className="w-full bg-transparent text-xs outline-none placeholder:text-muted"
            />
          </form>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-12 gap-3.5">
        <div className="col-span-12 overflow-hidden rounded border border-line bg-card shadow-card lg:col-span-5">
          {entities.loading ? (
            <LoadingBlock rows={10} />
          ) : entities.error ? (
            <ErrorState msg={entities.error} />
          ) : data.length === 0 ? (
            <EmptyState tip="没有匹配的实体" />
          ) : (
            <>
              <ul className="divide-y divide-line">
                {paged.map((e) => (
                  <li key={e.id}>
                    <button
                      onClick={() => setFilter("id", e.id)}
                      className={`flex h-[52px] w-full items-center gap-3 px-4 text-left ${
                        selectedId === e.id ? "bg-accent-soft" : "hover:bg-bg-soft"
                      }`}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="truncate text-sm font-semibold">
                            {e.name}
                          </span>
                          <Chip tone={TYPE_TONE[e.type] ?? "dim"}>
                            {ENTITY_TYPE_LABEL[e.type]}
                          </Chip>
                        </div>
                        <div className="truncate text-xs text-muted">
                          {e.aliases.join(" · ") || e.description}
                        </div>
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
              <Pagination
                page={page}
                pageSize={size}
                total={data.length}
                onPage={setPage}
                onPageSize={setSize}
              />
            </>
          )}
        </div>

        <div className="col-span-12 lg:col-span-7">
          <div className="rounded border border-line bg-card p-5 shadow-card">
            {selected ? (
              <div>
                <div className="flex items-center gap-2.5">
                  <h3 className="m-0 text-lg font-bold">{selected.name}</h3>
                  <Chip tone={TYPE_TONE[selected.type] ?? "dim"}>
                    {ENTITY_TYPE_LABEL[selected.type]}
                  </Chip>
                  {selected.status === "watch" && <Chip tone="amber">观察</Chip>}
                </div>
                {selected.aliases.length > 0 && (
                  <div className="mt-1 text-xs text-muted">
                    别名：{selected.aliases.join(" / ")}
                  </div>
                )}
                <p className="mt-3 text-sm leading-relaxed">
                  {selected.description}
                </p>
                <div className="num mt-3 text-left text-xs text-muted">
                  更新于 {selected.updated_at}
                </div>
              </div>
            ) : (
              <div className="flex h-[120px] items-center justify-center text-[13px] italic text-muted">
                点击左侧实体查看详情；关系结构见「图谱」页
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
