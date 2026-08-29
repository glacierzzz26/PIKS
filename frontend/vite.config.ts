import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

/**
 * PIKS 前端(去掉 Next.js 后的 Vite SPA)。
 * dev 用 proxy 复刻生产 nginx 分流(见 configs/nginx.conf):
 *   - /api/*  → Go web(:8090,JSON + 截图上传)
 *   - 交互页(/notes* /settings /chat /trades* /weekly /events/{id} /entities/{id} /reviews/{id}) → Go HTML 编辑页
 *   - 其余(SPA 只读页)由 Vite 自身 history fallback 提供
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
      "/notes": "http://localhost:8090",
      "/settings": "http://localhost:8090",
      "/chat": "http://localhost:8090",
      "/trades": "http://localhost:8090",
      "/weekly": "http://localhost:8090",
      "/events/": "http://localhost:8090",
      "/entities/": "http://localhost:8090",
      "/reviews/": "http://localhost:8090",
    },
  },
});
