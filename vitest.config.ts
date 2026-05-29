import { defineConfig } from 'vitest/config'
import dotenv from 'dotenv'
import { fileURLToPath } from 'url'

dotenv.config()

export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./apps/web/src', import.meta.url)),
      '#': fileURLToPath(new URL('./packages/ui/src', import.meta.url)),
    },
  },
  test: {
    globals: true,
    include: [
      'packages/**/*.test.ts',
      'apps/**/*.test.ts',
    ],
    env: process.env,
    testTimeout: Infinity,
  },
})
