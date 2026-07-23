// @vitest-environment node

import path from "node:path";
import {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  COMPONENTS_PACKAGE_NAME,
} from "@you-agent-factory/components";
import * as primitives from "@you-agent-factory/components/primitives";
import stylesCss from "@you-agent-factory/components/styles.css?inline";
import { createServer, type ViteDevServer } from "vite";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import viteConfig from "../../vite.config";
import { dashboardComponentsPackageName } from "./components-package-resolution";

const importerPath = path.join(
  import.meta.dirname,
  "components-package-resolution.test.ts",
);

describe("dashboard @you-agent-factory/components package resolution", () => {
  let viteServer: ViteDevServer;

  beforeAll(async () => {
    viteServer = await createServer(viteConfig);
  });

  afterAll(async () => {
    await viteServer.close();
  });

  it("imports the package root through the configured package path", () => {
    expect(dashboardComponentsPackageName).toBe(
      "@you-agent-factory/components",
    );
    expect(COMPONENTS_PACKAGE_NAME).toBe("@you-agent-factory/components");
  });

  it("imports the CSS entrypoint through the dashboard Vite resolver", async () => {
    expect(typeof stylesCss).toBe("string");

    const resolved = await viteServer.pluginContainer.resolveId(
      "@you-agent-factory/components/styles.css",
      importerPath,
      { ssr: false },
    );

    expect(resolved?.id).toContain("packages/components/src/styles.css");
  });

  it("imports a deep category path through the dashboard Vite resolver", () => {
    expect(primitives.COMPONENTS_CATEGORY).toBe("primitives");
  });

  it("resolves the CSS entrypoint with Vite resolveId", async () => {
    const resolved = await viteServer.pluginContainer.resolveId(
      "@you-agent-factory/components/styles.css",
      importerPath,
      { ssr: false },
    );

    expect(resolved?.id).toContain("packages/components/src/styles.css");
    expect(resolved?.id).not.toContain("index.ts");
  });

  it.each(COMPONENT_CATEGORY_EXPORT_PATHS)(
    "resolves deep import @you-agent-factory/components/%s with Vite resolveId",
    async (categoryPath) => {
      const resolved = await viteServer.pluginContainer.resolveId(
        `@you-agent-factory/components/${categoryPath}`,
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
