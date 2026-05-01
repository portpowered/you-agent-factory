import type {
  DashboardRuntime,
  DashboardSessionRuntime,
  DashboardSnapshot,
  DashboardTopology,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";

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
