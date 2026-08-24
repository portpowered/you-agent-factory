import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

import {
  discoverSemanticColorSourceRoots,
  scanSemanticColorTokens,
  scanSemanticColorTokensInRoots,
} from "./check-semantic-color-tokens";
import { forbiddenFoundationTokenNames } from "./semantic-color-foundation-policy";

const tempRoots: string[] = [];
const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  process.cwd(),
  "scripts/check-semantic-color-tokens.ts",
);
const packagePaletteCssPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "packages/components/src/styles/color-palette-presets.css",
);

function declaredFoundationTokenNames(source: string) {
  return [
    ...new Set(
      [...source.matchAll(/^\s*(--color-af-foundation-[a-z0-9-]+)\s*:/gm)].map(
        (match) => match[1],
      ),
    ),
  ].sort();
}

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

describe("foundation token policy", () => {
  it("keeps every canonical package foundation key represented by the forbidden policy", async () => {
    const packagePaletteCss = await readFile(packagePaletteCssPath, "utf8");
    const declaredTokens = declaredFoundationTokenNames(packagePaletteCss);
    const policyTokens = forbiddenFoundationTokenNames
      .filter((token) => token.startsWith("af-foundation-"))
      .map((token) => `--color-${token}`)
      .sort();

    expect(
      declaredTokens.filter((token) => !policyTokens.includes(token)),
    ).toEqual([]);
    expect(declaredTokens).toHaveLength(28);
  });

  it("rejects direct utility and variable consumption of package foundation keys with positions", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-foundation-guard-"),
    );
    tempRoots.push(rootDir);
    const packagePaletteCss = await readFile(packagePaletteCssPath, "utf8");
    const foundationTokens = declaredFoundationTokenNames(
      packagePaletteCss,
    ).map((token) => token.replace("--color-", ""));
    const source = foundationTokens
      .flatMap((token) => [
        `export const utility = "bg-${token}";`,
        `export const variable = "var(--color-${token})";`,
      ])
      .join("\n");
    await writeSourceFile(rootDir, "components/foundation.tsx", source);

    const violations = await scanSemanticColorTokens(rootDir);

    expect(violations).toHaveLength(foundationTokens.length * 2);
    expect(
      violations.every(
        (violation) => violation.kind === "foundation-color-token",
      ),
    ).toBe(true);
    expect(violations.map((violation) => violation.token)).toEqual(
      expect.arrayContaining([
        "bg-af-foundation-accent",
        "var(--color-af-foundation-accent)",
        "bg-af-foundation-worker-ink",
        "var(--color-af-foundation-worker-ink)",
      ]),
    );

    const firstUtility = violations.find(
      (violation) => violation.token === "bg-af-foundation-accent",
    );
    expect(firstUtility).toMatchObject({
      filePath: path.join(rootDir, "components/foundation.tsx"),
      position: {
        line: 1,
        column: source.indexOf("bg-af-foundation-accent") + 1,
      },
    });
  });

  it("continues rejecting every legacy foundation alias", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-legacy-guard-"),
    );
    tempRoots.push(rootDir);
    const legacyTokens = forbiddenFoundationTokenNames.filter(
      (token) => !token.startsWith("af-foundation-"),
    );
    const source = legacyTokens
      .flatMap((token) => [
        `export const utility = "bg-${token}";`,
        `export const variable = "var(--color-${token})";`,
      ])
      .join("\n");
    await writeSourceFile(rootDir, "components/legacy.tsx", source);

    const violations = await scanSemanticColorTokens(rootDir);

    expect(legacyTokens).toHaveLength(10);
    expect(violations).toHaveLength(legacyTokens.length * 2);
    expect(new Set(violations.map((violation) => violation.kind))).toEqual(
      new Set(["foundation-color-token"]),
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
