import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import { mergeConfig } from "vite";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "../src/category-paths.ts";
import { createComponentsPackageAliases } from "../src/vite-aliases.ts";

const storybookDir = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(storybookDir, "..");
const componentsPackageRoot = path.join(packageRoot, "src");

function createComponentsPackageResolvePlugin(
  packageSrcRoot: string,
): Plugin {
  const categoryAliases = new Map(
    COMPONENT_CATEGORY_EXPORT_PATHS.map((categoryPath) => [
      `youagentfactory/components/${categoryPath}`,
      path.join(packageSrcRoot, categoryPath, "index.ts"),
    ]),
  );

  return {
    name: "youagentfactory-components-package-resolve",
    resolveId(source) {
      if (source === "youagentfactory/components") {
        return path.join(packageSrcRoot, "index.ts");
      }

      const [specifierPath] = source.split("?", 1);
      if (specifierPath === "youagentfactory/components/styles.css") {
        return path.join(packageSrcRoot, "styles.css");
      }

      const categoryTarget = categoryAliases.get(specifierPath);
      if (categoryTarget) {
        return categoryTarget;
      }

      return null;
    },
  };
}

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  async viteFinal(config) {
    return mergeConfig(config, {
      plugins: [
        react(),
        tailwindcss(),
        createComponentsPackageResolvePlugin(componentsPackageRoot),
      ],
      resolve: {
        alias: createComponentsPackageAliases(componentsPackageRoot),
      },
    });
  },
};

export default config;
