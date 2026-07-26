import { describe, expect, it } from "vitest";

import { deriveResourceDetailState } from "./use-resource-detail-state";

describe("useResourceDetailState", () => {
  it("returns empty when the event-computed selection has no resource", () => {
    expect(
      deriveResourceDetailState({
        resource: null,
        workerNames: [],
        workstationNames: [],
      }),
    ).toEqual({ status: "empty" });
  });

  it("returns ready resource detail from event-computed selection data", () => {
    expect(
      deriveResourceDetailState({
        resource: {
          capacity: 2,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
        workerNames: ["reviewer"],
        workstationNames: ["Review"],
      }),
    ).toEqual({
      resource: {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
      status: "ready",
      workerNames: ["reviewer"],
      workstationNames: ["Review"],
    });
  });
});
