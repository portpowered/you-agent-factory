import { describe, expect, it } from "vitest";

import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
} from "./factory-graph-work-state-phase-styling";

describe("factory graph work state phase styling", () => {
  it.each([
    ["INITIAL", "border-af-info-border bg-af-info-surface", "queue", "text-af-info"],
    [
      "PROCESSING",
      "border-af-warning-border bg-af-warning-surface",
      "processing",
      "text-af-warning",
    ],
    [
      "TERMINAL",
      "border-af-success-border bg-af-success-surface",
      "terminal",
      "text-af-success",
    ],
    [
      "FAILED",
      "border-af-danger-border bg-af-danger-surface",
      "failed",
      "text-af-danger",
    ],
  ] as const)(
    "maps %s to phase surface and icon styling",
    (type, surfaceClass, iconKind, iconClass) => {
      expect(workStatePhaseSurfaceClassName(type)).toBe(surfaceClass);
      expect(workStatePhaseSemanticIconKind(type)).toBe(iconKind);
      expect(workStatePhaseSemanticIconClassName(type)).toBe(iconClass);
    },
  );

  it("falls back to neutral styling when workStateType is missing", () => {
    expect(workStatePhaseSurfaceClassName(undefined)).toBe(
      "border-af-border-strong bg-af-surface-raised",
    );
    expect(workStatePhaseSemanticIconKind(undefined)).toBe("queue");
    expect(workStatePhaseSemanticIconClassName(undefined)).toBe(
      "text-af-text-muted",
    );
  });
});
