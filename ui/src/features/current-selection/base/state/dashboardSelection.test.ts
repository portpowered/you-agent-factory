// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: selection-resolution regression coverage stays in one place for timeline readability.
// biome-ignore-all lint/style/noExcessiveLinesPerFile: selection-resolution regression coverage stays in one place for timeline readability.
import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";

import { buildFactoryTimelineSnapshot } from "../../../timeline/state/factoryTimelineStore";
import {
  type DashboardWorkItemSelection,
  type DashboardWorkstationRequestSelection,
  findFactoryWorkerInSnapshot,
  findWorkstationNodeIDForPlace,
  resolveDashboardSelection,
  workstationNamesReferencingWorkerInSnapshot,
} from "./dashboardSelection";

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = event(
  "event-1",
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
      ],
    },
  },
);

const workRequest = event("event-2", 2, FACTORY_EVENT_TYPES.workRequest, {
  type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "Selection Story",
      request_id: "request-selection-1",
      trace_id: "trace-selection-1",
      work_id: "work-selection-1",
      work_type_id: "story",
    },
  ],
});
workRequest.context.requestId = "request-selection-1";
workRequest.context.traceIds = ["trace-selection-1"];
workRequest.context.workIds = ["work-selection-1"];

const dispatchRequest = event(
  "event-3",
  3,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-selection-1",
    inputs: [
      {
        name: "Selection Story",
        request_id: "request-selection-1",
        trace_id: "trace-selection-1",
        work_id: "work-selection-1",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      outputs: [{ state: "review", workType: "story" }],
      worker: "reviewer",
    },
  },
);
dispatchRequest.context.dispatchId = "dispatch-selection-1";
dispatchRequest.context.traceIds = ["trace-selection-1"];
dispatchRequest.context.workIds = ["work-selection-1"];

describe("resolveDashboardSelection", () => {
  it("retains workstation-request selections while the projected request remains present", () => {
    const activeTick = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest, dispatchRequest],
      3,
    );
    const request =
      activeTick.workstationRequestsByDispatchID["dispatch-selection-1"];
    if (!request) {
      throw new Error("expected workstation request projection");
    }

    const selection: DashboardWorkstationRequestSelection = {
      dispatchId: request.dispatch_id,
      kind: "workstation-request",
      nodeId: "stale-node-id",
      request,
    };
    const resolved = resolveDashboardSelection({
      selection,
      snapshot: activeTick,
      workstationRequestsByDispatchID:
        activeTick.workstationRequestsByDispatchID,
    });

    expect(resolved).toMatchObject({
      dispatchId: "dispatch-selection-1",
      kind: "workstation-request",
      nodeId: "review",
    });
  });

  it("falls back to the default dashboard selection when the projected request disappears", () => {
    const activeTick = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest, dispatchRequest],
      3,
    );
    const request =
      activeTick.workstationRequestsByDispatchID["dispatch-selection-1"];
    if (!request) {
      throw new Error("expected workstation request projection");
    }

    const beforeDispatch = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest],
      2,
    );
    const resolved = resolveDashboardSelection({
      selection: {
        dispatchId: request.dispatch_id,
        kind: "workstation-request",
        nodeId: request.workstation_node_id,
        request,
      },
      snapshot: beforeDispatch,
      workstationRequestsByDispatchID:
        beforeDispatch.workstationRequestsByDispatchID,
    });

    expect(resolved).toEqual({ kind: "node", nodeId: "review" });
  });

  it("reanchors selected work-item detail to the current retained request node", () => {
    const workItem = {
      display_name: "Selection Story",
      trace_id: "trace-selection-1",
      work_id: "work-selection-1",
      work_type_id: "story",
    };
    const snapshot: DashboardSnapshot = {
      factory_state: "RUNNING",
      runtime: {
        active_executions_by_dispatch_id: {},
        current_work_items_by_place_id: {},
        place_occupancy_work_items_by_place_id: {
          "story:repair": [workItem],
        },
        session: {
          completed_count: 0,
          dispatched_count: 1,
          failed_count: 0,
          has_data: true,
          provider_sessions: [],
        },
        workstation_requests_by_dispatch_id: {
          "dispatch-repair-1": {
            dispatch_id: "dispatch-repair-1",
            started_at: "2026-04-16T12:00:03Z",
            transition_id: "repair",
            workstation_name: "Repair",
            workstation_node_id: "repair",
            work_items: [workItem],
          },
        },
      },
      tick_count: 4,
      topology: {
        edges: [],
        workstation_node_ids: ["review", "repair"],
        workstation_nodes_by_id: {
          repair: {
            input_places: [
              {
                kind: "work_state",
                place_id: "story:review",
                state_category: "PROCESSING",
                state_name: "review",
                work_type_name: "story",
              },
            ],
            node_id: "repair",
            output_places: [
              {
                kind: "work_state",
                place_id: "story:repair",
                state_category: "PROCESSING",
                state_name: "repair",
                work_type_name: "story",
              },
            ],
            transition_id: "repair",
            workstation_name: "Repair",
          },
          review: {
            input_places: [
              {
                kind: "work_state",
                place_id: "story:new",
                state_category: "INITIAL",
                state_name: "new",
                work_type_name: "story",
              },
            ],
            node_id: "review",
            output_places: [
              {
                kind: "work_state",
                place_id: "story:review",
                state_category: "PROCESSING",
                state_name: "review",
                work_type_name: "story",
              },
            ],
            transition_id: "review",
            workstation_name: "Review",
          },
        },
      },
      uptime_seconds: 12,
    };
    const selection: DashboardWorkItemSelection = {
      dispatchId: "dispatch-review-1",
      kind: "work-item",
      nodeId: "review",
      workItem,
    };

    const resolved = resolveDashboardSelection({
      selection,
      snapshot,
      workstationRequestsByDispatchID: {
        "dispatch-repair-1": {
          counts: {
            dispatched_count: 1,
            errored_count: 0,
            responded_count: 1,
          },
          dispatch_id: "dispatch-repair-1",
          dispatched_request_count: 1,
          errored_request_count: 0,
          inference_attempts: [],
          request_view: {
            input_work_items: [workItem],
            started_at: "2026-04-16T12:00:03Z",
            trace_ids: ["trace-selection-1"],
          },
          responded_request_count: 1,
          started_at: "2026-04-16T12:00:03Z",
          transition_id: "repair",
          work_items: [workItem],
          workstation_name: "Repair",
          workstation_node_id: "repair",
        },
      },
    });

    expect(resolved).toEqual({
      dispatchId: "dispatch-repair-1",
      kind: "work-item",
      nodeId: "repair",
      workItem,
    });
  });

  it("retains worker selections while the authored worker remains in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
    };

    const selection = {
      kind: "worker" as const,
      workerName: "reviewer",
    };

    expect(
      resolveDashboardSelection({
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("resolves authored workers and referencing workstation names from the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER", model: "gpt-5" }],
      workstations: [
        { name: "Review", worker: "reviewer" },
        { name: "Plan", worker: "reviewer" },
        { name: "Code", worker: "planner" },
      ],
    };

    expect(findFactoryWorkerInSnapshot(snapshot, "reviewer")).toEqual({
      name: "reviewer",
      type: "MODEL_WORKER",
      model: "gpt-5",
    });
    expect(
      workstationNamesReferencingWorkerInSnapshot(snapshot, "reviewer"),
    ).toEqual(["Review", "Plan"]);
  });

  it("retains factory-graph work-type selections while the work type remains in the factory", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    const resolved = resolveDashboardSelection({
      selection: { kind: "node", nodeId: "work-type:story" },
      snapshot,
    });

    expect(resolved).toEqual({ kind: "node", nodeId: "work-type:story" });
  });

  it("retains work-type selections while the authored work type remains in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    const selection = {
      kind: "work-type" as const,
      workTypeName: "story",
    };

    expect(
      resolveDashboardSelection({
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("falls back to the default dashboard selection when the selected factory-graph work type disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workTypes: [],
    };

    const resolved = resolveDashboardSelection({
      selection: { kind: "node", nodeId: "work-type:story" },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("retains factory-graph work-state node selections while the state remains in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    const selection = {
      kind: "node" as const,
      nodeId: "work-state:story:new",
    };

    expect(
      resolveDashboardSelection({
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("falls back to the default dashboard selection when the selected work-state graph node disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "node",
        nodeId: "work-state:story:removed",
      },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("falls back to the default dashboard selection when the selected work type disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "work-type",
        workTypeName: "removed-type",
      },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("falls back when topologyFactory omits the selected worker even if snapshot.factory still lists it", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [
        { name: "reviewer", type: "MODEL_WORKER" },
        { name: "spare", type: "MODEL_WORKER" },
      ],
    };

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "worker",
        workerName: "spare",
      },
      snapshot,
      topologyFactory: {
        ...snapshot.factory,
        workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
      },
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("falls back to the default dashboard selection when the selected worker disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
    };

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "worker",
        workerName: "removed-worker",
      },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("retains resource selections while the authored resource remains in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      resources: [{ name: "gpu", capacity: 2 }],
    };

    const selection = {
      kind: "resource" as const,
      resourceName: "gpu",
    };

    expect(
      resolveDashboardSelection({
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("falls back to the default dashboard selection when the selected resource disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      resources: [{ name: "gpu", capacity: 2 }],
    };

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "resource",
        resourceName: "removed-resource",
      },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("retains doc selections that exist only in the graph-editor pending factory", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    const selection = {
      kind: "doc" as const,
      targetPath: "factory/docs/playbook.md",
    };

    expect(
      resolveDashboardSelection({
        pendingFactoryDefinition: {
          name: "Current Factory",
          supportingFiles: {
            bundledFiles: [
              {
                content: { encoding: "utf-8", inline: "# Playbook\n" },
                targetPath: "factory/docs/playbook.md",
                type: "DOC",
              },
            ],
          },
        },
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("falls back when a doc selection is absent from both saved and pending factories", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    expect(
      resolveDashboardSelection({
        pendingFactoryDefinition: {
          name: "Current Factory",
          supportingFiles: { bundledFiles: [] },
        },
        selection: {
          kind: "doc",
          targetPath: "factory/docs/missing.md",
        },
        snapshot,
      }),
    ).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });

  it("surfaces replayed work content from projected runtime refs for tracked work-item selection", () => {
    const payloadStructureRequest = event(
      "event-payload-structure",
      0,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
          workTypes: [
            {
              name: "task",
              states: [
                { name: "init", type: "INITIAL" },
                { name: "review", type: "PROCESSING" },
              ],
            },
          ],
          workstations: [
            {
              id: "t-review",
              inputs: [{ state: "init", workType: "task" }],
              name: "Review",
              outputs: [{ state: "review", workType: "task" }],
              worker: "reviewer",
            },
          ],
        },
      },
    );
    const payloadWorkRequest = event(
      "event-payload-work-request",
      1,
      FACTORY_EVENT_TYPES.workRequest,
      {
        source: "external-submit",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            content: [{ type: "text", text: "hello from sse" }],
            name: "work-payload-1",
            traceId: "trace-work-payload-1",
            workId: "work-payload-1",
            workTypeName: "task",
          },
        ],
      },
    );
    payloadWorkRequest.context.requestId = "request/work-payload-1";
    payloadWorkRequest.context.traceIds = ["trace-work-payload-1"];
    payloadWorkRequest.context.workIds = ["work-payload-1"];

    const snapshot = buildFactoryTimelineSnapshot(
      [payloadStructureRequest, payloadWorkRequest],
      1,
    );
    const placeID = Object.entries(
      snapshot.runtime.current_work_items_by_place_id ?? {},
    ).find(([, workItems]) =>
      workItems.some((workItem) => workItem.work_id === "work-payload-1"),
    )?.[0];
    if (!placeID) {
      throw new Error("expected replayed work item on a current-work place");
    }
    const nodeID = findWorkstationNodeIDForPlace(snapshot, placeID);
    if (!nodeID) {
      throw new Error("expected workstation node for current-work place");
    }

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "work-item",
        nodeId: nodeID,
        workItem: {
          display_name: "work-payload-1",
          trace_id: "trace-work-payload-1",
          work_id: "work-payload-1",
          work_type_id: "task",
        },
      },
      snapshot,
    });

    expect(resolved).toMatchObject({
      kind: "work-item",
      nodeId: nodeID,
      workItem: {
        content: [{ type: "text", text: "hello from sse" }],
        payload_status: "RESOLVED",
        work_id: "work-payload-1",
      },
    });
  });
});
