import { describe, expect, it } from "vitest";

import {
  factoryGraphNodeVisualStateClassName,
  factoryGraphNodeVisualStatusSurfaceClassName,
} from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";

function visualState(
  overrides: Partial<Parameters<typeof resolveFactoryGraphVisualState>[0]> = {},
) {
  return resolveFactoryGraphVisualState({
    family: "work-state",
    ...overrides,
  });
}

describe("factoryGraphNodeVisualStatusSurfaceClassName", () => {
  it.each([
    ["quiet", ""],
    ["waiting", "border-info-border bg-info-container"],
    ["active", "border-af-warning-border bg-warning-container"],
    ["success", "border-af-success-border bg-success-container"],
    ["danger", "border-af-danger-border bg-error-container"],
  ] as const)(
    "derives the %s backing surface from its parent tone",
    (status, expected) => {
      expect(factoryGraphNodeVisualStatusSurfaceClassName(status)).toBe(
        expected,
      );
    },
  );
});

describe("factoryGraphNodeVisualStateClassName nested accents", () => {
  it("routes active border, glow, and ring accents through warning", () => {
    const activeFlow = factoryGraphNodeVisualStateClassName(
      visualState({ activeFlow: true }),
    );

    expect(activeFlow).toContain("border-af-warning-border");
    expect(activeFlow).toContain("shadow-af-graph-warning");
    expect(activeFlow).toContain("ring-af-warning-border");
    expect(activeFlow).not.toContain("border-af-success-border");
    expect(activeFlow).not.toContain("shadow-af-success-chip");
    expect(activeFlow).not.toContain("ring-af-success-border");
  });

  it("keeps a true success parent success-toned while active", () => {
    const successFlow = factoryGraphNodeVisualStateClassName(
      visualState({ activeFlow: true, lifecycle: "TERMINAL" }),
    );

    expect(successFlow).toContain("border-af-success-border");
    expect(successFlow).toContain("shadow-af-success-chip");
    expect(successFlow).toContain("ring-af-success-border");
    expect(successFlow).not.toContain("shadow-af-graph-warning");
  });

  it("keeps failed parents danger-toned", () => {
    const failed = factoryGraphNodeVisualStateClassName(
      visualState({ lifecycle: "FAILED" }),
    );

    expect(failed).toContain("border-af-danger-border");
    expect(failed).toContain("shadow-af-graph-danger");
    expect(failed).not.toContain("border-af-success-border");
  });
});
