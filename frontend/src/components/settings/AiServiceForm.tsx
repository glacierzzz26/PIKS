"use client";

import { Save } from "lucide-react";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import Field from "@/components/settings/Field";
import { useAiSettingsForm } from "@/components/settings/useAiSettingsForm";

/** AI 分层配置编辑（抽取 / 推理 / 视觉 + 日预算护栏），保存走 POST /api/v1/settings */
export default function AiServiceForm() {
  const { form, vals, set, saving, msg, save } = useAiSettingsForm();

  if (form.loading) {
    return (
      <div className="panel">
        <LoadingBlock rows={4} />
      </div>
    );
  }
  if (form.error) {
    return (
      <div className="panel">
        <ErrorState msg={form.error} />
      </div>
    );
  }
  if (!form.data) {
    return (
      <div className="panel">
        <p className="px-5 py-6 text-[13px] txt-faint">配置不可用</p>
      </div>
    );
  }

  const opts = form.data.model_options ?? [];

  return (
    <div className="form-card">
      <h3>AI 服务</h3>
      <p className="fnote">不同任务用不同档位的模型，并设每日花费上限</p>

      {msg && <p className={`mb-3 text-xs ${msg.ok ? "text-down" : "text-up"}`}>{msg.text}</p>}

      <Field label="AI 服务地址" hint="OpenAI 兼容 base_url，如 https://api.xxx.com/v1">
        <input value={vals.base_url} onChange={set("base_url")} placeholder="https://…" />
      </Field>

      <Field
        label="API Key"
        hint={form.data.key_masked ? `已配置：${form.data.key_masked}（留空则不改）` : "尚未配置"}
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
        <input type="number" min={0} value={vals.budget} onChange={set("budget")} />
      </Field>

      {form.data.model_note && <p className="text-xs text-amber">{form.data.model_note}</p>}

      <button onClick={save} disabled={saving} className="btn-save mt-4">
        <Save size={13} className="mr-1.5 inline" />
        {saving ? "保存中…" : "保存设置"}
      </button>
    </div>
  );
}
