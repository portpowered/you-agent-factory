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
const uiNodeModules = path.join(uiRoot, "node_modules");

/** React-owning deps must resolve through the dashboard ui install to avoid duplicate React in package tests. */
const sharedReactAliases = [
  {
    find: "react",
    replacement: path.join(uiNodeModules, "react"),
  },
  {
    find: "react-dom",
    replacement: path.join(uiNodeModules, "react-dom"),
  },
  {
    find: "react-dom/client",
    replacement: path.join(uiNodeModules, "react-dom/client"),
  },
  {
    find: "react/jsx-runtime",
    replacement: path.join(uiNodeModules, "react/jsx-runtime"),
  },
  {
    find: "react/jsx-dev-runtime",
    replacement: path.join(uiNodeModules, "react/jsx-dev-runtime"),
  },
  {
    find: "@radix-ui/react-collapsible",
    replacement: path.join(uiNodeModules, "@radix-ui/react-collapsible"),
  },
  {
    find: "@radix-ui/react-compose-refs",
    replacement: path.join(uiNodeModules, "@radix-ui/react-compose-refs"),
  },
  {
    find: "@radix-ui/react-dialog",
    replacement: path.join(uiNodeModules, "@radix-ui/react-dialog"),
  },
  {
    find: "@radix-ui/react-popover",
    replacement: path.join(uiNodeModules, "@radix-ui/react-popover"),
  },
  {
    find: "@radix-ui/react-scroll-area",
    replacement: path.join(uiNodeModules, "@radix-ui/react-scroll-area"),
  },
  {
    find: "@radix-ui/react-select",
    replacement: path.join(uiNodeModules, "@radix-ui/react-select"),
  },
  {
    find: "@radix-ui/react-slot",
    replacement: path.join(uiNodeModules, "@radix-ui/react-slot"),
  },
  {
    find: "@xyflow/react",
    replacement: path.join(uiNodeModules, "@xyflow/react"),
  },
  {
    find: "@xyflow/system",
    replacement: path.join(uiNodeModules, "@xyflow/system"),
  },
  {
    find: "recharts",
    replacement: path.join(uiNodeModules, "recharts"),
  },
  {
    find: "@testing-library/react",
    replacement: path.join(uiNodeModules, "@testing-library/react"),
  },
  {
    find: "@testing-library/user-event",
    replacement: path.join(uiNodeModules, "@testing-library/user-event"),
  },
] as const;

const sharedReactDedupe = [
  "react",
  "react-dom",
  "react-dom/client",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
  "@radix-ui/react-collapsible",
  "@radix-ui/react-compose-refs",
  "@radix-ui/react-dialog",
  "@radix-ui/react-popover",
  "@radix-ui/react-scroll-area",
  "@radix-ui/react-select",
  "@radix-ui/react-slot",
  "@xyflow/react",
  "@xyflow/system",
  "recharts",
] as const;

function createComponentsPackageResolvePlugin(packageRoot: string): Plugin {
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
      ...sharedReactAliases,
      ...createComponentsPackageAliases(componentsPackageRoot),
    ],
    dedupe: [...sharedReactDedupe],
  },
  test: {
    environment: "happy-dom",
    exclude: ["src/**/*.harness.test.ts"],
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
    setupFiles: ["./src/testing/vitest.setup.ts"],
    server: {
      deps: {
        moduleDirectories: [uiNodeModules],
        inline: [
          /^@you-agent-factory\/components/,
          ...sharedReactDedupe,
          "@testing-library/react",
          "@testing-library/user-event",
        ],
      },
    },
  },
});
