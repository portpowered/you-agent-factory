// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "@tailwindcss/node";
import { beforeAll, describe, expect, it } from "vitest";

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

describe("theme role migration regression (US-010)", () => {
  beforeAll(async () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const compiled = await compile(source, {
      base: path.dirname(stylesSourcePath),
      from: stylesSourcePath,
      onDependency: () => {},
    });
    injectCompiledRootRules(compiled.build([]));
  });

  it("exposes Material role tokens on the document root", () => {
    expect(readCssVariable("--color-primary")).toBeTruthy();
    expect(readCssVariable("--color-on-surface")).toBeTruthy();
    expect(readCssVariable("--color-surface-container-high")).toBeTruthy();
    expect(readCssVariable("--color-outline")).toBeTruthy();
  });

  it.each(
    COLOR_PALETTE_IDS,
  )("switches foundation background when palette %s is applied", (paletteId) => {
    applyDocumentColorPalette(paletteId);
    expect(document.documentElement.dataset.colorPalette).toBe(paletteId);

    const background = readCssVariable("--color-af-foundation-background");
    expect(background).toMatch(/^#[0-9a-f]{6}$/i);
    expect(background.toLowerCase()).not.toBe("");
  });

  it("keeps yellow primary accent across palette switches", () => {
    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);
      expect(
        readCssVariable("--color-af-foundation-accent").toLowerCase(),
      ).toBe("#f5c76f");
    }
  });

  it("exposes role-backed product af-* tokens through the compiled theme bundle", () => {
    applyDocumentColorPalette(COLOR_PALETTE_IDS[0]!);

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
