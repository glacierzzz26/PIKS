"use client";

import { useEffect, useState } from "react";
import { Save } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiPost, ENDPOINTS } from "@/lib/api";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import type { SettingsForm } from "@/lib/types";

const INPUT =
  "h-9 w-full rounded-sm border border-line bg-card px-3 text-sm outline-none focus:border-accent";

/** 表单项：标签 + 提示 + 控件 */
function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[13px] font-medium text-ink">{label}</label>
      {children}
      {hint && <p className="text-xs text-muted">{hint}</p>}
    </div>
  );
}

/** 设置（交互）：AI 分层配置编辑 —— 保存走 POST /api/v1/settings */
export default function Page() {
  const form = useData<SettingsForm>({ path: ENDPOINTS.settingsForm });
  const [vals, setVals] = useState({
    base_url: "",
    key: "",
    model_extract: "",
    model_reasoning: "",
    model_vision: "",
    budget: "0",
  });
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (form.data) {
      setVals({
        base_url: form.data.base_url,
        key: "",
        model_extract: form.data.model_extract,
        model_reasoning: form.data.model_reasoning,
        model_vision: form.data.model_vision,
        budget: form.data.budget || "0",
      });
    }
  }, [form.data]);

  const set =
    (k: keyof typeof vals) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
      setVals((v) => ({ ...v, [k]: e.target.value }));

  const save = async () => {
    setSaving(true);
    setMsg(null);
    try {
      await apiPost<{ ok: boolean }>(ENDPOINTS.settings, {
        ai_service_base_url: vals.base_url.trim(),
        ai_model_extract: vals.model_extract,
        ai_model_reasoning: vals.model_reasoning,
        ai_model_vision: vals.model_vision,
        ai_daily_token_budget: vals.budget,
        ai_api_key: vals.key,
      });
      setMsg({ ok: true, text: "已保存（密钥留空则保持原值不变）" });
      form.refresh();
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : String(e) });
    } finally {
      setSaving(false);
    }
  };

  const opts = form.data?.model_options ?? [];

  return (
    <div className="mx-auto max-w-[680px]">
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">设置</h1>
          <span className="text-[13px] text-muted">大模型分层配置（存 app_config 表）</span>
        </div>
      </div>

      {form.loading ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <LoadingBlock rows={4} />
        </div>
      ) : form.error ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <ErrorState msg={form.error} />
        </div>
      ) : !form.data ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <p className="px-4 py-6 text-[13px] text-muted">配置不可用</p>
        </div>
      ) : (
        <div className="mt-4 flex flex-col gap-4 rounded border border-line bg-card p-5 shadow-card">
          {msg && (
            <p className={`text-xs ${msg.ok ? "text-down" : "text-up"}`}>{msg.text}</p>
          )}

          <Field label="AI 服务地址" hint="OpenAI 兼容 base_url，如 https://api.xxx.com/v1">
            <input
              value={vals.base_url}
              onChange={set("base_url")}
              placeholder="https://…"
              className={INPUT}
            />
          </Field>

          <Field label="API Key" hint={form.data.key_masked ? `已配置：${form.data.key_masked}（留空则不改）` : "尚未配置"}>
            <input
              type="password"
              value={vals.key}
              onChange={set("key")}
              placeholder={form.data.key_masked ? "留空保持原密钥" : "粘贴 API Key"}
              className={INPUT}
            />
          </Field>

          <Field label="抽取模型">
            <select value={vals.model_extract} onChange={set("model_extract")} className={INPUT}>
              <option value="">选择模型</option>
              {opts.map((o) => (
                <option key={o} value={o}>{o}</option>
              ))}
            </select>
          </Field>

          <Field label="深度推理模型" hint="用于周报综述 / 交易复盘 / AI 对话">
            <select value={vals.model_reasoning} onChange={set("model_reasoning")} className={INPUT}>
              <option value="">选择模型</option>
              {opts.map((o) => (
                <option key={o} value={o}>{o}</option>
              ))}
            </select>
          </Field>

          <Field label="视觉模型" hint="用于截图导入/截图提问；留空回退抽取模型">
            <select value={vals.model_vision} onChange={set("model_vision")} className={INPUT}>
              <option value="">回退抽取模型</option>
              {opts.map((o) => (
                <option key={o} value={o}>{o}</option>
              ))}
            </select>
          </Field>

          <Field label="日 token 预算" hint="0 = 关闭预算护栏">
            <input
              type="number"
              min={0}
              value={vals.budget}
              onChange={set("budget")}
              className={INPUT}
            />
          </Field>

          {form.data.model_note && (
            <p className="text-xs text-amber">{form.data.model_note}</p>
          )}

          <button
            onClick={save}
            disabled={saving}
            className="inline-flex h-9 w-fit items-center gap-1.5 rounded-sm bg-accent px-5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            <Save size={13} />
            {saving ? "保存中…" : "保存设置"}
          </button>
        </div>
      )}
    </div>
  );
}
