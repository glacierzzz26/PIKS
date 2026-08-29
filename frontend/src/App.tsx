import { useEffect } from "react";
import { Routes, Route, useLocation } from "react-router-dom";
import AppShell from "@/components/layout/AppShell";
import Dashboard from "@/pages/dashboard";
import Events from "@/pages/events";
import Entities from "@/pages/entities";
import Graph from "@/pages/graph";
import Ladder from "@/pages/ladder";
import Flashes from "@/pages/flashes";
import Recon from "@/pages/recon";
import Reviews from "@/pages/reviews";

/**
 * SPA 路由：只注册 8 个只读分析页。
 * 交互页(/notes* /settings /chat /trades* /weekly /events/{id} /entities/{id} /reviews/{id})
 * 由 nginx 反代给 Go HTML 编辑页(见 configs/nginx.conf)，不在此注册；
 * catch-all 兜底做全量跳转，任何未拥有路径都落到 Go(经 nginx)。
 */
export default function App() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/events" element={<Events />} />
        <Route path="/entities" element={<Entities />} />
        <Route path="/graph" element={<Graph />} />
        <Route path="/ladder" element={<Ladder />} />
        <Route path="/flashes" element={<Flashes />} />
        <Route path="/recon" element={<Recon />} />
        <Route path="/reviews" element={<Reviews />} />
        <Route path="*" element={<GoRedirect />} />
      </Routes>
    </AppShell>
  );
}

/** 未注册路径：全量跳转让 nginx/Go 处理(编辑页、详情页、历史书签)。 */
function GoRedirect() {
  const { pathname, search, hash } = useLocation();
  useEffect(() => {
    window.location.assign(pathname + search + hash);
  }, [pathname, search, hash]);
  return null;
}
