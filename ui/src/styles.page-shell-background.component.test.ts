// @vitest-environment happy-dom

import path from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";

import { compileDashboardStyles } from "./test-support/compile-dashboard-styles";

const stylesSourcePath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "styles.css",
);

/** Canonical foundation blue from @theme (--color-af-foundation-background). */
const FOUNDATION_BACKGROUND = "#050b10";

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = rootBlocks.join("\n");
  document.head.appendChild(style);
}

describe("page-shell background", () => {
  beforeAll(async () => {
    const compiledCss = await compileDashboardStyles(stylesSourcePath);
    injectCompiledRootRules(compiledCss);
  });

  it("renders a flat token-backed fill on the document root", () => {
    const root = getComputedStyle(document.documentElement);
    expect(root.backgroundImage === "none" || root.backgroundImage === "").toBe(
      true,
    );
    expect(root.backgroundColor.toLowerCase()).toBe(FOUNDATION_BACKGROUND);
  });
});
// Component lane: requires DOM APIs.
