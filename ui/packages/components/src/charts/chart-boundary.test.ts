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
const chartsSrcDir = path.join(packageRoot, "src", "charts");
const dashboardSrcDir = path.resolve(packageRoot, "..", "..", "src");

async function createChartsFixture(
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
  return filePath;
}

describe("chart package boundary", () => {
  let tempRoots: string[] = [];

  afterEach(async () => {
    await Promise.all(
      tempRoots.map((tempRoot) =>
        rm(tempRoot, { force: true, recursive: true }),
      ),
    );
    tempRoots = [];
  });

  it("passes for the real chart package source tree", async () => {
    await expect(
      scanPackageBoundary(chartsSrcDir, dashboardSrcDir),
    ).resolves.toEqual({
      packageDir: path.join(packageRoot, "src"),
      violations: [],
    });
  });

  it("fails when chart code imports dashboard work-outcome feature modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "chart-boundary-work-outcome-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createChartsFixture(
      {
        "charts/work-chart-bridge.tsx":
          'import { WorkChart } from "../../dashboard-src/features/work-outcome/components/work-chart";\nexport function WorkChartBridge() { return <WorkChart ariaLabel="x" series={[]} />; }\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "features/work-outcome/components/work-chart.tsx",
      "export function WorkChart() { return null; }\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "dashboard-feature-import",
        importPath:
          "../../dashboard-src/features/work-outcome/components/work-chart",
        relativeFilePath: "src/charts/work-chart-bridge.tsx",
      }),
    ]);
  });

  it("fails when chart code imports generated dashboard API modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "chart-boundary-generated-api-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createChartsFixture(
      {
        "charts/api-types.tsx":
          'import type { FactoryWork } from "../../dashboard-src/api/generated/openapi";\nexport type ChartWork = FactoryWork;\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "api/generated/openapi.ts",
      "export type FactoryWork = {};\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "generated-client-import",
        importPath: "../../dashboard-src/api/generated/openapi",
        relativeFilePath: "src/charts/api-types.tsx",
      }),
    ]);
  });
});
