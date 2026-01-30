import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';

export default defineConfig({
  base: '/m/',
  plugins: [
    vue({
      template: {
        compilerOptions: {
          isCustomElement: (tag) => tag.startsWith('sl-'),
        },
      },
    }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '../webapp/src'),
      '@mobile': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 8087,
    strictPort: true,
    proxy: {
      '/api': 'http://127.0.0.1:8089',
    },
    fs: {
      allow: ['..'],
    },
  },
  css: {
    preprocessorOptions: {
      scss: {
        api: 'modern',
      },
    },
  },
  build: {
    emptyOutDir: true,
    sourcemap: false,
  },
});
