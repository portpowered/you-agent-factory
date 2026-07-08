// @vitest-environment node

import path from "node:path";
import { createServer, type ViteDevServer } from "vite";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  COMPONENTS_PACKAGE_NAME,
} from "@you-agent-factory/components";
import stylesCss from "@you-agent-factory/components/styles.css?inline";
import * as primitives from "@you-agent-factory/components/primitives";
import { dashboardComponentsPackageName } from "./components-package-resolution";
import viteConfig from "../../vite.config";

const importerPath = path.join(
  import.meta.dirname,
  "components-package-resolution.test.ts",
);
const packageGraphImporterPath = path.join(
  import.meta.dirname,
  "../../packages/components/src/graphs/graph-node-handle-badge.tsx",
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
    expect(dashboardComponentsPackageName).toBe("@you-agent-factory/components");
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

  it("resolves React Flow imports from package graph primitives to the dashboard singleton", async () => {
    const resolved = await viteServer.pluginContainer.resolveId(
      "@xyflow/react",
      packageGraphImporterPath,
      { ssr: false },
    );

    expect(resolved?.id).toMatch(/node_modules\/(?:\.vite\/deps\/)?@xyflow/);
    expect(resolved?.id).not.toContain("packages/components/node_modules");
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
