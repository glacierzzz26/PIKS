import type { Metadata } from "next";
import "./globals.css";
import AppShell from "@/components/layout/AppShell";

export const metadata: Metadata = {
  title: "PIKS · 投资知识系统",
  description: "A 股投资知识系统：事件、实体、涨停梯队与知识沉淀",
};

/** 主题初始化：读取 localStorage，避免暗色模式闪烁 */
const themeInit = `try{var t=localStorage.getItem("piks-theme");if(t)document.documentElement.setAttribute("data-theme",t);}catch(e){}`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInit }} />
      </head>
      <body className="font-sans">
        <AppShell>{children}</AppShell>
      </body>
    </html>
  );
}
