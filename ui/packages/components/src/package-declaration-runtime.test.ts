import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

import {
  formatDeclarationRuntimeViolations,
  scanDeclarationRuntimeReferences,
  verifyBundledPackage,
} from "../scripts/check-declaration-runtime.mjs";

const temporaryDirectories: string[] = [];
const execFileAsync = promisify(execFile);
const guardScriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../scripts/check-declaration-runtime.mjs",
);

async function createArtifact(files: Record<string, string>) {
  const root = await mkdtemp(
    path.join(tmpdir(), "components-declaration-runtime-"),
  );
  temporaryDirectories.push(root);

  await Promise.all(
    Object.entries(files).map(async ([relativePath, source]) => {
      const filePath = path.join(root, relativePath);
      await mkdir(path.dirname(filePath), { recursive: true });
      await writeFile(filePath, source);
    }),
  );
  return root;
}

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { force: true, recursive: true })),
  );
});

describe("components declaration/runtime guard", () => {
  it("passes the clean bundled components package output", async () => {
    const packageRoot = await createArtifact({
      "package.json": JSON.stringify({
        exports: {
          ".": {
            types: "./dist/index.d.ts",
            import: "./dist/index.js",
            default: "./dist/index.js",
          },
        },
      }),
      "dist/index.d.ts": 'export type { Widget } from "./widget";\n',
      "dist/index.js": "export {};\n",
      "dist/widget.d.ts": "export type Widget = string;\n",
      "dist/widget.js": "export {};\n",
    });

    await expect(verifyBundledPackage({ packageRoot })).resolves.toMatchObject({
      declarationCount: 2,
      runtimeCount: 2,
      diagnostics: [],
    });
  });

  it("reports the missing runtime sibling in the graph-shaped artifact", async () => {
    const distRoot = await createArtifact({
      "graphs/index.d.ts": 'export { GraphEdge } from "./graph-edge";\n',
      "graphs/index.js": "export {};\n",
      "graphs/graph-edge.d.ts":
        "export declare function GraphEdge(): unknown;\n",
    });

    const report = await scanDeclarationRuntimeReferences({ distRoot });

    expect(report.violations).toHaveLength(1);
    expect(
      formatDeclarationRuntimeViolations({
        distRoot,
        violations: report.violations,
      }),
    ).toEqual([
      "- graphs/index.d.ts -> ./graph-edge (runtime sibling not found; tried graphs/graph-edge.js, graphs/graph-edge.mjs, graphs/graph-edge.cjs, graphs/graph-edge.jsx, graphs/graph-edge/index.js, graphs/graph-edge/index.mjs, graphs/graph-edge/index.cjs, graphs/graph-edge/index.jsx)",
    ]);
  });

  it("passes an entrypoint-aligned artifact with a runtime counterpart", async () => {
    const distRoot = await createArtifact({
      "graphs/index.d.ts": 'export { GraphEdge } from "./graph-edge";\n',
      "graphs/index.js": "export {};\n",
      "graphs/graph-edge.d.ts":
        "export declare function GraphEdge(): unknown;\n",
      "graphs/graph-edge.js": "export function GraphEdge() {}\n",
    });

    await expect(
      scanDeclarationRuntimeReferences({ distRoot }),
    ).resolves.toMatchObject({
      declarationCount: 2,
      referenceCount: 1,
      violations: [],
    });
  });
});

describe("components package runtime conditions", () => {
  it("applies the guard to package runtime conditions", async () => {
    const packageRoot = await createArtifact({
      "package.json": JSON.stringify({
        exports: {
          ".": {
            types: "./dist/graphs/index.d.ts",
            import: "./dist/graphs/index.d.ts",
            default: "./dist/graphs/index.d.ts",
          },
        },
      }),
      "dist/graphs/index.d.ts": 'export * from "./graph-edge";\n',
      "dist/graphs/graph-edge.d.ts":
        "export declare const GraphEdge: unknown;\n",
    });

    await expect(verifyBundledPackage({ packageRoot })).resolves.toMatchObject({
      diagnostics: [
        expect.stringContaining("graphs/index.d.ts -> ./graph-edge"),
      ],
    });

    const alignedPackageRoot = await createArtifact({
      "package.json": JSON.stringify({
        exports: {
          ".": {
            types: "./dist/graphs/index.d.ts",
            import: "./dist/graphs/index.js",
            default: "./dist/graphs/index.js",
          },
        },
      }),
      "dist/graphs/index.d.ts": 'export * from "./graph-edge";\n',
      "dist/graphs/index.js": "export {};\n",
      "dist/graphs/graph-edge.d.ts":
        "export declare const GraphEdge: unknown;\n",
      "dist/graphs/graph-edge.js": "export {};\n",
    });

    await expect(
      verifyBundledPackage({ packageRoot: alignedPackageRoot }),
    ).resolves.toMatchObject({
      diagnostics: [],
    });
  });
});

describe("components declaration module resolution", () => {
  it("handles explicit extensions, directory entrypoints, and ignores non-relative imports", async () => {
    const distRoot = await createArtifact({
      "index.d.ts": [
        'import type { Widget } from "external-package";',
        'export type { Widget } from "./widget.js";',
        'export type { Entry } from "./entry";',
        "",
      ].join("\n"),
      "index.js": "export {};\n",
      "widget.d.ts": "export type Widget = string;\n",
      "widget.js": "export {};\n",
      "entry/index.d.ts": "export type Entry = string;\n",
      "entry/index.mjs": "export {};\n",
    });

    await expect(
      scanDeclarationRuntimeReferences({ distRoot }),
    ).resolves.toMatchObject({
      declarationCount: 3,
      referenceCount: 2,
      violations: [],
    });
  });

  it("keeps the strict CLI diagnostic actionable", async () => {
    const distRoot = await createArtifact({
      "graphs/index.d.ts": 'export * from "./missing";\n',
    });
    await expect(
      execFileAsync(process.execPath, [
        guardScriptPath,
        "--strict",
        "--dist-root",
        distRoot,
      ]),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("graphs/index.d.ts -> ./missing"),
    });

    const report = await scanDeclarationRuntimeReferences({ distRoot });
    expect(
      formatDeclarationRuntimeViolations({
        distRoot,
        violations: report.violations,
      }),
    ).toHaveLength(1);
  });
});
