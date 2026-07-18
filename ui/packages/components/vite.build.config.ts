import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./src/category-paths";

const packageRoot = path.dirname(fileURLToPath(import.meta.url));
const sourceRoot = path.join(packageRoot, "src");
const entryPoints = Object.fromEntries([
  ["index", path.join(sourceRoot, "index.ts")],
  ...COMPONENT_CATEGORY_EXPORT_PATHS.map((categoryPath) => [
    `${categoryPath}/index`,
    path.join(sourceRoot, categoryPath, "index.ts"),
  ]),
]);

function isExternalPackage(moduleId: string): boolean {
  return !moduleId.startsWith(".") && !path.isAbsolute(moduleId);
}

export default defineConfig({
  build: {
    copyPublicDir: false,
    emptyOutDir: false,
    lib: {
      entry: entryPoints,
      formats: ["es"],
    },
    minify: false,
    outDir: "dist",
    rollupOptions: {
      external: isExternalPackage,
      output: {
        assetFileNames: "assets/[name][extname]",
        chunkFileNames: "chunks/[name].js",
        entryFileNames: "[name].js",
      },
    },
    sourcemap: false,
  },
});
