import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../../../components/dashboard/fixtures/runtime";
import { deriveCurrentSelectionState } from "./useCurrentSelection.derived";

const reviewWorkItem: DashboardWorkItemRef = {
  display_name: "Selection Story",
  trace_id: "trace-selection-1",
  work_id: "work-selection-1",
  work_type_id: "story",
};

const reviewNode = {
  node_id: "review",
  transition_id: "review",
  workstation_name: "Review",
};

function buildSnapshot(overrides?: {
  completedWorkLabels?: string[];
  failedWorkLabels?: string[];
}): DashboardSnapshot {
  const runtime = buildEmptyDashboardRuntimeFixture();

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
    runtime: {
      ...runtime,
      place_token_counts: {
        "story:queued": 2,
      },
      session: {
        ...runtime.session,
        completed_work_labels: overrides?.completedWorkLabels ?? [],
        failed_work_labels: overrides?.failedWorkLabels ?? [],
        provider_sessions: [
          {
            dispatch_id: "dispatch-selection-1",
            outcome: "ACCEPTED",
            provider_session: {
              id: "dispatch-selection-1-session",
              kind: "session_id",
              provider: "codex",
            },
            transition_id: "review",
            workstation_name: "Review",
            work_items: [reviewWorkItem],
          },
        ],
      },
    },
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: ["review"],
      workstation_nodes_by_id: {
        review: reviewNode,
      },
    },
    uptime_seconds: 0,
  };
}

function buildReviewRequest(): DashboardWorkstationRequest {
  return {
    dispatch_id: "dispatch-selection-1",
    transition_id: "review",
    work_items: [reviewWorkItem],
    workstation_name: "Review",
    workstation_node_id: "review",
    work_type_ids: ["story"],
  };
}

describe("useCurrentSelectionDerivedState", () => {
  const snapshot = buildSnapshot();

  it("resolves selected work type fields when selection is work-type", () => {
    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: {},
      selection: { kind: "work-type", workTypeName: "story" },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedWorkTypeName).toBe("story");
    expect(result.selectedWorkType).toMatchObject({
      name: "story",
      handlingBehavior: ["DEFAULT"],
    });
  });

  it("resolves selected worker fields when selection is worker", () => {
    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: {},
      selection: { kind: "worker", workerName: "reviewer" },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedWorkerName).toBe("reviewer");
    expect(result.selectedWorker).toMatchObject({ name: "reviewer" });
    expect(result.selectedWorkerWorkstationNames).toEqual(["Review"]);
  });

  it("clears selected work type when the type is missing from the snapshot factory", () => {
    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: {},
      selection: { kind: "work-type", workTypeName: "missing" },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedWorkTypeName).toBe("missing");
    expect(result.selectedWorkType).toBeNull();
  });

  it("resolves node selection workstation requests and provider sessions", () => {
    const projected = {
      "dispatch-selection-1": buildReviewRequest(),
    };

    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: projected,
      selection: { kind: "node", nodeId: "review" },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedNode).toEqual(reviewNode);
    expect(result.selectedNodeWorkstationRequests).toHaveLength(1);
    expect(result.selectedNodeProviderSessions).toHaveLength(1);
  });

  it("resolves work-item selection history and dispatch attempts", () => {
    const projected = {
      "dispatch-selection-1": buildReviewRequest(),
    };

    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: projected,
      selection: {
        dispatchId: "dispatch-selection-1",
        kind: "work-item",
        nodeId: "review",
        workItem: reviewWorkItem,
      },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedWorkID).toBe("work-selection-1");
    expect(result.selectedWorkRequestHistory).toHaveLength(1);
    expect(result.selectedWorkDispatchAttempts).toHaveLength(1);
    expect(result.selectedWorkOperationHistory).toEqual([
      { kind: "workstation", request: buildReviewRequest() },
    ]);
  });

  it("resolves state-node place work items and token count", () => {
    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: {},
      selection: { kind: "state-node", placeId: "story:queued" },
      snapshot,
      terminalWorkDetail: null,
    });

    expect(result.selectedStatePlace?.place_id).toBe("story:queued");
    expect(result.selectedStateTokenCount).toBe(2);
  });

  it("builds completed and failed terminal work items from session labels", () => {
    const labeledSnapshot = buildSnapshot({
      completedWorkLabels: ["done-story"],
      failedWorkLabels: ["failed-story"],
    });

    const result = deriveCurrentSelectionState({
      projectedWorkstationRequestsByDispatchID: {},
      selection: null,
      snapshot: labeledSnapshot,
      terminalWorkDetail: null,
    });

    expect(result.completedWorkLabels).toEqual(["done-story"]);
    expect(result.failedWorkLabels).toEqual(["failed-story"]);
    expect(result.completedWorkItems).toHaveLength(1);
    expect(result.failedWorkItems).toHaveLength(1);
  });
});
