import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { formatTypedWorkItemLabel } from "../../../components/ui/formatters";
import {
  buildEdge,
  buildNode,
  type FactoryGraphEdge,
  type FactoryGraphNodeReference,
  type FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

export interface TraceDispatchNodeOverlay {
  dispatchId: string;
  displayLabel: string;
  inputSummary: string;
  outcome?: string;
  outputSummary: string;
}

export interface TraceDispatchFactoryGraphProjection {
  dispatchIdByNodeId: ReadonlyMap<string, string>;
  nodeIdByDispatchId: ReadonlyMap<string, string>;
  overlaysByNodeId: ReadonlyMap<string, TraceDispatchNodeOverlay>;
  topology: FactoryGraphTopology;
}

export function projectTraceDispatchesToFactoryGraph(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): TraceDispatchFactoryGraphProjection {
  const messages = getTraceDrilldownMessages(locale);
  const reservedWorkstationNames = new Set<string>();
  const nodes = dispatches.map((dispatch) => {
    const workstationName = resolveTraceWorkstationNodeName(
      dispatch,
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
    };
  });
  const nodeIdByDispatchId = new Map(
    nodes.map(({ dispatch, node }) => [dispatch.dispatch_id, node.id]),
  );
  const dispatchIdByNodeId = new Map(
    nodes.map(({ dispatch, node }) => [node.id, dispatch.dispatch_id]),
  );
  const overlaysByNodeId = new Map(
    nodes.map(({ node, overlay }) => [node.id, overlay]),
  );
  const edgeKeys = new Set<string>();
  const latestDispatchIDByChainingTraceID = new Map<string, string>();

  for (
    let currentIndex = 0;
    currentIndex < dispatches.length;
    currentIndex += 1
  ) {
    const currentDispatch = dispatches[currentIndex];
    const currentNodeId = nodeIdByDispatchId.get(currentDispatch.dispatch_id);
    if (!currentNodeId) {
      continue;
    }

    const predecessorDispatchIDs =
      resolveExplicitPredecessorDispatchIDs(
        currentDispatch,
        latestDispatchIDByChainingTraceID,
      ) ??
      resolveWorkItemProducerDispatchIDs(dispatches, currentIndex) ??
      resolveSequentialPredecessorDispatchIDs(dispatches, currentIndex) ??
      [];

    for (const producerDispatchID of predecessorDispatchIDs) {
      if (producerDispatchID === currentDispatch.dispatch_id) {
        continue;
      }

      const sourceNodeId = nodeIdByDispatchId.get(producerDispatchID);
      if (!sourceNodeId) {
        continue;
      }

      edgeKeys.add(`${sourceNodeId}->${currentNodeId}`);
    }

    for (const chainingTraceID of collectCurrentChainingTraceIDs(
      currentDispatch,
    )) {
      latestDispatchIDByChainingTraceID.set(
        chainingTraceID,
        currentDispatch.dispatch_id,
      );
    }
  }

  const nodeById = new Map(nodes.map(({ node }) => [node.id, node]));
  const edges: FactoryGraphEdge[] = [...edgeKeys]
    .map((edgeKey) => {
      const [sourceNodeId, targetNodeId] = edgeKey.split("->");
      const sourceNode = nodeById.get(sourceNodeId);
      const targetNode = nodeById.get(targetNodeId);
      if (!sourceNode || !targetNode) {
        return null;
      }

      if (
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
    dispatchIdByNodeId,
    nodeIdByDispatchId,
    overlaysByNodeId,
    topology: {
      edges,
      nodes: nodes.map(({ node }) => node),
    },
  };
}

function resolveTraceWorkstationNodeName(
  dispatch: DashboardTraceDispatch,
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

  const fallback = dispatch.dispatch_id.trim();
  reservedWorkstationNames.add(fallback);
  return fallback;
}

function resolveExplicitPredecessorDispatchIDs(
  dispatch: DashboardTraceDispatch,
  latestDispatchIDByChainingTraceID: Map<string, string>,
): string[] | null {
  const predecessorDispatchIDs = collectPreviousChainingTraceIDs(dispatch)
    .map((traceID) => latestDispatchIDByChainingTraceID.get(traceID))
    .filter((dispatchID): dispatchID is string => Boolean(dispatchID));

  return predecessorDispatchIDs.length > 0
    ? uniqueNonEmptyStrings(predecessorDispatchIDs)
    : null;
}

function resolveWorkItemProducerDispatchIDs(
  dispatches: DashboardTraceDispatch[],
  currentIndex: number,
): string[] | null {
  const currentDispatch = dispatches[currentIndex];
  const producerDispatchIDs = new Set<string>();

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

      producerDispatchIDs.add(producerDispatch.dispatch_id);
    }
  }

  return producerDispatchIDs.size > 0 ? [...producerDispatchIDs] : null;
}

function resolveSequentialPredecessorDispatchIDs(
  dispatches: DashboardTraceDispatch[],
  currentIndex: number,
): string[] | null {
  return currentIndex > 0 ? [dispatches[currentIndex - 1].dispatch_id] : null;
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
