export default defineNuxtConfig({
  compatibilityDate: '2025-07-01',
  css: ['~/assets/main.css'],
  devServer: { port: 3000 },
  runtimeConfig: { apiBase: process.env.API_BASE || 'http://127.0.0.1:8080' },
})
