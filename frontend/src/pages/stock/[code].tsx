"use client";

import { useParams } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import StockHeader from "@/components/stock/StockHeader";
import StockPosition from "@/components/stock/StockPosition";
import StockDecisions from "@/components/stock/StockDecisions";
import StockResearch from "@/components/stock/StockResearch";
import StockResearchDelta from "@/components/stock/StockResearchDelta";
import StockNotes from "@/components/stock/StockNotes";
import StockEvents from "@/components/stock/StockEvents";
import StockLimitUps from "@/components/stock/StockLimitUps";
import type { StockHub } from "@/lib/types";

/**
 * 个股中心（只读，聚合视图）。设计 frontend-ia §2.3。
 * 以 code 为主键，entity 为可选富化；区块顺序：我持有的 → 我判断的 → 外部信息 → 历史。
 */
export default function Page() {
  const { code = "" } = useParams();
  const hub = useData<StockHub>({ path: `${ENDPOINTS.stock.replace(":code", code)}` });

  if (hub.loading) {
    return (
      <div className="panel mt-6">
        <LoadingBlock rows={8} />
      </div>
    );
  }
  if (hub.error || !hub.data) {
    return (
      <div className="panel mt-6">
        <ErrorState msg={hub.error ?? "暂无数据"} />
      </div>
    );
  }
  const d = hub.data;

  return (
    <div>
      <StockHeader
        code={d.code}
        symbol={d.symbol}
        entity={d.entity}
        industry={d.industry}
      />

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>当时在看什么</h2>
          <span className="hint">每笔买入关联的研报 / 消息 / 笔记 —— 我为什么买它</span>
        </div>
        <div className="panel panel-pad">
          <StockDecisions trades={d.trades} />
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>我的持仓与交易</h2>
          <span className="hint">最新持仓快照 + 该股全部成交</span>
        </div>
        <div className="panel panel-pad">
          <StockPosition position={d.position} trades={d.trades} />
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>研究变化</h2>
          <span className="hint">最近两份深研的关键指标对比 —— 我的判断在变好还是变差</span>
        </div>
        <div className="panel panel-pad">
          <StockResearchDelta runs={d.research} />
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>深研报告</h2>
          <span className="hint">点右上「深研」发起一次新的 AI 分析</span>
        </div>
        <div className="panel panel-pad">
          <StockResearch runs={d.research} />
        </div>
      </section>

      <section className="two-col">
        <div>
          <div className="section-head">
            <span className="bar" />
            <h2>我的笔记</h2>
          </div>
          <div className="panel panel-pad">
            <StockNotes notes={d.notes} />
          </div>
        </div>
        <div>
          <div className="section-head">
            <span className="bar" />
            <h2>涨停记录</h2>
          </div>
          <div className="panel panel-pad">
            <StockLimitUps dates={d.limit_ups} />
          </div>
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>相关事件</h2>
          <span className="hint">与这只票相关的消息 · 时间倒序</span>
        </div>
        <div className="panel">
          <StockEvents events={d.events} />
        </div>
      </section>
    </div>
  );
}
