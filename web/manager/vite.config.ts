import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
	port: 5173,
	strictPort: true,
    // 代理 key 别写成 '/api'：字符串 key 是 url.startsWith() 前缀匹配，`/apikeys`
    // 也会被转到 manager（它只认 /api/*，于是回 404 page not found），菜单点进去是
    // 前端路由所以正常，直接敲 URL 就 404。加 `(/|$)` 让它只吃路径段。
    proxy: {
      '^/api(/|$)': 'http://localhost:9090',
      '^/internal(/|$)': 'http://localhost:9090',
    },
  },
})
