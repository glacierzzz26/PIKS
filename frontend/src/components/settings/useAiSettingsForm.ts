"use client";

import { useEffect, useState } from "react";
import { apiPost, ENDPOINTS } from "@/lib/api";
import { useData } from "@/hooks/useData";
import type { SettingsForm } from "@/lib/types";

export type AiFormVals = {
  base_url: string;
  key: string;
  model_extract: string;
  model_reasoning: string;
  model_vision: string;
  budget: string;
};

export type SaveMsg = { ok: boolean; text: string } | null;

const EMPTY: AiFormVals = {
  base_url: "",
  key: "",
  model_extract: "",
  model_reasoning: "",
  model_vision: "",
  budget: "0",
};

/** AI 配置表单状态 + 保存逻辑（表单数据加载、字段编辑、POST /settings） */
export function useAiSettingsForm() {
  const form = useData<SettingsForm>({ path: ENDPOINTS.settingsForm });
  const [vals, setVals] = useState<AiFormVals>(EMPTY);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<SaveMsg>(null);

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
    (k: keyof AiFormVals) =>
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

  return { form, vals, set, saving, msg, save };
}
