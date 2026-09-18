"use client";

import { useRef, useState } from "react";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import type { ImportPreview, PreviewAccount } from "@/lib/types";

/** 截图导入类型：今日交易 / 持仓 / 自选股。 */
export type ImportKind = "" | "trade" | "position" | "watchlist";

/** 配置类错误（AI/视觉模型未配）→ 提示去 /settings，其余错误原样展示（不误导）。 */
export function isConfigError(msg: string): boolean {
  return /配置|设置|未配置/.test(msg);
}

/**
 * 截图导入状态机（桌面 ImportFlow 与手机 /m/upload 共用）。
 * 选类型 → 上传识别（不落库）→ 预览可编辑 → 确认入库（含自选镜像）。
 */
export function useTradeImport(onDone?: () => void) {
  const [kind, setKind] = useState<ImportKind>("");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const upload = async () => {
    if (!kind || !file) return;
    setBusy(true);
    setErr(null);
    setPreview(null);
    setMsg(null);
    try {
      const fd = new FormData();
      fd.append("type", kind);
      fd.append("file", file);
      setPreview(await apiUpload<ImportPreview>(ENDPOINTS.tradesImport, fd));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const patch = <S extends "trades" | "positions" | "watch", T>(
    section: S,
    i: number,
    p: Partial<T>
  ) =>
    setPreview((prev) => {
      if (!prev) return prev;
      const arr = [...(prev[section] as unknown as T[])];
      arr[i] = { ...arr[i], ...p };
      return { ...prev, [section]: arr };
    });

  // 账户汇总行（issue #19）：非数组，单独 patch。空串 = 截图没有该项（确认时落 NULL）。
  const patchAccount = (p: Partial<PreviewAccount>) =>
    setPreview((prev) => (prev ? { ...prev, account: { ...prev.account, ...p } } : prev));

  // 整组取消/恢复移出勾选（仅影响 change==='remove' 行）
  const toggleRemoveAll = (include: boolean) =>
    setPreview((prev) =>
      prev
        ? {
            ...prev,
            watch: prev.watch.map((w) =>
              w.change === "remove" ? { ...w, include } : w
            ),
          }
        : prev
    );

  const reset = () => {
    setPreview(null);
    setFile(null);
    setKind("");
    if (inputRef.current) inputRef.current.value = "";
  };

  const confirm = async () => {
    if (!preview) return;
    setBusy(true);
    setErr(null);
    try {
      await apiPost<{ ok?: boolean; applied?: number }>(ENDPOINTS.tradesConfirm, preview);
      setMsg(preview.kind === "watchlist" ? "自选已同步" : "已确认入库");
      reset();
      onDone?.();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return {
    kind,
    setKind,
    file,
    setFile,
    busy,
    err,
    setErr,
    msg,
    setMsg,
    preview,
    inputRef,
    upload,
    confirm,
    reset,
    patch,
    patchAccount,
    toggleRemoveAll,
  };
}
