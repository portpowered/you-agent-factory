import { describe, expect, it } from "vitest";

import { AUTHORED_BODY_TEXT_CLASS } from "./authored-body-text";

/** Transitional surface/border tokens migrated in AUTHORED_BODY_TEXT_CLASS. */
const FORBIDDEN_TRANSITIONAL_PATTERNS = [
  /\bbg-af-surface-(subtle|raised)\b/,
  /\bbg-af-surface\b/,
  /\bbg-af-background\b/,
  /\bbg-af-bg\b/,
  /\bborder-af-border\b/,
  /\bborder-af-border-strong\b/,
  /\bbg-af-overlay\b/,
];

describe("AUTHORED_BODY_TEXT_CLASS color roles", () => {
  it("uses Material role utilities for container, pre, and inline code surfaces", () => {
    expect(AUTHORED_BODY_TEXT_CLASS).toContain("border-outline");
    expect(AUTHORED_BODY_TEXT_CLASS).toContain("bg-surface-container-high");
    expect(AUTHORED_BODY_TEXT_CLASS).toContain("bg-surface-container-low");
    expect(AUTHORED_BODY_TEXT_CLASS).toContain("[&_code]:bg-surface-container");
  });

  it.each(FORBIDDEN_TRANSITIONAL_PATTERNS.map((pattern) => [pattern.source]))(
    "does not include transitional token %s",
    (patternSource) => {
      const pattern = new RegExp(patternSource);
      expect(AUTHORED_BODY_TEXT_CLASS).not.toMatch(pattern);
    },
  );
});
