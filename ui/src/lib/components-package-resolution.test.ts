// @vitest-environment node

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
    // Comment-only placeholder CSS may inline as an empty string before token migration.
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
