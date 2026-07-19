import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const packageRoot = path.dirname(fileURLToPath(import.meta.url));

function isExternalPackage(moduleId: string): boolean {
  return !moduleId.startsWith(".") && !path.isAbsolute(moduleId);
}

export default defineConfig({
  build: {
    copyPublicDir: false,
    emptyOutDir: false,
    lib: {
      entry: path.join(packageRoot, "src", "index.ts"),
      formats: ["es"],
    },
    minify: false,
    outDir: "dist",
    rollupOptions: {
      external: isExternalPackage,
      output: { entryFileNames: "index.js" },
    },
    sourcemap: false,
  },
});
