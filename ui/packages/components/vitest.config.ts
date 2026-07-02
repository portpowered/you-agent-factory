import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./src/category-paths";
import { createComponentsPackageAliases } from "./src/vite-aliases";

const componentsPackageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "src",
);

function createComponentsPackageResolvePlugin(
  packageRoot: string,
): Plugin {
  const categoryAliases = new Map(
    COMPONENT_CATEGORY_EXPORT_PATHS.map((categoryPath) => [
      `youagentfactory/components/${categoryPath}`,
      path.join(packageRoot, categoryPath, "index.ts"),
    ]),
  );

  return {
    name: "youagentfactory-components-package-resolve",
    resolveId(source) {
      if (source === "youagentfactory/components") {
        return path.join(packageRoot, "index.ts");
      }

      const [specifierPath] = source.split("?", 1);
      if (specifierPath === "youagentfactory/components/styles.css") {
        return path.join(packageRoot, "styles.css");
      }

      const categoryTarget = categoryAliases.get(specifierPath);
      if (categoryTarget) {
        return categoryTarget;
      }

      return null;
    },
  };
}

export default defineConfig({
  plugins: [createComponentsPackageResolvePlugin(componentsPackageRoot)],
  resolve: {
    alias: createComponentsPackageAliases(componentsPackageRoot),
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
    server: {
      deps: {
        inline: [/^youagentfactory\/components/],
      },
    },
  },
});
