import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { formatTypedWorkItemLabel } from "../../../components/ui/formatters";
import {
  buildEdge,
  buildNode,
  type FactoryGraphEdge,
  type FactoryGraphNode,
  type FactoryGraphNodeReference,
  type FactoryGraphTopology,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import type {
  TraceRelationPathEndpoint,
  TraceRelationPathEntry,
} from "./trace-relation-path";
import {
  type TraceSelectionIdentity,
  traceSelectionAttempt,
  traceSelectionIdentitiesForDispatch,
} from "./trace-selection";

export interface TraceDispatchNodeOverlay {
  dispatchId: string;
  displayLabel: string;
  inputSummary: string;
  outcome?: string;
  outputSummary: string;
}

export interface TraceDispatchFactoryGraphProjection {
  dispatchIdByNodeId: ReadonlyMap<string, string>;
  lineageStatus: "resolved" | "unresolved";
  nodeIdByDispatchId: ReadonlyMap<string, string>;
  overlaysByNodeId: ReadonlyMap<string, TraceDispatchNodeOverlay>;
  relations: readonly TraceRelationPathEntry[];
  selectionIdentitiesByNodeId: ReadonlyMap<
    string,
    readonly TraceSelectionIdentity[]
  >;
  traceNodeIdByFactoryNodeId: ReadonlyMap<string, string>;
  topology: FactoryGraphTopology;
}

interface TraceDispatchProjectionNode {
  dispatch: DashboardTraceDispatch;
  node: FactoryGraphNode;
  overlay: TraceDispatchNodeOverlay;
  selectionIdentities: readonly TraceSelectionIdentity[];
  traceNodeId: string;
}

export function projectTraceDispatchesToFactoryGraph(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): TraceDispatchFactoryGraphProjection {
  const nodes = buildTraceDispatchProjectionNodes(dispatches, locale);
  const nodeIdByDispatchId = new Map(
    nodes.map(({ dispatch, node }) => [dispatch.dispatch_id, node.id]),
  );
  const dispatchIdByNodeId = new Map(
    nodes.map(({ dispatch, node }) => [node.id, dispatch.dispatch_id]),
  );
  const overlaysByNodeId = new Map(
    nodes.map(({ node, overlay }) => [node.id, overlay]),
  );
  const selectionIdentitiesByNodeId = new Map(
    nodes.map(({ node, selectionIdentities }) => [
      node.id,
      selectionIdentities,
    ]),
  );
  const traceNodeIdByFactoryNodeId = new Map(
    nodes.map(({ node, traceNodeId }) => [node.id, traceNodeId]),
  );
  const { edges, lineageStatus, relations } = buildTraceLineageEdges(
    dispatches,
    nodes,
    dispatchIdByNodeId,
  );

  return {
    dispatchIdByNodeId,
    lineageStatus,
    nodeIdByDispatchId,
    overlaysByNodeId,
    relations,
    selectionIdentitiesByNodeId,
    traceNodeIdByFactoryNodeId,
    topology: {
      edges,
      nodes: nodes.map(({ node }) => node),
    },
  };
}

function buildTraceDispatchProjectionNodes(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): TraceDispatchProjectionNode[] {
  const messages = getTraceDrilldownMessages(locale);
  const reservedWorkstationNames = new Set<string>();
  const usedTraceNodeIDs = new Set<string>();
  const dispatchCountsByID = new Map<string, number>();
  for (const dispatch of dispatches) {
    dispatchCountsByID.set(
      dispatch.dispatch_id,
      (dispatchCountsByID.get(dispatch.dispatch_id) ?? 0) + 1,
    );
  }

  return dispatches.map((dispatch) => {
    const selectionIdentities = traceSelectionIdentitiesForDispatch(dispatch);
    const workstationName = resolveTraceWorkstationNodeName(
      dispatch,
      selectionIdentities[0],
      reservedWorkstationNames,
    );
    const workstationKey: FactoryGraphNodeReference = {
      kind: "workstation",
      name: workstationName,
    };
    const node = buildNode(workstationKey);

    return {
      dispatch,
      node,
      overlay: {
        dispatchId: dispatch.dispatch_id,
        displayLabel:
          dispatch.workstation_name?.trim() ||
          dispatch.transition_id?.trim() ||
          messages.unknownWorkstationLabel,
        inputSummary: summarizeWorkItems(dispatch.input_items, locale),
        outcome: dispatch.outcome,
        outputSummary: summarizeWorkItems(dispatch.output_items, locale),
      } satisfies TraceDispatchNodeOverlay,
      selectionIdentities,
      traceNodeId: resolveTraceNodeID(
        dispatch,
        selectionIdentities[0],
        dispatchCountsByID,
        usedTraceNodeIDs,
      ),
    };
  });
}

function buildTraceLineageEdges(
  dispatches: DashboardTraceDispatch[],
  nodes: TraceDispatchProjectionNode[],
  dispatchIdByNodeId: ReadonlyMap<string, string>,
): {
  edges: FactoryGraphEdge[];
  lineageStatus: "resolved" | "unresolved";
  relations: TraceRelationPathEntry[];
} {
  const edgeKeys = new Set<string>();
  const latestNodeIDByChainingTraceID = new Map<string, string>();
  const nodeIDsByIndex = nodes.map(({ node }) => node.id);
  const nodesByID = new Map(nodes.map(({ node, ...rest }) => [node.id, rest]));
  const relations: TraceRelationPathEntry[] = [];
  let hasUnresolvedLineage = false;

  for (
    let currentIndex = 0;
    currentIndex < dispatches.length;
    currentIndex += 1
  ) {
    const currentDispatch = dispatches[currentIndex];
    const currentNodeId = nodeIDsByIndex[currentIndex];
    if (!currentNodeId) {
      continue;
    }

    const predecessorNodeIDs =
      resolveExplicitPredecessorNodeIDs(
        currentDispatch,
        latestNodeIDByChainingTraceID,
      ) ??
      resolveWorkItemProducerNodeIDs(
        dispatches,
        currentIndex,
        nodeIDsByIndex,
      ) ??
      [];

    if (currentIndex > 0 && predecessorNodeIDs.length === 0) {
      hasUnresolvedLineage = true;
    }

    for (const producerNodeID of predecessorNodeIDs) {
      if (
        producerNodeID !== currentNodeId &&
        dispatchIdByNodeId.has(producerNodeID)
      ) {
        edgeKeys.add(`${producerNodeID}->${currentNodeId}`);
        const sourceNode = nodesByID.get(producerNodeID);
        const targetNode = nodesByID.get(currentNodeId);
        if (sourceNode && targetNode) {
          relations.push({
            id: `predecessor|${sourceNode.traceNodeId}|${targetNode.traceNodeId}`,
            kind: "predecessor",
            relationType: "PREDECESSOR",
            source: traceDispatchPathEndpoint(sourceNode),
            target: traceDispatchPathEndpoint(targetNode),
          });
        }
      }
    }

    for (const chainingTraceID of collectCurrentChainingTraceIDs(
      currentDispatch,
    )) {
      latestNodeIDByChainingTraceID.set(chainingTraceID, currentNodeId);
    }
  }

  const nodeById = new Map(nodes.map(({ node }) => [node.id, node]));
  const edges: FactoryGraphEdge[] = [...edgeKeys]
    .map((edgeKey) => {
      const [sourceNodeId, targetNodeId] = edgeKey.split("->");
      const sourceNode = nodeById.get(sourceNodeId);
      const targetNode = nodeById.get(targetNodeId);
      if (
        !sourceNode ||
        !targetNode ||
        sourceNode.key.kind !== "workstation" ||
        targetNode.key.kind !== "workstation"
      ) {
        return null;
      }

      return buildEdge(
        "workstation-on-continue",
        sourceNode.key,
        targetNode.key,
      );
    })
    .filter((edge): edge is FactoryGraphEdge => Boolean(edge))
    .sort((left, right) => left.id.localeCompare(right.id));

  return {
    edges,
    lineageStatus: hasUnresolvedLineage ? "unresolved" : "resolved",
    relations: relations.sort((left, right) => left.id.localeCompare(right.id)),
  };
}

function traceDispatchPathEndpoint({
  dispatch,
  overlay,
  selectionIdentities,
}: Omit<
  TraceDispatchProjectionNode,
  "node" | "traceNodeId"
>): TraceRelationPathEndpoint {
  return {
    dispatchID: dispatch.dispatch_id,
    label: overlay.displayLabel,
    selectionIdentities,
    workID: selectionIdentities[0]?.work_id || undefined,
  };
}

function resolveTraceWorkstationNodeName(
  dispatch: DashboardTraceDispatch,
  selection: TraceSelectionIdentity | undefined,
  reservedWorkstationNames: Set<string>,
): string {
  const candidates = [
    dispatch.workstation_name?.trim(),
    dispatch.transition_id?.trim(),
    dispatch.dispatch_id.trim(),
  ].filter((value): value is string => Boolean(value));

  for (const candidate of candidates) {
    if (!reservedWorkstationNames.has(candidate)) {
      reservedWorkstationNames.add(candidate);
      return candidate;
    }
  }

  const fallbackBase = `${dispatch.dispatch_id.trim()}#attempt-${selection?.attempt ?? traceSelectionAttempt(dispatch)}`;
  let fallback = fallbackBase;
  let suffix = 2;
  while (reservedWorkstationNames.has(fallback)) {
    fallback = `${fallbackBase}-${suffix}`;
    suffix += 1;
  }
  reservedWorkstationNames.add(fallback);
  return fallback;
}

function resolveTraceNodeID(
  dispatch: DashboardTraceDispatch,
  selection: TraceSelectionIdentity | undefined,
  dispatchCountsByID: ReadonlyMap<string, number>,
  usedTraceNodeIDs: Set<string>,
): string {
  const dispatchID = dispatch.dispatch_id.trim();
  if ((dispatchCountsByID.get(dispatch.dispatch_id) ?? 0) === 1) {
    usedTraceNodeIDs.add(dispatchID);
    return dispatchID;
  }

  const identityKey = selection
    ? `${selection.dispatch_id}#${selection.work_id || "none"}#attempt-${selection.attempt}`
    : `${dispatchID}#attempt-${traceSelectionAttempt(dispatch)}`;
  let traceNodeID = identityKey;
  let suffix = 2;
  while (usedTraceNodeIDs.has(traceNodeID)) {
    traceNodeID = `${identityKey}-${suffix}`;
    suffix += 1;
  }
  usedTraceNodeIDs.add(traceNodeID);
  return traceNodeID;
}

function resolveExplicitPredecessorNodeIDs(
  dispatch: DashboardTraceDispatch,
  latestNodeIDByChainingTraceID: Map<string, string>,
): string[] | null {
  const predecessorNodeIDs = collectPreviousChainingTraceIDs(dispatch)
    .map((traceID) => latestNodeIDByChainingTraceID.get(traceID))
    .filter((nodeID): nodeID is string => Boolean(nodeID));

  return predecessorNodeIDs.length > 0
    ? uniqueNonEmptyStrings(predecessorNodeIDs)
    : null;
}

function resolveWorkItemProducerNodeIDs(
  dispatches: DashboardTraceDispatch[],
  currentIndex: number,
  nodeIDsByIndex: string[],
): string[] | null {
  const currentDispatch = dispatches[currentIndex];
  const producerNodeIDs = new Set<string>();

  for (const inputItem of currentDispatch.input_items ?? []) {
    for (
      let producerIndex = 0;
      producerIndex < currentIndex;
      producerIndex += 1
    ) {
      const producerDispatch = dispatches[producerIndex];
      const matchingOutput = producerDispatch.output_items?.find(
        (outputItem) => outputItem.work_id === inputItem.work_id,
      );

      if (!matchingOutput) {
        continue;
      }

      const producerNodeID = nodeIDsByIndex[producerIndex];
      if (producerNodeID) {
        producerNodeIDs.add(producerNodeID);
      }
    }
  }

  return producerNodeIDs.size > 0 ? [...producerNodeIDs] : null;
}

function collectCurrentChainingTraceIDs(
  dispatch: DashboardTraceDispatch,
): string[] {
  const chainingTraceIDs = [
    dispatch.current_chaining_trace_id,
    ...(dispatch.output_items ?? []).map(
      (item) => item.current_chaining_trace_id,
    ),
  ];

  return uniqueNonEmptyStrings(chainingTraceIDs);
}

function collectPreviousChainingTraceIDs(
  dispatch: DashboardTraceDispatch,
): string[] {
  return uniqueNonEmptyStrings([
    ...(dispatch.previous_chaining_trace_ids ?? []),
    ...(dispatch.input_items ?? []).flatMap(
      (item) => item.previous_chaining_trace_ids ?? [],
    ),
  ]);
}

function uniqueNonEmptyStrings(values: Array<string | undefined>): string[] {
  const seen = new Set<string>();

  for (const value of values) {
    const nextValue = value?.trim();
    if (!nextValue) {
      continue;
    }
    seen.add(nextValue);
  }

  return [...seen];
}

function summarizeWorkItems(
  workItems: DashboardWorkItemRef[] | undefined,
  locale?: string,
): string {
  if (!workItems || workItems.length === 0) {
    return getTraceDrilldownMessages(locale).noBatchRelations;
  }

  const labels = dedupeWorkItems(workItems).map(formatTypedWorkItemLabel);
  if (labels.length <= 2) {
    return labels.join(", ");
  }

  return `${labels.slice(0, 2).join(", ")} +${labels.length - 2}`;
}

function dedupeWorkItems(
  workItems: DashboardWorkItemRef[],
): DashboardWorkItemRef[] {
  const itemsByID = new Map<string, DashboardWorkItemRef>();

  for (const workItem of workItems) {
    if (workItem.work_id) {
      itemsByID.set(workItem.work_id, workItem);
    }
  }

  return [...itemsByID.values()];
}
