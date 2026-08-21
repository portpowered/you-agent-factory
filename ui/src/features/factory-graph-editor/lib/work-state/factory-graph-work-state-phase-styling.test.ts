import { describe, expect, it } from "vitest";

import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
  workStatePhaseSwatchClassName,
} from "../work-state/factory-graph-work-state-phase-styling";

describe("factory graph work state phase styling", () => {
  it.each([
    ["INITIAL", "queue", "text-info", "border-info-border bg-info-container"],
    [
      "PROCESSING",
      "processing",
      "text-warning",
      "border-af-warning-border bg-warning-container",
    ],
    [
      "TERMINAL",
      "terminal",
      "text-success",
      "border-af-success-border bg-success-container",
    ],
    [
      "FAILED",
      "failed",
      "text-error",
      "border-af-danger-border bg-error-container",
    ],
  ] as const)(
    "maps %s to phase surface and icon styling",
    (type, iconKind, iconClass, surfaceClass) => {
      expect(workStatePhaseSurfaceClassName(type)).toBe(surfaceClass);
      expect(workStatePhaseSwatchClassName(type)).toBe(surfaceClass);
      expect(workStatePhaseSemanticIconKind(type)).toBe(iconKind);
      expect(workStatePhaseSemanticIconClassName(type)).toBe(iconClass);
    },
  );

  it("falls back to neutral styling when workStateType is missing", () => {
    expect(workStatePhaseSurfaceClassName(undefined)).toBe(
      "border-outline bg-surface",
    );
    expect(workStatePhaseSemanticIconKind(undefined)).toBe("queue");
    expect(workStatePhaseSemanticIconClassName(undefined)).toBe(
      "text-on-surface-variant",
    );
  });

  it("falls back to neutral styling for a future raw category", () => {
    expect(workStatePhaseSurfaceClassName("PAUSED_BY_POLICY")).toBe(
      "border-outline bg-surface",
    );
    expect(workStatePhaseSemanticIconKind("PAUSED_BY_POLICY")).toBe("queue");
    expect(workStatePhaseSemanticIconClassName("PAUSED_BY_POLICY")).toBe(
      "text-on-surface-variant",
    );
  });
});
