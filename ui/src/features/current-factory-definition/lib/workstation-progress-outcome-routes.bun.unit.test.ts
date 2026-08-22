import { describe, expect, it } from "bun:test";
import {
  workstationHasZAxisIncompleteForConnections,
  workstationSupportsProgressOutcomeFailureRoute,
  workstationSupportsProgressOutcomeRoutes,
} from "./workstation-progress-outcome-routes";

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

  it("returns false for logical-move workstations", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "LOGICAL_MOVE",
      }),
    ).toBe(false);
  });

  it("returns true for a standard model processor with a trimmed worker stopToken", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        assignedWorkerStopToken: "<COMPLETE>",
      }),
    ).toBe(true);
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_INVOKE",
        behavior: "STANDARD",
        assignedWorkerStopToken: "  DONE  ",
      }),
    ).toBe(true);
  });

  it("returns false for a standard model processor with empty or whitespace-only worker stopToken", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        assignedWorkerStopToken: "",
      }),
    ).toBe(false);
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        assignedWorkerStopToken: "   ",
      }),
    ).toBe(false);
  });

  it("returns true when both workstation stopWords and worker stopToken are configured", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["DONE"],
        assignedWorkerStopToken: "<COMPLETE>",
      }),
    ).toBe(true);
  });

  it("returns false when neither workstation stopWords nor worker stopToken are configured", () => {
    expect(
      workstationSupportsProgressOutcomeRoutes({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: undefined,
        assignedWorkerStopToken: undefined,
      }),
    ).toBe(false);
  });
});

describe("workstationSupportsProgressOutcomeFailureRoute", () => {
  it("returns false for logical-move workstations", () => {
    expect(
      workstationSupportsProgressOutcomeFailureRoute({
        type: "LOGICAL_MOVE",
      }),
    ).toBe(false);
  });

  it("returns true for a standard model processor without stopWords", () => {
    expect(
      workstationSupportsProgressOutcomeFailureRoute({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
      }),
    ).toBe(true);
  });

  it("returns true for a standard model processor with stopWords", () => {
    expect(
      workstationSupportsProgressOutcomeFailureRoute({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["DONE"],
      }),
    ).toBe(true);
  });
});

describe("workstationHasZAxisIncompleteForConnections", () => {
  it("returns true for a standard model processor without stopWords", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
      }),
    ).toBe(true);
  });

  it("returns false for a standard model processor with configured stopWords", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["DONE"],
      }),
    ).toBe(false);
  });

  it("returns false for a repeater workstation without stopWords", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "REPEATER",
      }),
    ).toBe(false);
  });

  it("returns false for classifier workstations", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
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

  it("returns false for a standard model processor with a trimmed worker stopToken", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        assignedWorkerStopToken: "<COMPLETE>",
      }),
    ).toBe(false);
  });

  it("returns true when neither workstation stopWords nor worker stopToken are configured", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
      }),
    ).toBe(true);
  });

  it("returns false when both workstation stopWords and worker stopToken are configured", () => {
    expect(
      workstationHasZAxisIncompleteForConnections({
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        stopWords: ["DONE"],
        assignedWorkerStopToken: "<COMPLETE>",
      }),
    ).toBe(false);
  });
});
