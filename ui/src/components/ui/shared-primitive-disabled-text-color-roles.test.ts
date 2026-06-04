import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

const IN_SCOPE_FILES = [
  "input.tsx",
  "expandable-panel-trigger.tsx",
  "chart.tsx",
  "dashboard-action-button.tsx",
] as const;

const FORBIDDEN_TRANSITIONAL_DISABLED_TEXT = /\btext-af-text-disabled\b/;

function readComponentSource(fileName: string): string {
  return readFileSync(join(UI_COMPONENTS_DIR, fileName), "utf8");
}

function expectNoTransitionalDisabledText(source: string): void {
  expect(source).not.toMatch(FORBIDDEN_TRANSITIONAL_DISABLED_TEXT);
}

describe("shared primitive disabled text color roles", () => {
  it.each(IN_SCOPE_FILES)(
    "does not use transitional text-af-text-disabled in %s",
    (fileName) => {
      expectNoTransitionalDisabledText(readComponentSource(fileName));
    },
  );

  it("maps input placeholder and disabled copy to on-surface-disabled role utilities", () => {
    const source = readComponentSource("input.tsx");

    expect(source).toContain("placeholder:text-on-surface-disabled");
    expect(source).toContain("disabled:text-on-surface-disabled");
    expectNoTransitionalDisabledText(source);
  });

  it("maps expandable panel trigger disabled copy to on-surface-disabled", () => {
    const source = readComponentSource("expandable-panel-trigger.tsx");

    expect(source).toContain("disabled:text-on-surface-disabled");
    expectNoTransitionalDisabledText(source);
  });

  it("maps chart hidden legend toggle copy to text-on-surface-disabled", () => {
    const source = readComponentSource("chart.tsx");

    expect(source).toMatch(/hidden\s*\?\s*"text-on-surface-disabled"/);
    expectNoTransitionalDisabledText(source);
  });

  it("maps dashboard action button spinner circle to text-on-surface-disabled", () => {
    const source = readComponentSource("dashboard-action-button.tsx");

    expect(source).toContain('className="text-on-surface-disabled"');
    expectNoTransitionalDisabledText(source);
  });
});
