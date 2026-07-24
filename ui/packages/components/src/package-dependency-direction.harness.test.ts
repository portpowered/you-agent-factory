import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

import { scanPackageDependencyDirection } from "../scripts/check-package-dependency-direction.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const scriptPath = path.join(
  packageRoot,
  "scripts",
  "check-package-dependency-direction.mjs",
);

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

describe("package dependency-direction harness", () => {
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
      scanPackageDependencyDirection(path.join(packageRoot, "src")),
    ).resolves.toEqual({
      packageDir: packageRoot,
      violations: [],
    });
  });

  it("flags lower layers importing higher package layers", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-dependency-direction-violations-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "primitives/bad-primitive.tsx":
          'import { SettingsSection } from "../recipes/settings-section";\nexport function BadPrimitive() { return <SettingsSection />; }\n',
        "recipes/settings-section.tsx":
          "export function SettingsSection() { return <section />; }\n",
        "tokens/bad-tokens.ts":
          'import { cn } from "../utilities/cn";\nexport const tokenValue = cn("x");\n',
        "utilities/cn.ts":
          'export function cn(...values: string[]) { return values.join(" "); }\n',
      },
      tempRoot,
    );

    const report = await scanPackageDependencyDirection(packageSrcDir);

    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "package-layer-violation",
          importPath: "../recipes/settings-section",
          relativeFilePath: "src/primitives/bad-primitive.tsx",
          sourceLayer: "primitives",
          targetLayer: "recipes",
        }),
        expect.objectContaining({
          kind: "package-layer-violation",
          importPath: "../utilities/cn",
          relativeFilePath: "src/tokens/bad-tokens.ts",
          sourceLayer: "tokens",
          targetLayer: "utilities",
        }),
      ]),
    );
  });

  it("allows valid dependency directions across package layers", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-dependency-direction-allowed-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "forms/field.tsx":
          'import { PackageText } from "../primitives/package-text";\nimport { cn } from "../utilities/cn";\nexport function Field() { return <PackageText className={cn("text-body-medium")} />; }\n',
        "primitives/package-text.tsx":
          'import { cn } from "../utilities/cn";\nexport function PackageText({ className }: { className?: string }) { return <p className={className} />; }\n',
        "recipes/settings-section.tsx":
          'import { Field } from "../forms/field";\nexport function SettingsSection() { return <Field />; }\n',
        "utilities/cn.ts":
          'export function cn(...values: string[]) { return values.join(" "); }\n',
      },
      tempRoot,
    );

    await expect(
      scanPackageDependencyDirection(packageSrcDir),
    ).resolves.toEqual({
      packageDir: path.dirname(packageSrcDir),
      violations: [],
    });
  });

  it("flags production imports of testing support modules", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-dependency-direction-testing-import-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "primitives/bad-primitive.tsx":
          'import { renderPackageComponent } from "../testing/render";\nexport function BadPrimitive() { return renderPackageComponent(<span />); }\n',
        "testing/render.tsx":
          "export function renderPackageComponent(node: unknown) { return node; }\n",
      },
      tempRoot,
    );

    const report = await scanPackageDependencyDirection(packageSrcDir);

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "testing-support-import",
        importPath: "../testing/render",
        relativeFilePath: "src/primitives/bad-primitive.tsx",
        sourceLayer: "primitives",
        targetLayer: "testing",
      }),
    ]);
  });

  it("flags lower layers importing higher package layers through directory index files", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-dependency-direction-index-imports-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "primitives/bad.ts":
          'import { SettingsSection } from "../recipes";\nexport const bad = SettingsSection;\n',
        "recipes/index.ts":
          "export function SettingsSection() { return null; }\n",
      },
      tempRoot,
    );

    const report = await scanPackageDependencyDirection(packageSrcDir);

    expect(report.violations).toEqual([
      expect.objectContaining({
        kind: "package-layer-violation",
        importPath: "../recipes",
        relativeFilePath: "src/primitives/bad.ts",
        sourceLayer: "primitives",
        targetLayer: "recipes",
      }),
    ]);
  });

  it("CLI fails with actionable output for dependency-direction violations", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "package-dependency-direction-cli-failure-"),
    );
    tempRoots.push(tempRoot);

    const packageSrcDir = await createPackageTree(
      {
        "primitives/bad.tsx":
          'import { SettingsSection } from "@you-agent-factory/components/recipes";\nexport function Bad() { return <SettingsSection />; }\n',
        "recipes/index.ts":
          "export function SettingsSection() { return null; }\n",
      },
      tempRoot,
    );

    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: path.dirname(packageSrcDir),
        env: {
          ...process.env,
          AGENT_FACTORY_COMPONENTS_SRC_DIR: packageSrcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "@you-agent-factory/components package dependency-direction check failed:",
      ),
    });
  }, 60_000);
});
