"use client";

import { useNavigate, useParams } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import EventDetail from "@/components/events/EventDetail";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { EventItem } from "@/lib/types";

/** /events/:id 兜底路由：按 id 打开事件详情抽屉（对齐原 Go 详情页入口） */
export default function Page() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const events = useData<EventItem[]>({ path: ENDPOINTS.events });
  const event = events.data?.find((e) => e.id === id) ?? null;

  if (events.loading) {
    return (
      <div className="mt-10 rounded border border-line bg-card shadow-card">
        <LoadingBlock rows={3} />
      </div>
    );
  }
  if (events.error) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <ErrorState msg={events.error} />
      </div>
    );
  }
  if (!event) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <EmptyState tip="事件不存在或已被归档" />
      </div>
    );
  }
  return <EventDetail event={event} onClose={() => navigate("/events")} />;
}
