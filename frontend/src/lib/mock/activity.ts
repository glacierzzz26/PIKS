/** 演示数据：看板统计 / 管线任务 / 对账（对齐 dashboard.go / recon.go 投影） */

export const KB_STATS = [
  { label: "结构化事件", value: 486 },
  { label: "统一实体", value: 213 },
  { label: "知识笔记", value: 57 },
  { label: "交易记录", value: 134 },
];

export type SnapRow = {
  date: string;
  emotion_score: number;
  emotion_state: string;
  limit_up: number;
  limit_down: number;
  broken_limit: number;
  max_board: number;
};

export const SNAP_HISTORY: SnapRow[] = [
  { date: "2026-08-29", emotion_score: 71, emotion_state: "偏暖", limit_up: 64, limit_down: 5, broken_limit: 23, max_board: 6 },
  { date: "2026-08-28", emotion_score: 66, emotion_state: "偏暖", limit_up: 52, limit_down: 8, broken_limit: 26, max_board: 5 },
  { date: "2026-08-27", emotion_score: 58, emotion_state: "中性", limit_up: 45, limit_down: 11, broken_limit: 28, max_board: 4 },
  { date: "2026-08-26", emotion_score: 49, emotion_state: "中性", limit_up: 38, limit_down: 14, broken_limit: 31, max_board: 4 },
  { date: "2026-08-25", emotion_score: 41, emotion_state: "转冷", limit_up: 29, limit_down: 18, broken_limit: 35, max_board: 3 },
];

export type TaskRun = {
  command: string;
  status: "ok" | "failed" | "running";
  time: string;
  note?: string;
};

export const TASK_RUNS: TaskRun[] = [
  { command: "reconcile", status: "ok", time: "16:02" },
  { command: "daily-review", status: "ok", time: "15:58" },
  { command: "market-state", status: "ok", time: "15:47" },
  { command: "entity-build", status: "ok", time: "15:41" },
  { command: "quote-collector", status: "ok", time: "15:31" },
  { command: "cluster", status: "failed", time: "15:22", note: "重审视 Pass 超时,下次重试" },
  { command: "worker", status: "ok", time: "15:10" },
  { command: "collector", status: "ok", time: "14:55" },
];

export type ReconRow = {
  date: string;
  flashes: number;
  events: number;
  anomalies: number;
  status: "ok" | "warn" | "failed";
  note?: string;
};

export const RECON_ROWS: ReconRow[] = [
  { date: "2026-08-29", flashes: 142, events: 38, anomalies: 0, status: "ok" },
  { date: "2026-08-28", flashes: 156, events: 41, anomalies: 1, status: "warn", note: "2 条快讯未命中事件抽取(重试中)" },
  { date: "2026-08-27", flashes: 133, events: 35, anomalies: 0, status: "ok" },
  { date: "2026-08-26", flashes: 128, events: 33, anomalies: 2, status: "warn", note: "涨停池源延迟 40 分钟" },
  { date: "2026-08-25", flashes: 119, events: 30, anomalies: 0, status: "ok" },
  { date: "2026-08-24", flashes: 61, events: 12, anomalies: 1, status: "failed", note: "非交易日,仅处理盘后快讯" },
];
