import type { Config } from "tailwindcss";

/**
 * PIKS 设计令牌 —— 以 2026-09 全站重设计 token 为基准。
 * 颜色走 CSS 变量（见 globals.css），支持明/暗主题（[data-theme] 控制）。
 * 保留 accent* 别名（指向品牌色），使既有 className 无需大改。
 */
const config: Config = {
  darkMode: ["class", '[data-theme="dark"]'],
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "var(--bg)",
        "bg-soft": "var(--bg-soft)",
        card: "var(--card)",
        "card-soft": "var(--card-soft)",
        ink: "var(--ink)",
        muted: "var(--muted)",
        faint: "var(--ink-faint)",
        line: "var(--line)",
        "line-strong": "var(--line-strong)",
        brand: "var(--brand)",
        "brand-2": "var(--brand-2)",
        accent: "var(--accent)",
        "accent-ink": "var(--accent-ink)",
        "accent-soft": "var(--accent-soft)",
        up: "var(--red)", // 涨红（A 股习惯）
        down: "var(--green)", // 跌绿（A 股习惯）
        amber: "var(--amber)",
        warn: "var(--warn)",
        gold: "var(--gold)",
        "red-soft": "var(--red-soft)",
        "green-soft": "var(--green-soft)",
        "amber-soft": "var(--amber-soft)",
        "gold-soft": "var(--gold-soft)",
        thead: "var(--thead)",
        hover: "var(--hover)",
        soft: "var(--soft)",
      },
      fontFamily: {
        sans: [
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "PingFang SC",
          "Hiragino Sans GB",
          "Microsoft YaHei",
          "sans-serif",
        ],
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Consolas",
          "monospace",
        ],
      },
      borderRadius: { DEFAULT: "16px", sm: "9px" },
      fontSize: {
        "2xs": ["11px", "14px"],
      },
      boxShadow: {
        card: "var(--shadow)",
        pop: "var(--shadow-lg)",
      },
    },
  },
  plugins: [],
};
export default config;
