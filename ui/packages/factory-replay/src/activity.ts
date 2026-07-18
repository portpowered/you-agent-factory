import type { FactoryDefinition } from "@you-agent-factory/client";

import type {
  FactoryActiveDispatchEvidence,
  FactoryActivityProjection,
  FactoryActivityProjectionInput,
  FactoryActivityProjectionIssue,
  FactoryDispatchOverlayProjection,
  FactoryDispatchRouteEvidence,
} from "./activity-contract.js";
import { projectFactoryLoad } from "./load.js";
import { projectFactoryTopology } from "./topology.js";
import type {
  FactoryTopologyConnection,
  FactoryTopologyProjection,
} from "./topology-contract.js";
import {
  factoryTopologyEntityId,
  factoryTopologyNodeId,
} from "./topology-identity.js";

type Workstation = NonNullable<FactoryDefinition["workstations"]>[number];

interface ActivityIndex {
  resourcesByName: Map<string, { id: string; nodeId: string }>;
  topology: FactoryTopologyProjection;
  workersByName: Map<string, { id: string; nodeId: string }>;
  workstationsByTransition: Map<
    string,
    { id: string; nodeId: string; workstation: Workstation }
  >;
  workStates: Map<string, string>;
}

function sortedByIdentity<T extends { id?: string; name: string }>(
  values: readonly T[] | undefined,
): T[] {
  return [...(values ?? [])].sort((left, right) => {
    const identityDifference = factoryTopologyEntityId(
      left.id,
      left.name,
    ).localeCompare(factoryTopologyEntityId(right.id, right.name));
    return identityDifference || left.name.localeCompare(right.name);
  });
}

/** Build the stable public identity of one active Dispatch overlay. */
export function factoryDispatchOverlayId(dispatchId: string): string {
  return `dispatch:${dispatchId}`;
}

/** Build the stable public identity used when an overlay references Work. */
export function factoryWorkProjectionId(workId: string): string {
  return `work:${workId}`;
}

/** Project selected-tick active Dispatch overlays and their topology activity. */
export function projectFactoryActivity(
  input: FactoryActivityProjectionInput,
): FactoryActivityProjection {
  const issues = new Map<string, FactoryActivityProjectionIssue>();
  const index = indexFactory(input.factory, input.selectedTick);
  const dispatches = normalizeDispatches(input.activeDispatches, issues);
  const activeDispatchOverlays = dispatches.map((dispatch) =>
    projectOverlay(dispatch, index, issues),
  );
  const load = projectFactoryLoad({
    activeDispatches: dispatches.map((dispatch) => ({
      id: dispatch.id,
      ...(dispatch.resourceNames
        ? {
            resourceClaims: dispatch.resourceNames.map((resourceName) => ({
              resourceName,
            })),
          }
        : {}),
    })),
    factory: input.factory,
    selectedTick: input.selectedTick,
    works: [],
  });
  appendLoadIssues(load.issues, issues);
  return {
    activeDispatchOverlays: activeDispatchOverlays.sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    activeWorkstationNodeIds: [
      ...new Set(
        activeDispatchOverlays.flatMap((overlay) =>
          overlay.workstationNodeId ? [overlay.workstationNodeId] : [],
        ),
      ),
    ].sort(),
    issues: [...issues.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    resourceOccupancy: load.resourceOccupancy,
    selectedTick: input.selectedTick,
  };
}

function indexFactory(
  factory: FactoryDefinition | undefined,
  selectedTick: number,
): ActivityIndex {
  const topology = projectFactoryTopology({ factory, selectedTick });
  const resourcesByName = new Map<string, { id: string; nodeId: string }>();
  const workersByName = new Map<string, { id: string; nodeId: string }>();
  const workstationsByTransition = new Map<
    string,
    { id: string; nodeId: string; workstation: Workstation }
  >();
  const workStates = new Map<string, string>();
  for (const resource of sortedByIdentity(factory?.resources)) {
    const id = factoryTopologyEntityId(resource.id, resource.name);
    resourcesByName.set(resource.name, {
      id,
      nodeId: factoryTopologyNodeId("resource", id),
    });
  }
  for (const worker of sortedByIdentity(factory?.workers)) {
    const id = factoryTopologyEntityId(worker.id, worker.name);
    const value = { id, nodeId: factoryTopologyNodeId("worker", id) };
    workersByName.set(worker.name, value);
    workersByName.set(id, value);
  }
  for (const workType of sortedByIdentity(factory?.workTypes)) {
    const workTypeId = factoryTopologyEntityId(workType.id, workType.name);
    for (const state of sortedByIdentity(workType.states)) {
      const stateId = `${workTypeId}:${factoryTopologyEntityId(state.id, state.name)}`;
      const nodeId = factoryTopologyNodeId("work-state", stateId);
      for (const typeReference of [workType.name, workTypeId]) {
        for (const stateReference of [state.name, state.id?.trim()]) {
          if (stateReference) {
            workStates.set(`${typeReference}\u0000${stateReference}`, nodeId);
          }
        }
      }
    }
  }
  for (const workstation of sortedByIdentity(factory?.workstations)) {
    const id = factoryTopologyEntityId(workstation.id, workstation.name);
    const value = {
      id,
      nodeId: factoryTopologyNodeId("workstation", id),
      workstation,
    };
    workstationsByTransition.set(workstation.name, value);
    workstationsByTransition.set(id, value);
  }
  return {
    resourcesByName,
    topology,
    workersByName,
    workstationsByTransition,
    workStates,
  };
}

function projectOverlay(
  dispatch: FactoryActiveDispatchEvidence,
  index: ActivityIndex,
  issues: Map<string, FactoryActivityProjectionIssue>,
): FactoryDispatchOverlayProjection {
  const resolved = dispatch.transitionId
    ? index.workstationsByTransition.get(dispatch.transitionId)
    : undefined;
  if (!resolved && dispatch.transitionId) {
    addIssue(issues, {
      code: "UNRESOLVED_WORKSTATION",
      dispatchId: dispatch.id,
      id: `dispatch:${dispatch.id}:workstation:${dispatch.transitionId}`,
      message: `Dispatch ${dispatch.id} refers to unknown workstation transition ${dispatch.transitionId}.`,
      reference: dispatch.transitionId,
    });
  }
  const workerName = resolved?.workstation.worker?.trim();
  const worker = workerName ? index.workersByName.get(workerName) : undefined;
  if (workerName && !worker) {
    addIssue(issues, {
      code: "UNRESOLVED_WORKER",
      dispatchId: dispatch.id,
      id: `dispatch:${dispatch.id}:worker:${workerName}`,
      message: `Dispatch ${dispatch.id} refers to unknown worker ${workerName}.`,
      reference: workerName,
    });
  }
  const resources = resolveResources(dispatch, index, issues);
  const routeNodeIds = resolveRouteNodeIds(dispatch, index, issues);
  const connectionIds = resolveConnections(
    dispatch,
    index.topology,
    resolved?.nodeId,
    worker?.nodeId,
    resources?.map((resource) => resource.nodeId),
    routeNodeIds,
    issues,
  );
  const workIds = dispatch.workIds
    ? [...new Set(dispatch.workIds)].sort()
    : undefined;
  return {
    connectionIds,
    dispatchId: dispatch.id,
    evidence: {
      resources: dispatch.resourceNames ? "known" : "unavailable",
      route: dispatch.inputRoutes ? "known" : "unavailable",
      work: dispatch.workIds ? "known" : "unavailable",
      worker: worker ? "known" : "unavailable",
      workstation: resolved ? "known" : "unavailable",
    },
    id: factoryDispatchOverlayId(dispatch.id),
    ...(resources
      ? {
          resourceIds: resources.map((resource) => resource.id).sort(),
          resourceNodeIds: resources.map((resource) => resource.nodeId).sort(),
        }
      : {}),
    startedTick: dispatch.startedTick,
    ...(dispatch.transitionId ? { transitionId: dispatch.transitionId } : {}),
    ...(worker ? { workerId: worker.id, workerNodeId: worker.nodeId } : {}),
    ...(workIds
      ? {
          workIds,
          workProjectionIds: workIds.map(factoryWorkProjectionId),
        }
      : {}),
    ...(resolved
      ? {
          workstationId: resolved.id,
          workstationNodeId: resolved.nodeId,
        }
      : {}),
  };
}

function resolveResources(
  dispatch: FactoryActiveDispatchEvidence,
  index: ActivityIndex,
  issues: Map<string, FactoryActivityProjectionIssue>,
): Array<{ id: string; nodeId: string }> | undefined {
  if (!dispatch.resourceNames) return undefined;
  const resources = new Map<string, { id: string; nodeId: string }>();
  for (const resourceName of dispatch.resourceNames) {
    const resource = index.resourcesByName.get(resourceName);
    if (resource) {
      resources.set(resource.id, resource);
      continue;
    }
    addIssue(issues, {
      code: "UNRESOLVED_RESOURCE",
      dispatchId: dispatch.id,
      id: `dispatch:${dispatch.id}:resource:${resourceName}`,
      message: `Dispatch ${dispatch.id} refers to unknown resource ${resourceName}.`,
      reference: resourceName,
    });
  }
  return [...resources.values()];
}

function resolveRouteNodeIds(
  dispatch: FactoryActiveDispatchEvidence,
  index: ActivityIndex,
  issues: Map<string, FactoryActivityProjectionIssue>,
): string[] | undefined {
  if (!dispatch.inputRoutes) return undefined;
  const nodeIds = new Set<string>();
  for (const route of dispatch.inputRoutes) {
    const nodeId = resolveRouteNodeId(route, index);
    if (nodeId) {
      nodeIds.add(nodeId);
      continue;
    }
    const reference = `${route.workTypeId ?? "?"}:${route.stateId ?? route.stateName ?? "?"}`;
    addIssue(issues, {
      code: "UNRESOLVED_ROUTE",
      dispatchId: dispatch.id,
      id: `dispatch:${dispatch.id}:route:${reference}`,
      message: `Dispatch ${dispatch.id} input route ${reference} is not present in the selected topology.`,
      reference,
    });
  }
  return [...nodeIds].sort();
}

function resolveRouteNodeId(
  route: FactoryDispatchRouteEvidence,
  index: ActivityIndex,
): string | undefined {
  if (!route.workTypeId) return undefined;
  for (const stateReference of [route.stateId, route.stateName]) {
    if (!stateReference) continue;
    const nodeId = index.workStates.get(
      `${route.workTypeId}\u0000${stateReference}`,
    );
    if (nodeId) return nodeId;
  }
  return undefined;
}

function resolveConnections(
  dispatch: FactoryActiveDispatchEvidence,
  topology: FactoryTopologyProjection,
  workstationNodeId: string | undefined,
  workerNodeId: string | undefined,
  resourceNodeIds: readonly string[] | undefined,
  routeNodeIds: readonly string[] | undefined,
  issues: Map<string, FactoryActivityProjectionIssue>,
): string[] {
  if (!workstationNodeId) return [];
  if (!topology.ok) {
    addIssue(issues, {
      code: "UNAVAILABLE_TOPOLOGY_PATH",
      dispatchId: dispatch.id,
      id: `dispatch:${dispatch.id}:topology-path-unavailable`,
      message: `Dispatch ${dispatch.id} topology paths are unavailable because the selected topology is invalid.`,
    });
    return [];
  }
  const resourceSet = resourceNodeIds && new Set(resourceNodeIds);
  const routeSet = routeNodeIds && new Set(routeNodeIds);
  return topology.connections
    .filter((connection) =>
      isRelevantConnection(
        connection,
        workstationNodeId,
        workerNodeId,
        resourceSet,
        routeSet,
      ),
    )
    .map((connection) => connection.id)
    .sort();
}

function isRelevantConnection(
  connection: FactoryTopologyConnection,
  workstationNodeId: string,
  workerNodeId: string | undefined,
  resourceNodeIds: ReadonlySet<string> | undefined,
  routeNodeIds: ReadonlySet<string> | undefined,
): boolean {
  if (
    connection.kind === "worker-assignment" &&
    workerNodeId &&
    connection.source.nodeId === workerNodeId &&
    connection.target.nodeId === workstationNodeId
  ) {
    return true;
  }
  if (
    connection.kind === "workstation-resource" &&
    resourceNodeIds?.has(connection.source.nodeId) &&
    connection.target.nodeId === workstationNodeId
  ) {
    return true;
  }
  return Boolean(
    connection.kind === "workstation-input" &&
      routeNodeIds?.has(connection.source.nodeId) &&
      connection.target.nodeId === workstationNodeId,
  );
}

function normalizeDispatches(
  dispatches: readonly FactoryActiveDispatchEvidence[],
  issues: Map<string, FactoryActivityProjectionIssue>,
): FactoryActiveDispatchEvidence[] {
  const grouped = new Map<string, FactoryActiveDispatchEvidence[]>();
  for (const dispatch of dispatches) {
    const values = grouped.get(dispatch.id) ?? [];
    values.push(dispatch);
    grouped.set(dispatch.id, values);
  }
  const normalized: FactoryActiveDispatchEvidence[] = [];
  for (const [dispatchId, values] of grouped) {
    const keyed = new Map(values.map((value) => [evidenceKey(value), value]));
    if (keyed.size > 1) {
      addIssue(issues, {
        code: "CONTRADICTORY_DISPATCH_EVIDENCE",
        dispatchId,
        id: `dispatch:${dispatchId}:contradictory-evidence`,
        message: `Dispatch ${dispatchId} has contradictory selected-tick activity evidence.`,
      });
    }
    normalized.push(
      [...keyed.values()].sort((left, right) =>
        evidenceKey(left).localeCompare(evidenceKey(right)),
      )[0] as FactoryActiveDispatchEvidence,
    );
  }
  return normalized.sort((left, right) => left.id.localeCompare(right.id));
}

function evidenceKey(dispatch: FactoryActiveDispatchEvidence): string {
  const sorted = (values: readonly string[] | undefined) =>
    values ? [...values].sort().join("\u0000") : "?";
  return [
    dispatch.startedTick,
    dispatch.transitionId ?? "?",
    sorted(dispatch.workIds),
    sorted(dispatch.resourceNames),
    dispatch.inputRoutes
      ? dispatch.inputRoutes
          .map(
            (route) =>
              `${route.workTypeId ?? "?"}\u0000${route.stateId ?? "?"}\u0000${route.stateName ?? "?"}`,
          )
          .sort()
          .join("\u0002")
      : "?",
  ].join("\u0001");
}

function appendLoadIssues(
  loadIssues: readonly {
    code: string;
    dispatchId?: string;
    id: string;
    message: string;
    reference?: string;
    resourceId?: string;
  }[],
  issues: Map<string, FactoryActivityProjectionIssue>,
): void {
  for (const issue of loadIssues) {
    const code =
      issue.code === "UNRESOLVED_RESOURCE_CLAIM"
        ? "UNRESOLVED_RESOURCE"
        : issue.code;
    if (
      code !== "INVALID_RESOURCE_CAPACITY" &&
      code !== "MISSING_FACTORY" &&
      code !== "RESOURCE_CAPACITY_EXCEEDED" &&
      code !== "UNRESOLVED_RESOURCE"
    ) {
      continue;
    }
    addIssue(issues, { ...issue, code });
  }
}

function addIssue(
  issues: Map<string, FactoryActivityProjectionIssue>,
  issue: FactoryActivityProjectionIssue,
): void {
  issues.set(issue.id, issue);
}
