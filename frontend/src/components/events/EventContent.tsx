"use client";

import { Link } from "react-router-dom";
import type { EventItem } from "@/lib/types";
import { Chip } from "@/components/ui/Num";

type Affected = EventItem["affected"][number];

/**
 * 事实 + 影响实体两块（issue #83 P-4 / P8 从 EventDetail 拆出，守「≤150 行」硬规则）。
 *
 * 展示单元 = 簇：成员的 facts/affected **并集**在 `cluster_facts` / `cluster_affected`，
 * canonical 单条的子集在 `facts` / `affected`。有并集就用并集（那是整件事的全貌），
 * 无并集（未聚类/单成员）回落到本事件自己的。
 *
 * ⚠️ 并集是**字面去重**，不做模糊归并 —— 数字分歧不走这里静默择一，而是由
 * `EventConflicts` 单独暴露（两条都在）。
 */
export default function EventContent({ event }: { event: EventItem }) {
  const clustered = (event.cluster_facts?.length ?? 0) > 0;
  const facts = clustered ? event.cluster_facts! : event.facts;
  const affected = clustered ? event.cluster_affected ?? [] : event.affected;

  return (
    <>
      <Section title="事实（Fact）">
        {clustered && (
          <div className="mb-1.5 text-xs text-faint">
            以下是这件事**各家报道**的事实汇总（不是单条）。
          </div>
        )}
        <ul className="m-0 list-disc pl-5">
          {facts.map((f, i) => (
            <li key={i} className="mb-1.5 text-sm leading-relaxed">
              {f}
            </li>
          ))}
        </ul>
      </Section>

      <Section title="影响实体">
        <div className="flex flex-wrap gap-1.5">
          {affected.map((a, i) =>
            a.code ? (
              <Link key={i} to={`/stock/${a.code}`} className="st st-accent no-underline">
                {a.entity_name ?? a.word}
              </Link>
            ) : (
              <Chip key={i} tone="accent">
                {a.entity_name ?? a.word}
              </Chip>
            )
          )}
        </div>
      </Section>
    </>
  );
}

export function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mt-5 border-t border-line pt-3.5">
      <h3 className="mb-2 text-[15px] font-bold">{title}</h3>
      <div className="text-sm leading-relaxed">{children}</div>
    </div>
  );
}

export type { Affected };
