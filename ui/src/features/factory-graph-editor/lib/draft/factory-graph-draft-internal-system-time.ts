import type {
  FactoryWorkState,
  FactoryWorkstation,
  FactoryWorkstationIO,
  FactoryWorkType,
} from "./factory-graph-draft-types";

const SYSTEM_TIME_PREFIX = "__system_time";

export function isInternalSystemTimeIdentifier(
  value: string | null | undefined,
): boolean {
  const normalized = value?.trim();
  return (
    normalized === SYSTEM_TIME_PREFIX ||
    normalized?.startsWith(`${SYSTEM_TIME_PREFIX}:`) === true
  );
}

export function isInternalSystemTimeWorkType(
  workType: Pick<FactoryWorkType, "id" | "name">,
): boolean {
  return (
    isInternalSystemTimeIdentifier(workType.id) ||
    isInternalSystemTimeIdentifier(workType.name)
  );
}

export function isInternalSystemTimeWorkState(
  state: Pick<FactoryWorkState, "id" | "name">,
  workTypeName: string | null | undefined,
): boolean {
  return (
    isInternalSystemTimeIdentifier(workTypeName) ||
    isInternalSystemTimeIdentifier(state.id) ||
    isInternalSystemTimeIdentifier(state.name)
  );
}

export function isInternalSystemTimeWorkstation(
  workstation: Pick<FactoryWorkstation, "id" | "name">,
): boolean {
  return (
    isInternalSystemTimeIdentifier(workstation.id) ||
    isInternalSystemTimeIdentifier(workstation.name)
  );
}

export function isInternalSystemTimeIO(
  route: Pick<FactoryWorkstationIO, "state" | "workType">,
): boolean {
  return (
    isInternalSystemTimeIdentifier(route.workType) ||
    isInternalSystemTimeIdentifier(route.state) ||
    isInternalSystemTimeIdentifier(`${route.workType}:${route.state}`)
  );
}

export function isInternalSystemTimeGraphNodeId(nodeId: string): boolean {
  return (
    nodeId.startsWith("work-type:__system_time") ||
    nodeId.startsWith("work-state:__system_time") ||
    nodeId.startsWith("workstation:__system_time")
  );
}

export function isInternalSystemTimeGraphEdgeId(edgeId: string): boolean {
  return edgeId.includes(":__system_time");
}
