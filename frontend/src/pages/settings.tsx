"use client";

import { Lock } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { SETTINGS_CFG } from "@/lib/mock/trading";
import { Chip } from "@/components/ui/Num";

type SettingSection = { group: string; rows: string[][] };

/** 设置（只读；对齐 dev /settings）：AI 模型分层配置展示，编辑留在 Go 端 */
export default function Page() {
  const cfg = useData<SettingSection[]>({
    path: ENDPOINTS.settings,
    fallback: () => SETTINGS_CFG,
  });
  const sections = cfg.data ?? [];

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">设置</h1>
          <span className="text-[13px] text-muted">
            大模型配置（存 app_config 表）
          </span>
        </div>
      </div>

      <div className="mx-auto mt-2 flex max-w-[680px] items-center justify-center gap-1.5 rounded border border-dashed border-line bg-card-soft py-2.5 text-xs text-muted">
        <Lock size={12} />
        只读视图：修改配置请在 Go 端 /settings 操作
      </div>

      <div className="mx-auto mt-4 grid max-w-[680px] gap-3.5">
        {sections.map((sec) => (
          <div
            key={sec.group}
            className="rounded border border-line bg-card p-4 shadow-card"
          >
            <h2 className="card-title">{sec.group}</h2>
            <table className="w-full border-collapse text-sm">
              <tbody>
                {sec.rows.map(([k, v]) => (
                  <tr key={k} className="border-b border-line last:border-0">
                    <td className="py-2 pr-3 text-[13px] text-muted">{k}</td>
                    <td className="py-2 text-right">
                      {k === "状态" ? (
                        <Chip tone={v === "正常" ? "down" : "amber"}>{v}</Chip>
                      ) : (
                        <span className="num font-mono text-xs">{v}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      </div>
    </div>
  );
}
