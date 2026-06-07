import { act, type RenderResult, render } from "@testing-library/react";
import type { ReactNode } from "react";
import type {
  DashboardActiveExecution,
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../../../components/dashboard/fixtures/runtime";
import {
  settleCurrentSelectionEffects,
  waitForCurrentSelection,
} from "../../../../testing/current-selection-test-utils";
import { buildReplayFixtureTimelineSnapshot } from "../../../../testing/replay-fixtures";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { resolveDashboardSelection } from "../../state/dashboardSelection";
import type { DashboardSelection } from "../../state/selection-types";
import {
  resetSelectionHistoryStore,
  useSelectionHistoryStore,
} from "../../state/selectionHistoryStore";
import type { CurrentSelectionState } from "./useCurrentSelection";
import { useCurrentSelection } from "./useCurrentSelection";

vi.mock("./useCurrentSelection.derived", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("./useCurrentSelection.derived")>();

  return {
    ...actual,
    useTerminalWorkDetailCleanup: () => undefined,
  };
});

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

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
      trace_ids: inputWorkItems.flatMap((workItem) =>
        workItem.trace_id ? [workItem.trace_id] : [],
      ),
    },
    response:
      outputWorkItems.length > 0
        ? {
            output_work_items: outputWorkItems,
          }
        : undefined,
    transition_id: transitionID,
    workstation_name:
      TEST_TOPOLOGY.workstation_nodes_by_id[transitionID]?.workstation_name,
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
      trace_ids: inputWorkItems.flatMap((workItem) =>
        workItem.trace_id ? [workItem.trace_id] : [],
      ),
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
      TEST_TOPOLOGY.workstation_nodes_by_id[workstationNodeID]
        ?.workstation_name,
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
    workstation_name:
      TEST_TOPOLOGY.workstation_nodes_by_id[transitionID]?.workstation_name,
  };
}

function buildInferenceAttempt({
  attempt,
  dispatchID,
  response,
  worktree,
}: {
  attempt: number;
  dispatchID: string;
  response?: string;
  worktree?: string;
}): DashboardInferenceAttempt {
  return {
    attempt,
    dispatch_id: dispatchID,
    inference_request_id: `${dispatchID}/inference/${attempt}`,
    prompt: `Prompt ${attempt}`,
    request_time: `2026-04-08T12:00:0${attempt}Z`,
    response,
    transition_id: "review",
    worktree,
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
    trace_ids: workItems.flatMap((workItem) =>
      workItem.trace_id ? [workItem.trace_id] : [],
    ),
    transition_id: workstationNodeID,
    work_items: workItems,
    workstation_name:
      TEST_TOPOLOGY.workstation_nodes_by_id[workstationNodeID]
        ?.workstation_name,
    workstation_node_id: workstationNodeID,
    work_type_ids: workItems.flatMap((workItem) =>
      workItem.work_type_id ? [workItem.work_type_id] : [],
    ),
  };
}

function buildSnapshot({
  activeExecution,
  inferenceAttemptsByDispatchID = {},
  providerSessions = [],
  runtimeRequestsByDispatchID = {},
}: {
  activeExecution?: DashboardActiveExecution;
  inferenceAttemptsByDispatchID?: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >;
  providerSessions?: DashboardProviderSessionAttempt[];
  runtimeRequestsByDispatchID?: Record<
    string,
    DashboardRuntimeWorkstationRequest
  >;
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
      active_workstation_node_ids: activeExecution
        ? [activeExecution.workstation_node_id]
        : [],
      current_work_items_by_place_id: activeExecution
        ? { "story:review": activeExecution.work_items ?? [] }
        : {},
      inference_attempts_by_dispatch_id: inferenceAttemptsByDispatchID,
      session: {
        ...runtime.session,
        provider_sessions: providerSessions,
      },
      workstation_requests_by_dispatch_id: runtimeRequestsByDispatchID,
    },
  };
}

function seedResolvedSelection({
  selection,
  snapshot,
  terminalWorkDetail = null,
  workstationRequestsByDispatchID = {},
}: {
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  terminalWorkDetail?: null;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): void {
  act(() => {
    useSelectionHistoryStore.getState().replacePresent({
      selection: resolveDashboardSelection({
        selection,
        snapshot,
        topologyFactory: snapshot.factory,
        workstationRequestsByDispatchID,
      }),
      terminalWorkDetail,
    });
  });
}

function seedSelectedWork(
  dispatchID: string,
  nodeID: string,
  workItem: DashboardWorkItemRef,
  snapshot?: DashboardSnapshot,
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): void {
  if (!snapshot) {
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
    return;
  }

  seedResolvedSelection({
    selection: {
      dispatchId: dispatchID,
      kind: "work-item",
      nodeId: nodeID,
      workItem,
    },
    snapshot,
    workstationRequestsByDispatchID,
  });
}

function readDispatchHistory(selection: CurrentSelectionState): string {
  return selection.selectedWorkRequestHistory
    .map((request) => request.dispatch_id)
    .join(",");
}

function readProjectedHistory(selection: CurrentSelectionState): string {
  return selection.selectedWorkWorkstationRequests
    .map((request) => request.dispatch_id)
    .join(",");
}

function readProviderHistory(selection: CurrentSelectionState): string {
  return selection.selectedWorkProviderSessions
    .map((attempt) => attempt.dispatch_id)
    .join(",");
}

function readDispatchAttempts(selection: CurrentSelectionState): string {
  return selection.selectedWorkDispatchAttempts
    .map((attempt) => attempt.dispatch_id)
    .join(",");
}

function readHistoryInferenceAttempts(
  selection: CurrentSelectionState,
): string {
  return selection.selectedWorkRequestHistory
    .flatMap((request) =>
      "inference_attempts" in request
        ? request.inference_attempts.map(
            (attempt) => attempt.inference_request_id,
          )
        : [],
    )
    .join(",");
}

function readProviderSessions(selection: CurrentSelectionState): string {
  return selection.selectedWorkProviderSessions
    .map((attempt) => attempt.provider_session?.id ?? "missing")
    .join(",");
}

type CurrentSelectionHookProps = {
  sessionID?: string;
  snapshot: DashboardSnapshot;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
};

type CurrentSelectionProbeHandle = {
  get current(): CurrentSelectionState;
  rerender: (nextProps: CurrentSelectionHookProps) => void;
  unmount: () => void;
};

function CurrentSelectionProbe(
  props: CurrentSelectionHookProps & {
    stateRef: { current: CurrentSelectionState | null };
  },
): ReactNode {
  const state = useCurrentSelection({
    sessionID: props.sessionID ?? "~default",
    snapshot: props.snapshot,
    workstationRequestsByDispatchID: props.workstationRequestsByDispatchID,
  });
  props.stateRef.current = state;
  return null;
}

async function renderCurrentSelectionHook(
  initialProps: CurrentSelectionHookProps,
): Promise<CurrentSelectionProbeHandle> {
  const stateRef = { current: null as CurrentSelectionState | null };
  let view: RenderResult | null = null;

  await act(async () => {
    view = render(
      <CurrentSelectionProbe {...initialProps} stateRef={stateRef} />,
    );
  });
  await settleCurrentSelectionEffects();

  if (view == null || stateRef.current == null) {
    throw new Error("CurrentSelectionProbe did not mount");
  }

  return {
    get current() {
      if (stateRef.current == null) {
        throw new Error("CurrentSelectionProbe is not mounted");
      }
      return stateRef.current;
    },
    rerender(nextProps: CurrentSelectionHookProps) {
      view?.rerender(
        <CurrentSelectionProbe {...nextProps} stateRef={stateRef} />,
      );
    },
    unmount() {
      view?.unmount();
      stateRef.current = null;
    },
  };
}

describe("useCurrentSelection", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
    } as ReturnType<typeof useCurrentFactoryDocument>);
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

    const snapshot = buildSnapshot({
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
    });

    seedSelectedWork(
      "dispatch-review-active",
      "review",
      selectedWorkItem,
      snapshot,
      projectedRequests,
    );
    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: projectedRequests,
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-review-active",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "dispatch-review-active",
      );
      expect(readProviderHistory(result.current)).toBe(
        "dispatch-review-active",
      );
      expect(readProviderSessions(result.current)).toBe("sess-active");
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

    const snapshot = buildSnapshot({
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
    });

    seedSelectedWork(
      "dispatch-review-completed",
      "review",
      outputWorkItem,
      snapshot,
      projectedRequests,
    );
    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: projectedRequests,
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-review-completed",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "dispatch-review-completed",
      );
      expect(readProviderHistory(result.current)).toBe(
        "dispatch-review-completed",
      );
      expect(readProviderSessions(result.current)).toBe("sess-completed");
    });
  });

  it("falls back to normalized runtime workstation requests when the cached projection map is empty", async () => {
    const selectedWorkItem = buildWorkItem(
      "work-runtime-fallback",
      "Runtime Fallback Story",
    );
    const inferenceAttemptsByDispatchID = {
      "dispatch-review-runtime-fallback": {
        "dispatch-review-runtime-fallback/inference/2": buildInferenceAttempt({
          attempt: 2,
          dispatchID: "dispatch-review-runtime-fallback",
          response: "Second response",
        }),
        "dispatch-review-runtime-fallback/inference/1": buildInferenceAttempt({
          attempt: 1,
          dispatchID: "dispatch-review-runtime-fallback",
          response: "First response",
          worktree: "tree-runtime",
        }),
      },
    };
    const runtimeRequestsByDispatchID = {
      "dispatch-review-runtime-fallback": buildRuntimeWorkstationRequest({
        dispatchID: "dispatch-review-runtime-fallback",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:04Z",
      }),
    };

    seedSelectedWork(
      "dispatch-review-runtime-fallback",
      "review",
      selectedWorkItem,
    );
    const result = await renderCurrentSelectionHook({
      snapshot: buildSnapshot({
        inferenceAttemptsByDispatchID,
        providerSessions: [
          buildProviderSessionAttempt({
            dispatchID: "dispatch-review-runtime-fallback",
            sessionID: "sess-runtime-fallback",
            workItems: [selectedWorkItem],
          }),
        ],
        runtimeRequestsByDispatchID,
      }),
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(readProviderHistory(result.current)).toBe(
        "dispatch-review-runtime-fallback",
      );
      expect(readProviderSessions(result.current)).toBe(
        "sess-runtime-fallback",
      );
      expect(readHistoryInferenceAttempts(result.current)).toBe(
        "dispatch-review-runtime-fallback/inference/1,dispatch-review-runtime-fallback/inference/2",
      );
    });
  });

  it("preserves selected work detail when the same work moves to a later workstation request", async () => {
    const selectedWorkItem = buildWorkItem(
      "work-reanchored",
      "Reanchored Story",
    );
    const activeExecution = buildActiveExecution(
      "dispatch-review-active",
      [selectedWorkItem],
      "2026-04-08T12:00:03Z",
    );
    const initialProjectedRequests = {
      "dispatch-review-active": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-review-active",
        inputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:03Z",
      }),
    };
    const rerenderedProjectedRequests = {
      "dispatch-repair-completed": buildProjectedWorkstationRequest({
        dispatchID: "dispatch-repair-completed",
        outputWorkItems: [selectedWorkItem],
        startedAt: "2026-04-08T12:00:09Z",
        workstationNodeID: "repair",
      }),
    };

    seedSelectedWork("dispatch-review-active", "review", selectedWorkItem);
    const result = await renderCurrentSelectionHook({
      snapshot: buildSnapshot({
        activeExecution,
        providerSessions: [
          buildProviderSessionAttempt({
            dispatchID: "dispatch-review-active",
            sessionID: "sess-review-active",
            workItems: [selectedWorkItem],
          }),
        ],
        runtimeRequestsByDispatchID: {
          "dispatch-review-active": buildRuntimeWorkstationRequest({
            dispatchID: "dispatch-review-active",
            inputWorkItems: [selectedWorkItem],
            startedAt: "2026-04-08T12:00:03Z",
          }),
        },
      }),
      workstationRequestsByDispatchID: initialProjectedRequests,
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-review-active",
      );
      expect(result.current.selectedWorkID ?? "").toBe("work-reanchored");
    });

    act(() => {
      result.rerender({
        snapshot: buildSnapshot({
          providerSessions: [
            buildProviderSessionAttempt({
              dispatchID: "dispatch-repair-completed",
              sessionID: "sess-repair-completed",
              transitionID: "repair",
              workItems: [selectedWorkItem],
            }),
          ],
          runtimeRequestsByDispatchID: {
            "dispatch-repair-completed": buildRuntimeWorkstationRequest({
              dispatchID: "dispatch-repair-completed",
              outputWorkItems: [selectedWorkItem],
              startedAt: "2026-04-08T12:00:09Z",
              transitionID: "repair",
            }),
          },
        }),
        workstationRequestsByDispatchID: rerenderedProjectedRequests,
      });
    });
    await settleCurrentSelectionEffects();

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-repair-completed",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "dispatch-repair-completed",
      );
      expect(readProviderHistory(result.current)).toBe(
        "dispatch-repair-completed",
      );
      expect(result.current.selectedWorkID ?? "").toBe("work-reanchored");
    });
  });

  it("orders mixed selected-work history newest-first and collapses duplicate provider attempts per dispatch", async () => {
    const selectedWorkItem = buildWorkItem("work-shared", "Shared Story");
    const unrelatedWorkItem = buildWorkItem(
      "work-unrelated",
      "Unrelated Story",
    );
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
    const result = await renderCurrentSelectionHook({
      snapshot: buildSnapshot({
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
      }),
      workstationRequestsByDispatchID: projectedRequests,
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(readProviderHistory(result.current)).toBe(
        "dispatch-review-active,dispatch-review-output,dispatch-review-old",
      );
      expect(readProviderSessions(result.current)).toBe(
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
      replaySnapshot.runtime.active_executions_by_dispatch_id?.[
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

    const result = await renderCurrentSelectionHook({
      snapshot: replaySnapshot,
      workstationRequestsByDispatchID:
        replaySnapshot.workstationRequestsByDispatchID,
    });

    await waitForCurrentSelection(() => {
      expect(readDispatchHistory(result.current)).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(readProjectedHistory(result.current)).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(readDispatchAttempts(result.current)).toBe(
        "062f0677-3b56-42f7-9a04-dc92997c7bf7,17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      );
      expect(readProviderHistory(result.current)).toBe("");
      expect(readProviderSessions(result.current)).toBe("");
    });
  });
  it("derives selected worker data from the snapshot factory document", async () => {
    const snapshot = buildSnapshot({
      activeExecution: buildActiveExecution(
        "dispatch-review-active",
        [buildWorkItem("work-active", "Active Story")],
        "2026-04-08T12:00:03Z",
      ),
    });
    snapshot.factory = {
      workers: [{ name: "writer", type: "MODEL_WORKER", model: "gpt-5.2" }],
      workstations: [
        { name: "Review", worker: "writer" },
        { name: "Plan", worker: "writer" },
      ],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "worker", workerName: "writer" },
        terminalWorkDetail: null,
      });
    });

    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(result.current.selectedWorkerName ?? "").toBe("writer");
      expect(result.current.selectedWorker?.type ?? "").toBe("MODEL_WORKER");
      expect(result.current.selectedWorkerWorkstationNames.join(",")).toBe(
        "Review,Plan",
      );
      expect(result.current.selection?.kind ?? "").toBe("worker");
    });
  });

  it("derives selected resource name from resource dashboard selection", async () => {
    const snapshot = buildSnapshot({
      activeExecution: buildActiveExecution(
        "dispatch-review-active",
        [buildWorkItem("work-active", "Active Story")],
        "2026-04-08T12:00:03Z",
      ),
    });
    snapshot.factory = {
      resources: [{ name: "gpu", capacity: 2 }],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "resource", resourceName: "gpu" },
        terminalWorkDetail: null,
      });
    });

    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(result.current.selectedResourceName ?? "").toBe("gpu");
      expect(result.current.selection?.kind ?? "").toBe("resource");
    });
  });

  it("falls back resource selection when the resource disappears from the factory document", async () => {
    const snapshot = buildSnapshot({
      activeExecution: buildActiveExecution(
        "dispatch-review-active",
        [buildWorkItem("work-active", "Active Story")],
        "2026-04-08T12:00:03Z",
      ),
    });
    snapshot.factory = {
      resources: [{ name: "gpu", capacity: 2 }],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "resource", resourceName: "removed-resource" },
        terminalWorkDetail: null,
      });
    });

    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(result.current.selection?.kind ?? "").toBe("node");
      expect(result.current.selectedResourceName ?? "").toBe("");
    });

    snapshot.factory = {
      resources: [{ name: "gpu", capacity: 2 }],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "resource", resourceName: "gpu" },
        terminalWorkDetail: null,
      });
    });

    act(() => {
      result.rerender({
        snapshot,
        workstationRequestsByDispatchID: {},
      });
    });
    await settleCurrentSelectionEffects();

    await waitForCurrentSelection(() => {
      expect(result.current.selection?.kind ?? "").toBe("resource");
      expect(result.current.selectedResourceName ?? "").toBe("gpu");
    });
  });

  it("falls back worker selection when the worker disappears from the factory document", async () => {
    const snapshot = buildSnapshot({
      activeExecution: buildActiveExecution(
        "dispatch-review-active",
        [buildWorkItem("work-active", "Active Story")],
        "2026-04-08T12:00:03Z",
      ),
    });
    snapshot.factory = {
      workers: [{ name: "writer", type: "MODEL_WORKER" }],
      workstations: [{ name: "Review", worker: "writer" }],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "worker", workerName: "removed-worker" },
        terminalWorkDetail: null,
      });
    });

    const result = await renderCurrentSelectionHook({
      snapshot,
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(result.current.selection?.kind ?? "").toBe("node");
      expect(result.current.selectedWorkerName ?? "").toBe("");
      expect(result.current.selectedWorker?.type ?? "").toBe("");
    });

    snapshot.factory = {
      workers: [{ name: "writer", type: "MODEL_WORKER" }],
      workstations: [{ name: "Review", worker: "writer" }],
    };

    act(() => {
      useSelectionHistoryStore.getState().replacePresent({
        selection: { kind: "worker", workerName: "writer" },
        terminalWorkDetail: null,
      });
    });

    act(() => {
      result.rerender({
        snapshot,
        workstationRequestsByDispatchID: {},
      });
    });
    await settleCurrentSelectionEffects();

    await waitForCurrentSelection(() => {
      expect(result.current.selection?.kind ?? "").toBe("worker");
      expect(result.current.selectedWorkerName ?? "").toBe("writer");
    });
  });

  it("drops the previous session's selected work when the session ID changes", async () => {
    const selectedWorkItem = buildWorkItem("work-active", "Active Story");
    seedSelectedWork("dispatch-review-active", "review", selectedWorkItem);

    const result = await renderCurrentSelectionHook({
      sessionID: "~default",
      snapshot: buildSnapshot({
        activeExecution: buildActiveExecution(
          "dispatch-review-active",
          [selectedWorkItem],
          "2026-04-08T12:00:03Z",
        ),
      }),
      workstationRequestsByDispatchID: {},
    });

    await waitForCurrentSelection(() => {
      expect(result.current.selectedWorkID ?? "").toBe("work-active");
    });

    act(() => {
      result.rerender({
        sessionID: "session-beta",
        snapshot: buildSnapshot({
          activeExecution: buildActiveExecution(
            "dispatch-review-beta",
            [buildWorkItem("work-beta", "Beta Story")],
            "2026-04-08T12:01:03Z",
          ),
        }),
        workstationRequestsByDispatchID: {},
      });
    });
    await settleCurrentSelectionEffects();

    await waitForCurrentSelection(() => {
      expect(result.current.selectedWorkID ?? "").toBe("");
      expect(readDispatchHistory(result.current)).toBe("");
    });
  });
});
