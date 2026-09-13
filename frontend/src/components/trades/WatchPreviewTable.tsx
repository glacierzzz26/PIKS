"use client";

import { Chip } from "@/components/ui/Num";
import type { PreviewWatch } from "@/lib/types";

/** 自选镜像预览：将加入（add）/ 已在（keep）/ 将移出（remove）。
 *  remove 组默认勾选 + 整组一键取消（用户拍板：镜像语义默认移除，但可整组否决）。 */
export default function WatchPreviewTable({
  rows,
  onPatch,
  onToggleRemoveAll,
}: {
  rows: PreviewWatch[];
  onPatch: (i: number, p: Partial<PreviewWatch>) => void;
  onToggleRemoveAll: (include: boolean) => void;
}) {
  const removes = rows.filter((r) => r.change === "remove");
  const allRemoveOff = removes.length > 0 && removes.every((r) => !r.include);
  const adds = rows.filter((r) => r.change === "add").length;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-3 text-xs">
        <span className="text-muted">
          将加入 <b style={{ color: "var(--red)" }}>{adds}</b> 只 · 将移出{" "}
          <b>{removes.length}</b> 只
        </span>
        {removes.length > 0 && (
          <button
            onClick={() => onToggleRemoveAll(allRemoveOff)}
            className="inline-flex h-7 items-center rounded-[9px] border border-line bg-card px-2.5 text-[12px] text-muted hover:text-accent"
          >
            {allRemoveOff ? "恢复移出勾选" : "整组取消移出"}
          </button>
        )}
        <span className="text-faint">移出 = 退出自选，历史 / 深研 / 笔记保留</span>
      </div>

      <div className="overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th style={{ textAlign: "left" }}>选</th>
              <th style={{ textAlign: "left" }}>代码</th>
              <th style={{ textAlign: "left" }}>名称</th>
              <th style={{ textAlign: "left" }}>动作</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={`${r.change}-${r.code}`}>
                <td className="px-2 py-1.5">
                  {r.change === "keep" ? (
                    <span className="text-faint">—</span>
                  ) : (
                    <input
                      type="checkbox"
                      checked={r.include}
                      onChange={(e) => onPatch(i, { include: e.target.checked })}
                    />
                  )}
                </td>
                <td className="num-t" style={{ textAlign: "left" }}>{r.code}</td>
                <td style={{ textAlign: "left" }}>{r.name}</td>
                <td style={{ textAlign: "left" }}>
                  {r.change === "add" && <Chip tone="up">加入自选</Chip>}
                  {r.change === "keep" && <Chip tone="dim">已在自选</Chip>}
                  {r.change === "remove" && <Chip tone="amber">移出自选</Chip>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
