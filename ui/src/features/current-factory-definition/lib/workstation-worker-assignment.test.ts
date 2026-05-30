import { describe, expect, it } from "vitest";
import { workstationRequiresWorkerAssignment } from "./workstation-worker-assignment";

describe("workstationRequiresWorkerAssignment", () => {
  it("returns false for LOGICAL_MOVE workstations even when a worker field is present", () => {
    expect(
      workstationRequiresWorkerAssignment({
        type: "LOGICAL_MOVE",
      }),
    ).toBe(false);
  });

  it("returns true for MODEL_WORKSTATION fixtures used in graph-editor tests", () => {
    expect(
      workstationRequiresWorkerAssignment({
        type: "MODEL_WORKSTATION",
      }),
    ).toBe(true);
  });

  it("returns true for MODEL_INVOKE and CLASSIFIER_WORKSTATION fixtures", () => {
    expect(
      workstationRequiresWorkerAssignment({
        type: "MODEL_INVOKE",
      }),
    ).toBe(true);
    expect(
      workstationRequiresWorkerAssignment({
        type: "CLASSIFIER_WORKSTATION",
      }),
    ).toBe(true);
  });

  it("defaults omitted workstation type to MODEL_WORKSTATION and requires a worker", () => {
    expect(workstationRequiresWorkerAssignment({})).toBe(true);
  });
});
