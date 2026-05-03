import { act, render, screen, waitFor } from "@testing-library/react";

import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../components/dashboard/fixtures/runtime";
import { resetSelectionHistoryStore, useSelectionHistoryStore } from "./state/selectionHistoryStore";
import { buildReplayFixtureTimelineSnapshot } from "../../testing/replay-fixtures";
import { useCurrentSelection } from "./useCurrentSelection";

const TEST_TOPOLOGY: DashboardSnapshot["topology"] = {
  edges: [],
  workstation_node_ids: ["review", "repair"],
  workstation_nodes_by_id: {
    repair: {
      node_id: "repair",
      transition_id: "repair",
      workstation_name: "Repair",
    },
    review: {
      node_id: "review",
      transition_id: "review",
      workstation_name: "Review",
    },
  },
};

function buildWorkItem(
  workID: string,
  displayName: string,
  traceID = workID.replace("work-", "trace-"),
): DashboardWorkItemRef {
  return {
    display_name: displayName,
    trace_id: traceID,
    work_id: workID,
    work_type_id: "story",
  };
}

function buildRuntimeWorkstationRequest({
  dispatchID,
  inputWorkItems = [],
  outputWorkItems = [],
  startedAt,
  transitionID = "review",
}: {
  dispatchID: string;
  inputWorkItems?: DashboardWorkItemRef[];
  outputWorkItems?: DashboardWorkItemRef[];
  startedAt: string;
  transitionID?: string;
}): DashboardRuntimeWorkstationRequest {
  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: outputWorkItems.length > 0 ? 1 : 0,
    },
    dispatch_id: dispatchID,
    request: {
      input_work_items: inputWorkItems,
      started_at: startedAt,
      trace_ids: inputWorkItems.flatMap((workItem) => (workItem.trace_id ? [workItem.trace_id] : [])),
    },
    response:
      outputWorkItems.length > 0
        ? {
            output_work_items: outputWorkItems,
          }
        : undefined,
    transition_id: transitionID,
    workstation_name: TEST_TOPOLOGY.workstation_nodes_by_id[transitionID]?.workstation_name,
  } satisfies DashboardRuntimeWorkstationRequest;
}

function buildProjectedWorkstationRequest({
  dispatchID,
  inputWorkItems = [],
  outputWorkItems = [],
  startedAt,
  workstationNodeID = "review",
}: {
  dispatchID: string;
  inputWorkItems?: DashboardWorkItemRef[];
  outputWorkItems?: DashboardWorkItemRef[];
  startedAt: string;
  workstationNodeID?: string;
}): DashboardWorkstationRequest {
  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: outputWorkItems.length > 0 ? 1 : 0,
    },
    dispatch_id: dispatchID,
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [],
    request_view: {
      input_work_items: inputWorkItems,
      started_at: startedAt,
      trace_ids: inputWorkItems.flatMap((workItem) => (workItem.trace_id ? [workItem.trace_id] : [])),
    },
    responded_request_count: outputWorkItems.length > 0 ? 1 : 0,
    response_view:
      outputWorkItems.length > 0
        ? {
            output_work_items: outputWorkItems,
          }
        : undefined,
    started_at: startedAt,
    transition_id: workstationNodeID,
    work_items: outputWorkItems.length > 0 ? outputWorkItems : inputWorkItems,
    workstation_name:
      TEST_TOPOLOGY.workstation_nodes_by_id[workstationNodeID]?.workstation_name,
    workstation_node_id: workstationNodeID,
  };
}

function buildProviderSessionAttempt({
  dispatchID,
  sessionID,
  workItems,
  transitionID = "review",
}: {
  dispatchID: string;
  sessionID: string;
  workItems: DashboardWorkItemRef[];
  transitionID?: string;
}): DashboardProviderSessionAttempt {
  return {
    dispatch_id: dispatchID,
    outcome: "ACCEPTED",
    provider_session: {
      id: sessionID,
      kind: "session_id",
      provider: "codex",
    },
    transition_id: transitionID,
    work_items: workItems,
    workstation_name: TEST_TOPOLOGY.workstation_nodes_by_id[transitionID]?.workstation_name,
  };
}

function buildActiveExecution(
  dispatchID: string,
  workItems: DashboardWorkItemRef[],
  startedAt: string,
  workstationNodeID = "review",
): DashboardActiveExecution {
  return {
    dispatch_id: dispatchID,
    started_at: startedAt,
    trace_ids: workItems.flatMap((workItem) => (workItem.trace_id ? [workItem.trace_id] : [])),
    transition_id: workstationNodeID,
    work_items: workItems,
    workstation_name: TEST_TOPOLOGY.workstation_nodes_by_id[workstationNodeID]?.workstation_name,
    workstation_node_id: workstationNodeID,
    work_type_ids: workItems.flatMap((workItem) =>
      workItem.work_type_id ? [workItem.work_type_id] : [],
    ),
  };
}

function buildSnapshot({
  activeExecution,
  providerSessions = [],
  runtimeRequestsByDispatchID = {},
}: {
  activeExecution?: DashboardActiveExecution;
  providerSessions?: DashboardProviderSessionAttempt[];
  runtimeRequestsByDispatchID?: Record<string, DashboardRuntimeWorkstationRequest>;
}): DashboardSnapshot {
  const runtime = buildEmptyDashboardRuntimeFixture();

  return {
    factory_state: activeExecution ? "RUNNING" : "IDLE",
    tick_count: 12,
    topology: TEST_TOPOLOGY,
    uptime_seconds: 45,
    runtime: {
      ...runtime,
      active_dispatch_ids: activeExecution ? [activeExecution.dispatch_id] : [],
      active_executions_by_dispatch_id: activeExecution
        ? { [activeExecution.dispatch_id]: activeExecution }
        : {},
      active_workstation_node_ids: activeExecution ? [activeExecution.workstation_node_id] : [],
      current_work_items_by_place_id: activeExecution
        ? { "story:review": activeExecution.work_items ?? [] }
        : {},
      session: {
        ...runtime.session,
        provider_sessions: providerSessions,
      },
      workstation_requests_by_dispatch_id: runtimeRequestsByDispatchID,
    },
  };
}

function seedSelectedWork(dispatchID: string, nodeID: string, workItem: DashboardWorkItemRef): void {
  act(() => {
    useSelectionHistoryStore.getState().replacePresent({
      selection: {
        dispatchId: dispatchID,
        kind: "work-item",
        nodeId: nodeID,
        workItem,
      },
      terminalWorkDetail: null,
    });
  });
}

function SelectionHarness({
  snapshot,
  workstationRequestsByDispatchID,
}: {
  snapshot: DashboardSnapshot;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
}) {
  const currentSelection = useCurrentSelection({
    snapshot,
    workstationRequestsByDispatchID,
  });

  return (
    <>
      <div data-testid="dispatch-history">
        {currentSelection.selectedWorkRequestHistory.map((request) => request.dispatch_id).join(",")}
      </div>
      <div data-testid="projected-history">
        {currentSelection.selectedWorkWorkstationRequests.map((request) => request.dispatch_id).join(",")}
      </div>
      <div data-testid="provider-history">
        {currentSelection.selectedWorkProviderSessions.map((attempt) => attempt.dispatch_id).join(",")}
      </div>
      <div data-testid="dispatch-attempts">
        {currentSelection.selectedWorkDispatchAttempts
          .map((attempt) => attempt.dispatch_id)
          .join(",")}
      </div>
      <div data-testid="provider-sessions">
        {currentSelection.selectedWorkProviderSessions
          .map((attempt) => attempt.provider_session?.id ?? "missing")
          .join(",")}
      </div>
    </>
  );
}

describe("useCurrentSelection", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("derives active-only selected-work history from dispatch-keyed workstation requests", async () => {
    const selectedWorkItem = buildWorkItem("work-active", "Active Story");
    const activeExecution = buildActiveExecution(
      "dispatch-review-active",
      [selectedWorkItem],
      "2026-04-08T12:00:03Z",
    );
    const projectedRequests = {
      "dispatch-review-active": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-active",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:03Z",
      }),
    };

    seedSelectedWork("dispatch-review-active", "review", selectedWorkItem);
    render(
      <SelectionHarness
        snapshot={buildSnapshot({
          activeExecution,
          providerSessions: [
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-active",
              sessionID: "sess-active",
              workItems: [selectedWorkItem],
            }),
            buildProviderSessionAttempt({
              dispatchID: "dispatch-unrelated",
              sessionID: "sess-unrelated",
              workItems: [buildWorkItem("work-unrelated", "Unrelated Story")],
            }),
          ],
          runtimeRequestsByDispatchID: {
            "dispatch-review-active": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-review-active",
              inputWorkItems: [selectedWorkItem],
              startedAt: "2026-04-08T12:00:03Z",
            }),
          },
        })}
        workstationRequestsByDispatchID={projectedRequests}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-history").textContent).toBe("dispatch-review-active");
      expect(screen.getByTestId("projected-history").textContent).toBe("dispatch-review-active");
      expect(screen.getByTestId("provider-history").textContent).toBe("dispatch-review-active");
      expect(screen.getByTestId("provider-sessions").textContent).toBe("sess-active");
    });
  });

  it("keeps completed-only history when the selected work is present only in dispatch outputs", async () => {
    const outputWorkItem = buildWorkItem("work-completed", "Completed Story");
    const projectedRequests = {
      "dispatch-review-completed": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-completed",
        outputWorkItems: [outputWorkItem],
        startedAt: "2026-04-08T12:00:02Z",
      }),
    };

    seedSelectedWork("dispatch-review-completed", "review", outputWorkItem);
    render(
      <SelectionHarness
        snapshot={buildSnapshot({
          providerSessions: [
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-completed",
              sessionID: "sess-completed",
              workItems: [outputWorkItem],
            }),
          ],
          runtimeRequestsByDispatchID: {
            "dispatch-review-completed": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-review-completed",
              outputWorkItems: [outputWorkItem],
              startedAt: "2026-04-08T12:00:02Z",
            }),
          },
        })}
        workstationRequestsByDispatchID={projectedRequests}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-history").textContent).toBe("dispatch-review-completed");
      expect(screen.getByTestId("projected-history").textContent).toBe("dispatch-review-completed");
      expect(screen.getByTestId("provider-history").textContent).toBe("dispatch-review-completed");
      expect(screen.getByTestId("provider-sessions").textContent).toBe("sess-completed");
    });
  });

  it("falls back to normalized runtime workstation requests when the cached projection map is empty", async () => {
    const selectedWorkItem = buildWorkItem("work-runtime-fallback", "Runtime Fallback Story");
    const runtimeRequestsByDispatchID = {
      "dispatch-review-runtime-fallback": buildRuntimeWorkstationRequest({
        dispatchID: "dispatch-review-runtime-fallback",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:04Z",
      }),
    };

    seedSelectedWork("dispatch-review-runtime-fallback", "review", selectedWorkItem);
    render(
      <SelectionHarness
        snapshot={buildSnapshot({
          providerSessions: [
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-runtime-fallback",
              sessionID: "sess-runtime-fallback",
              workItems: [selectedWorkItem],
            }),
          ],
          runtimeRequestsByDispatchID,
        })}
        workstationRequestsByDispatchID={{}}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-history").textContent).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(screen.getByTestId("projected-history").textContent).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(screen.getByTestId("provider-history").textContent).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(screen.getByTestId("provider-sessions").textContent).toBe("sess-runtime-fallback");
    });
  });

  it("orders mixed selected-work history newest-first and collapses duplicate provider attempts per dispatch", async () => {
    const selectedWorkItem = buildWorkItem("work-shared", "Shared Story");
    const unrelatedWorkItem = buildWorkItem("work-unrelated", "Unrelated Story");
    const activeExecution = buildActiveExecution(
      "dispatch-review-active",
      [selectedWorkItem],
      "2026-04-08T12:00:03Z",
    );
    const projectedRequests = {
      "dispatch-review-active": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-active",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:03Z",
      }),
      "dispatch-review-old": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-old",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:01Z",
      }),
      "dispatch-review-output": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-output",
        outputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:02Z",
      }),
    };

    seedSelectedWork("dispatch-review-active", "review", selectedWorkItem);
    render(
      <SelectionHarness
        snapshot={buildSnapshot({
          activeExecution,
          providerSessions: [
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-old",
              sessionID: "sess-old-1",
              workItems: [selectedWorkItem],
            }),
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-output",
              sessionID: "sess-output",
              workItems: [selectedWorkItem],
            }),
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-old",
              sessionID: "sess-old-2",
              workItems: [selectedWorkItem],
            }),
            buildProviderSessionAttempt({
              dispatchID: "dispatch-unrelated",
              sessionID: "sess-unrelated",
              workItems: [unrelatedWorkItem],
            }),
            buildProviderSessionAttempt({
              dispatchID: "dispatch-review-active",
              sessionID: "sess-active",
              workItems: [selectedWorkItem],
            }),
          ],
          runtimeRequestsByDispatchID: {
            "dispatch-review-active": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-review-active",
              inputWorkItems: [selectedWorkItem],
              startedAt: "2026-04-08T12:00:03Z",
            }),
            "dispatch-review-old": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-review-old",
              inputWorkItems: [selectedWorkItem],
              startedAt: "2026-04-08T12:00:01Z",
            }),
            "dispatch-review-output": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-review-output",
              outputWorkItems: [selectedWorkItem],
              startedAt: "2026-04-08T12:00:02Z",
            }),
          },
        })}
        workstationRequestsByDispatchID={projectedRequests}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-history").textContent).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(screen.getByTestId("projected-history").textContent).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(screen.getByTestId("provider-history").textContent).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(screen.getByTestId("provider-sessions").textContent).toBe(
        "sess-active,sess-output,sess-old-2",
      );
    });
  });

  it("materializes replay-2 selected-work dispatch history for process work", async () => {
    const replaySnapshot = buildReplayFixtureTimelineSnapshot(
      "runtimeConfigInterfaceConsolidation",
      8,
    );
    const selectedWorkItem =
      replaySnapshot.dashboard.runtime.active_executions_by_dispatch_id?.[
        "062f0677-3b56-42f7-9a04-dc92997c7bf7"
      ]?.work_items?.[0];

    if (!selectedWorkItem) {
      throw new Error("expected replay snapshot to include active work-task-1");
    }

    expect(
      replaySnapshot.workstationRequestsByDispatchID[
        "062f0677-3b56-42f7-9a04-dc92997c7bf7"
      ]?.work_items.map((workItem) => workItem.work_id),
    ).toEqual(["work-task-1"]);
    expect(
      replaySnapshot.workstationRequestsByDispatchID[
        "17c38f40-de4e-4d5f-bd44-649a2bf4a284"
      ]?.work_items.map((workItem) => workItem.work_id),
    ).toEqual([
      "batch-request-f91ca780f375ef7b750bc316dee05bd6-runtime-config-interface-consolidation",
      "work-task-1",
    ]);

    seedSelectedWork(
      "062f0677-3b56-42f7-9a04-dc92997c7bf7",
      "process",
      selectedWorkItem,
    );

    render(
      <SelectionHarness
        snapshot={replaySnapshot.dashboard}
        workstationRequestsByDispatchID={replaySnapshot.workstationRequestsByDispatchID}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-history").textContent).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(screen.getByTestId("projected-history").textContent).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(screen.getByTestId("dispatch-attempts").textContent).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(screen.getByTestId("provider-history").textContent).toBe("");
      expect(screen.getByTestId("provider-sessions").textContent).toBe("");
    });
  });
});

