import {
  type FactoryActivityProjection,
  type FactoryTopologyProjection,
  type FactoryWorkProgressProjection,
  type FactoryWorkProgressStateEvidence,
  projectFactoryActivity,
  projectFactoryTopology,
  projectFactoryWorkProgress,
} from "../../../../../../../packages/factory-replay/src/index.js";
import type {
  DashboardRuntime,
  DashboardSnapshot,
} from "../../../../../api/dashboard";
import type { FactoryWorkItem } from "../../../../../api/events";
import { projectRuntime } from "../projectRuntime";
import { projectTopology } from "../projectTopology";
import type { ReplayWorldState, WorldDispatch } from "../types";

export interface HostedFactoryReplayProjection {
  activity: FactoryActivityProjection;
  topology: FactoryTopologyProjection;
  workProgress: FactoryWorkProgressProjection;
}

interface HostedDashboardProjection {
  factoryReplay: HostedFactoryReplayProjection;
  runtime: DashboardRuntime;
  topology: DashboardSnapshot["topology"];
}

export function projectHostedDashboard(
  state: ReplayWorldState,
): HostedDashboardProjection {
  const factoryReplay = projectHostedFactoryReplay(state);
  return {
    factoryReplay,
    runtime: projectHostedRuntime(state, factoryReplay),
    topology: projectTopology(state.topology, factoryReplay.topology),
  };
}

export function projectHostedFactoryReplay(
  state: ReplayWorldState,
): HostedFactoryReplayProjection {
  const activeDispatches = Object.values(state.activeDispatches).filter(
    (dispatch) => !dispatch.systemOnly,
  );

  return {
    activity: projectFactoryActivity({
      activeDispatches: activeDispatches.map((dispatch) => ({
        id: dispatch.dispatchID,
        ...(dispatch.resourceEvidenceAvailable === false
          ? {}
          : {
              resourceNames: dispatch.resources.map(
                (resource) => resource.resourceID,
              ),
            }),
        startedTick: dispatch.startedTick ?? state.tick_count,
        transitionId: dispatch.transitionID,
        workIds: dispatchWorkIDs(dispatch),
      })),
      factory: state.factory,
      selectedTick: state.tick_count,
    }),
    topology: projectFactoryTopology({
      factory: state.factory,
      selectedTick: state.tick_count,
    }),
    workProgress: projectFactoryWorkProgress({
      activeWorkIds: activeDispatches.flatMap(dispatchWorkIDs),
      factory: state.factory,
      selectedTick: state.tick_count,
      works: Object.values(state.workItemsByID).map((work) => ({
        id: work.id,
        state: workProgressState(state, work),
        workTypeId: work.work_type_id || undefined,
      })),
    }),
  };
}

export function emptyHostedFactoryReplayProjection(
  selectedTick: number,
): HostedFactoryReplayProjection {
  return {
    activity: projectFactoryActivity({
      activeDispatches: [],
      selectedTick,
    }),
    topology: projectFactoryTopology({ selectedTick }),
    workProgress: projectFactoryWorkProgress({
      activeWorkIds: [],
      selectedTick,
      works: [],
    }),
  };
}

function projectHostedRuntime(
  state: ReplayWorldState,
  projection: HostedFactoryReplayProjection,
): DashboardRuntime {
  const runtime = projectRuntime(state);
  const activeExecutions = projectHostedActiveExecutions(runtime, projection);
  const workstationActivity = projectHostedWorkstationActivity(
    activeExecutions,
    projection,
  );

  return {
    ...runtime,
    active_dispatch_ids: projection.activity.activeDispatches.map(
      (dispatch) => dispatch.id,
    ),
    active_executions_by_dispatch_id: activeExecutions,
    active_workstation_node_ids: projectHostedActiveWorkstationIDs(
      state,
      projection,
    ),
    in_flight_dispatch_count: projection.activity.activeDispatches.length,
    place_token_counts: projectHostedPlaceTokenCounts(
      state,
      runtime.place_token_counts ?? {},
      projection,
    ),
    session: {
      ...runtime.session,
      completed_count: projection.workProgress.counts.completed,
      failed_count: projection.workProgress.counts.failed,
    },
    workstation_activity_by_node_id: workstationActivity,
  };
}

function projectHostedActiveExecutions(
  runtime: DashboardRuntime,
  projection: HostedFactoryReplayProjection,
): NonNullable<DashboardRuntime["active_executions_by_dispatch_id"]> {
  return Object.fromEntries(
    projection.activity.activeDispatches.flatMap((dispatch) => {
      const execution = runtime.active_executions_by_dispatch_id?.[dispatch.id];
      return execution
        ? [
            [
              dispatch.id,
              {
                ...execution,
                workstation_node_id:
                  dispatch.workstationId ?? execution.workstation_node_id,
              },
            ] as const,
          ]
        : [];
    }),
  );
}

function projectHostedWorkstationActivity(
  activeExecutions: NonNullable<
    DashboardRuntime["active_executions_by_dispatch_id"]
  >,
  projection: HostedFactoryReplayProjection,
): NonNullable<DashboardRuntime["workstation_activity_by_node_id"]> {
  return Object.fromEntries(
    projection.activity.activeWorkstationIds.map((workstationID) => {
      const executions = Object.values(activeExecutions).filter(
        (execution) => execution.workstation_node_id === workstationID,
      );
      return [
        workstationID,
        {
          active_dispatch_ids: executions.map(
            (execution) => execution.dispatch_id,
          ),
          active_work_items: executions.flatMap(
            (execution) => execution.work_items ?? [],
          ),
          trace_ids: [
            ...new Set(
              executions.flatMap((execution) => execution.trace_ids ?? []),
            ),
          ],
          workstation_node_id: workstationID,
        },
      ] satisfies [
        string,
        NonNullable<
          DashboardRuntime["workstation_activity_by_node_id"]
        >[string],
      ];
    }),
  );
}

function projectHostedActiveWorkstationIDs(
  state: ReplayWorldState,
  projection: HostedFactoryReplayProjection,
): string[] {
  return [
    ...new Set([
      ...projection.activity.activeWorkstationIds,
      ...Object.values(state.activeDispatches).flatMap((dispatch) =>
        dispatch.systemOnly ? [dispatch.transitionID] : [],
      ),
    ]),
  ].sort();
}

function projectHostedPlaceTokenCounts(
  state: ReplayWorldState,
  legacyCounts: Record<string, number>,
  projection: HostedFactoryReplayProjection,
): Record<string, number> {
  const counts = { ...legacyCounts };
  const resourceLabelsByID = new Map(
    projection.topology.nodes.flatMap((node) =>
      node.kind === "resource" ? [[node.entityId, node.label] as const] : [],
    ),
  );

  for (const occupancy of projection.activity.resourceOccupancy) {
    if (occupancy.evidence !== "known") {
      continue;
    }
    const resourceLabel = resourceLabelsByID.get(occupancy.resourceId);
    const resource = state.topology.resources?.find(
      (candidate) =>
        candidate.id === occupancy.resourceId ||
        candidate.id === resourceLabel ||
        candidate.name === resourceLabel,
    );
    const resourcePlace = state.topology.places?.find(
      (place) => place.type_id === resource?.id,
    );
    if (
      resourcePlace &&
      occupancy.availableQuantity !== undefined &&
      (resourcePlace.id in counts || occupancy.occupiedQuantity !== 0)
    ) {
      counts[resourcePlace.id] = occupancy.availableQuantity;
    }
  }
  return counts;
}

function dispatchWorkIDs(dispatch: WorldDispatch): string[] {
  return [
    ...dispatch.workItems.map((work) => work.work_id),
    ...dispatch.consumedTokens.flatMap((token) =>
      token.work_id ? [token.work_id] : [],
    ),
  ];
}

function workProgressState(
  state: ReplayWorldState,
  work: FactoryWorkItem,
): FactoryWorkProgressStateEvidence | undefined {
  if (state.failedWorkItemsByID[work.id]) {
    return { category: "FAILED" };
  }
  if (state.terminalWorkByID[work.id]) {
    return { category: "TERMINAL" };
  }

  const place = work.place_id
    ? state.topology.places?.find((candidate) => candidate.id === work.place_id)
    : undefined;
  const name = work.state || place?.state;
  if (name) {
    return { name };
  }

  const category = progressStateCategory(place?.category);
  return category ? { category } : undefined;
}

function progressStateCategory(
  category: string | undefined,
): FactoryWorkProgressStateEvidence["category"] | undefined {
  switch (category) {
    case "FAILED":
    case "INITIAL":
    case "PROCESSING":
    case "TERMINAL":
      return category;
    default:
      return undefined;
  }
}
