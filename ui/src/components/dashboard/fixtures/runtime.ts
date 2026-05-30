import type {
  DashboardRuntime,
  DashboardSessionRuntime,
  DashboardSnapshot,
  DashboardTopology,
  DashboardWorkstationNode,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";

import { mediumBranchingDashboardTopology } from "./topologies";

const DEFAULT_FIXTURE_OBSERVED_AT = "2026-04-08T12:00:00Z";

export type DashboardRuntimeOverlay = (
  runtime: DashboardRuntime,
  topology: DashboardTopology,
) => DashboardRuntime;

function buildEmptyDashboardSessionRuntime(): DashboardSessionRuntime {
  return {
    has_data: true,
    dispatched_count: 1,
    completed_count: 0,
    failed_count: 0,
    completed_work_labels: [],
    failed_work_labels: [],
  };
}

export function buildEmptyDashboardRuntimeFixture(): DashboardRuntime {
  return {
    in_flight_dispatch_count: 0,
    place_token_counts: {
      "agent-slot:available": 2,
      "quality-gate:ready": 1,
      "story:ready": 3,
    },
    session: buildEmptyDashboardSessionRuntime(),
  };
}

function firstExistingNodeID(topology: DashboardTopology, preferredNodeID: string): string {
  return topology.workstation_nodes_by_id[preferredNodeID]
    ? preferredNodeID
    : (topology.workstation_node_ids[0] ?? preferredNodeID);
}

function namedWorkItem(name: string): DashboardWorkItemRef {
  return {
    work_id: `work-${name}`,
    work_type_id: "story",
    display_name: name
      .split("-")
      .map((part) => part[0]?.toUpperCase() + part.slice(1))
      .join(" "),
    trace_id: `trace-${name}`,
  };
}

export const multimodalActiveWorkPayload = {
  content: [
    { text: "Primary selected-work payload text", type: "text" as const },
    { json: { priority: 1 }, type: "JSON" as const },
    { file: "screenshot.png", type: "image" as const },
  ],
  payload_status: "RESOLVED",
} satisfies Pick<DashboardWorkItemRef, "content" | "payload_status">;

function workItemWithMultimodalPayload(workItem: DashboardWorkItemRef): DashboardWorkItemRef {
  return {
    ...workItem,
    ...multimodalActiveWorkPayload,
  };
}

function mapWorkItemsWithMultimodalPayload(
  workItems: DashboardWorkItemRef[] | undefined,
): DashboardWorkItemRef[] {
  return (workItems ?? []).map(workItemWithMultimodalPayload);
}

function appendProviderSession(
  runtime: DashboardRuntime,
  attempt: NonNullable<DashboardSessionRuntime["provider_sessions"]>[number],
): DashboardRuntime {
  return {
    ...runtime,
    session: {
      ...runtime.session,
      provider_sessions: [...(runtime.session.provider_sessions ?? []), attempt],
    },
  };
}

function workstationBehaviorFromTopology(
  workstationKind: DashboardWorkstationNode["workstation_kind"],
): NonNullable<
  NonNullable<CanonicalFactoryDefinition["workstations"]>[number]["behavior"]
> {
  switch (workstationKind?.toUpperCase()) {
    case "CRON":
      return "CRON";
    case "POLLER":
      return "POLLER";
    case "REPEATER":
      return "REPEATER";
    default:
      return "STANDARD";
  }
}

export function factoryFromDashboardTopology(
  topology: DashboardTopology,
): CanonicalFactoryDefinition {
  const resources = new Map<
    string,
    NonNullable<CanonicalFactoryDefinition["resources"]>[number]
  >();
  const workers = new Map<
    string,
    NonNullable<CanonicalFactoryDefinition["workers"]>[number]
  >();
  const workTypes = new Map<
    string,
    NonNullable<CanonicalFactoryDefinition["workTypes"]>[number]
  >();

  function appendWorkState(
    place: NonNullable<DashboardWorkstationNode["input_places"]>[number],
  ) {
    if (
      place.kind !== "work_state" ||
      !place.type_id ||
      !place.state_value
    ) {
      return;
    }

    const workType:
      NonNullable<CanonicalFactoryDefinition["workTypes"]>[number] =
      workTypes.get(place.type_id) ?? {
      name: place.type_id,
      states: [],
    };
    if (!workType.states.some((state) => state.name === place.state_value)) {
      workType.states.push({
        name: place.state_value,
        type: place.state_category ?? "PROCESSING",
      });
    }
    workTypes.set(place.type_id, workType);
  }

  function appendResource(
    place: NonNullable<DashboardWorkstationNode["input_places"]>[number],
  ) {
    if (
      place.kind !== "resource" ||
      !place.type_id ||
      resources.has(place.type_id)
    ) {
      return;
    }

    resources.set(place.type_id, {
      capacity: 1,
      name: place.type_id,
    });
  }

  const workstations = topology.workstation_node_ids.map((nodeId) => {
    const workstation = topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      throw new Error(`Missing topology workstation fixture node ${nodeId}.`);
    }
    const inputPlaces = workstation.input_places ?? [];
    const outputPlaces = workstation.output_places ?? [];

    for (const place of [...inputPlaces, ...outputPlaces]) {
      appendWorkState(place);
      appendResource(place);
    }

    if (workstation.worker_type && !workers.has(workstation.worker_type)) {
      workers.set(workstation.worker_type, {
        model: "gpt-5-mini",
        name: workstation.worker_type,
        type: "MODEL_WORKER",
      });
    }

    const inputs = inputPlaces.flatMap((place) => {
      if (
        place.kind !== "work_state" ||
        !place.type_id ||
        !place.state_value
      ) {
        return [];
      }
      return {
        state: place.state_value,
        workType: place.type_id,
      };
    });
    const outputs = outputPlaces.flatMap((place) => {
      if (
        place.kind !== "work_state" ||
        !place.type_id ||
        !place.state_value ||
        place.state_category === "FAILED"
      ) {
        return [];
      }
      return {
        state: place.state_value,
        workType: place.type_id,
      };
    });
    const onFailure = outputPlaces.flatMap((place) => {
      if (
        place.kind !== "work_state" ||
        !place.type_id ||
        !place.state_value ||
        place.state_category !== "FAILED"
      ) {
        return [];
      }
      return {
        state: place.state_value,
        workType: place.type_id,
      };
    });

    return {
      behavior: workstationBehaviorFromTopology(workstation.workstation_kind),
      id: workstation.node_id,
      inputs,
      name: workstation.workstation_name ?? workstation.transition_id,
      onFailure,
      outputs,
      resources: inputPlaces.flatMap((place) =>
        place.kind === "resource" && place.type_id
          ? [{ capacity: 1, name: place.type_id }]
          : [],
      ),
      type: "MODEL_WORKSTATION" as const,
      worker: workstation.worker_type ?? "",
    };
  });

  return {
    name: "dashboard-fixture",
    resources: [...resources.values()],
    workers: [...workers.values()],
    workTypes: [...workTypes.values()],
    workstations,
  };
}

export const activeWorkRuntimeOverlay: DashboardRuntimeOverlay = (runtime, topology) => {
  const nodeID = firstExistingNodeID(topology, "review");
  const workstation = topology.workstation_nodes_by_id[nodeID];
  const workItem = namedWorkItem("active-story");
  const dispatchID = `dispatch-${nodeID}-active`;

  return appendProviderSession({
    ...runtime,
    in_flight_dispatch_count: runtime.in_flight_dispatch_count + 1,
    active_dispatch_ids: [...(runtime.active_dispatch_ids ?? []), dispatchID],
    active_workstation_node_ids: [...(runtime.active_workstation_node_ids ?? []), nodeID],
    current_work_items_by_place_id: {
      ...(runtime.current_work_items_by_place_id ?? {}),
      "story:implemented": [workItem],
    },
    active_executions_by_dispatch_id: {
      ...(runtime.active_executions_by_dispatch_id ?? {}),
      [dispatchID]: {
        dispatch_id: dispatchID,
        workstation_node_id: nodeID,
        transition_id: workstation?.transition_id ?? nodeID,
        workstation_name: workstation?.workstation_name,
        started_at: DEFAULT_FIXTURE_OBSERVED_AT,
        work_type_ids: workItem.work_type_id ? [workItem.work_type_id] : [],
        work_items: [workItem],
        trace_ids: workItem.trace_id ? [workItem.trace_id] : [],
        consumed_tokens: [
          {
            token_id: "token-active-story",
            place_id: "story:implemented",
            name: workItem.display_name,
            work_id: workItem.work_id,
            work_type_id: workItem.work_type_id ?? "story",
            trace_id: workItem.trace_id,
            created_at: DEFAULT_FIXTURE_OBSERVED_AT,
            entered_at: DEFAULT_FIXTURE_OBSERVED_AT,
          },
        ],
      },
    },
    place_token_counts: {
      ...(runtime.place_token_counts ?? {}),
      "story:implemented": (runtime.place_token_counts?.["story:implemented"] ?? 0) + 1,
    },
    workstation_activity_by_node_id: {
      ...(runtime.workstation_activity_by_node_id ?? {}),
      [nodeID]: {
        workstation_node_id: nodeID,
        active_dispatch_ids: [dispatchID],
        active_work_items: [workItem],
        trace_ids: workItem.trace_id ? [workItem.trace_id] : [],
      },
    },
    session: {
      ...runtime.session,
      dispatched_count: Math.max(runtime.session.dispatched_count, 1),
    },
  }, {
    dispatch_id: dispatchID,
    transition_id: workstation?.transition_id ?? nodeID,
    workstation_name: workstation?.workstation_name,
    outcome: "ACCEPTED",
    provider_session: {
      provider: "codex",
      kind: "session_id",
      id: "sess-active-story",
    },
    work_items: [workItem],
  });
};

export const activeWorkWithMultimodalPayloadRuntimeOverlay: DashboardRuntimeOverlay = (
  runtime,
  topology,
) => {
  const enrichedRuntime = activeWorkRuntimeOverlay(runtime, topology);
  const enrichedWorkItems = (workItems: DashboardWorkItemRef[] | undefined) =>
    mapWorkItemsWithMultimodalPayload(workItems);

  return {
    ...enrichedRuntime,
    active_executions_by_dispatch_id: Object.fromEntries(
      Object.entries(enrichedRuntime.active_executions_by_dispatch_id ?? {}).map(
        ([dispatchID, execution]) => [
          dispatchID,
          {
            ...execution,
            work_items: enrichedWorkItems(execution.work_items),
          },
        ],
      ),
    ),
    current_work_items_by_place_id: Object.fromEntries(
      Object.entries(enrichedRuntime.current_work_items_by_place_id ?? {}).map(
        ([placeID, workItems]) => [placeID, enrichedWorkItems(workItems)],
      ),
    ),
    session: {
      ...enrichedRuntime.session,
      provider_sessions: enrichedRuntime.session.provider_sessions?.map((attempt) => ({
        ...attempt,
        work_items: enrichedWorkItems(attempt.work_items),
      })),
    },
    workstation_activity_by_node_id: Object.fromEntries(
      Object.entries(enrichedRuntime.workstation_activity_by_node_id ?? {}).map(
        ([nodeID, activity]) => [
          nodeID,
          {
            ...activity,
            active_work_items: enrichedWorkItems(activity.active_work_items),
          },
        ],
      ),
    ),
  };
};

export const retryAttemptRuntimeOverlay: DashboardRuntimeOverlay = (runtime, topology) => {
  const nodeID = firstExistingNodeID(topology, "implement");
  const workstation = topology.workstation_nodes_by_id[nodeID];
  const workItem = namedWorkItem("retry-story");

  return appendProviderSession(runtime, {
    dispatch_id: `dispatch-${nodeID}-retry`,
    transition_id: workstation?.transition_id ?? nodeID,
    workstation_name: workstation?.workstation_name,
    outcome: "RETRY",
    provider_session: {
      provider: "codex",
      kind: "session_id",
      id: "sess-retry-story",
    },
    work_items: [workItem],
  });
};

export const failedOutcomeRuntimeOverlay: DashboardRuntimeOverlay = (runtime, topology) => {
  const nodeID = firstExistingNodeID(topology, "repair");
  const workstation = topology.workstation_nodes_by_id[nodeID];
  const workItem = namedWorkItem("failed-story");
  const dispatchID = `dispatch-${nodeID}-failed`;
  const failureMessage = "Provider rate limit exceeded while generating the repair.";
  const failureReason = "provider_rate_limit";

  return appendProviderSession(
    {
      ...runtime,
      session: {
        ...runtime.session,
        failed_count: runtime.session.failed_count + 1,
        failed_by_work_type: {
          ...(runtime.session.failed_by_work_type ?? {}),
          story: (runtime.session.failed_by_work_type?.story ?? 0) + 1,
        },
        failed_work_details_by_work_id: {
          ...(runtime.session.failed_work_details_by_work_id ?? {}),
          [workItem.work_id]: {
            dispatch_id: dispatchID,
            failure_message: failureMessage,
            failure_reason: failureReason,
            transition_id: workstation?.transition_id ?? nodeID,
            workstation_name: workstation?.workstation_name,
            work_item: workItem,
          },
        },
        failed_work_labels: [...(runtime.session.failed_work_labels ?? []), "Failed Story"],
      },
    },
    {
      dispatch_id: dispatchID,
      failure_message: failureMessage,
      failure_reason: failureReason,
      transition_id: workstation?.transition_id ?? nodeID,
      workstation_name: workstation?.workstation_name,
      outcome: "FAILED",
      provider_session: {
        provider: "codex",
        kind: "session_id",
        id: "sess-failed-story",
      },
      work_items: [workItem],
    },
  );
};

export const rejectedOutcomeRuntimeOverlay: DashboardRuntimeOverlay = (runtime, topology) => {
  const nodeID = firstExistingNodeID(topology, "review");
  const workstation = topology.workstation_nodes_by_id[nodeID];
  const workItem = namedWorkItem("rejected-story");

  return appendProviderSession(runtime, {
    dispatch_id: `dispatch-${nodeID}-rejected`,
    transition_id: workstation?.transition_id ?? nodeID,
    workstation_name: workstation?.workstation_name,
    outcome: "REJECTED",
    provider_session: {
      provider: "codex",
      kind: "session_id",
      id: "sess-rejected-story",
    },
    work_items: [workItem],
  });
};

export const dashboardRuntimeOverlays = {
  activeWork: activeWorkRuntimeOverlay,
  retryAttempt: retryAttemptRuntimeOverlay,
  failedOutcome: failedOutcomeRuntimeOverlay,
  rejectedOutcome: rejectedOutcomeRuntimeOverlay,
} satisfies Record<string, DashboardRuntimeOverlay>;

export function buildDashboardSnapshotFixture(
  topology: DashboardTopology = mediumBranchingDashboardTopology,
  overlays: DashboardRuntimeOverlay[] = [],
): DashboardSnapshot {
  return {
    factory: factoryFromDashboardTopology(topology),
    factory_state: overlays.length > 0 ? "RUNNING" : "IDLE",
    uptime_seconds: 61,
    tick_count: 42,
    topology,
    runtime: overlays.reduce(
      (runtime, overlay) => overlay(runtime, topology),
      buildEmptyDashboardRuntimeFixture(),
    ),
  };
}

export const dashboardSemanticSnapshotFixtures = {
  activeWork: buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    activeWorkRuntimeOverlay,
  ]),
  retryAttempt: buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    retryAttemptRuntimeOverlay,
  ]),
  failedOutcome: buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    failedOutcomeRuntimeOverlay,
  ]),
  rejectedOutcome: buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    rejectedOutcomeRuntimeOverlay,
  ]),
} satisfies Record<string, DashboardSnapshot>;
