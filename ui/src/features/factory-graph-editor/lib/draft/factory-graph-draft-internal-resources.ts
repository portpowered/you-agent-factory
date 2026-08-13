import type {
  CanonicalFactoryDefinition,
  FactoryWorkstation,
  FactoryWorkstationIO,
  FactoryWorkType,
} from "./factory-graph-draft-types";

const RESOURCE_AVAILABLE_STATE = "available";

export function collectFactoryGraphResourceNames(
  factoryDefinition: Pick<CanonicalFactoryDefinition, "resources">,
): Set<string> {
  return new Set(
    (factoryDefinition.resources ?? [])
      .map((resource) => resource.name.trim())
      .filter((name) => name.length > 0),
  );
}

export function isInternalResourceAvailabilityWorkType(
  workType: Pick<FactoryWorkType, "id" | "name" | "states">,
  resourceNames: ReadonlySet<string>,
): boolean {
  const identifier = workType.id?.trim() || workType.name.trim();
  return (
    resourceNames.has(identifier) &&
    workType.states.some(
      (state) => state.name.trim() === RESOURCE_AVAILABLE_STATE,
    )
  );
}

export function isInternalResourceAvailabilityIO(
  route: Pick<FactoryWorkstationIO, "state" | "workType">,
  resourceNames: ReadonlySet<string>,
): boolean {
  return (
    resourceNames.has(route.workType.trim()) &&
    route.state.trim() === RESOURCE_AVAILABLE_STATE
  );
}

export function isInternalResourceAvailabilityWorkstation(
  workstation: Pick<
    FactoryWorkstation,
    | "id"
    | "inputs"
    | "name"
    | "onContinue"
    | "onFailure"
    | "onRejection"
    | "outputs"
    | "worker"
  >,
  resourceNames: ReadonlySet<string>,
): boolean {
  const identifier = workstation.id?.trim() || workstation.name.trim();
  if (!resourceNames.has(identifier)) {
    return false;
  }

  const routes = [
    ...workstationIOEntries(workstation.inputs),
    ...workstationIOEntries(workstation.outputs),
    ...workstationIOEntries(workstation.onContinue),
    ...workstationIOEntries(workstation.onFailure),
    ...workstationIOEntries(workstation.onRejection),
  ];
  return (
    (workstation.worker?.trim() ?? "").length === 0 &&
    routes.every((route) =>
      isInternalResourceAvailabilityIO(route, resourceNames),
    )
  );
}

export function isInternalResourceAvailabilityGraphNodeId(
  nodeId: string,
  resourceNames: ReadonlySet<string>,
): boolean {
  for (const resourceName of resourceNames) {
    if (
      nodeId === `work-type:${resourceName}` ||
      nodeId === `work-state:${resourceName}:${RESOURCE_AVAILABLE_STATE}` ||
      nodeId === `workstation:${resourceName}`
    ) {
      return true;
    }
  }
  return false;
}

export function isInternalResourceAvailabilityGraphEdgeId(
  edgeId: string,
  resourceNames: ReadonlySet<string>,
): boolean {
  for (const resourceName of resourceNames) {
    if (
      edgeId.includes(`work-type:${resourceName}`) ||
      edgeId.includes(
        `work-state:${resourceName}:${RESOURCE_AVAILABLE_STATE}`,
      ) ||
      edgeId.includes(`workstation:${resourceName}`)
    ) {
      return true;
    }
  }
  return false;
}

type FactoryWorkstationIORoute =
  | FactoryWorkstationIO
  | FactoryWorkstationIO[]
  | null
  | undefined;

function workstationIOEntries(
  routes: FactoryWorkstationIORoute,
): FactoryWorkstationIO[] {
  if (!routes) {
    return [];
  }
  return Array.isArray(routes) ? routes : [routes];
}
