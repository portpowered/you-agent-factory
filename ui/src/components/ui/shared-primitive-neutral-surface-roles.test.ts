import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { inputVariants } from "./input";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

function readComponentSource(fileName: string): string {
  return readFileSync(join(UI_COMPONENTS_DIR, fileName), "utf8");
}

function expectNoTransitionalNeutralSurfaces(source: string): void {
  expect(source).not.toMatch(/\bbg-af-surface-(subtle|raised)\b/);
  expect(source).not.toMatch(/\bborder-af-border\b/);
  expect(source).not.toMatch(/\btext-af-text(?!-)/);
}

function expectRoleBasedNeutralSurfaces(className: string): void {
  expect(className).toContain("border-outline");
  expect(className).toMatch(/\btext-on-surface(-variant)?\b/);
  expect(className).toMatch(/\bbg-surface-container-(low|high)\b/);
}

describe("shared primitive neutral surface roles", () => {
  it.each([
    "dashboard-shell.tsx",
    "dialog.tsx",
    "popover.tsx",
  ])("uses role-based neutral surfaces in %s", (fileName) => {
    const source = readComponentSource(fileName);

    expect(source).toContain("border-outline");
    expect(source).toMatch(/\btext-on-surface(-variant)?\b/);
    expect(source).toMatch(/\bbg-surface-container-(low|high)\b/);
    expectNoTransitionalNeutralSurfaces(source);
  });

  it("keeps package-backed input primitives on role-based neutral surfaces", () => {
    expectRoleBasedNeutralSurfaces(inputVariants());
    expectNoTransitionalNeutralSurfaces(inputVariants());
  });

  it("uses role-based neutral borders and text in table.tsx", () => {
    const source = readComponentSource("table.tsx");

    expect(source).toContain("border-outline");
    expect(source).toMatch(/\btext-on-surface(-variant)?\b/);
    expectNoTransitionalNeutralSurfaces(source);
  });

  it("maps standard list neutral and selected rows to surface role tokens", () => {
    const source = readComponentSource("standard-list-selection.tsx");

    expect(source).toContain(
      "border-outline bg-surface-container-high text-on-surface",
    );
    expect(source).toContain(
      "border-outline-variant bg-surface-container-low text-on-surface",
    );
  });

  it("maps button outline and ghost neutral chrome to role tokens", () => {
    const source = readComponentSource("button.tsx");

    expect(source).toContain(
      "border-outline bg-surface-container-high text-on-surface",
    );
    expect(source).toContain("text-on-surface-variant");
  });
});
