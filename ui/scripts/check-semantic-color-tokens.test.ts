import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { scanSemanticColorTokens } from "./check-semantic-color-tokens";

const tempRoots: string[] = [];

async function writeSourceFile(
  rootDir: string,
  relativePath: string,
  content: string,
) {
  const absolutePath = path.join(rootDir, relativePath);
  await mkdir(path.dirname(absolutePath), { recursive: true });
  await writeFile(absolutePath, content, "utf8");
}

describe("scanSemanticColorTokens", () => {
  afterEach(async () => {
    await Promise.all(
      tempRoots.map(async (rootDir) => {
        await rm(rootDir, { force: true, recursive: true });
      }),
    );
    tempRoots.length = 0;
  });

  it("reports slash-opacity color utilities, opacity shortcuts, local alpha math, and forbidden foundation tokens", async () => {
    const rootDir = await mkdtemp(
      path.join(os.tmpdir(), "semantic-color-token-guard-"),
    );
    tempRoots.push(rootDir);
    await writeSourceFile(
      rootDir,
      "features/example/example.tsx",
      [
        'export function Example() {',
        '  return <div className="text-af-ink/72 text-af-danger-ink opacity-80 [background:rgb(from var(--color-af-overlay) r g b / 0.16)]" style={{ color: "var(--color-af-ink)" }} />;',
        "}",
      ].join("\n"),
    );

    const violations = await scanSemanticColorTokens(rootDir);

    expect(violations).toHaveLength(6);
    expect(violations.map((violation) => violation.kind)).toEqual([
      "alpha-color-utility",
      "foundation-color-token",
      "foundation-color-token",
      "opacity-utility",
      "alpha-color-expression",
      "foundation-color-token",
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
