import { Navigate, Outlet, Route, Routes, useParams } from "react-router-dom";
import AppShell from "@/components/layout/AppShell";
import Watchlist from "@/pages/home";
import Dashboard from "@/pages/dashboard";
import Events from "@/pages/events";
import EventByID from "@/pages/event/[id]";
import Entities from "@/pages/entities";
import Graph from "@/pages/graph";
import Ladder from "@/pages/ladder";
import Messages from "@/pages/messages";
import Help from "@/pages/help";
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
import Reports from "@/pages/reports";
import ReportDetail from "@/pages/report/[runId]";
import Analyst from "@/pages/analyst";
import Stock from "@/pages/stock/[code]";
import MobileUpload from "@/pages/m/upload";

/** 桌面外壳：左侧栏 + 内容区（所有常规页共用）。 */
function ShellLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}

/**
 * SPA 路由：全部页面（只读分析页 + 交互页）均由 React 提供。
 * 首页 = 今天（自选 + 今天该看什么，/）；市场看板在 /market（个股轴心 IA）。
 * 消息页 = 重要消息(事件) + 快讯双 tab（P6-2），/flashes 旧深链落到快讯 tab。
 * 详情兜底：/events/:id 打开事件抽屉；/entities/:id 重定向到实体库选中；
 * /reviews/:id 由列表页接管（无独立详情）。未知路径回到首页。
 *
 * 手机投递页 `/m/upload` 在 AppShell 布局之外（无侧栏）：用无路径 layout route
 * 包住全部桌面页，/m/upload 作为兄弟路由挂在外层——静态段在 v6 路由排名中胜出，
 * 不会落进 AppShell 的 `*` 兜底，也不会双渲染出侧栏。
 */
export default function App() {
  return (
    <Routes>
      <Route element={<ShellLayout />}>
        <Route path="/" element={<Watchlist />} />
        <Route path="/market" element={<Dashboard />} />
        <Route path="/events" element={<Events />} />
        <Route path="/events/:id" element={<EventByID />} />
        <Route path="/entities" element={<Entities />} />
        <Route path="/entities/:id" element={<EntityRedirect />} />
        <Route path="/graph" element={<Graph />} />
        <Route path="/ladder" element={<Ladder />} />
        {/* /flashes 旧深链：落到消息页快讯 tab（P6-2 合并，保留路由不断链） */}
        <Route path="/flashes" element={<Messages />} />
        <Route path="/help" element={<Help />} />
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
        {/* 研报体裁（P9-2 §6）：独立入口，与个股分析并存不互替（D-R1） */}
        <Route path="/reports" element={<Reports />} />
        <Route path="/reports/:runId" element={<ReportDetail />} />
        <Route path="/stock/:code" element={<Stock />} />
        <Route path="/chat" element={<Chat />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>

      {/* 手机投递页：无侧栏，独立布局 */}
      <Route path="/m" element={<Navigate to="/m/upload" replace />} />
      <Route path="/m/upload" element={<MobileUpload />} />
    </Routes>
  );
}

/** /entities/:id → /entities?id=:id（实体库支持 id 选中并高亮） */
function EntityRedirect() {
  const { id } = useParams();
  return <Navigate to={`/entities?id=${id}`} replace />;
}
