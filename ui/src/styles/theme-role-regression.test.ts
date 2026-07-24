// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";
import { compileDashboardStyles } from "../test-support/compile-dashboard-styles";
import { applyDocumentColorPalette } from "../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../theme/color-palette";

const stylesDir = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(stylesDir, "../..");
const repoRoot = path.resolve(uiRoot, "..");
const stylesSourcePath = path.join(uiRoot, "src", "styles.css");

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const paletteBlocks =
    compiledCss.match(/\[data-color-palette="[^"]+"\][^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = [...rootBlocks, ...paletteBlocks].join("\n");
  document.head.appendChild(style);
}

function readCssVariable(name: string): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
}

function parseHexColor(hex: string): readonly [number, number, number] {
  const normalized = hex.trim().replace("#", "");
  if (normalized.length !== 6) {
    throw new Error(`expected 6-digit hex, got ${hex}`);
  }
  const value = Number.parseInt(normalized, 16);
  return [(value >> 16) & 0xff, (value >> 8) & 0xff, value & 0xff] as const;
}

function relativeLuminance([r, g, b]: readonly [
  number,
  number,
  number,
]): number {
  const [rs, gs, bs] = [r, g, b].map((channel) => {
    const scaled = channel / 255;
    return scaled <= 0.03928
      ? scaled / 12.92
      : ((scaled + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs;
}

function contrastRatio(
  foreground: readonly [number, number, number],
  background: readonly [number, number, number],
): number {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

describe("theme role migration regression (US-010)", () => {
  beforeAll(async () => {
    const compiledCss = await compileDashboardStyles(stylesSourcePath);
    injectCompiledRootRules(compiledCss);
  });

  it("exposes Material role tokens on the document root", () => {
    expect(readCssVariable("--color-primary")).toBeTruthy();
    expect(readCssVariable("--color-on-surface")).toBeTruthy();
    expect(readCssVariable("--color-surface-container-high")).toBeTruthy();
    expect(readCssVariable("--color-outline")).toBeTruthy();
  });

  it.each(COLOR_PALETTE_IDS)(
    "switches foundation background when palette %s is applied",
    (paletteId) => {
      applyDocumentColorPalette(paletteId);
      expect(document.documentElement.dataset.colorPalette).toBe(paletteId);

      const background = readCssVariable("--color-af-foundation-background");
      expect(background).toMatch(/^#[0-9a-f]{6}$/i);
      expect(background.toLowerCase()).not.toBe("");
    },
  );

  it("separates factory-dark page background from shared surface roles", () => {
    applyDocumentColorPalette("factory-dark");

    const background = readCssVariable("--color-af-foundation-background");
    const surface = readCssVariable("--color-af-foundation-surface");
    const backgroundLuminance = relativeLuminance(parseHexColor(background));
    const surfaceLuminance = relativeLuminance(parseHexColor(surface));

    expect(backgroundLuminance).toBeLessThan(surfaceLuminance);
    expect(surfaceLuminance - backgroundLuminance).toBeGreaterThan(0.01);
    expect(background).toBe("#050b10");
    expect(surface).toBe("#181f2b");
  });

  it.each(COLOR_PALETTE_IDS)(
    "resolves readable primary on-accent ink for palette %s",
    (paletteId) => {
      applyDocumentColorPalette(paletteId);

      const primary = readCssVariable("--color-af-foundation-accent");
      const accentInk = readCssVariable("--color-af-foundation-accent-ink");
      const canvas = readCssVariable("--color-af-foundation-canvas");

      expect(primary).toMatch(/^#[0-9a-f]{6}$/i);
      expect(accentInk).toMatch(/^#[0-9a-f]{6}$/i);
      expect(accentInk.toLowerCase()).not.toBe(primary.toLowerCase());

      const contrast = contrastRatio(
        parseHexColor(accentInk),
        parseHexColor(primary),
      );
      expect(contrast).toBeGreaterThanOrEqual(4.5);

      if (paletteId === "factory-light") {
        expect(accentInk.toLowerCase()).not.toBe(canvas.toLowerCase());
        expect(relativeLuminance(parseHexColor(accentInk))).toBeLessThan(
          relativeLuminance(parseHexColor(primary)),
        );
      }
    },
  );

  it("keeps yellow primary accent across palette switches", () => {
    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);
      expect(
        readCssVariable("--color-af-foundation-accent").toLowerCase(),
      ).toBe("#f5c76f");
    }
  });

  it("exposes role-backed product af-* tokens through the compiled theme bundle", () => {
    const [paletteID] = COLOR_PALETTE_IDS;
    expect(paletteID).toBeDefined();

    applyDocumentColorPalette(paletteID);

    expect(readCssVariable("--color-af-text")).toBe(
      readCssVariable("--color-on-surface"),
    );
    expect(readCssVariable("--color-af-text-muted")).toBe(
      readCssVariable("--color-on-surface-variant"),
    );
    expect(readCssVariable("--color-af-surface")).toBe(
      readCssVariable("--color-surface"),
    );
    expect(readCssVariable("--color-af-accent")).toBe(
      readCssVariable("--color-primary"),
    );
  });

  it("documents phased rollout and cleanup in the rollout guide", () => {
    const rolloutPath = path.join(
      repoRoot,
      "docs/internal/development/material-color-role-migration-rollout.md",
    );
    const source = readFileSync(rolloutPath, "utf8");

    expect(source).toContain("## Rollout order");
    expect(source).toContain("## Cleanup phase");
    expect(source).toMatch(/Taxonomy.*US-001/s);
    expect(source).toContain("color-role-tokens.test.ts");
  });
});
