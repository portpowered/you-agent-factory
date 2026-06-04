import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

/** Transitional accent/text tokens migrated to Material role utilities. */
const FORBIDDEN_TRANSITIONAL_ACCENT_TEXT_PATTERNS = [
  /\btext-af-accent\b/,
  /\bbg-af-accent\b/,
  /\btext-af-on-accent\b/,
  /\bbg-af-accent-surface\b/,
  /\btext-af-text-disabled\b/,
  /\btext-af-text-subtle\b/,
];

function readCalendarSource(): string {
  return readFileSync(join(UI_COMPONENTS_DIR, "calendar.tsx"), "utf8");
}

function expectNoForbiddenTransitionalAccentText(source: string): void {
  for (const pattern of FORBIDDEN_TRANSITIONAL_ACCENT_TEXT_PATTERNS) {
    expect(source).not.toMatch(pattern);
  }
}

describe("calendar color roles", () => {
  it("maps selected, today, outside, disabled, and weekday cells to role utilities", () => {
    const source = readCalendarSource();

    expect(source).toMatch(/day_button:[\s\S]*aria-selected:bg-primary/);
    expect(source).toMatch(/day_button:[\s\S]*aria-selected:text-on-primary/);
    expect(source).toMatch(
      /outside:[\s\S]*aria-selected:bg-primary-container/,
    );
    expect(source).toMatch(
      /outside:[\s\S]*aria-selected:text-on-primary-container/,
    );
    expect(source).toMatch(/today:\s*"[^"]*text-primary/);
    expect(source).toMatch(/disabled:\s*"[^"]*text-on-surface-disabled/);
    expect(source).toMatch(/outside:[\s\S]*text-on-surface-disabled/);
    expect(source).toMatch(/weekday:[\s\S]*text-on-surface-variant/);

    expectNoForbiddenTransitionalAccentText(source);
  });

  it("preserves shell chrome, nav buttons, focus ring, and day hover overlay", () => {
    const source = readCalendarSource();

    expect(source).toContain("border-outline");
    expect(source).toContain("bg-surface-container-low");
    expect(source).toContain("buttonVariants");
    expect(source).toContain("focus-visible:ring-af-focus-ring");
    expect(source).toContain("hover:bg-af-overlay");
    expectNoForbiddenTransitionalAccentText(source);
  });
});
