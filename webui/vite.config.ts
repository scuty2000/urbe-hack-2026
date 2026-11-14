import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

// The SPA is mounted by the Go server under /dashboard/, so every generated
// asset URL must carry that prefix. In development Vite proxies /api to the
// backend so fetch calls stay same-origin and the lf_session cookie works.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const apiTarget = env.VITE_DEV_API_TARGET || "http://localhost:3000";

  return {
    base: "/dashboard/",
    plugins: [vue(), tailwindcss()],
    server: {
      port: 5173,
      proxy: {
        "/api": {
          target: apiTarget,
          changeOrigin: false,
        },
      },
    },
    build: {
      outDir: "dist",
      emptyOutDir: true,
    },
  };
});
