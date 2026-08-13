// @vitest-environment node

import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { compileDashboardStyles } from "../test-support/compile-dashboard-styles";

const stylesSourcePath = fileURLToPath(
  new URL("../styles.css", import.meta.url),
);

function compactCss(css: string): string {
  return css
    .replace(/\s+/g, " ")
    .replace(/\s*([{}:;,])\s*/g, "$1")
    .trim();
}

describe("native dashboard scrollbar presentation", () => {
  it("compiles the shared semantic geometry and interaction roles", async () => {
    const css = compactCss(await compileDashboardStyles(stylesSourcePath));

    expect(css).toContain(
      ":where(*){scrollbar-color:var(--color-outline-variant) transparent;scrollbar-width:thin;}",
    );
    expect(css).toContain(
      ":where(*)::-webkit-scrollbar{height:10px;width:10px;}",
    );
    expect(css).toContain(
      ":where(*)::-webkit-scrollbar-track{background:transparent;}",
    );
    expect(css).toContain(
      ":where(*)::-webkit-scrollbar-thumb{background-clip:padding-box;background-color:var(--color-outline-variant);border:1px solid transparent;border-radius:999px;}",
    );
    expect(css).toContain(
      ":where(*)::-webkit-scrollbar-thumb:hover,:where(*)::-webkit-scrollbar-thumb:active{background-color:var(--color-on-surface-variant);}",
    );
  });

  it("returns scrollbar presentation to the user agent in forced colors", async () => {
    const css = compactCss(await compileDashboardStyles(stylesSourcePath));

    expect(css).toContain(
      "@media (forced-colors:active){:where(*){forced-color-adjust:auto;scrollbar-color:auto;scrollbar-width:auto;}",
    );
    expect(css).toContain(
      ":where(*)::-webkit-scrollbar,:where(*)::-webkit-scrollbar-thumb,:where(*)::-webkit-scrollbar-track{all:revert;}",
    );
  });
});
