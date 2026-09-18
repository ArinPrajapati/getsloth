import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    // Needed for real-phone dev testing through Cloudflare tunnels.
    // Production hosting is separate; this only affects Vite dev server.
    allowedHosts: ['.trycloudflare.com', '.arinprajapti.com', '.arin.work']
  },
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts', 'src/**/*.d.ts', 'src/main.ts', 'src/xterm-terminal.ts'],
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80
      }
    }
  }
});
