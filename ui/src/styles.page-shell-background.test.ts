// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "@tailwindcss/node";
import { beforeAll, describe, expect, it } from "vitest";

const stylesSourcePath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "styles.css",
);

/** Canonical foundation blue from @theme (--color-af-foundation-background). */
const FOUNDATION_BACKGROUND = "#0a1117";

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = rootBlocks.join("\n");
  document.head.appendChild(style);
}

describe("page-shell background", () => {
  beforeAll(async () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const compiled = await compile(source, {
      base: path.dirname(stylesSourcePath),
      from: stylesSourcePath,
      onDependency: () => {},
    });
    injectCompiledRootRules(compiled.build([]));
  });

  it("renders a flat token-backed fill on the document root", () => {
    const root = getComputedStyle(document.documentElement);
    expect(root.backgroundImage === "none" || root.backgroundImage === "").toBe(
      true,
    );
    expect(root.backgroundColor.toLowerCase()).toBe(FOUNDATION_BACKGROUND);
  });
});
