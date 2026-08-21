import { describe, expect, it } from "vitest";

import {
  isFactoryGraphKnownWorkStateType,
  workStatePhaseSemanticIconClassName,
  workStatePhaseSurfaceClassName,
} from "./work-state-presentation.js";

describe("Factory graph work-state compatibility presentation", () => {
  it("uses exact canonical membership instead of alias normalization", () => {
    expect(isFactoryGraphKnownWorkStateType("INITIAL")).toBe(true);
    expect(
      ["QUEUED", "initial", " INITIAL ", "PROCESSING "].map(
        isFactoryGraphKnownWorkStateType,
      ),
    ).toEqual([false, false, false, false]);
  });

  it("keeps unfamiliar categories neutral while preserving known phase styling", () => {
    expect(workStatePhaseSurfaceClassName("QUEUED")).toContain(
      "border-outline bg-surface",
    );
    expect(workStatePhaseSemanticIconClassName("QUEUED")).toBe(
      "text-on-surface-variant",
    );
    expect(workStatePhaseSurfaceClassName("INITIAL")).toContain(
      "border-info-border bg-info-container",
    );
    expect(workStatePhaseSemanticIconClassName("INITIAL")).toBe("text-info");
  });
});
