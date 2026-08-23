import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";

import {
  generateColorPaletteTokens,
  renderColorPaletteTokens,
} from "../../scripts/color-palette-token-generator";
import { compileDashboardStyles } from "../test-support/compile-dashboard-styles";
import { COLOR_PALETTE_IDS } from "./color-palette";
import {
  COLOR_PALETTE_TOKENS,
  getColorPaletteTokens,
} from "./color-palette-tokens";

const themeDirectory = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(themeDirectory, "../..");
const stylesSourcePath = path.join(uiRoot, "src", "styles.css");
const generatedPath = path.join(themeDirectory, "color-palette-tokens.ts");

let compiledCss = "";

beforeAll(async () => {
  compiledCss = await compileDashboardStyles(stylesSourcePath);
});

describe("compiled dashboard color palette tokens", () => {
  it.each(COLOR_PALETTE_IDS)(
    "exports complete foundation and semantic values for %s",
    (paletteId) => {
      const tokens = getColorPaletteTokens(paletteId);

      expect(tokens.foundation.background).toMatch(/^#[\da-f]{6}$/i);
      expect(tokens.foundation.surface).toMatch(/^#[\da-f]{6}$/i);
      expect(tokens.foundation.ink).toMatch(/^#[\da-f]{6}$/i);
      expect(tokens.semantic.surface.default).toBe(tokens.foundation.surface);
      expect(tokens.semantic.foreground.default).toBe(tokens.foundation.ink);
      expect(tokens.semantic.accent.default).toBe(tokens.foundation.accent);
      expect(tokens.semantic.code).toBe(tokens.foundation.codeInk);
      expect(tokens.semantic.info.default).toBe(tokens.foundation.info);
      expect(tokens.semantic.success.default).toBe(tokens.foundation.success);
      expect(tokens.semantic.danger.default).toBe(tokens.foundation.danger);
      expect(tokens.semantic.overlay.solid).toBe(tokens.foundation.overlay);
      expect(tokens.semantic.surface.muted).toMatch(/^#[\da-f]{8}$/i);
      expect(tokens.semantic.outline.variant).toMatch(/^#[\da-f]{8}$/i);
      expect(tokens.semantic.foreground.muted).toMatch(/^#[\da-f]{8}$/i);
    },
  );

  it("matches a fresh projection from the compiled CSS", async () => {
    const generatedSource = await readFile(generatedPath, "utf8");
    const freshGraph = generateColorPaletteTokens(compiledCss);

    expect(freshGraph).toEqual(COLOR_PALETTE_TOKENS);
    expect(renderColorPaletteTokens()).toBe(generatedSource);
  });

  it("names the affected palette when a compiled preset block is missing", () => {
    const brokenCss = removePaletteBlock(compiledCss, "slate");

    expect(() => generateColorPaletteTokens(brokenCss)).toThrow(
      "palette=slate",
    );
    expect(generateColorPaletteTokens(compiledCss)).toEqual(
      COLOR_PALETTE_TOKENS,
    );
  });
});

function removePaletteBlock(css: string, paletteId: string): string {
  const blockPattern = new RegExp(
    `(?:^|\\n)\\s*(?::root\\s*,\\s*)?\\[data-color-palette="${paletteId}"\\]\\s*\\{[^{}]*\\}\\s*`,
  );
  const brokenCss = css.replace(blockPattern, "");
  if (brokenCss === css) {
    throw new Error(`test setup could not remove palette=${paletteId}`);
  }
  return brokenCss;
}
