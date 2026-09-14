"use client";

/** 表单项：标签 + 提示 + 控件（设置页复用） */
export default function Field({
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
