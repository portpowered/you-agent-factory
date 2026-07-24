import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

import { scanPackageBoundary } from "../scripts/check-package-boundary.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const scriptPath = path.join(
  packageRoot,
  "scripts",
  "check-package-boundary.mjs",
);
const dashboardSrcDir = path.resolve(packageRoot, "..", "..", "src");

async function createPackageTree(
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

describe("package boundary harness", () => {
  let tempRoots: string[] = [];

  afterEach(async () => {
    await Promise.all(
      tempRoots.map((tempRoot) =>
        rm(tempRoot, { force: true, recursive: true }),
      ),
    );
    tempRoots = [];
  });

  it("passes for the real package source tree", async () => {
    await expect(
      scanPackageBoundary(path.join(packageRoot, "src"), dashboardSrcDir),
    ).resolves.toEqual({
      packageDir: packageRoot,
      violations: [],
    });
  });

  it("flags dashboard API, feature, generated client, and i18n imports", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-boundary-dashboard-imports-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "widgets/api-widget.tsx":
          'import { apiClient } from "../../dashboard-src/api/client";\nexport function ApiWidget() { return null; }\n',
        "widgets/generated-widget.tsx":
          'import type { Factory } from "../../dashboard-src/api/generated/openapi";\nexport function GeneratedWidget() { return null; }\n',
        "widgets/feature-widget.tsx":
          'import { DashboardScreen } from "../../dashboard-src/features/dashboard/components/dashboard-screen";\nexport function FeatureWidget() { return null; }\n',
        "widgets/i18n-widget.tsx":
          'import { AppLocaleProvider } from "../../dashboard-src/i18n/app-locale";\nexport function I18nWidget() { return null; }\n',
        "widgets/session-widget.tsx":
          'import { DashboardSessionProvider } from "../../dashboard-src/features/dashboard/session/dashboard-session-provider";\nexport function SessionWidget() { return null; }\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "api/client.ts",
      "export const apiClient = {};\n",
    );
    await createDashboardFixture(
      tempRoot,
      "api/generated/openapi.ts",
      "export type Factory = {};\n",
    );
    await createDashboardFixture(
      tempRoot,
      "features/dashboard/components/dashboard-screen.tsx",
      "export function DashboardScreen() { return null; }\n",
    );
    await createDashboardFixture(
      tempRoot,
      "i18n/app-locale.tsx",
      "export function AppLocaleProvider() { return null; }\n",
    );
    await createDashboardFixture(
      tempRoot,
      "features/dashboard/session/dashboard-session-provider.tsx",
      "export function DashboardSessionProvider() { return null; }\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "dashboard-api-import",
          importPath: "../../dashboard-src/api/client",
          relativeFilePath: "src/widgets/api-widget.tsx",
        }),
        expect.objectContaining({
          kind: "generated-client-import",
          importPath: "../../dashboard-src/api/generated/openapi",
          relativeFilePath: "src/widgets/generated-widget.tsx",
        }),
        expect.objectContaining({
          kind: "dashboard-feature-import",
          importPath:
            "../../dashboard-src/features/dashboard/components/dashboard-screen",
          relativeFilePath: "src/widgets/feature-widget.tsx",
        }),
        expect.objectContaining({
          kind: "dashboard-i18n-import",
          importPath: "../../dashboard-src/i18n/app-locale",
          relativeFilePath: "src/widgets/i18n-widget.tsx",
        }),
        expect.objectContaining({
          kind: "dashboard-session-provider-import",
          importPath:
            "../../dashboard-src/features/dashboard/session/dashboard-session-provider",
          relativeFilePath: "src/widgets/session-widget.tsx",
        }),
      ]),
    );
  });

  it("flags app-only runtime modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-boundary-runtime-imports-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "runtime/query.tsx":
          'import { useQuery } from "@tanstack/react-query";\nexport function QueryWidget() { return useQuery({ queryKey: ["x"], queryFn: async () => null }); }\n',
        "runtime/state.tsx":
          'import { create } from "zustand";\nexport const useWidgetState = create(() => ({}));\n',
        "runtime/monaco.tsx":
          'import Editor from "@monaco-editor/react";\nexport function MonacoWidget() { return <Editor />; }\n',
        "runtime/sonner.tsx":
          'import { toast } from "sonner";\nexport function SonnerWidget() { toast("hi"); return null; }\n',
        "runtime/grid.tsx":
          'import { GridLayout } from "react-grid-layout";\nexport function GridWidget() { return <GridLayout layout={[]} width={320} />; }\n',
      },
      tempRoot,
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      path.join(tempRoot, "dashboard-src"),
    );

    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "app-runtime-module",
          importPath: "@tanstack/react-query",
          relativeFilePath: "src/runtime/query.tsx",
        }),
        expect.objectContaining({
          kind: "app-runtime-module",
          importPath: "zustand",
          relativeFilePath: "src/runtime/state.tsx",
        }),
        expect.objectContaining({
          kind: "app-runtime-module",
          importPath: "@monaco-editor/react",
          relativeFilePath: "src/runtime/monaco.tsx",
        }),
        expect.objectContaining({
          kind: "app-runtime-module",
          importPath: "sonner",
          relativeFilePath: "src/runtime/sonner.tsx",
        }),
        expect.objectContaining({
          kind: "app-runtime-module",
          importPath: "react-grid-layout",
          relativeFilePath: "src/runtime/grid.tsx",
        }),
      ]),
    );
  });

  it("passes for allowed package-owned and third-party presentation imports", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-boundary-allowed-imports-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "allowed/primitive.tsx":
          'import { cn } from "../utilities/cn";\nexport function AllowedPrimitive({ className }: { className?: string }) { return <span className={cn("text-body-medium", className)} />; }\n',
        "utilities/cn.ts":
          'export function cn(...values: string[]) { return values.join(" "); }\n',
      },
      tempRoot,
    );

    await expect(
      scanPackageBoundary(packageSrcDir, path.join(tempRoot, "dashboard-src")),
    ).resolves.toEqual({
      packageDir: path.dirname(packageSrcDir),
      violations: [],
    });
  });

  it("flags dashboard API directory imports that resolve through index files", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-boundary-dashboard-index-imports-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "widgets/bad.ts":
          'import { apiClient } from "../../dashboard-src/api";\nexport const bad = apiClient;\n',
      },
      tempRoot,
    );
    const fixtureDashboardSrcDir = path.join(tempRoot, "dashboard-src");

    await createDashboardFixture(
      tempRoot,
      "api/index.ts",
      "export const apiClient = {};\n",
    );

    const report = await scanPackageBoundary(
      packageSrcDir,
      fixtureDashboardSrcDir,
    );

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "dashboard-api-import",
        importPath: "../../dashboard-src/api",
        relativeFilePath: "src/widgets/bad.ts",
      }),
    ]);
  });

  it("CLI fails with actionable output for boundary violations", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-boundary-cli-failure-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "widgets/bad.tsx":
          'import { toast } from "sonner";\nexport function BadWidget() { toast("nope"); return null; }\n',
      },
      tempRoot,
    );

    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: path.dirname(packageSrcDir),
        env: {
          ...process.env,
          AGENT_FACTORY_COMPONENTS_SRC_DIR: packageSrcDir,
          AGENT_FACTORY_DASHBOARD_SRC_DIR: path.join(tempRoot, "dashboard-src"),
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "@you-agent-factory/components package boundary check failed:",
      ),
    });
  }, 60_000);
});
