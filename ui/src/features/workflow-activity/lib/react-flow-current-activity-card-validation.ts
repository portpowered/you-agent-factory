import type { FactoryValidationTarget } from "../../../api/factory-validation";
import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  filterValidationHandleErrorsForWorkstation,
  projectFactoryValidationTargets,
  type FactoryValidationGraphProjection,
  validationTargetIsRenderedForWorkstation,
} from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import {
  factoryGraphNodeIdForWorkState,
  factoryGraphNodeIdForWorkstation,
  factoryGraphNodeIdForWorkType,
} from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import { findFactoryWorkstationByNodeId } from "./current-activity-factory-graph-layout";

const WORK_TYPE_GRAPH_NODE_PREFIX = "work-type:";
const WORKSTATION_GRAPH_NODE_PREFIX = "workstation:";

export function mergeFactoryValidationTargets(
  ...targetGroups: ReadonlyArray<readonly FactoryValidationTarget[]>
): FactoryValidationGraphProjection {
  const targets: FactoryValidationTarget[] = [];
  for (const group of targetGroups) {
    targets.push(...group);
  }
  return projectFactoryValidationTargets(targets);
}

export function saveErrorNoticeMessages(error: unknown): string[] {
  if (error instanceof CurrentFactoryDefinitionError) {
    const messages = new Set<string>();
    if (error.message.trim().length > 0) {
      messages.add(error.message);
    }
    for (const target of error.targets ?? []) {
      if (target.message.trim().length > 0) {
        messages.add(target.message);
      }
    }
    return [...messages];
  }

  if (error instanceof Error && error.message.trim().length > 0) {
    return [error.message];
  }

  return [];
}

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

function collectWorkstationMessagesForSelection(
  messagesByNodeId: ReadonlyMap<
    string,
    readonly FactoryValidationTarget[]
  >,
  nodeIds: ReadonlySet<string>,
  workstation: WorkstationProgressOutcomeRouteContext | undefined,
): string[] {
  const messages = new Set<string>();

  for (const [nodeId, targets] of messagesByNodeId) {
    if (!nodeIds.has(nodeId)) {
      continue;
    }

    for (const target of targets) {
      if (!validationTargetIsRenderedForWorkstation(target, workstation)) {
        continue;
      }
      messages.add(target.message);
    }
  }

  return [...messages];
}

function workstationGraphNodeIdsForSelection(
  factoryDefinition: CanonicalFactoryDefinition | undefined,
  selectionNodeId: string,
): ReadonlySet<string> {
  if (selectionNodeId.startsWith(WORKSTATION_GRAPH_NODE_PREFIX)) {
    return new Set([selectionNodeId]);
  }

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

function findCanonicalWorkstationByNodeId(
  factoryDefinition: CanonicalFactoryDefinition | undefined,
  nodeId: string,
): WorkstationProgressOutcomeRouteContext | undefined {
  if (!factoryDefinition) {
    return undefined;
  }

  const normalizedNodeId = nodeId.startsWith(WORKSTATION_GRAPH_NODE_PREFIX)
    ? nodeId.slice(WORKSTATION_GRAPH_NODE_PREFIX.length)
    : nodeId;

  return factoryDefinition.workstations?.find(
    (candidate) =>
      candidate.id === normalizedNodeId || candidate.name === normalizedNodeId,
  );
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

  const workstation = findCanonicalWorkstationByNodeId(
    args.factoryDefinition,
    args.selectionNodeId,
  );

  const messages = new Set(
    collectWorkstationMessagesForSelection(
      args.projection.workstationMessagesByNodeId,
      workstationNodeIds,
      workstation,
    ),
  );

  for (const nodeId of workstationNodeIds) {
    const handleErrors = args.projection.handleErrorsByNodeId.get(nodeId);
    if (!handleErrors) {
      continue;
    }
    for (const error of filterValidationHandleErrorsForWorkstation(
      handleErrors,
      workstation,
    ).values()) {
      messages.add(error.message);
    }
  }

  return [...messages];
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
