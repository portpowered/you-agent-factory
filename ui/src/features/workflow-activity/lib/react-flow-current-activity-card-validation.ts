import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { FactoryValidationGraphProjection } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import {
  factoryGraphNodeIdForWorkState,
  factoryGraphNodeIdForWorkstation,
  factoryGraphNodeIdForWorkType,
} from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import { findFactoryWorkstationByNodeId } from "./current-activity-factory-graph-layout";

const WORK_TYPE_GRAPH_NODE_PREFIX = "work-type:";

function collectMessagesForNodeIds(
  messagesByNodeId: ReadonlyMap<string, readonly { message: string }[]>,
  nodeIds: ReadonlySet<string>,
): string[] {
  const messages = new Set<string>();

  for (const [nodeId, targets] of messagesByNodeId) {
    if (!nodeIds.has(nodeId)) {
      continue;
    }

    for (const target of targets) {
      messages.add(target.message);
    }
  }

  return [...messages];
}

function workstationGraphNodeIdsForSelection(
  factoryDefinition: CanonicalFactoryDefinition | undefined,
  selectionNodeId: string,
): ReadonlySet<string> {
  const workstation = findFactoryWorkstationByNodeId(
    factoryDefinition,
    selectionNodeId,
  );
  if (!workstation) {
    return new Set();
  }

  return new Set(
    [
      factoryGraphNodeIdForWorkstation(selectionNodeId),
      factoryGraphNodeIdForWorkstation(workstation.node_id),
      factoryGraphNodeIdForWorkstation(workstation.workstation_name),
    ].filter((value, index, values) => values.indexOf(value) === index),
  );
}

function workTypeGraphNodeIdsForSelection(
  selectionNodeId: string,
): ReadonlySet<string> {
  const normalizedNodeId = selectionNodeId.startsWith(
    WORK_TYPE_GRAPH_NODE_PREFIX,
  )
    ? selectionNodeId
    : factoryGraphNodeIdForWorkType(selectionNodeId);

  return new Set([normalizedNodeId, selectionNodeId]);
}

function workStateGraphNodeIdsForPlaceId(placeId: string): ReadonlySet<string> {
  return new Set([factoryGraphNodeIdForWorkState(placeId), placeId]);
}

export function validationMessagesForSelectedWorkstation(args: {
  factoryDefinition?: CanonicalFactoryDefinition;
  projection: FactoryValidationGraphProjection;
  selectionNodeId: string | null | undefined;
}): string[] {
  if (!args.selectionNodeId) {
    return [];
  }

  const workstationNodeIds = workstationGraphNodeIdsForSelection(
    args.factoryDefinition,
    args.selectionNodeId,
  );
  if (workstationNodeIds.size === 0) {
    return [];
  }

  return collectMessagesForNodeIds(
    args.projection.workstationMessagesByNodeId,
    workstationNodeIds,
  );
}

export function validationMessagesForGraphSelection(args: {
  factoryDefinition?: CanonicalFactoryDefinition;
  projection: FactoryValidationGraphProjection;
  selectionNodeId?: string | null;
  selectionPlaceId?: string | null;
}): string[] {
  const messages = new Set<string>();

  if (args.selectionNodeId) {
    for (const message of validationMessagesForSelectedWorkstation({
      factoryDefinition: args.factoryDefinition,
      projection: args.projection,
      selectionNodeId: args.selectionNodeId,
    })) {
      messages.add(message);
    }

    for (const message of collectMessagesForNodeIds(
      args.projection.workTypeMessagesByNodeId,
      workTypeGraphNodeIdsForSelection(args.selectionNodeId),
    )) {
      messages.add(message);
    }
  }

  if (args.selectionPlaceId) {
    for (const message of collectMessagesForNodeIds(
      args.projection.workStateMessagesByNodeId,
      workStateGraphNodeIdsForPlaceId(args.selectionPlaceId),
    )) {
      messages.add(message);
    }
  }

  return [...messages];
}
