// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "@tailwindcss/node";
import { beforeAll, describe, expect, it } from "vitest";

import { applyDocumentColorPalette } from "../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../theme/color-palette";

const stylesDir = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../styles",
);
const uiRoot = path.resolve(stylesDir, "..");
const stylesSourcePath = path.join(uiRoot, "styles.css");

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

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));
const FEATURES_DIR = join(UI_COMPONENTS_DIR, "../../features");

function readComponentSource(fileName: string): string {
  return readFileSync(join(UI_COMPONENTS_DIR, fileName), "utf8");
}

function readFeatureSource(relativePath: string): string {
  return readFileSync(join(FEATURES_DIR, relativePath), "utf8");
}

const PRIMARY_EMPHASIS_SOURCES = [
  { label: "button", read: () => readComponentSource("button.tsx") },
  {
    label: "standard-list-selection",
    read: () => readComponentSource("standard-list-selection.tsx"),
  },
  { label: "surface-panel", read: () => readComponentSource("surface-panel.tsx") },
  { label: "calendar", read: () => readComponentSource("calendar.tsx") },
  {
    label: "dashboard-status-pill",
    read: () => readComponentSource("dashboard-status-pill.tsx"),
  },
  {
    label: "current-selection-selectable-button",
    read: () =>
      readFeatureSource(
        "current-selection/base/components/current-selection-selectable-button.tsx",
      ),
  },
  {
    label: "dashboard-header-option-menu",
    read: () =>
      readFeatureSource("header/components/dashboard-header-option-menu.tsx"),
  },
] as const;

const PRIMARY_FOREGROUND_ROLE_TOKENS = [
  "text-on-primary",
  "text-on-primary-container",
] as const;

function expectNoForcedWhitePrimaryEmphasis(source: string): void {
  expect(source).not.toMatch(/\btext-white\b/);
  expect(source).not.toMatch(/\btext-on-inverse\b/);
  expect(source).not.toMatch(
    /var\(--color-af-foundation-canvas\)[^;]*on-primary|on-primary[^;]*var\(--color-af-foundation-canvas\)/,
  );
}

describe("shared primitive primary-emphasis color roles (website-color-reblanacing-003)", () => {
  beforeAll(async () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const compiled = await compile(source, {
      base: path.dirname(stylesSourcePath),
      from: stylesSourcePath,
      onDependency: () => {},
    });
    injectCompiledRootRules(compiled.build([]));
  });

  it.each(
    COLOR_PALETTE_IDS,
  )(
    "keeps palette-aware primary emphasis foreground tokens for palette %s",
    (paletteId) => {
      applyDocumentColorPalette(paletteId);

      const accentInk = readCssVariable("--color-af-foundation-accent-ink");
      const accentStrong = readCssVariable("--color-af-foundation-accent-strong");
      const canvas = readCssVariable("--color-af-foundation-canvas");

      expect(accentInk.toLowerCase()).not.toBe(canvas.toLowerCase());
      expect(accentInk.toLowerCase()).not.toBe(accentStrong.toLowerCase());
      expect(accentInk).toBe("#1a2228");
    },
  );

  it("maps default button tone to primary fill and on-primary ink", () => {
    const source = readComponentSource("button.tsx");

    expect(source).toContain("border-primary bg-primary text-on-primary");
    expect(source).not.toContain("text-white");
    expectNoForcedWhitePrimaryEmphasis(source);
  });

  it("maps standard list accent rows to primary-container with on-primary ink", () => {
    const source = readComponentSource("standard-list-selection.tsx");

    expect(source).toContain(
      "border-primary bg-primary-container text-on-primary",
    );
    expectNoForcedWhitePrimaryEmphasis(source);
  });

  it("maps surface panel selected tone to primary-container with on-primary-container ink", () => {
    const source = readComponentSource("surface-panel.tsx");

    expect(source).toContain(
      "border-primary bg-primary-container text-on-primary-container",
    );
    expectNoForcedWhitePrimaryEmphasis(source);
  });

  it("maps calendar selected states to primary role foreground tokens", () => {
    const source = readComponentSource("calendar.tsx");

    expect(source).toMatch(/aria-selected:bg-primary aria-selected:text-on-primary/);
    expect(source).toMatch(
      /aria-selected:bg-primary-container aria-selected:text-on-primary-container/,
    );
    expectNoForcedWhitePrimaryEmphasis(source);
  });

  it.each(PRIMARY_EMPHASIS_SOURCES)(
    "keeps $label primary-emphasis surfaces on shared role tokens instead of forced white",
    ({ read }) => {
      const source = read();

      expect(source).toMatch(/\bbg-primary(-container)?\b/);
      expect(
        PRIMARY_FOREGROUND_ROLE_TOKENS.some((token) => source.includes(token)),
      ).toBe(true);
      expectNoForcedWhitePrimaryEmphasis(source);
    },
  );

  it("routes dashboard status pill active tone through primary-container emphasis", () => {
    const source = readComponentSource("dashboard-status-pill.tsx");

    expect(source).toContain(
      'active: "border-primary bg-primary-container text-on-primary"',
    );
    expectNoForcedWhitePrimaryEmphasis(source);
  });

  it("routes current selection and header menu selected states through primary-container emphasis", () => {
    const selectableButton = readFeatureSource(
      "current-selection/base/components/current-selection-selectable-button.tsx",
    );
    const headerMenuItem = readFeatureSource(
      "header/components/dashboard-header-option-menu.tsx",
    );

    expect(selectableButton).toContain(
      'selected && "border-primary bg-primary-container text-on-primary"',
    );
    expect(headerMenuItem).toContain(
      '? "border-primary bg-primary-container text-on-primary"',
    );
    expectNoForcedWhitePrimaryEmphasis(selectableButton);
    expectNoForcedWhitePrimaryEmphasis(headerMenuItem);
  });
});
