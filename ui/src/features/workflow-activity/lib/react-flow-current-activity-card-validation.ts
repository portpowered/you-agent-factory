import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { FactoryValidationGraphProjection } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import { findFactoryWorkstationByNodeId } from "./current-activity-factory-graph-layout";

const WORKSTATION_GRAPH_NODE_PREFIX = "workstation:";

export function validationMessagesForSelectedWorkstation(args: {
  factoryDefinition?: CanonicalFactoryDefinition;
  projection: FactoryValidationGraphProjection;
  selectionNodeId: string | null | undefined;
}): string[] {
  if (!args.selectionNodeId) {
    return [];
  }

  const workstation = findFactoryWorkstationByNodeId(
    args.factoryDefinition,
    args.selectionNodeId,
  );
  if (!workstation) {
    return [];
  }

  const subjectIds = new Set(
    [workstation.node_id, workstation.workstation_name].filter(
      (value): value is string => value.trim().length > 0,
    ),
  );
  const messages = new Set<string>();

  for (const [nodeId, targets] of args.projection.workstationMessagesByNodeId) {
    const graphNodeSuffix = nodeId.startsWith(WORKSTATION_GRAPH_NODE_PREFIX)
      ? nodeId.slice(WORKSTATION_GRAPH_NODE_PREFIX.length)
      : nodeId;
    if (!subjectIds.has(graphNodeSuffix)) {
      continue;
    }

    for (const target of targets) {
      messages.add(target.message);
    }
  }

  return [...messages];
}
