import type {
  FactoryGraphEdge,
  FactoryGraphNodeKey,
} from "./factory-graph-draft-types";

export function buildEdgeRemovalDescription(edge: FactoryGraphEdge) {
  switch (edge.kind) {
    case "worker-assignment":
      return `This will unassign ${describeNodeLabel(
        edge.source,
      )} from ${describeNodeLabel(
        edge.target,
      )}. The workstation will need another worker before topology save can succeed.`;
    case "worker-resource":
      return `This will remove ${describeNodeLabel(
        edge.source,
      )} from ${describeNodeLabel(
        edge.target,
      )}'s available resources in the pending draft.`;
    case "workstation-resource":
      return `This will remove ${describeNodeLabel(
        edge.source,
      )} from ${describeNodeLabel(
        edge.target,
      )}'s required resources in the pending draft.`;
    case "workstation-input":
      return `This will stop routing ${describeNodeLabel(
        edge.source,
      )} into ${describeNodeLabel(edge.target)}.`;
    case "workstation-output":
      return `This will remove the success route from ${describeNodeLabel(
        edge.source,
      )} to ${describeNodeLabel(edge.target)}.`;
    case "workstation-on-continue":
      return `This will remove the continue route from ${describeNodeLabel(
        edge.source,
      )} to ${describeNodeLabel(edge.target)}.`;
    case "workstation-on-failure":
      return `This will remove the failure route from ${describeNodeLabel(
        edge.source,
      )} to ${describeNodeLabel(edge.target)}.`;
    case "workstation-on-rejection":
      return `This will remove the rejection route from ${describeNodeLabel(
        edge.source,
      )} to ${describeNodeLabel(edge.target)}.`;
    case "work-type-state":
      return "";
  }
}

export function describeEdgeLabel(edge: FactoryGraphEdge) {
  switch (edge.kind) {
    case "worker-assignment":
      return `${describeNodeLabel(edge.source)} assignment`;
    case "worker-resource":
    case "workstation-resource":
      return `${describeNodeLabel(edge.source)} resource link`;
    case "workstation-input":
      return `${describeNodeLabel(edge.source)} input route`;
    case "workstation-output":
      return `${describeNodeLabel(edge.source)} success route`;
    case "workstation-on-continue":
      return `${describeNodeLabel(edge.source)} continue route`;
    case "workstation-on-failure":
      return `${describeNodeLabel(edge.source)} failure route`;
    case "workstation-on-rejection":
      return `${describeNodeLabel(edge.source)} rejection route`;
    case "work-type-state":
      return `${describeNodeLabel(edge.source)} state membership`;
  }
}

function describeNodeLabel(key: FactoryGraphNodeKey) {
  return key.kind === "work-state" ? `${key.workTypeName}:${key.stateName}` : key.name;
}
