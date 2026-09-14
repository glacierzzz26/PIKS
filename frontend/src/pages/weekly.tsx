"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useData } from "@/hooks/useData";
import { useUrlState } from "@/hooks/useUrlState";
import { apiPost, ENDPOINTS } from "@/lib/api";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import SummaryPanel, { type GenMsg } from "@/components/weekly/SummaryPanel";
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
    <div className="panel">
      <div className="flex h-12 items-center gap-2 border-b border-line px-5">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">{title}</h2>
        {count !== undefined && <span className="num text-xs text-faint">{count}</span>}
      </div>
      {children}
    </div>
  );
}

/** 分段空态（小号，保持表格高度节奏） */
function Mini({ label }: { label: string }) {
  return <p className="px-4 py-4 text-[13px] text-faint">{label}</p>;
}

/** 周报（交互）：周导航 + 五段聚合 + AI 综述生成 */
export default function Page() {
  const [query, setParam] = useUrlState();
  const offset = Number(query.offset ?? "0") || 0;
  const [genMsg, setGenMsg] = useState<GenMsg | null>(null);
  const [generating, setGenerating] = useState(false);

  const { data, loading, error, refresh } = useData<WeeklyDetail>({
    path: ENDPOINTS.weeklyDetail,
    params: { offset: String(offset) },
  });

  const go = (delta: number) => {
    setParam("offset", offset + delta === 0 ? "" : String(offset + delta));
  };

  // 后端空周聚合返回 null（Go nil slice）——统一兜底成空数组，避免 .length 崩页（同其它页）。
  const snaps = data?.snaps ?? [];
  const events = data?.events ?? [];
  const notes = data?.notes ?? [];
  const trades = data?.trades ?? [];
  const positions = data?.positions ?? [];

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
      <div className="page-head">
        <div>
          <h1>周报</h1>
          <div className="psub">
            汇总本周的快照 / 事件 / 笔记 / 交易，可让 AI 写一段综述
          </div>
        </div>
        <div className="meta">
          <button
            onClick={() => go(-1)}
            className="inline-flex h-8 items-center gap-1 rounded-[10px] border border-line bg-card px-2.5 text-xs text-muted hover:text-accent"
          >
            <ChevronLeft size={13} />
            上一周
          </button>
          <Chip tone={offset === 0 ? "accent" : "dim"}>
            {offset === 0 ? "本周" : `往前 ${offset} 周`}
          </Chip>
          <button
            onClick={() => go(1)}
            className="inline-flex h-8 items-center gap-1 rounded-[10px] border border-line bg-card px-2.5 text-xs text-muted hover:text-accent"
          >
            下一周
            <ChevronRight size={13} />
          </button>
        </div>
      </div>

      {loading ? (
        <div className="panel">
          <LoadingBlock rows={5} />
        </div>
      ) : error ? (
        <div className="panel">
          <ErrorState msg={error} />
        </div>
      ) : !data ? (
        <div className="panel">
          <EmptyState tip="周报数据不可用" />
        </div>
      ) : (
        <div className="flex flex-col gap-3.5">
          {/* AI 综述 */}
          <SummaryPanel
            data={data}
            generating={generating}
            genMsg={genMsg}
            onGenerate={generate}
          />

          {/* 五段聚合 */}
          <Card title="行情快照" count={snaps.length}>
            {snaps.length === 0 ? (
              <Mini label="本周无行情快照" />
            ) : (
              <SnapSection snaps={snaps} />
            )}
          </Card>

          <div className="grid gap-3.5 lg:grid-cols-2">
            <Card title="本周事件" count={events.length}>
              {events.length === 0 ? (
                <Mini label="本周无结构化事件" />
              ) : (
                <EventSection events={events} />
              )}
            </Card>
            <Card title="本周沉淀" count={notes.length}>
              {notes.length === 0 ? (
                <Mini label="本周无个人笔记沉淀" />
              ) : (
                <NoteSection notes={notes} />
              )}
            </Card>
          </div>

          <Card title="本周交易" count={trades.length}>
            {trades.length === 0 ? (
              <Mini label="本周无成交记录" />
            ) : (
              <TradeSection trades={trades} />
            )}
          </Card>

          <Card title="周末持仓快照" count={positions.length}>
            {positions.length === 0 ? (
              <Mini label="本周末无持仓快照" />
            ) : (
              <PositionSection positions={positions} />
            )}
          </Card>
        </div>
      )}
    </div>
  );
}
