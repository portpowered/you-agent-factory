import type { FactoryDefinition } from "@you-agent-factory/client";

import type {
  FactoryActiveResourceClaimsEvidence,
  FactoryLoadProjection,
  FactoryLoadProjectionInput,
  FactoryLoadProjectionIssue,
  FactoryResourceOccupancyProjection,
  FactoryWorkStateCountProjection,
  FactoryWorkStateOccupancyEvidence,
} from "./load-contract.js";
import {
  factoryTopologyEntityId,
  factoryTopologyNodeId,
} from "./topology-identity.js";

const SYSTEM_WORK_TYPE_ID = "__system_time";

interface IndexedResource {
  capacity?: number;
  id: string;
  name: string;
}

interface IndexedWorkState {
  id: string;
  name: string;
  nodeId: string;
  workTypeId: string;
}

interface FactoryLoadIndex {
  resources: IndexedResource[];
  resourcesByName: Map<string, IndexedResource>;
  states: IndexedWorkState[];
  statesByWorkType: Map<string, Map<string, IndexedWorkState>>;
}

/** Project selected-tick Work State counts and resource occupancy evidence. */
export function projectFactoryLoad(
  input: FactoryLoadProjectionInput,
): FactoryLoadProjection {
  if (!input.factory) {
    return {
      issues: [
        {
          code: "MISSING_FACTORY",
          id: "missing-factory",
          message:
            "No Factory load topology is available at the selected tick.",
        },
      ],
      resourceOccupancy: [],
      selectedTick: input.selectedTick,
      workStateCounts: [],
    };
  }

  const issues = new Map<string, FactoryLoadProjectionIssue>();
  const index = indexFactory(input.factory);
  const workStateCounts = projectWorkStateCounts(input.works, index, issues);
  const resourceOccupancy = projectResourceOccupancy(
    input.activeDispatches,
    index,
    issues,
  );
  return {
    issues: [...issues.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    resourceOccupancy,
    selectedTick: input.selectedTick,
    workStateCounts,
  };
}

function indexFactory(factory: FactoryDefinition): FactoryLoadIndex {
  const resources = [...(factory.resources ?? [])]
    .map((resource) => {
      const capacity =
        Number.isFinite(resource.capacity) && resource.capacity >= 0
          ? resource.capacity
          : undefined;
      return {
        ...(capacity === undefined ? {} : { capacity }),
        id: factoryTopologyEntityId(resource.id, resource.name),
        name: resource.name,
      };
    })
    .sort((left, right) => left.id.localeCompare(right.id));
  const resourcesByName = new Map(
    resources.map((resource) => [resource.name, resource]),
  );
  const states: IndexedWorkState[] = [];
  const statesByWorkType = new Map<string, Map<string, IndexedWorkState>>();
  for (const workType of factory.workTypes ?? []) {
    const workTypeId = factoryTopologyEntityId(workType.id, workType.name);
    const statesByReference = new Map<string, IndexedWorkState>();
    for (const state of workType.states) {
      const stateId = `${workTypeId}:${factoryTopologyEntityId(state.id, state.name)}`;
      const indexed = {
        id: stateId,
        name: state.name,
        nodeId: factoryTopologyNodeId("work-state", stateId),
        workTypeId,
      };
      states.push(indexed);
      statesByReference.set(state.name, indexed);
      if (state.id?.trim()) statesByReference.set(state.id, indexed);
    }
    statesByWorkType.set(workType.name, statesByReference);
    if (workType.id?.trim()) {
      statesByWorkType.set(workType.id, statesByReference);
    }
  }
  states.sort((left, right) => left.nodeId.localeCompare(right.nodeId));
  return { resources, resourcesByName, states, statesByWorkType };
}

function projectWorkStateCounts(
  works: readonly FactoryWorkStateOccupancyEvidence[] | undefined,
  index: FactoryLoadIndex,
  issues: Map<string, FactoryLoadProjectionIssue>,
): FactoryWorkStateCountProjection[] {
  if (!works) {
    return index.states.map((state) => ({
      evidence: "unavailable",
      workStateId: state.id,
      workStateNodeId: state.nodeId,
      workTypeId: state.workTypeId,
    }));
  }

  const workById = groupWorkEvidence(works);
  const workIdsByState = new Map<string, Set<string>>();
  for (const [workId, evidence] of workById) {
    if (evidence.every((work) => work.workTypeId === SYSTEM_WORK_TYPE_ID)) {
      continue;
    }
    const resolved = evidence.map((work) => resolveWorkState(work, index));
    if (resolved.some((state) => !state)) {
      const work = evidence.find((item) => !resolveWorkState(item, index));
      addIssue(issues, {
        code: "UNRESOLVED_WORK_STATE",
        id: `work:${workId}:unresolved-state`,
        message: `Work ${workId} does not identify a known Work State.`,
        reference: [work?.workTypeId, work?.stateId ?? work?.stateName]
          .filter(Boolean)
          .join(":"),
        workId,
      });
      continue;
    }
    const statesById = new Map(
      resolved.flatMap((state) => (state ? [[state.nodeId, state]] : [])),
    );
    if (statesById.size > 1) {
      addIssue(issues, {
        code: "CONTRADICTORY_WORK_STATE",
        id: `work:${workId}:contradictory-state`,
        message: `Work ${workId} has contradictory selected-tick Work State evidence.`,
        workId,
      });
      continue;
    }
    const state = statesById.values().next().value;
    if (!state) continue;
    const workIds = workIdsByState.get(state.nodeId) ?? new Set<string>();
    workIds.add(workId);
    workIdsByState.set(state.nodeId, workIds);
  }
  return index.states.map((state) => {
    const workIds = [...(workIdsByState.get(state.nodeId) ?? [])].sort();
    return {
      count: workIds.length,
      evidence: "known",
      workIds,
      workStateId: state.id,
      workStateNodeId: state.nodeId,
      workTypeId: state.workTypeId,
    };
  });
}

function groupWorkEvidence(
  works: readonly FactoryWorkStateOccupancyEvidence[],
): Map<string, FactoryWorkStateOccupancyEvidence[]> {
  const grouped = new Map<string, FactoryWorkStateOccupancyEvidence[]>();
  for (const work of works) {
    if (!work.id) continue;
    const evidence = grouped.get(work.id) ?? [];
    evidence.push(structuredClone(work));
    grouped.set(work.id, evidence);
  }
  return grouped;
}

function resolveWorkState(
  work: FactoryWorkStateOccupancyEvidence,
  index: FactoryLoadIndex,
): IndexedWorkState | undefined {
  if (!work.workTypeId) return undefined;
  const states = index.statesByWorkType.get(work.workTypeId);
  return work.stateId
    ? states?.get(work.stateId)
    : work.stateName
      ? states?.get(work.stateName)
      : undefined;
}

function projectResourceOccupancy(
  dispatches: readonly FactoryActiveResourceClaimsEvidence[] | undefined,
  index: FactoryLoadIndex,
  issues: Map<string, FactoryLoadProjectionIssue>,
): FactoryResourceOccupancyProjection[] {
  const occupiedByResource = new Map<string, number>();
  const unavailableResources = new Set<string>();
  const normalizedDispatches = normalizeDispatchEvidence(dispatches, issues);
  const evidenceAvailable = normalizedDispatches?.every(
    (dispatch) => dispatch.resourceClaims !== undefined,
  );
  for (const dispatch of normalizedDispatches ?? []) {
    for (const claim of dispatch.resourceClaims ?? []) {
      const resource = index.resourcesByName.get(claim.resourceName);
      if (!resource) {
        addIssue(issues, {
          code: "UNRESOLVED_RESOURCE_CLAIM",
          dispatchId: dispatch.id,
          id: `dispatch:${dispatch.id}:resource:${claim.resourceName}`,
          message: `Dispatch ${dispatch.id} claims unknown resource ${claim.resourceName}.`,
          reference: claim.resourceName,
        });
        continue;
      }
      const quantity = claim.quantity ?? 1;
      if (!Number.isFinite(quantity) || quantity <= 0) {
        unavailableResources.add(resource.id);
        addIssue(issues, {
          code: "CONTRADICTORY_RESOURCE_CLAIM",
          dispatchId: dispatch.id,
          id: `dispatch:${dispatch.id}:resource:${resource.id}:invalid-quantity`,
          message: `Dispatch ${dispatch.id} has an invalid claim quantity for resource ${resource.id}.`,
          resourceId: resource.id,
        });
        continue;
      }
      occupiedByResource.set(
        resource.id,
        (occupiedByResource.get(resource.id) ?? 0) + quantity,
      );
    }
  }
  return index.resources.map((resource) =>
    projectOneResource(
      resource,
      occupiedByResource.get(resource.id) ?? 0,
      evidenceAvailable === true && !unavailableResources.has(resource.id),
      issues,
    ),
  );
}

function normalizeDispatchEvidence(
  dispatches: readonly FactoryActiveResourceClaimsEvidence[] | undefined,
  issues: Map<string, FactoryLoadProjectionIssue>,
): FactoryActiveResourceClaimsEvidence[] | undefined {
  if (!dispatches) return undefined;
  const grouped = new Map<string, FactoryActiveResourceClaimsEvidence[]>();
  for (const dispatch of dispatches) {
    const evidence = grouped.get(dispatch.id) ?? [];
    evidence.push(dispatch);
    grouped.set(dispatch.id, evidence);
  }
  const normalized: FactoryActiveResourceClaimsEvidence[] = [];
  for (const [dispatchId, evidence] of grouped) {
    const distinct = new Map(
      evidence.map((item) => [resourceClaimsKey(item.resourceClaims), item]),
    );
    if (distinct.size > 1) {
      addIssue(issues, {
        code: "CONTRADICTORY_RESOURCE_CLAIM",
        dispatchId,
        id: `dispatch:${dispatchId}:contradictory-resource-claims`,
        message: `Dispatch ${dispatchId} has contradictory selected-tick resource claims.`,
      });
      normalized.push({ id: dispatchId });
      continue;
    }
    const only = distinct.values().next().value;
    if (only) normalized.push(only);
  }
  return normalized;
}

function resourceClaimsKey(
  claims: readonly { quantity?: number; resourceName: string }[] | undefined,
): string {
  if (!claims) return "unavailable";
  return [...claims]
    .map((claim) => `${claim.resourceName}\u0000${claim.quantity ?? 1}`)
    .sort()
    .join("\u0001");
}

function projectOneResource(
  resource: IndexedResource,
  occupiedQuantity: number,
  occupancyAvailable: boolean,
  issues: Map<string, FactoryLoadProjectionIssue>,
): FactoryResourceOccupancyProjection {
  const base = {
    ...(resource.capacity === undefined ? {} : { capacity: resource.capacity }),
    capacityEvidence:
      resource.capacity === undefined
        ? ("unavailable" as const)
        : ("known" as const),
    resourceId: resource.id,
    resourceNodeId: factoryTopologyNodeId("resource", resource.id),
  };
  if (resource.capacity === undefined) {
    addIssue(issues, {
      code: "INVALID_RESOURCE_CAPACITY",
      id: `resource:${resource.id}:invalid-capacity`,
      message: `Resource ${resource.id} does not have a valid canonical capacity.`,
      resourceId: resource.id,
    });
  }
  if (!occupancyAvailable) return { ...base, evidence: "unavailable" };
  if (resource.capacity !== undefined && occupiedQuantity > resource.capacity) {
    addIssue(issues, {
      code: "RESOURCE_CAPACITY_EXCEEDED",
      id: `resource:${resource.id}:capacity-exceeded`,
      message: `Resource ${resource.id} has ${occupiedQuantity} occupied units for capacity ${resource.capacity}.`,
      resourceId: resource.id,
    });
  }
  return {
    ...base,
    ...(resource.capacity === undefined
      ? {}
      : {
          availableQuantity: Math.max(0, resource.capacity - occupiedQuantity),
        }),
    evidence: "known",
    occupiedQuantity,
  };
}

function addIssue(
  issues: Map<string, FactoryLoadProjectionIssue>,
  issue: FactoryLoadProjectionIssue,
): void {
  issues.set(issue.id, issue);
}
