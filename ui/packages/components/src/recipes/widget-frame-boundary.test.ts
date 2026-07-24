import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

import { scanPackageBoundary } from "../../scripts/check-package-boundary.mjs";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
);
const recipesSrcDir = path.join(packageRoot, "src", "recipes");
const dashboardSrcDir = path.resolve(packageRoot, "..", "..", "src");

async function createRecipesFixture(
  files: Record<string, string>,
  tempRoot: string,
) {
  const packageSrcDir = path.join(tempRoot, "src");
  await mkdir(packageSrcDir, { recursive: true });

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(packageSrcDir, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return packageSrcDir;
}

async function createDashboardFixture(
  tempRoot: string,
  relativeFilePath: string,
  contents: string,
) {
  const filePath = path.join(tempRoot, "dashboard-src", relativeFilePath);
  await mkdir(path.dirname(filePath), { recursive: true });
  await writeFile(filePath, contents);
}

describe("widget frame package boundary", () => {
  let tempRoots: string[] = [];

  afterEach(async () => {
    await Promise.all(
      tempRoots.map((tempRoot) =>
        rm(tempRoot, { force: true, recursive: true }),
      ),
    );
    tempRoots = [];
  });

  it("passes for the real recipes source tree", async () => {
    await expect(
      scanPackageBoundary(recipesSrcDir, dashboardSrcDir),
    ).resolves.toEqual({
      packageDir: path.join(packageRoot, "src"),
      violations: [],
    });
  });

  it("fails when recipe code imports dashboard bento feature modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "widget-frame-boundary-bento-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createRecipesFixture(
      {
        "recipes/bento-bridge.tsx":
          'import { AgentBentoCard } from "../../dashboard-src/features/bento/components/agent-bento";\nexport function BentoBridge() { return <AgentBentoCard title="x">y</AgentBentoCard>; }\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "features/bento/components/agent-bento.tsx",
      "export function AgentBentoCard() { return null; }\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "dashboard-feature-import",
        importPath: "../../dashboard-src/features/bento/components/agent-bento",
        relativeFilePath: "src/recipes/bento-bridge.tsx",
      }),
    ]);
  });

  it("fails when recipe code imports dashboard widget frame modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "widget-frame-boundary-dashboard-frame-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createRecipesFixture(
      {
        "recipes/dashboard-frame-bridge.tsx":
          'import { DashboardWidgetFrame } from "../../dashboard-src/features/bento/components/dashboard-widget-frame/dashboard-widget-frame";\nexport function DashboardFrameBridge() { return <DashboardWidgetFrame title="x" widgetId="y">z</DashboardWidgetFrame>; }\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "features/bento/components/dashboard-widget-frame/dashboard-widget-frame.tsx",
      "export function DashboardWidgetFrame() { return null; }\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "dashboard-feature-import",
        importPath:
          "../../dashboard-src/features/bento/components/dashboard-widget-frame/dashboard-widget-frame",
        relativeFilePath: "src/recipes/dashboard-frame-bridge.tsx",
      }),
    ]);
  });

  it("fails when recipe code imports react-grid-layout", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "widget-frame-boundary-grid-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createRecipesFixture(
      {
        "recipes/grid-layout-bridge.tsx":
          'import { GridLayout } from "react-grid-layout";\nexport function GridBridge() { return <GridLayout layout={[]} width={320} />; }\n',
      },
      tempRoot,
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      path.join(tempRoot, "dashboard-src"),
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "app-runtime-module",
        importPath: "react-grid-layout",
        relativeFilePath: "src/recipes/grid-layout-bridge.tsx",
      }),
    ]);
  });
});
