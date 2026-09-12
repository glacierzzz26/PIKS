"use client";

import type { SnapRow, TaskRun } from "@/lib/types";

/** 情绪分色：偏热→红、偏暖→warn、谨慎→绿（A 股习惯，与 HTML gauge 一致） */
function emotionColor(score: number): string {
  if (score >= 60) return "var(--red)";
  if (score >= 45) return "var(--warn)";
  return "var(--green)";
}

/** 半环仪表盘（对齐 HTML .gauge-box），按真实情绪分画弧 */
export function Gauge({ score, state }: { score: number; state: string }) {
  const R = 63;
  const total = Math.PI * R; // ≈197.9
  const offset = total * (1 - Math.max(0, Math.min(100, score)) / 100);
  const color = emotionColor(score);
  return (
    <div className="gauge-box">
      <div className="gtitle">市场情绪分（0–100）</div>
      <svg className="gauge-svg" width="150" height="84" viewBox="0 0 150 84">
        <path
          d="M12 78 A63 63 0 0 1 138 78"
          fill="none"
          stroke="var(--bg-soft)"
          strokeWidth="11"
          strokeLinecap="round"
        />
        <path
          d="M12 78 A63 63 0 0 1 138 78"
          fill="none"
          stroke={color}
          strokeWidth="11"
          strokeLinecap="round"
          strokeDasharray={total.toFixed(1)}
          strokeDashoffset={offset.toFixed(1)}
        />
      </svg>
      <div className="gauge-val" style={{ color }}>
        {score} · {state}
      </div>
    </div>
  );
}

/** 近 N 日情绪走势 sparkline（对齐 HTML .spark-wrap），按真实快照点绘制 */
export function EmotionSpark({ history }: { history: SnapRow[] }) {
  if (history.length === 0) return null;
  // 时间正序（history 通常最新在前）
  const rows = [...history].reverse();
  const W = 260;
  const H = 86;
  const padL = 10;
  const padR = 10;
  const top = 14;
  const bottom = 80;
  const n = rows.length;
  const x = (i: number) =>
    n === 1 ? padL : padL + (i * (W - padL - padR)) / (n - 1);
  const y = (s: number) => bottom - (Math.max(0, Math.min(100, s)) / 100) * (bottom - top);
  const pts = rows.map((r, i) => [x(i), y(r.emotion_score)] as const);
  const line = pts.map(([px, py]) => `${px.toFixed(1)},${py.toFixed(1)}`).join(" L");
  const area = `M${line} L${pts[n - 1][0].toFixed(1)},${bottom} L${pts[0][0].toFixed(1)},${bottom} Z`;
  const last = rows[n - 1];
  const delta = last.emotion_score - rows[0].emotion_score;
  const lastX = pts[n - 1][0];

  return (
    <div className="spark-wrap">
      <div className="spark-title">
        近 {n} 日情绪走势
        <span className="tag">
          {delta >= 0 ? "+" : ""}
          {delta} 分
        </span>
      </div>
      <svg width="100%" height="86" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-hidden="true">
        <defs>
          <linearGradient id="piks-spark" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--brand)" stopOpacity=".22" />
            <stop offset="100%" stopColor="var(--brand)" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={area} fill="url(#piks-spark)" />
        <path
          d={`M${line}`}
          fill="none"
          stroke="var(--brand)"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
        />
        {pts.map(([px, py], i) => (
          <circle
            key={i}
            cx={px}
            cy={py}
            r={i === n - 1 ? 4.5 : 3}
            fill="var(--card)"
            stroke="var(--brand)"
            strokeWidth="2"
          />
        ))}
        <text x={lastX - 6} y={10} fontSize="10" fontWeight="700" fill="var(--brand)" textAnchor="end">
          {last.emotion_score}
        </text>
      </svg>
      <table className="mini-table">
        <thead>
          <tr>
            <th>日期</th>
            <th>情绪</th>
            <th>涨停</th>
            <th>跌停</th>
            <th>炸板</th>
            <th>最高板</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.date} className={i === n - 1 ? "today" : ""}>
              <td>{r.date.slice(5)}</td>
              <td>
                {r.emotion_score} {r.emotion_state}
              </td>
              <td>{r.limit_up}</td>
              <td>{r.limit_down}</td>
              <td>{r.broken_limit}</td>
              <td>{r.max_board}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 管线状态节点（对齐 HTML .pipe / .pnode） */
export function Pipeline({ runs }: { runs: TaskRun[] }) {
  const dot = (status: TaskRun["status"]) =>
    status === "ok" ? "var(--green)" : status === "failed" ? "var(--red)" : "var(--warn)";
  return (
    <div className="px-5 pb-4">
      <div
        className="pipe"
        style={{ gridTemplateColumns: `repeat(${Math.max(1, runs.length)}, 1fr)` }}
      >
        {runs.map((t) => (
          <div key={t.command} className="pnode">
            <span className="dot" style={{ border: "4px solid", borderColor: dot(t.status) }} />
            <b>{t.command}</b>
            <span className="t">{t.time}</span>
          </div>
        ))}
      </div>
      <div className="pipe-legend">
        <span className="li">
          <span className="d d-ok" />
          正常
        </span>
        <span className="li">
          <span className="d d-run" />
          运行中 / 跳过（预算耗尽等，如实记录）
        </span>
        <span className="li">
          <span className="d d-fail" />
          失败（不阻断，下次重试）
        </span>
      </div>
    </div>
  );
}
