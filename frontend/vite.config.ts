import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

/**
 * PIKS 前端(Vite SPA)。
 * dev 用 proxy 复刻生产 nginx 分流(见 configs/nginx.conf):
 *   - /api/* → Go web(:8090,JSON + 截图上传)
 *   - 其余(全部 SPA 页面)由 Vite 自身 history fallback 提供
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
