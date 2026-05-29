import {
  isSystemTimeWorkType,
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
} from "../../timeline/state/timeline/systemTime";
import type {
  FactoryGraphNode,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import { nodeKeyId } from "./factory-graph-draft-types";

export {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_PENDING_PLACE_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
} from "../../timeline/state/timeline/systemTime";

export function isSystemTimeGraphWorkTypeNode(node: FactoryGraphNode): boolean {
  return node.key.kind === "work-type" && isSystemTimeWorkType(node.key.name);
}

export function isSystemTimeGraphWorkStateNode(
  node: FactoryGraphNode,
): boolean {
  return (
    node.key.kind === "work-state" &&
    isSystemTimeWorkType(node.key.workTypeName)
  );
}

export function isSystemTimeGraphWorkstationNode(
  node: FactoryGraphNode,
): boolean {
  return (
    node.key.kind === "workstation" &&
    node.key.name === SYSTEM_TIME_EXPIRY_TRANSITION_ID
  );
}

export function isHiddenSystemTimeGraphNode(node: FactoryGraphNode): boolean {
  return (
    isSystemTimeGraphWorkTypeNode(node) ||
    isSystemTimeGraphWorkStateNode(node) ||
    isSystemTimeGraphWorkstationNode(node)
  );
}

export function systemTimeGraphNodeId(
  kind: "work-type" | "work-state" | "workstation",
  ...parts: string[]
): string {
  switch (kind) {
    case "work-type":
      return nodeKeyId({ kind: "work-type", name: parts[0] ?? "" });
    case "work-state":
      return nodeKeyId({
        kind: "work-state",
        stateName: parts[1] ?? "",
        workTypeName: parts[0] ?? "",
      });
    case "workstation":
      return nodeKeyId({ kind: "workstation", name: parts[0] ?? "" });
  }
}

export function filterFactoryGraphTopologyForCustomerDisplay(
  topology: FactoryGraphTopology,
): FactoryGraphTopology {
  const hiddenNodeIds = new Set(
    topology.nodes.filter(isHiddenSystemTimeGraphNode).map((node) => node.id),
  );

  return {
    edges: topology.edges.filter(
      (edge) =>
        !hiddenNodeIds.has(edge.sourceId) && !hiddenNodeIds.has(edge.targetId),
    ),
    nodes: topology.nodes.filter((node) => !hiddenNodeIds.has(node.id)),
  };
}
