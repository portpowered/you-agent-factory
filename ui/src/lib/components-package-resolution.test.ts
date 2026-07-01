// @vitest-environment node

import { readFileSync } from "node:fs";
import path from "node:path";
import { createServer, type ViteDevServer } from "vite";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  COMPONENTS_PACKAGE_NAME,
} from "youagentfactory/components";
import stylesCss from "youagentfactory/components/styles.css?inline";
import * as primitives from "youagentfactory/components/primitives";
import { dashboardComponentsPackageName } from "./components-package-resolution";
import viteConfig from "../../vite.config";

const importerPath = path.join(
  import.meta.dirname,
  "components-package-resolution.test.ts",
);
const componentsPackageStylesDir = path.resolve(
  import.meta.dirname,
  "..",
  "..",
  "packages",
  "components",
  "src",
  "styles",
);

describe("dashboard youagentfactory/components package resolution", () => {
  let viteServer: ViteDevServer;

  beforeAll(async () => {
    viteServer = await createServer(viteConfig);
  });

  afterAll(async () => {
    await viteServer.close();
  });

  it("imports the package root through the configured package path", () => {
    expect(dashboardComponentsPackageName).toBe("youagentfactory/components");
    expect(COMPONENTS_PACKAGE_NAME).toBe("youagentfactory/components");
  });

  it("imports the CSS entrypoint through the dashboard Vite resolver", () => {
    expect(typeof stylesCss).toBe("string");

    const palettePresets = readFileSync(
      path.join(componentsPackageStylesDir, "color-palette-presets.css"),
      "utf8",
    );
    const layoutTokens = readFileSync(
      path.join(componentsPackageStylesDir, "layout-role-tokens.css"),
      "utf8",
    );
    expect(palettePresets).toContain("--color-af-foundation-background");
    expect(layoutTokens).toContain("--spacing-layout-tight:");
  });

  it("imports a deep category path through the dashboard Vite resolver", () => {
    expect(primitives.COMPONENTS_CATEGORY).toBe("primitives");
  });

  it("resolves the CSS entrypoint with Vite resolveId", async () => {
    const resolved = await viteServer.pluginContainer.resolveId(
      "youagentfactory/components/styles.css",
      importerPath,
      { ssr: false },
    );

    expect(resolved?.id).toContain("packages/components/src/styles.css");
    expect(resolved?.id).not.toContain("index.ts");
  });

  it.each(COMPONENT_CATEGORY_EXPORT_PATHS)(
    "resolves deep import youagentfactory/components/%s with Vite resolveId",
    async (categoryPath) => {
      const resolved = await viteServer.pluginContainer.resolveId(
        `youagentfactory/components/${categoryPath}`,
        importerPath,
        { ssr: false },
      );

      expect(resolved?.id).toContain(
        `packages/components/src/${categoryPath}/index.ts`,
      );
      expect(resolved?.id).not.toContain("index.ts/");
    },
  );
});
