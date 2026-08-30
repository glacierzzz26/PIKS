"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight, Wand2 } from "lucide-react";
import { useData } from "@/hooks/useData";
import { useUrlState } from "@/hooks/useUrlState";
import { apiPost, ENDPOINTS } from "@/lib/api";
import MarkdownBody from "@/components/md/MarkdownBody";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import {
  SnapSection,
  EventSection,
  NoteSection,
  TradeSection,
  PositionSection,
} from "@/components/weekly/Sections";
import type { WeeklyDetail } from "@/lib/types";

const GEN_STATUS: Record<string, { tone: "dim" | "amber" | "up" | "down"; label: string }> = {
  ok: { tone: "down", label: "AI 综述已生成" },
  nodata: { tone: "amber", label: "本周无行情/事件/沉淀数据，无可综述" },
  noconfig: { tone: "amber", label: "AI 未配置：请先到设置页填写服务地址与密钥" },
  budget: { tone: "amber", label: "今日 AI 预算已用尽，综述暂缓（预算恢复后重试）" },
  failed: { tone: "up", label: "综述生成失败，请重试" },
};

/** 分段卡片容器 */
function Card({
  title,
  count,
  children,
}: {
  title: string;
  count?: number;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded border border-line bg-card shadow-card">
      <div className="flex h-12 items-center gap-2 border-b border-line px-4">
        <h2 className="card-title mb-0">{title}</h2>
        {count !== undefined && <span className="num text-xs text-muted">{count}</span>}
      </div>
      {children}
    </div>
  );
}

/** 分段空态（小号，保持表格高度节奏） */
function Mini({ label }: { label: string }) {
  return <p className="px-4 py-4 text-[13px] text-muted">{label}</p>;
}

/** 周报（交互）：周导航 + 五段聚合 + AI 综述生成 */
export default function Page() {
  const [query, setParam] = useUrlState();
  const offset = Number(query.offset ?? "0") || 0;
  const [genMsg, setGenMsg] = useState<{ tone: "dim" | "amber" | "up" | "down"; label: string } | null>(null);
  const [generating, setGenerating] = useState(false);

  const { data, loading, error, refresh } = useData<WeeklyDetail>({
    path: ENDPOINTS.weeklyDetail,
    params: { offset: String(offset) },
  });

  const go = (delta: number) => {
    setParam("offset", offset + delta === 0 ? "" : String(offset + delta));
  };

  const generate = async () => {
    setGenerating(true);
    setGenMsg(null);
    try {
      const res = await apiPost<{ status: string }>(
        `${ENDPOINTS.weeklyGenerate}?offset=${offset}`
      );
      setGenMsg(GEN_STATUS[res.status] ?? { tone: "amber", label: `未知状态：${res.status}` });
      refresh();
    } catch (e) {
      setGenMsg({ tone: "up", label: e instanceof Error ? e.message : String(e) });
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">周报</h1>
          <span className="num text-[13px] text-muted">
            {data ? `${data.week}` : "—"}
          </span>
          <span className="ml-auto text-xs text-muted">{data?.range ?? ""}</span>
        </div>
        <div className="mt-3 flex items-center gap-1.5">
          <button
            onClick={() => go(-1)}
            className="inline-flex h-8 items-center gap-1 rounded-sm border border-line bg-card px-2.5 text-xs text-muted hover:text-accent"
          >
            <ChevronLeft size={13} />
            上一周
          </button>
          <Chip tone={offset === 0 ? "accent" : "dim"}>
            {offset === 0 ? "本周" : `往前 ${offset} 周`}
          </Chip>
          <button
            onClick={() => go(1)}
            className="inline-flex h-8 items-center gap-1 rounded-sm border border-line bg-card px-2.5 text-xs text-muted hover:text-accent"
          >
            下一周
            <ChevronRight size={13} />
          </button>
        </div>
      </div>

      {loading ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <LoadingBlock rows={5} />
        </div>
      ) : error ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <ErrorState msg={error} />
        </div>
      ) : !data ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <EmptyState tip="周报数据不可用" />
        </div>
      ) : (
        <div className="mt-4 flex flex-col gap-3.5">
          {/* AI 综述 */}
          <div className="rounded border border-line bg-card shadow-card">
            <div className="flex h-12 items-center gap-2 border-b border-line px-4">
              <h2 className="card-title mb-0">AI 综述</h2>
              {data.summary && (
                <span className="num text-xs text-muted">
                  {data.summary.model} · {data.summary.tokens.toLocaleString()} tokens
                </span>
              )}
              <button
                onClick={generate}
                disabled={generating}
                className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-sm bg-accent px-3 text-xs font-medium text-white no-underline hover:opacity-90 disabled:opacity-50"
              >
                <Wand2 size={13} />
                {generating ? "生成中…" : data.summary ? "重新生成" : "生成 AI 综述"}
              </button>
            </div>
            {genMsg && (
              <div className="flex items-center gap-2 border-b border-line px-4 py-2.5">
                <Chip tone={genMsg.tone}>{genMsg.label}</Chip>
              </div>
            )}
            {data.summary ? (
              <div className="px-5 py-4">
                <MarkdownBody content={data.summary.summary} />
                <p className="num mt-3 border-t border-line pt-2 text-[11px] text-muted">
                  生成于 {data.summary.updated_at}
                </p>
              </div>
            ) : (
              <p className="px-4 py-4 text-[13px] text-muted">{data.summary_note}</p>
            )}
          </div>

          {/* 五段聚合 */}
          <Card title="行情快照" count={data.snaps.length}>
            {data.snaps.length === 0 ? (
              <Mini label="本周无行情快照" />
            ) : (
              <SnapSection snaps={data.snaps} />
            )}
          </Card>

          <div className="grid gap-3.5 lg:grid-cols-2">
            <Card title="本周事件" count={data.events.length}>
              {data.events.length === 0 ? (
                <Mini label="本周无结构化事件" />
              ) : (
                <EventSection events={data.events} />
              )}
            </Card>
            <Card title="本周沉淀" count={data.notes.length}>
              {data.notes.length === 0 ? (
                <Mini label="本周无个人笔记沉淀" />
              ) : (
                <NoteSection notes={data.notes} />
              )}
            </Card>
          </div>

          <Card title="本周交易" count={data.trades.length}>
            {data.trades.length === 0 ? (
              <Mini label="本周无成交记录" />
            ) : (
              <TradeSection trades={data.trades} />
            )}
          </Card>

          <Card title="周末持仓快照" count={data.positions.length}>
            {data.positions.length === 0 ? (
              <Mini label="本周末无持仓快照" />
            ) : (
              <PositionSection positions={data.positions} />
            )}
          </Card>
        </div>
      )}
    </div>
  );
}
