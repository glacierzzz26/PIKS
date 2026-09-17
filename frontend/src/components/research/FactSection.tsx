"use client";

import type { AbnormalVolumeDay, ResearchEvidence, ResearchMetrics } from "@/lib/types";
import { PRICE_ROWS, VOLUME_ROWS } from "./metricRows";
import MetricGroup from "./MetricGroup";
import EventGroup from "./EventGroup";
import Scorecard from "./Scorecard";
import AbnormalVolumeDetail from "./AbnormalVolumeDetail";

/** Fact 区：全部数字来自确定性计算（可下钻 Evidence），非 AI 生成。 */
export default function FactSection({
  metrics,
  evidence,
}: {
  metrics: ResearchMetrics;
  evidence: ResearchEvidence[];
}) {
  const bySection = (s: string) => evidence.filter((e) => e.section === s);
  // 放量异常日逐日明细（issue #3）：引擎已落库，此处只展示（Fact，非重算）。
  // 旧报告无此字段 → 空数组 → 组件走空态，不臆造。
  const abnormal =
    (metrics.volume?.abnormal_volume_detail as AbnormalVolumeDay[] | undefined) ??
    [];
  return (
    <section>
      <div className="mb-2 flex items-baseline gap-2">
        <h2 className="m-0 text-[15px] font-bold tracking-wide">一、事实（机器算的）</h2>
        <span className="text-[12px] text-faint">
          确定性计算，非 AI 生成；每个数字可溯源到 Evidence
        </span>
      </div>
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <MetricGroup
          title="股价表现"
          rows={PRICE_ROWS}
          data={metrics.price as Record<string, unknown> | undefined}
          evidence={bySection("price")}
        />
        <MetricGroup
          title="成交量价"
          rows={VOLUME_ROWS}
          data={metrics.volume as Record<string, unknown> | undefined}
          evidence={bySection("volume")}
        />
        <EventGroup metrics={metrics} evidence={bySection("events")} />
        <Scorecard metrics={metrics} />
      </div>
      {metrics.volume && (
        <div className="mt-3">
          <AbnormalVolumeDetail rows={abnormal} />
        </div>
      )}
    </section>
  );
}
