import { access, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { describe, expect, it } from "vitest";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./category-paths";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");

async function importBuiltEntry(relativePath: string) {
  return import(pathToFileURL(path.join(distRoot, relativePath)).href);
}

describe("components package production build", () => {
  it("emits ESM and declarations for the root and every category", async () => {
    const entryPaths = [
      "index",
      ...COMPONENT_CATEGORY_EXPORT_PATHS.map(
        (categoryPath) => `${categoryPath}/index`,
      ),
    ];

    await Promise.all(
      entryPaths.flatMap((entryPath) => [
        access(path.join(distRoot, `${entryPath}.js`)),
        access(path.join(distRoot, `${entryPath}.d.ts`)),
      ]),
    );
  });

  it("loads representative exports from emitted JavaScript", async () => {
    const [root, primitives, utilities, charts, graphs] = await Promise.all([
      importBuiltEntry("index.js"),
      importBuiltEntry("primitives/index.js"),
      importBuiltEntry("utilities/index.js"),
      importBuiltEntry("charts/index.js"),
      importBuiltEntry("graphs/index.js"),
    ]);

    expect(root.COMPONENTS_PACKAGE_NAME).toBe("@you-agent-factory/components");
    expect(primitives.Button).toBeDefined();
    expect(utilities.cn("alpha", false, "beta")).toBe("alpha beta");
    expect(charts.ChartContainer).toBeDefined();
    expect(graphs.GraphNodeShell).toBeDefined();
  });

  it("copies a self-contained stylesheet and asset tree", async () => {
    const pendingStylesheets = [path.join(distRoot, "styles.css")];
    const visitedStylesheets = new Set<string>();

    while (pendingStylesheets.length > 0) {
      const stylesheetPath = pendingStylesheets.pop();
      if (!stylesheetPath || visitedStylesheets.has(stylesheetPath)) continue;
      visitedStylesheets.add(stylesheetPath);

      const styles = await readFile(stylesheetPath, "utf8");
      const referencedPaths = [
        ...styles.matchAll(/@import\s+["'](.+?)["']/g),
        ...styles.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g),
      ].map((match) => match[1]);

      for (const referencedPath of referencedPaths) {
        if (/^(?:[a-z]+:|data:|#)/i.test(referencedPath)) continue;
        const resolvedPath = path.resolve(
          path.dirname(stylesheetPath),
          referencedPath.split(/[?#]/, 1)[0],
        );
        await access(resolvedPath);
        if (path.extname(resolvedPath) === ".css") {
          pendingStylesheets.push(resolvedPath);
        }
      }
    }

    expect(visitedStylesheets.size).toBeGreaterThan(1);
  });
});
