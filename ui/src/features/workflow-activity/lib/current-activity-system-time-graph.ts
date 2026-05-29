import type { FactoryGraphNode } from "../../factory-graph-editor/lib/factory-graph-draft-types";

export const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
export const SYSTEM_TIME_EXPIRY_TRANSITION_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:expire`;

export function isSystemTimeWorkType(workTypeID: string | undefined): boolean {
  return workTypeID === SYSTEM_TIME_WORK_TYPE_ID;
}

export function isSystemTimeFactoryGraphNode(node: FactoryGraphNode): boolean {
  switch (node.key.kind) {
    case "work-state":
      return isSystemTimeWorkType(node.key.workTypeName);
    case "work-type":
      return isSystemTimeWorkType(node.key.name);
    case "workstation":
      return node.key.name === SYSTEM_TIME_EXPIRY_TRANSITION_ID;
    default:
      return false;
  }
}

export function isSystemTimeCurrentActivityNodeId(nodeId: string): boolean {
  if (nodeId === `workstation:${SYSTEM_TIME_EXPIRY_TRANSITION_ID}`) {
    return true;
  }
  if (nodeId === `work-type:${SYSTEM_TIME_WORK_TYPE_ID}`) {
    return true;
  }
  if (nodeId.startsWith(`work-state:${SYSTEM_TIME_WORK_TYPE_ID}:`)) {
    return true;
  }
  return nodeId.startsWith(`place:${SYSTEM_TIME_WORK_TYPE_ID}:`);
}

export function edgeTouchesSystemTimeNode(
  fromNodeId: string,
  toNodeId: string,
): boolean {
  return (
    isSystemTimeCurrentActivityNodeId(fromNodeId) ||
    isSystemTimeCurrentActivityNodeId(toNodeId)
  );
}
