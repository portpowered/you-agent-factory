import { describe, expect, it } from "vitest";
import { workstationSupportsProgressOutcomeRoutes } from "./workstation-progress-outcome-routes";

describe("workstationSupportsProgressOutcomeRoutes", () => {
  it("returns false for a standard model processor without stopWords", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
      }),
    ).toBe(false);
  });

  it("returns false for a standard model processor with empty or whitespace-only stopWords", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: [],
      }),
    ).toBe(false);
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["", "   "],
      }),
    ).toBe(false);
  });

  it("returns true for a standard model processor with a trimmed stop word entry", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["DONE"],
      }),
    ).toBe(true);
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_INVOKE",
        stopWords: ["  BLOCKED  "],
      }),
    ).toBe(true);
  });

  it("returns true for a repeater workstation without stopWords", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "REPEATER",
      }),
    ).toBe(true);
  });

  it("returns false for classifier workstations", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "CLASSIFIER_WORKSTATION",
        behavior: "STANDARD",
        classificationRoutes: [
          {
            label: "ready",
            outputs: [{ state: "done", workType: "story" }],
          },
        ],
      }),
    ).toBe(false);
  });
});
