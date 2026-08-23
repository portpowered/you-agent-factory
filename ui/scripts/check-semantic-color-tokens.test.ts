import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

import {
  discoverSemanticColorSourceRoots,
  scanSemanticColorTokens,
  scanSemanticColorTokensInRoots,
} from "./check-semantic-color-tokens";

const tempRoots: string[] = [];
const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  process.cwd(),
  "scripts/check-semantic-color-tokens.ts",
);

async function writeSourceFile(
  rootDir: string,
  relativePath: string,
  content: string,
) {
  const absolutePath = path.join(rootDir, relativePath);
  await mkdir(path.dirname(absolutePath), { recursive: true });
  await writeFile(absolutePath, content, "utf8");
}

async function writeExcludedColorSources(sourceRoot: string) {
  const excludedPaths = [
    "features/ignored.test.ts",
    "features/ignored.test.tsx",
    "features/ignored.stories.tsx",
    "generated/ignored.ts",
    "testing/ignored.ts",
    "stories/ignored.ts",
    "messages/ignored.ts",
  ];
  await Promise.all(
    excludedPaths.map((relativePath) =>
      writeSourceFile(
        sourceRoot,
        relativePath,
        'export const ignored = "bg-red-500/50";\n',
      ),
    ),
  );
}

afterEach(async () => {
  await Promise.all(
    tempRoots.map(async (rootDir) => {
      await rm(rootDir, { force: true, recursive: true });
    }),
  );
  tempRoots.length = 0;
});

describe("semantic color source roots", () => {
  it("discovers dashboard and package roots while preserving focused-root mode", async () => {
    const uiRoot = await mkdtemp(path.join(os.tmpdir(), "semantic-color-ui-"));
    tempRoots.push(uiRoot);
    const packageSourceRoot = path.join(
      uiRoot,
      "packages",
      "components",
      "src",
    );
    await mkdir(path.join(uiRoot, "src"), { recursive: true });
    await mkdir(packageSourceRoot, { recursive: true });
    await mkdir(path.join(uiRoot, "packages", "without-src"), {
      recursive: true,
    });

    const roots = await discoverSemanticColorSourceRoots(null, uiRoot);

    expect(
      roots.map((root) =>
        path.relative(uiRoot, root.sourceDirectory).split(path.sep).join("/"),
      ),
    ).toEqual(["src", "packages/components/src"]);

    await expect(
      discoverSemanticColorSourceRoots(packageSourceRoot, uiRoot),
    ).resolves.toEqual([
      {
        reportDirectory: path.join(uiRoot, "packages", "components"),
        sourceDirectory: packageSourceRoot,
      },
    ]);
  });

  it("scans every supplied root and applies exclusions relative to each root", async () => {
    const uiRoot = await mkdtemp(path.join(os.tmpdir(), "semantic-color-ui-"));
    tempRoots.push(uiRoot);
    const dashboardSourceRoot = path.join(uiRoot, "src");
    const packageSourceRoot = path.join(
      uiRoot,
      "packages",
      "components",
      "src",
    );
    const sourceRoots = [
      {
        reportDirectory: uiRoot,
        sourceDirectory: dashboardSourceRoot,
      },
      {
        reportDirectory: uiRoot,
        sourceDirectory: packageSourceRoot,
      },
    ];

    await writeSourceFile(
      dashboardSourceRoot,
      "features/dashboard.tsx",
      'export const dashboard = "bg-blue-500/50";\n',
    );
    await writeSourceFile(
      packageSourceRoot,
      "features/package.tsx",
      'export const packageSource = "bg-red-500/50";\n',
    );

    for (const sourceRoot of [dashboardSourceRoot, packageSourceRoot]) {
      await writeExcludedColorSources(sourceRoot);
    }

    const violations = await scanSemanticColorTokensInRoots(sourceRoots);

    expect(violations).toHaveLength(2);
    expect(
      violations.map((violation) => ({
        filePath: path
          .relative(uiRoot, violation.filePath)
          .split(path.sep)
          .join("/"),
        rootDirectory: path
          .relative(uiRoot, violation.rootDirectory)
          .split(path.sep)
          .join("/"),
        token: violation.token,
      })),
    ).toEqual(
      expect.arrayContaining([
        {
          filePath: "src/features/dashboard.tsx",
          rootDirectory: "src",
          token: "bg-blue-500/50",
        },
        {
          filePath: "packages/components/src/features/package.tsx",
          rootDirectory: "packages/components/src",
          token: "bg-red-500/50",
        },
      ]),
    );
  });
});

describe("scanSemanticColorTokens", () => {
  it("reports slash-opacity color utilities, opacity shortcuts, filter color math, local alpha math, and forbidden foundation tokens", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-token-guard-"),
    );
    tempRoots.push(rootDir);
    await writeSourceFile(
      rootDir,
      "features/example/example.tsx",
      [
        "export function Example() {",
        '  return <div className="text-af-ink/72 text-af-danger-ink opacity-80 brightness-105 [background:rgb(from var(--color-af-overlay) r g b / 0.16)]" style={{ color: "var(--color-af-ink)" }} />;',
        "}",
      ].join("\n"),
    );

    const violations = await scanSemanticColorTokens(rootDir);

    expect(violations).toHaveLength(7);
    expect(violations.map((violation) => violation.kind)).toEqual([
      "alpha-color-utility",
      "foundation-color-token",
      "foundation-color-token",
      "opacity-utility",
      "filter-color-utility",
      "alpha-color-expression",
      "foundation-color-token",
    ]);
  });

  it("reports forbidden helper-layer foundation tokens and opacity utilities in ui/src/styles.css", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-token-guard-"),
    );
    tempRoots.push(rootDir);
    await writeSourceFile(
      rootDir,
      "styles.css",
      [
        ".example-active {",
        "  @apply fill-af-success-ink;",
        "}",
        ".example-semantic {",
        "  @apply fill-af-danger-ink;",
        "}",
        ".example-muted {",
        "  @apply opacity-50;",
        "}",
      ].join("\n"),
    );

    const violations = await scanSemanticColorTokens(rootDir);

    expect(violations).toHaveLength(3);
    expect(violations.map((violation) => violation.kind)).toEqual([
      "foundation-color-token",
      "foundation-color-token",
      "opacity-utility",
    ]);
  });

  it("ignores allowed files, full-opacity visibility utilities, and documented exceptions", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-token-guard-"),
    );
    tempRoots.push(rootDir);
    await writeSourceFile(
      rootDir,
      "features/example/example.tsx",
      [
        "export function Example() {",
        '  return <div className="opacity-0 opacity-100" />;',
        "}",
      ].join("\n"),
    );
    await writeSourceFile(
      rootDir,
      "features/example/guarded.tsx",
      [
        "export function Guarded() {",
        "  // semantic-color-exception: system-integration",
        '  return <div style={{ color: "rgb(from var(--color-af-overlay) r g b / 0.16)" }} />;',
        "}",
      ].join("\n"),
    );
    await writeSourceFile(
      rootDir,
      "features/example/example.test.tsx",
      'export const ignored = "text-af-ink/72 opacity-75";\n',
    );

    const violations = await scanSemanticColorTokens(rootDir);

    expect(violations).toHaveLength(0);
  });
});

describe("semantic color command", () => {
  it("uses AGENT_FACTORY_UI_SRC_DIR as a focused root and reports root-relative diagnostics", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-command-"),
    );
    tempRoots.push(tempRoot);
    const focusedSourceRoot = path.join(tempRoot, "focused", "src");
    const otherSourceRoot = path.join(tempRoot, "other", "src");
    await writeSourceFile(
      focusedSourceRoot,
      "features/clean.tsx",
      'export const clean = "bg-primary";\n',
    );
    await writeSourceFile(
      otherSourceRoot,
      "features/other.tsx",
      'export const other = "bg-red-500/50";\n',
    );

    await expect(
      execFileAsync("bun", [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: focusedSourceRoot,
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining(
        "Semantic color token guard passed: src (0 violations).",
      ),
    });

    await expect(
      execFileAsync("bun", [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: otherSourceRoot,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("src/features/other.tsx:1:"),
    });
  });
});
