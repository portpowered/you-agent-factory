import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "@tailwindcss/node";
import { beforeAll, describe, expect, it } from "vitest";

const stylesSourcePath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "styles.css",
);

function extractPageShellRootBlock(css: string): string {
  const blocks = css.match(/:root\s*\{[^}]*\}/g) ?? [];
  return (
    blocks.find((block) => block.includes("background-color: var(--color-af-bg)")) ?? ""
  );
}

describe("page-shell background", () => {
  let compiledStyles = "";

  beforeAll(async () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const compiled = await compile(source, {
      base: path.dirname(stylesSourcePath),
      from: stylesSourcePath,
      onDependency: () => {},
    });
    compiledStyles = compiled.build([]);
  });

  it("keeps :root on a flat --color-af-bg fill without gradient layers", () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const rootBlock = extractPageShellRootBlock(compiledStyles);

    expect(source).toContain("background-color: var(--color-af-bg);");
    expect(source).toContain("background-image: none;");
    expect(source).not.toMatch(
      /@layer base\s*\{[^}]*:root\s*\{[^}]*(?:linear-gradient|radial-gradient)/s,
    );

    expect(rootBlock).toContain("background-color: var(--color-af-bg)");
    expect(rootBlock).toContain("background-image: none");
    expect(rootBlock).not.toMatch(/gradient/i);
    expect(rootBlock).not.toContain("linear-gradient");
    expect(rootBlock).not.toContain("radial-gradient");
  });
});
