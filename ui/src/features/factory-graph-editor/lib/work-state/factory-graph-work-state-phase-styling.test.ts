import { describe, expect, it } from "vitest";

import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
  workStatePhaseSwatchClassName,
} from "../work-state/factory-graph-work-state-phase-styling";

describe("factory graph work state phase styling", () => {
  it.each([
    ["INITIAL", "queue", "text-info"],
    ["PROCESSING", "processing", "text-warning"],
    ["TERMINAL", "terminal", "text-success"],
    ["FAILED", "failed", "text-error"],
  ] as const)(
    "maps %s to phase surface and icon styling",
    (type, iconKind, iconClass) => {
      const surfaceClass = "border-info-border bg-info-container";

      expect(workStatePhaseSurfaceClassName(type)).toBe(surfaceClass);
      expect(workStatePhaseSwatchClassName(type)).toBe(surfaceClass);
      expect(workStatePhaseSemanticIconKind(type)).toBe(iconKind);
      expect(workStatePhaseSemanticIconClassName(type)).toBe(iconClass);
    },
  );

  it("falls back to neutral styling when workStateType is missing", () => {
    expect(workStatePhaseSurfaceClassName(undefined)).toBe(
      "border-info-border bg-info-container",
    );
    expect(workStatePhaseSemanticIconKind(undefined)).toBe("queue");
    expect(workStatePhaseSemanticIconClassName(undefined)).toBe(
      "text-on-surface-variant",
    );
  });
});
