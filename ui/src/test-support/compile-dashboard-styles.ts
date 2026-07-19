import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile, type Resolver } from "@tailwindcss/node";

const uiRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
);
const componentsPackageStylesPath = path.join(
  uiRoot,
  "packages",
  "components",
  "src",
  "styles.css",
);
const factoryVisualizersPackageStylesPath = path.join(
  uiRoot,
  "packages",
  "factory-visualizers",
  "src",
  "styles.css",
);

export function createComponentsPackageCssResolver(): Resolver {
  return async (id) => {
    if (id === "@you-agent-factory/components/styles.css") {
      return componentsPackageStylesPath;
    }
    if (id === "@you-agent-factory/factory-visualizers/styles.css") {
      return factoryVisualizersPackageStylesPath;
    }
    return undefined;
  };
}

export async function compileDashboardStyles(
  stylesSourcePath: string,
): Promise<string> {
  const source = readFileSync(stylesSourcePath, "utf8");
  const compiled = await compile(source, {
    base: path.dirname(stylesSourcePath),
    from: stylesSourcePath,
    onDependency: () => {},
    customCssResolver: createComponentsPackageCssResolver(),
  });
  return compiled.build([]);
}
