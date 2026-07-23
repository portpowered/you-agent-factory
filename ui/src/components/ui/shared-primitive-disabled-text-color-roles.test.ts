import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { inputVariants } from "./input";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

const IN_SCOPE_FILES = ["expandable-panel-trigger.tsx"] as const;

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

  it("maps package-backed input placeholder and disabled copy to on-surface-disabled role utilities", () => {
    const className = inputVariants();

    expect(className).toContain("placeholder:text-on-surface-disabled");
    expect(className).toContain("disabled:text-on-surface-disabled");
    expectNoTransitionalDisabledText(className);
  });

  it("maps expandable panel trigger disabled copy to on-surface-disabled", () => {
    const source = readComponentSource("expandable-panel-trigger.tsx");

    expect(source).toContain("disabled:text-on-surface-disabled");
    expectNoTransitionalDisabledText(source);
  });
});
