import { renderHook } from "@testing-library/react";

import { useWorkerDetailState } from "./use-worker-detail-state";

describe("useWorkerDetailState", () => {
  it("returns empty when the event-computed selection has no worker", () => {
    const { result } = renderHook(() =>
      useWorkerDetailState({ worker: null, workstationNames: [] }),
    );

    expect(result.current).toEqual({ status: "empty" });
  });

  it("returns ready worker detail from event-computed selection data", () => {
    const { result } = renderHook(() =>
      useWorkerDetailState({
        worker: {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
        workstationNames: ["Review"],
      }),
    );

    expect(result.current).toEqual({
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
