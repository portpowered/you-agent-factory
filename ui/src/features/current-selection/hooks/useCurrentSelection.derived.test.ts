import { renderHook } from "@testing-library/react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../../components/dashboard/fixtures/runtime";
import { useCurrentSelectionDerivedState } from "./useCurrentSelection.derived";

function buildSnapshot(): DashboardSnapshot {
  return {
    factory: {
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [],
          name: "Review",
          outputs: [],
          worker: "reviewer",
        },
      ],
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    },
    factory_state: "IDLE",
    runtime: buildEmptyDashboardRuntimeFixture(),
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 0,
  };
}

describe("useCurrentSelectionDerivedState", () => {
  const snapshot = buildSnapshot();

  it("resolves selected work type fields when selection is work-type", () => {
    const { result } = renderHook(() =>
      useCurrentSelectionDerivedState({
        projectedWorkstationRequestsByDispatchID: {},
        selection: { kind: "work-type", workTypeName: "story" },
        snapshot,
        terminalWorkDetail: null,
      }),
    );

    expect(result.current.selectedWorkTypeName).toBe("story");
    expect(result.current.selectedWorkType).toMatchObject({
      name: "story",
      handlingBehavior: ["DEFAULT"],
    });
  });

  it("resolves selected worker fields when selection is worker", () => {
    const { result } = renderHook(() =>
      useCurrentSelectionDerivedState({
        projectedWorkstationRequestsByDispatchID: {},
        selection: { kind: "worker", workerName: "reviewer" },
        snapshot,
        terminalWorkDetail: null,
      }),
    );

    expect(result.current.selectedWorkerName).toBe("reviewer");
    expect(result.current.selectedWorker).toMatchObject({ name: "reviewer" });
    expect(result.current.selectedWorkerWorkstationNames).toEqual(["Review"]);
  });

  it("clears selected work type when the type is missing from the snapshot factory", () => {
    const { result } = renderHook(() =>
      useCurrentSelectionDerivedState({
        projectedWorkstationRequestsByDispatchID: {},
        selection: { kind: "work-type", workTypeName: "missing" },
        snapshot,
        terminalWorkDetail: null,
      }),
    );

    expect(result.current.selectedWorkTypeName).toBe("missing");
    expect(result.current.selectedWorkType).toBeNull();
  });
});
