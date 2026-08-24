import { describe, expect, it } from "vitest";

import {
  factoryGraphNodeVisualNestedAccentClassName,
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

/** Exact class tokens; `!bg-warning` is a substring of `!bg-warning-container`. */
function classTokens(className: string): string[] {
  return className.split(" ").filter(Boolean);
}

describe("factoryGraphNodeVisualStateClassName work occupancy fill", () => {
  it("keeps an empty processing work state on the translucent container fill", () => {
    const tokens = classTokens(
      factoryGraphNodeVisualStateClassName(
        visualState({ lifecycle: "PROCESSING" }),
      ),
    );

    expect(tokens).toContain("!bg-warning-container");
    expect(tokens).not.toContain("!bg-warning");
  });

  it("keeps an empty processing work state on default label ink", () => {
    const className = factoryGraphNodeVisualStateClassName(
      visualState({ lifecycle: "PROCESSING" }),
    );

    expect(classTokens(className)).not.toContain("!text-on-warning");
    expect(classTokens(className)).toContain("factory-light:!text-on-warning");
  });

  it("fills a held processing work state solidly", () => {
    const tokens = classTokens(
      factoryGraphNodeVisualStateClassName(
        visualState({ activeWork: true, lifecycle: "PROCESSING" }),
      ),
    );

    expect(tokens).toContain("!bg-warning");
    expect(tokens).not.toContain("!bg-warning-container");
  });

  it("inverts work-state and workstation label ink on a solid fill", () => {
    const className = factoryGraphNodeVisualStateClassName(
      visualState({ activeWork: true, lifecycle: "PROCESSING" }),
    );

    for (const selector of [
      "data-factory-entity-title",
      "data-graph-semantic-icon",
      "data-place-state-value",
      "data-place-work-type",
      "data-state-value",
      "data-state-work-type",
      "data-workstation-runtime-label",
    ]) {
      expect(className).toContain(`[&_[${selector}]]:!text-on-warning`);
    }
  });

  it("leaves labels that sit on their own nested surface uninverted", () => {
    const className = factoryGraphNodeVisualStateClassName(
      visualState({ activeWork: true, lifecycle: "PROCESSING" }),
    );

    for (const selector of [
      "data-active-work-duration",
      "data-active-work-label",
      "data-workstation-scheduling-label",
    ]) {
      expect(className).not.toContain(`[&_[${selector}]]:!text-on-warning`);
    }
  });

  it("inverts held label ink in the node's own tone", () => {
    const tokens = classTokens(
      factoryGraphNodeVisualStateClassName(
        visualState({ activeWork: true, lifecycle: "TERMINAL" }),
      ),
    );

    expect(tokens).toContain("!bg-success");
    expect(tokens).toContain("[&_[data-state-value]]:!text-on-success");
  });
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

describe("factoryGraphNodeVisualNestedAccentClassName breaker text", () => {
  it.each([
    [undefined, "text-on-surface"],
    ["INITIAL", "text-on-info-container"],
    ["PROCESSING", "text-on-warning-container"],
    ["TERMINAL", "text-on-success-container"],
    ["FAILED", "text-on-error-container"],
  ] as const)("uses semantic %s parent ink", (lifecycle, expectedInk) => {
    const className = factoryGraphNodeVisualNestedAccentClassName(
      visualState({ lifecycle }),
    );

    expect(className).toContain(
      `[&_[data-workstation-guard-card]]:!${expectedInk}`,
    );
    expect(className).not.toContain("border");
    expect(className).not.toContain("bg-");
  });

  it("uses the opaque parent ink when the node holds work", () => {
    const className = factoryGraphNodeVisualNestedAccentClassName(
      visualState({ activeWork: true, lifecycle: "PROCESSING" }),
    );

    expect(className).toBe(
      "[&_[data-workstation-guard-card]]:!text-on-warning",
    );
  });
});
