import path from "node:path";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./category-paths";

type ComponentsPackageAlias = {
  find: string | RegExp;
  replacement: string;
};

/** Vite/Vitest alias entries for `youagentfactory/components` root, CSS, and deep category paths. */
export function createComponentsPackageAliases(
  componentsPackageRoot: string,
): ComponentsPackageAlias[] {
  const aliases: ComponentsPackageAlias[] = [
    {
      // Exact root match only; a prefix alias incorrectly captures subpaths such as
      // `youagentfactory/components/styles.css`.
      find: /^youagentfactory\/components$/,
      replacement: path.join(componentsPackageRoot, "index.ts"),
    },
    {
      find: /^youagentfactory\/components\/styles\.css(\?.*)?$/,
      replacement: path.join(componentsPackageRoot, "styles.css"),
    },
  ];

  for (const categoryPath of COMPONENT_CATEGORY_EXPORT_PATHS) {
    aliases.push({
      find: `youagentfactory/components/${categoryPath}`,
      replacement: path.join(
        componentsPackageRoot,
        categoryPath,
        "index.ts",
      ),
    });
  }

  return aliases;
}
