import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

const IN_SCOPE_FILES = [
  "button.tsx",
  "table.tsx",
  "standard-list-selection.tsx",
  "expandable-panel-trigger.tsx",
] as const;

const FORBIDDEN_OVERLAY_HOVER = /\bhover:bg-af-overlay\b/;
const FORBIDDEN_OVERLAY_SELECTED = /\bdata-\[state=selected\]:bg-af-overlay\b/;

function readComponentSource(fileName: string): string {
  return readFileSync(join(UI_COMPONENTS_DIR, fileName), "utf8");
}

function expectNoTransitionalOverlayHover(source: string): void {
  expect(source).not.toMatch(FORBIDDEN_OVERLAY_HOVER);
  expect(source).not.toMatch(FORBIDDEN_OVERLAY_SELECTED);
}

describe("shared primitive overlay hover color roles", () => {
  it.each(IN_SCOPE_FILES)(
    "does not use transitional overlay hover or selected tokens in %s",
    (fileName) => {
      expectNoTransitionalOverlayHover(readComponentSource(fileName));
    },
  );

  it("maps button ghost, outline, and secondary hover to surface-container role utilities", () => {
    const source = readComponentSource("button.tsx");

    expect(source).toContain("hover:bg-surface-container-low");
    expect(source).toContain("hover:bg-surface-container-highest");
    expect(source).toContain("hover:bg-surface-container");
    expectNoTransitionalOverlayHover(source);
  });

  it("maps table row hover and selected state to surface-container role utilities", () => {
    const source = readComponentSource("table.tsx");

    expect(source).toContain("hover:bg-surface-container");
    expect(source).toContain("data-[state=selected]:bg-surface-container-low");
    expectNoTransitionalOverlayHover(source);
  });

  it("maps standard list neutral row hover to surface-container", () => {
    const source = readComponentSource("standard-list-selection.tsx");

    expect(source).toContain("hover:bg-surface-container");
    expectNoTransitionalOverlayHover(source);
  });

  it("maps expandable panel section and compact trigger hover to surface-container-highest", () => {
    const source = readComponentSource("expandable-panel-trigger.tsx");

    const highestHoverCount = (source.match(/\bhover:bg-surface-container-highest\b/g) ?? [])
      .length;
    expect(highestHoverCount).toBeGreaterThanOrEqual(2);
    expectNoTransitionalOverlayHover(source);
  });
});
