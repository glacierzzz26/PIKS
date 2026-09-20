import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

/**
 * PIKS 前端(Vite SPA)。
 * dev 用 proxy 复刻生产 nginx 分流(见 configs/nginx.conf):
 *   - /api/* → Go web(:8090,JSON + 截图上传)
 *   - 其余(全部 SPA 页面)由 Vite 自身 history fallback 提供
 *
 * ⚠️ dev 下 :8090 是**直接跑 `./bin/web`**(宿主进程),不是 gateway 容器。生产里 :8090 归
 * gateway(反代到私网 web:8090),对外行为一致,故本 proxy 目标无需随容器拆分改(2026-09-20)。
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 3100,
    proxy: {
      "/api": "http://localhost:8090",
    },
  },
});
