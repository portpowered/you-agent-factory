import {
  type FactoryActivityProjection,
  type FactoryTopologyProjection,
  type FactoryWorkProgressProjection,
  type FactoryWorkProgressStateEvidence,
  projectFactoryActivity,
  projectFactoryTopology,
  projectFactoryWorkProgress,
} from "../../../../../../../packages/factory-replay/src/index.js";
import type { FactoryWorkItem } from "../../../../../api/events";
import type { ReplayWorldState, WorldDispatch } from "../types";

export interface HostedFactoryReplayProjection {
  activity: FactoryActivityProjection;
  topology: FactoryTopologyProjection;
  workProgress: FactoryWorkProgressProjection;
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
