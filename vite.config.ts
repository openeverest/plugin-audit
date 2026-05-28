import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'backend/dist',
    emptyOutDir: false,
    lib: {
      entry: 'src/main.tsx',
      formats: ['es'],
      fileName: () => 'main.js',
    },
    rollupOptions: {
      // React is provided by the OpenEverest host at runtime via PluginApi.
      external: ['react', 'react-dom'],
    },
  },
  server: {
    port: 3001,
    cors: true,
  },
});
