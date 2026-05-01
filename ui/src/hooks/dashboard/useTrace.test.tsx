import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FACTORY_EVENT_TYPES } from "../../api/events";
import type { FactoryEvent } from "../../api/events";
import { TraceWorkstationPath } from "../../features/trace-drilldown/trace-workstation-path";
import { buildFactoryTimelineSnapshot } from "../../state/factoryTimelineStore";
import { buildReplayFixtureTimelineSnapshot } from "../../testing/replay-fixtures";
import { expandTraceWithCausalPredecessors } from "./useTrace";

vi.mock("../../features/trace-drilldown/trace-elk-layout", () => ({
  getCachedTraceGraphLayout: () => null,
  async layoutTraceGraphWithElk<TNode>(nodes: TNode[]): Promise<TNode[]> {
    return nodes;
  },
  traceGraphLayoutKey: () => "trace-layout-test",
}));

vi.mock("@xyflow/react", async () => {
  return {
    Background: () => null,
    Controls: () => null,
    Handle: () => null,
    MarkerType: { ArrowClosed: "arrowclosed" },
    Position: { Left: "left", Right: "right" },
    ReactFlow: ({
      edges,
      nodes,
    }: {
      edges: Array<{ id: string; source: string; target: string }>;
      nodes: Array<{ id: string }>;
    }) => (
      <div
        data-edges={JSON.stringify(edges)}
        data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
        data-testid="trace-react-flow"
      />
    ),
    applyNodeChanges: (
      _changes: Array<Record<string, unknown>>,
      nodes: Array<Record<string, unknown>>,
    ) => nodes,
  };
});

const baseEventTime = Date.parse("2026-04-22T18:00:00Z");
const reviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", work_type: "story" }],
  name: "Review",
  outputs: [{ state: "review", work_type: "story" }],
  worker: "reviewer",
};
const completeWorkstation = {
  id: "complete",
  inputs: [{ state: "review", work_type: "story" }],
  name: "Complete",
  outputs: [{ state: "done", work_type: "story" }],
  worker: "completer",
};

function timelineEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: new Date(baseEventTime + tick * 1_000).toISOString(),
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

function withContext(
  event: FactoryEvent,
  context: Partial<FactoryEvent["context"]>,
): FactoryEvent {
  return {
    ...event,
    context: {
      ...event.context,
      ...context,
    },
  };
}

function renderedEdgePairs(): string[] {
  const edgePayload = screen.getByTestId("trace-react-flow").getAttribute("data-edges");
  if (!edgePayload) {
    throw new Error("Expected mock React Flow edges to be captured.");
  }

  return (JSON.parse(edgePayload) as Array<{ source: string; target: string }>)
    .map((edge) => `${edge.source}->${edge.target}`)
    .sort();
}

function buildInitialStructureEvent(): FactoryEvent {
  return timelineEvent("event-initial-structure", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      work_types: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [reviewWorkstation, completeWorkstation],
    },
  });
}

function buildFanInTimelineEvents(): FactoryEvent[] {
  const workRequest = withContext(
    timelineEvent("event-work-request", 2, FACTORY_EVENT_TYPES.workRequest, {
      source: "api",
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          current_chaining_trace_id: "chain-a",
          name: "Plan Input",
          trace_id: "chain-a",
          work_id: "work-plan-input",
          work_type_id: "story",
        },
        {
          current_chaining_trace_id: "chain-b",
          name: "Research Input",
          trace_id: "chain-b",
          work_id: "work-research-input",
          work_type_id: "story",
        },
      ],
    }),
    {
      requestId: "request-chain",
      traceIds: ["chain-a", "chain-b"],
      workIds: ["work-plan-input", "work-research-input"],
    },
  );
  const planDispatchRequest = withContext(
    timelineEvent("event-plan-dispatch-request", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
      current_chaining_trace_id: "chain-a",
      dispatchId: "dispatch-plan",
      inputs: [
        {
          current_chaining_trace_id: "chain-a",
          name: "Plan Input",
          trace_id: "chain-a",
          work_id: "work-plan-input",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-plan",
      traceIds: ["chain-a"],
      workIds: ["work-plan-input"],
    },
  );
  const planDispatchResponse = withContext(
    timelineEvent("event-plan-dispatch-response", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
      current_chaining_trace_id: "chain-a",
      dispatchId: "dispatch-plan",
      durationMillis: 450,
      outcome: "ACCEPTED",
      outputWork: [
        {
          current_chaining_trace_id: "chain-a",
          name: "Reviewed Story",
          trace_id: "chain-a",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-plan",
      traceIds: ["chain-a"],
      workIds: ["work-plan-input"],
    },
  );
  const researchDispatchRequest = withContext(
    timelineEvent("event-research-dispatch-request", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
      current_chaining_trace_id: "chain-b",
      dispatchId: "dispatch-research",
      inputs: [
        {
          current_chaining_trace_id: "chain-b",
          name: "Research Input",
          trace_id: "chain-b",
          work_id: "work-research-input",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-research",
      traceIds: ["chain-b"],
      workIds: ["work-research-input"],
    },
  );
  const researchDispatchResponse = withContext(
    timelineEvent("event-research-dispatch-response", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
      current_chaining_trace_id: "chain-b",
      dispatchId: "dispatch-research",
      durationMillis: 420,
      outcome: "ACCEPTED",
      outputWork: [
        {
          current_chaining_trace_id: "chain-b",
          name: "Research Context",
          trace_id: "chain-b",
          work_id: "work-research-context",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-research",
      traceIds: ["chain-b"],
      workIds: ["work-research-input"],
    },
  );
  const implementDispatchRequest = withContext(
    timelineEvent("event-implement-dispatch-request", 7, FACTORY_EVENT_TYPES.dispatchRequest, {
      current_chaining_trace_id: "chain-a",
      dispatchId: "dispatch-implement",
      inputs: [
        {
          current_chaining_trace_id: "chain-a",
          name: "Reviewed Story",
          trace_id: "chain-a",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
        {
          current_chaining_trace_id: "chain-b",
          name: "Research Context",
          trace_id: "chain-b",
          work_id: "work-research-context",
          work_type_id: "story",
        },
      ],
      previous_chaining_trace_ids: ["chain-a", "chain-b"],
      transitionId: "complete",
      workstation: completeWorkstation,
    }),
    {
      dispatchId: "dispatch-implement",
      traceIds: ["chain-a", "chain-b"],
      workIds: ["work-reviewed-story", "work-research-context"],
    },
  );
  const implementDispatchResponse = withContext(
    timelineEvent("event-implement-dispatch-response", 8, FACTORY_EVENT_TYPES.dispatchResponse, {
      current_chaining_trace_id: "chain-a",
      dispatchId: "dispatch-implement",
      durationMillis: 900,
      outcome: "ACCEPTED",
      outputWork: [
        {
          current_chaining_trace_id: "chain-a",
          name: "Implemented Story",
          previous_chaining_trace_ids: ["chain-a", "chain-b"],
          trace_id: "chain-a",
          work_id: "work-result",
          work_type_id: "story",
        },
      ],
      previous_chaining_trace_ids: ["chain-a", "chain-b"],
      transitionId: "complete",
      workstation: completeWorkstation,
    }),
    {
      dispatchId: "dispatch-implement",
      traceIds: ["chain-a", "chain-b"],
      workIds: ["work-reviewed-story", "work-research-context"],
    },
  );

  return [
    buildInitialStructureEvent(),
    workRequest,
    planDispatchRequest,
    planDispatchResponse,
    researchDispatchRequest,
    researchDispatchResponse,
    implementDispatchRequest,
    implementDispatchResponse,
  ];
}

function buildLegacyTimelineEvents(): FactoryEvent[] {
  const workRequest = withContext(
    timelineEvent("event-legacy-work-request", 2, FACTORY_EVENT_TYPES.workRequest, {
      source: "api",
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Legacy Story",
          trace_id: "trace-legacy",
          work_id: "work-legacy",
          work_type_id: "story",
        },
      ],
    }),
    {
      requestId: "request-legacy",
      traceIds: ["trace-legacy"],
      workIds: ["work-legacy"],
    },
  );
  const reviewDispatchRequest = withContext(
    timelineEvent("event-legacy-review-request", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
      dispatchId: "dispatch-legacy-review",
      inputs: [
        {
          name: "Legacy Story",
          trace_id: "trace-legacy",
          work_id: "work-legacy",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-legacy-review",
      traceIds: ["trace-legacy"],
      workIds: ["work-legacy"],
    },
  );
  const reviewDispatchResponse = withContext(
    timelineEvent("event-legacy-review-response", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
      dispatchId: "dispatch-legacy-review",
      durationMillis: 360,
      outcome: "ACCEPTED",
      outputWork: [
        {
          name: "Legacy Review",
          trace_id: "trace-legacy",
          work_id: "work-legacy-reviewed",
          work_type_id: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    }),
    {
      dispatchId: "dispatch-legacy-review",
      traceIds: ["trace-legacy"],
      workIds: ["work-legacy"],
    },
  );
  const completeDispatchRequest = withContext(
    timelineEvent("event-legacy-complete-request", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
      dispatchId: "dispatch-legacy-complete",
      inputs: [
        {
          name: "Legacy Review",
          trace_id: "trace-legacy",
          work_id: "work-legacy-reviewed",
          work_type_id: "story",
        },
      ],
      transitionId: "complete",
      workstation: completeWorkstation,
    }),
    {
      dispatchId: "dispatch-legacy-complete",
      traceIds: ["trace-legacy"],
      workIds: ["work-legacy-reviewed"],
    },
  );
  const completeDispatchResponse = withContext(
    timelineEvent("event-legacy-complete-response", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
      dispatchId: "dispatch-legacy-complete",
      durationMillis: 640,
      outcome: "ACCEPTED",
      outputWork: [
        {
          name: "Legacy Done",
          trace_id: "trace-legacy",
          work_id: "work-legacy-done",
          work_type_id: "story",
        },
      ],
      transitionId: "complete",
      workstation: completeWorkstation,
    }),
    {
      dispatchId: "dispatch-legacy-complete",
      traceIds: ["trace-legacy"],
      workIds: ["work-legacy-reviewed"],
    },
  );

  return [
    buildInitialStructureEvent(),
    workRequest,
    reviewDispatchRequest,
    reviewDispatchResponse,
    completeDispatchRequest,
    completeDispatchResponse,
  ];
}

describe("expandTraceWithCausalPredecessors", () => {
  afterEach(() => {
    cleanup();
  });

  it("merges predecessor traces from selected-tick events and renders fan-in drilldown edges", async () => {
    const snapshot = buildFactoryTimelineSnapshot(buildFanInTimelineEvents(), 8);
    const groupedTrace = snapshot.tracesByWorkID["work-result"];

    expect(
      snapshot.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-implement"
      ]?.request?.input_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Research Context",
        trace_id: "chain-b",
        work_id: "work-research-context",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Reviewed Story",
        trace_id: "chain-a",
        work_id: "work-reviewed-story",
        work_type_id: "story",
      },
    ]);
    expect(
      snapshot.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-implement"
      ]?.response?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Implemented Story",
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: "work-result",
        work_type_id: "story",
      },
    ]);
    expect(groupedTrace.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "dispatch-plan",
      "dispatch-implement",
    ]);
    expect(groupedTrace.workstation_sequence).toEqual(["Review", "Complete"]);

    const expandedTrace = expandTraceWithCausalPredecessors(
      groupedTrace,
      snapshot.tracesByWorkID,
    );
    if (!expandedTrace) {
      throw new Error("Expected expanded trace to be present.");
    }

    expect(expandedTrace.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "dispatch-plan",
      "dispatch-research",
      "dispatch-implement",
    ]);
    expect(expandedTrace.transition_ids).toEqual(["complete", "review"]);
    expect(expandedTrace.work_ids).toEqual([
      "work-plan-input",
      "work-research-context",
      "work-research-input",
      "work-result",
      "work-reviewed-story",
    ]);
    expect(
      expandedTrace.dispatches.find(
        (dispatch) => dispatch.dispatch_id === "dispatch-implement",
      ),
    ).toMatchObject({
      previous_chaining_trace_ids: ["chain-a", "chain-b"],
      trace_ids: ["chain-a", "chain-b"],
    });

    render(<TraceWorkstationPath dispatches={expandedTrace.dispatches} />);

    await waitFor(() => {
      expect(
        renderedEdgePairs().filter((edge) => edge.endsWith("->dispatch-implement")),
      ).toEqual([
        "dispatch-plan->dispatch-implement",
        "dispatch-research->dispatch-implement",
      ]);
    });
  });

  it("leaves legacy grouped traces on the fallback path when predecessor metadata is absent", async () => {
    const snapshot = buildFactoryTimelineSnapshot(buildLegacyTimelineEvents(), 6);
    const groupedTrace = snapshot.tracesByWorkID["work-legacy-done"];
    const expandedTrace = expandTraceWithCausalPredecessors(
      groupedTrace,
      snapshot.tracesByWorkID,
    );

    if (!expandedTrace) {
      throw new Error("Expected legacy trace to be present.");
    }

    expect(expandedTrace.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "dispatch-legacy-review",
      "dispatch-legacy-complete",
    ]);
    expect(
      expandedTrace.dispatches.every(
        (dispatch) => dispatch.previous_chaining_trace_ids === undefined,
      ),
    ).toBe(true);
    expect(expandedTrace.request_ids).toEqual(["request-legacy"]);
    expect(expandedTrace.workstation_sequence).toEqual(["Review", "Complete"]);

    render(<TraceWorkstationPath dispatches={expandedTrace.dispatches} />);

    await waitFor(() => {
      expect(renderedEdgePairs()).toEqual([
        "dispatch-legacy-review->dispatch-legacy-complete",
      ]);
    });
  });

  it("materializes replay-2 process work into trace-grid-card input data", () => {
    const snapshot = buildReplayFixtureTimelineSnapshot(
      "runtimeConfigInterfaceConsolidation",
      8,
    );

    expect(Object.keys(snapshot.tracesByWorkID).sort()).toEqual(
      expect.arrayContaining(["work-task-1", "work-task-2"]),
    );

    const workTaskOneTrace = expandTraceWithCausalPredecessors(
      snapshot.tracesByWorkID["work-task-1"],
      snapshot.tracesByWorkID,
    );
    const workTaskTwoTrace = expandTraceWithCausalPredecessors(
      snapshot.tracesByWorkID["work-task-2"],
      snapshot.tracesByWorkID,
    );

    expect(workTaskOneTrace).toBeDefined();
    expect(workTaskTwoTrace).toBeDefined();

    expect(workTaskOneTrace?.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      "2e11ef2e-b2b6-4328-8c59-9391d99953a0",
    ]);
    expect(workTaskTwoTrace?.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "17c38f40-de4e-4d5f-bd44-649a2bf4a284",
      "2e11ef2e-b2b6-4328-8c59-9391d99953a0",
    ]);

    expect(
      workTaskOneTrace?.dispatches.map((dispatch) => ({
        dispatch_id: dispatch.dispatch_id,
        input_work_ids: (dispatch.input_items ?? []).map((item) => item.work_id),
        output_work_ids: (dispatch.output_items ?? []).map((item) => item.work_id),
        transition_id: dispatch.transition_id,
      })),
    ).toEqual([
      {
        dispatch_id: "17c38f40-de4e-4d5f-bd44-649a2bf4a284",
        input_work_ids: ["batch-request-f91ca780f375ef7b750bc316dee05bd6-runtime-config-interface-consolidation"],
        output_work_ids: [
          "batch-request-f91ca780f375ef7b750bc316dee05bd6-runtime-config-interface-consolidation",
          "work-task-1",
        ],
        transition_id: "setup-workspace",
      },
      {
        dispatch_id: "2e11ef2e-b2b6-4328-8c59-9391d99953a0",
        input_work_ids: ["batch-request-f91ca780f375ef7b750bc316dee05bd6-work-1"],
        output_work_ids: [
          "batch-request-f91ca780f375ef7b750bc316dee05bd6-work-1",
          "work-task-2",
        ],
        transition_id: "setup-workspace",
      },
    ]);
    expect(
      workTaskTwoTrace?.dispatches.map((dispatch) => ({
        dispatch_id: dispatch.dispatch_id,
        input_work_ids: (dispatch.input_items ?? []).map((item) => item.work_id),
        output_work_ids: (dispatch.output_items ?? []).map((item) => item.work_id),
        transition_id: dispatch.transition_id,
      })),
    ).toEqual([
      {
        dispatch_id: "17c38f40-de4e-4d5f-bd44-649a2bf4a284",
        input_work_ids: ["batch-request-f91ca780f375ef7b750bc316dee05bd6-runtime-config-interface-consolidation"],
        output_work_ids: [
          "batch-request-f91ca780f375ef7b750bc316dee05bd6-runtime-config-interface-consolidation",
          "work-task-1",
        ],
        transition_id: "setup-workspace",
      },
      {
        dispatch_id: "2e11ef2e-b2b6-4328-8c59-9391d99953a0",
        input_work_ids: ["batch-request-f91ca780f375ef7b750bc316dee05bd6-work-1"],
        output_work_ids: [
          "batch-request-f91ca780f375ef7b750bc316dee05bd6-work-1",
          "work-task-2",
        ],
        transition_id: "setup-workspace",
      },
    ]);
  });
});
