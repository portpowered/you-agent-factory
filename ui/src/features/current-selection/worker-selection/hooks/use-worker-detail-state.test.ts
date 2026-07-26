import { describe, expect, it } from "vitest";

import { deriveWorkerDetailState } from "./use-worker-detail-state";

describe("useWorkerDetailState", () => {
  it("returns empty when the event-computed selection has no worker", () => {
    expect(
      deriveWorkerDetailState({ worker: null, workstationNames: [] }),
    ).toEqual({ status: "empty" });
  });

  it("returns ready worker detail from event-computed selection data", () => {
    expect(
      deriveWorkerDetailState({
        worker: {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
        workstationNames: ["Review"],
      }),
    ).toEqual({
      status: "ready",
      worker: {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      workstationNames: ["Review"],
    });
  });
});
