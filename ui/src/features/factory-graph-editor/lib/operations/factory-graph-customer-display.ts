import {
  isSystemTimeWorkType,
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
} from "../../../timeline/state/timeline/systemTime";
import type {
  FactoryGraphNode,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import { nodeKeyId } from "../draft/factory-graph-draft-types";

export {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_PENDING_PLACE_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
} from "../../../timeline/state/timeline/systemTime";

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

function isEmptyWorkerGraphNode(node: FactoryGraphNode): boolean {
  return (
    node.key.kind === "worker" && (node.key.name ?? "").trim().length === 0
  );
}

export function isHiddenCustomerDisplayGraphNode(
  node: FactoryGraphNode,
): boolean {
  return isHiddenSystemTimeGraphNode(node) || isEmptyWorkerGraphNode(node);
}

export function systemTimeGraphNodeId(
  kind: "work-type" | "work-state" | "workstation",
  ...parts: string[]
): string {
  switch (kind) {
    case "work-type":
      if (parts.length > 1) {
        return nodeKeyId({
          kind: "work-type",
          id: parts[0],
          name: parts[1] ?? "",
        });
      }
      return nodeKeyId({ kind: "work-type", name: parts[0] ?? "" });
    case "work-state":
      if (parts.length > 2) {
        return nodeKeyId({
          kind: "work-state",
          stateId: parts[2],
          stateName: parts[3] ?? parts[2] ?? "",
          workTypeId: parts[0],
          workTypeName: parts[1] ?? parts[0] ?? "",
        });
      }
      return nodeKeyId({
        kind: "work-state",
        stateName: parts[1] ?? "",
        workTypeName: parts[0] ?? "",
      });
    case "workstation":
      if (parts.length > 1) {
        return nodeKeyId({
          kind: "workstation",
          id: parts[0],
          name: parts[1] ?? "",
        });
      }
      return nodeKeyId({ kind: "workstation", name: parts[0] ?? "" });
  }
}

export function filterFactoryGraphTopologyForCustomerDisplay(
  topology: FactoryGraphTopology,
): FactoryGraphTopology {
  const hiddenNodeIds = new Set(
    topology.nodes
      .filter(isHiddenCustomerDisplayGraphNode)
      .map((node) => node.id),
  );

  return {
    edges: topology.edges.filter(
      (edge) =>
        !hiddenNodeIds.has(edge.sourceId) && !hiddenNodeIds.has(edge.targetId),
    ),
    nodes: topology.nodes.filter((node) => !hiddenNodeIds.has(node.id)),
  };
}
