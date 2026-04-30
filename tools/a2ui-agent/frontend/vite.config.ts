import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'url';
import path from 'path';

// ESM __dirname equivalent
const __dirname = path.dirname(fileURLToPath(import.meta.url));
// frontend → a2ui-agent → tools → A2UI repo root
const repoRoot = path.resolve(__dirname, '../../..');

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@a2ui/react/v0_9': path.join(repoRoot, 'renderers/react/src/v0_9'),
      '@a2ui/react/v0_8': path.join(repoRoot, 'renderers/react/src/v0_8'),
      '@a2ui/react': path.join(repoRoot, 'renderers/react/src/v0_9'),
      '@a2ui/web_core/v0_9': path.join(repoRoot, 'renderers/web_core/src/v0_9'),
      '@a2ui/web_core/v0_8': path.join(repoRoot, 'renderers/web_core/src/v0_8'),
      '@a2ui/web_core': path.join(repoRoot, 'renderers/web_core/src/v0_9'),
      '@a2ui/markdown-it': path.join(repoRoot, 'renderers/markdown/markdown-it/src/markdown'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/v1': 'http://localhost:8081',
    },
  },
});
