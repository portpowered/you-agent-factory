import { defineConfig } from 'tsup'

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['esm'],
  platform: 'node',
  target: 'node20',
  sourcemap: false,
  clean: true,
  dts: false,
  splitting: false,
  minify: false,
  noExternal: ['@agentclientprotocol/sdk', 'cross-spawn', 'zod'],
  banner: {
    js: '#!/usr/bin/env node\nimport { createRequire } from "node:module";\nconst require = createRequire(import.meta.url);'
  }
})
