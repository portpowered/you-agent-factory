import {
  type FactoryActivityProjection,
  type FactoryLoadProjection,
  type FactoryWorkStateOccupancyEvidence,
  type FactoryTopologyProjection,
  type FactoryWorkProgressProjection,
  type FactoryWorkProgressStateEvidence,
  projectFactoryActivity,
  projectFactoryLoad,
  projectFactoryTopology,
  projectFactoryWorkProgress,
} from "../../../../../../packages/factory-replay/src/index.js";
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
  load: FactoryLoadProjection;
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
        inputRoutes: dispatch.workItems.map((work) =>
          dispatchInputRoute(state, dispatch, work),
        ),
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
    load: projectFactoryLoad({
      activeDispatches: activeDispatches.map((dispatch) => ({
        id: dispatch.dispatchID,
        ...(dispatch.resourceEvidenceAvailable === false
          ? {}
          : {
              resourceClaims: dispatch.resources.map((resource) => ({
                resourceName: resource.resourceID,
              })),
            }),
      })),
      factory: state.factory,
      selectedTick: state.tick_count,
      works: hostedWorkStateEvidence(state, activeDispatches),
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
    load: projectFactoryLoad({ selectedTick, works: [] }),
    topology: projectFactoryTopology({ selectedTick }),
    workProgress: projectFactoryWorkProgress({
      activeWorkIds: [],
      selectedTick,
      works: [],
    }),
  };
}

function hostedWorkStateEvidence(
  state: ReplayWorldState,
  activeDispatches: WorldDispatch[],
): FactoryWorkStateOccupancyEvidence[] {
  const activeWorkIDs = new Set(activeDispatches.flatMap(dispatchWorkIDs));
  return Object.values(state.workItemsByID).flatMap((work) => {
    if (activeWorkIDs.has(work.id)) {
      return [];
    }
    const place = work.place_id
      ? state.topology.places?.find(
          (candidate) => candidate.id === work.place_id,
        )
      : undefined;
    const stateName = work.state || place?.state;
    return [
      {
        id: work.id,
        ...(stateName ? { stateName } : {}),
        workTypeId: work.work_type_id,
      },
    ];
  });
}

function dispatchInputRoute(
  state: ReplayWorldState,
  dispatch: WorldDispatch,
  work: WorldDispatch["workItems"][number],
) {
  const consumedToken = dispatch.consumedTokens.find(
    (token) => token.work_id === work.work_id,
  );
  const place = consumedToken
    ? state.topology.places?.find(
        (candidate) => candidate.id === consumedToken.place_id,
      )
    : undefined;
  return {
    ...(work.state || place?.state
      ? { stateName: work.state || place?.state }
      : {}),
    ...(work.work_type_id ? { workTypeId: work.work_type_id } : {}),
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
    active_dispatch_ids: projection.activity.activeDispatchOverlays.map(
      (dispatch) => dispatch.dispatchId,
    ),
    active_executions_by_dispatch_id: activeExecutions,
    active_workstation_node_ids: projectHostedActiveWorkstationIDs(
      state,
      projection,
    ),
    in_flight_dispatch_count: projection.activity.activeDispatchOverlays.length,
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
    projection.activity.activeDispatchOverlays.flatMap((dispatch) => {
      const execution =
        runtime.active_executions_by_dispatch_id?.[dispatch.dispatchId];
      return execution
        ? [
            [
              dispatch.dispatchId,
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
    projection.activity.activeDispatchOverlays.flatMap((dispatch) => {
      const workstationID = dispatch.workstationId;
      if (!workstationID) {
        return [];
      }
      const executions = Object.values(activeExecutions).filter(
        (execution) => execution.workstation_node_id === workstationID,
      );
      return [
        [
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
        ],
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
      ...projection.activity.activeDispatchOverlays.flatMap((dispatch) =>
        dispatch.workstationId ? [dispatch.workstationId] : [],
      ),
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
