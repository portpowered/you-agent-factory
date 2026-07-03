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
const uiRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const sharedReactAliases = [
  {
    find: "react",
    replacement: path.join(uiRoot, "node_modules/react"),
  },
  {
    find: "react-dom",
    replacement: path.join(uiRoot, "node_modules/react-dom"),
  },
  {
    find: "react/jsx-runtime",
    replacement: path.join(uiRoot, "node_modules/react/jsx-runtime"),
  },
  {
    find: "react/jsx-dev-runtime",
    replacement: path.join(uiRoot, "node_modules/react/jsx-dev-runtime"),
  },
  {
    find: "@radix-ui/react-select",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-select"),
  },
] as const;

function createComponentsPackageResolvePlugin(
  packageRoot: string,
): Plugin {
  const categoryAliases = new Map(
    COMPONENT_CATEGORY_EXPORT_PATHS.map((categoryPath) => [
      `@you-agent-factory/components/${categoryPath}`,
      path.join(packageRoot, categoryPath, "index.ts"),
    ]),
  );

  return {
    name: "you-agent-factory-components-package-resolve",
    resolveId(source) {
      if (source === "@you-agent-factory/components") {
        return path.join(packageRoot, "index.ts");
      }

      const [specifierPath] = source.split("?", 1);
      if (specifierPath === "@you-agent-factory/components/styles.css") {
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
    alias: [
      { find: /^react$/, replacement: path.join(uiRoot, "node_modules/react") },
      {
        find: /^react-dom$/,
        replacement: path.join(uiRoot, "node_modules/react-dom"),
      },
      {
        find: /^react\/jsx-runtime$/,
        replacement: path.join(uiRoot, "node_modules/react/jsx-runtime"),
      },
      {
        find: /^react\/jsx-dev-runtime$/,
        replacement: path.join(uiRoot, "node_modules/react/jsx-dev-runtime"),
      },
      {
        find: /^@radix-ui\/react-select$/,
        replacement: path.join(uiRoot, "node_modules/@radix-ui/react-select"),
      },
      {
        find: /^@radix-ui\/react-slot$/,
        replacement: path.join(uiRoot, "node_modules/@radix-ui/react-slot"),
      },
      ...sharedReactAliases,
      ...createComponentsPackageAliases(componentsPackageRoot),
    ],
    dedupe: [
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
      "@radix-ui/react-select",
      "@radix-ui/react-slot",
    ],
  },
  test: {
    environment: "happy-dom",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx", "scripts/**/*.test.mjs"],
    setupFiles: ["./src/testing/vitest.setup.ts"],
    server: {
      deps: {
        moduleDirectories: [
          path.join(uiRoot, "node_modules"),
          "node_modules",
        ],
        inline: [
          /^@you-agent-factory\/components/,
          "@radix-ui/react-select",
          "@radix-ui/react-slot",
          "@testing-library/react",
        ],
      },
    },
  },
});
