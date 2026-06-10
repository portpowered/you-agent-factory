import { renderHook } from "@testing-library/react";

import { useResourceDetailState } from "./use-resource-detail-state";

describe("useResourceDetailState", () => {
  it("returns empty when the event-computed selection has no resource", () => {
    const { result } = renderHook(() =>
      useResourceDetailState({
        resource: null,
        workerNames: [],
        workstationNames: [],
      }),
    );

    expect(result.current).toEqual({ status: "empty" });
  });

  it("returns ready resource detail from event-computed selection data", () => {
    const { result } = renderHook(() =>
      useResourceDetailState({
        resource: {
          capacity: 2,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
        workerNames: ["reviewer"],
        workstationNames: ["Review"],
      }),
    );

    expect(result.current).toEqual({
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
