"use client";

import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Save, Scale, ChevronRight } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiPost, ENDPOINTS } from "@/lib/api";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import type { SettingsForm } from "@/lib/types";

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
    <div className="frow">
      <label>{label}</label>
      {children}
      {hint && <p className="tip">{hint}</p>}
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
    <div>
      <div className="page-head">
        <div>
          <h1>设置</h1>
          <div className="psub">大模型分层配置（存 app_config 表）</div>
        </div>
      </div>

      {form.loading ? (
        <div className="panel">
          <LoadingBlock rows={4} />
        </div>
      ) : form.error ? (
        <div className="panel">
          <ErrorState msg={form.error} />
        </div>
      ) : !form.data ? (
        <div className="panel">
          <p className="px-5 py-6 text-[13px] text-faint">配置不可用</p>
        </div>
      ) : (
        <div className="max-w-[720px]">
          <div className="form-card">
            <h3>AI 服务</h3>
            <p className="fnote">抽取 / 推理 / 视觉三档模型 + 日预算护栏</p>

            {msg && (
              <p
                className="mb-3 text-xs"
                style={{ color: msg.ok ? "var(--green)" : "var(--red)" }}
              >
                {msg.text}
              </p>
            )}

            <Field label="AI 服务地址" hint="OpenAI 兼容 base_url，如 https://api.xxx.com/v1">
              <input
                value={vals.base_url}
                onChange={set("base_url")}
                placeholder="https://…"
              />
            </Field>

            <Field
              label="API Key"
              hint={
                form.data.key_masked
                  ? `已配置：${form.data.key_masked}（留空则不改）`
                  : "尚未配置"
              }
            >
              <input
                type="password"
                value={vals.key}
                onChange={set("key")}
                placeholder={form.data.key_masked ? "留空保持原密钥" : "粘贴 API Key"}
              />
            </Field>

            <Field label="抽取模型">
              <select value={vals.model_extract} onChange={set("model_extract")}>
                <option value="">选择模型</option>
                {opts.map((o) => (
                  <option key={o} value={o}>{o}</option>
                ))}
              </select>
            </Field>

            <Field label="深度推理模型" hint="用于周报综述 / 交易复盘 / AI 对话">
              <select value={vals.model_reasoning} onChange={set("model_reasoning")}>
                <option value="">选择模型</option>
                {opts.map((o) => (
                  <option key={o} value={o}>{o}</option>
                ))}
              </select>
            </Field>

            <Field label="视觉模型" hint="用于截图导入/截图提问；留空回退抽取模型">
              <select value={vals.model_vision} onChange={set("model_vision")}>
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
              />
            </Field>

            {form.data.model_note && (
              <p className="text-xs" style={{ color: "var(--amber)" }}>
                {form.data.model_note}
              </p>
            )}

            <button onClick={save} disabled={saving} className="btn-save mt-4">
              <Save size={13} className="mr-1.5 inline" />
              {saving ? "保存中…" : "保存设置"}
            </button>
          </div>

          {/* 对账：日常运维子入口（原侧栏项降级至此，个股轴心 IA） */}
          <div className="form-card mt-4">
            <h3>运维</h3>
            <p className="fnote">管线巡检与对账</p>
            <Link
              to="/recon"
              className="flex items-center justify-between rounded-[10px] border border-line px-3 py-2.5 text-[13px] no-underline hover:border-accent"
            >
              <span className="inline-flex items-center gap-2">
                <Scale size={15} className="text-muted" />
                <span>
                  对账
                  <span className="ml-2 text-[12px] text-faint">
                    快讯 → 事件 → 实体 的链路巡检
                  </span>
                </span>
              </span>
              <ChevronRight size={14} className="text-faint" />
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}
