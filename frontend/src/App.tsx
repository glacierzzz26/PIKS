import { Navigate, Route, Routes, useParams } from "react-router-dom";
import AppShell from "@/components/layout/AppShell";
import Watchlist from "@/pages/watchlist";
import Dashboard from "@/pages/dashboard";
import Events from "@/pages/events";
import EventByID from "@/pages/event/[id]";
import Entities from "@/pages/entities";
import Graph from "@/pages/graph";
import Ladder from "@/pages/ladder";
import Flashes from "@/pages/flashes";
import Recon from "@/pages/recon";
import Reviews from "@/pages/reviews";
import Notes from "@/pages/notes";
import NoteNew from "@/pages/note/new";
import NoteDetail from "@/pages/note/[id]";
import NoteEdit from "@/pages/note/[id]/edit";
import Weekly from "@/pages/weekly";
import Trades from "@/pages/trades";
import Chat from "@/pages/chat";
import Settings from "@/pages/settings";
import Research from "@/pages/research";
import Analyst from "@/pages/analyst";
import Stock from "@/pages/stock/[code]";

/**
 * SPA 路由：全部页面（只读分析页 + 交互页）均由 React 提供。
 * 首页 = 自选（/），市场看板迁至 /market（个股轴心 IA，见 phase5/design/frontend-ia.md）。
 * 详情兜底：/events/:id 打开事件抽屉；/entities/:id 重定向到实体库选中；
 * /reviews/:id 由列表页接管（无独立详情）。未知路径回到首页。
 */
export default function App() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Watchlist />} />
        <Route path="/market" element={<Dashboard />} />
        <Route path="/events" element={<Events />} />
        <Route path="/events/:id" element={<EventByID />} />
        <Route path="/entities" element={<Entities />} />
        <Route path="/entities/:id" element={<EntityRedirect />} />
        <Route path="/graph" element={<Graph />} />
        <Route path="/ladder" element={<Ladder />} />
        <Route path="/flashes" element={<Flashes />} />
        <Route path="/recon" element={<Recon />} />
        <Route path="/reviews" element={<Reviews />} />
        <Route path="/notes" element={<Notes />} />
        <Route path="/notes/new" element={<NoteNew />} />
        <Route path="/notes/:id/edit" element={<NoteEdit />} />
        <Route path="/notes/:id" element={<NoteDetail />} />
        <Route path="/weekly" element={<Weekly />} />
        <Route path="/trades" element={<Trades />} />
        <Route path="/research" element={<Analyst />} />
        <Route path="/research/:runId" element={<Research />} />
        <Route path="/stock/:code" element={<Stock />} />
        <Route path="/chat" element={<Chat />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppShell>
  );
}

/** /entities/:id → /entities?id=:id（实体库支持 id 选中并高亮） */
function EntityRedirect() {
  const { id } = useParams();
  return <Navigate to={`/entities?id=${id}`} replace />;
}
