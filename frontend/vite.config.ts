import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import http from 'node:http'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        // 复用与后端的 keep-alive 连接，避免分块上传大量顺序请求时的连接抖动
        agent: new http.Agent({ keepAlive: true, maxSockets: 8 }),
      },
    },
  },
})
