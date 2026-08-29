import type { Config } from "tailwindcss";

/**
 * PIKS 设计令牌 —— 以现有 Go 模板版 base.css 为基准，
 * 颜色走 CSS 变量以支持明暗主题切换（light/dark 由 [data-theme] 控制）。
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
        line: "var(--line)",
        "line-strong": "var(--line-strong)",
        accent: "var(--accent)",
        "accent-ink": "var(--accent-ink)",
        "accent-soft": "var(--accent-soft)",
        up: "var(--red)", // 涨红（A 股习惯）
        down: "var(--green)", // 跌绿（A 股习惯）
        amber: "var(--amber)",
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
      borderRadius: { DEFAULT: "14px", sm: "9px" },
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
