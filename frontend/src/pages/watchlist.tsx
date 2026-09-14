"use client";

import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import WatchOverview from "@/components/watch/WatchOverview";
import WatchTable from "@/components/watch/WatchTable";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Watchlist } from "@/lib/types";

/** 首页：我的自选。自选列表 + 顶部紧凑市场概览条（设计 frontend-ia §2.3）。 */
export default function Page() {
  const wl = useData<Watchlist>({ path: ENDPOINTS.watchlist });
  const items = wl.data?.items ?? [];

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>我的自选</h1>
          <div className="psub">关注与持仓一屏尽览，点代码进入个股中心</div>
        </div>
        <div className="meta">
          <span className="st st-accent">共 {wl.loading ? "…" : items.length} 只</span>
          <span className="st st-dim">
            同花顺自选镜像同步
          </span>
        </div>
      </div>

      <WatchOverview />

      {wl.loading ? (
        <div className="panel">
          <LoadingBlock rows={6} />
        </div>
      ) : wl.error ? (
        <div className="panel">
          <ErrorState msg={wl.error} />
        </div>
      ) : items.length === 0 ? (
        <div className="panel">
          <EmptyState tip="自选为空 —— 自选通过同花顺自选截图镜像同步到 PIKS" />
        </div>
      ) : (
        <div className="panel">
          <WatchTable items={items} />
        </div>
      )}

      <div className="mt-3 text-[12px] text-faint">
        想找一只没在自选里的票？到
        <Link to="/entities" className="mx-1 no-underline hover:text-accent">
          实体库
        </Link>
        查代码，或
        <Link to="/ladder" className="mx-1 no-underline hover:text-accent">
          涨停梯队
        </Link>
        看当日强势股。
      </div>
    </div>
  );
}
