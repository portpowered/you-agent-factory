import type {
  DashboardWorkItemRef,
  DashboardWorkRelation,
} from "../../../api/dashboard/types";
import {
  buildEdge,
  buildNode,
  nodeKeyId,
  type FactoryGraphEdge,
  type FactoryGraphNode,
  type FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

export interface TraceRelationNodeOverlay {
  displayLabel: string;
  endpointKey: string;
  relationStates: string[];
  relationTypes: string[];
  workID?: string;
}

export interface TraceRelationEdgeOverlay {
  ariaLabel: string;
  relationType: string;
  requiredState?: string;
  requestId?: string;
}

export interface TraceRelationFactoryGraphProjection {
  edgeOverlaysByEdgeId: ReadonlyMap<string, TraceRelationEdgeOverlay>;
  endpointKeyByNodeId: ReadonlyMap<string, string>;
  nodeIdByEndpointKey: ReadonlyMap<string, string>;
  overlaysByNodeId: ReadonlyMap<string, TraceRelationNodeOverlay>;
  topology: FactoryGraphTopology;
}

export interface ProjectTraceRelationsToFactoryGraphOptions {
  locale?: string;
  workItemsByWorkId?: ReadonlyMap<string, DashboardWorkItemRef>;
}

interface RelationEndpointRecord {
  displayLabel: string;
  endpointKey: string;
  order: number;
  relationStates: Set<string>;
  relationTypes: Set<string>;
  workID?: string;
  workName?: string;
}

export function projectTraceRelationsToFactoryGraph(
  relations: DashboardWorkRelation[],
  options: ProjectTraceRelationsToFactoryGraphOptions = {},
): TraceRelationFactoryGraphProjection {
  const { locale, workItemsByWorkId } = options;
  const messages = getTraceDrilldownMessages(locale);
  const endpointRecords = new Map<string, RelationEndpointRecord>();

  relations.forEach((relation, index) => {
    const source = relationEndpoint(relation, "source", index, locale);
    const target = relationEndpoint(relation, "target", index, locale);

    upsertEndpointRecord(endpointRecords, source, index * 2);
    upsertEndpointRecord(endpointRecords, target, index * 2 + 1);

    const sourceRecord = endpointRecords.get(source.endpointKey);
    const targetRecord = endpointRecords.get(target.endpointKey);
    sourceRecord?.relationTypes.add(relation.type);
    targetRecord?.relationTypes.add(relation.type);
    if (relation.required_state) {
      sourceRecord?.relationStates.add(relation.required_state);
      targetRecord?.relationStates.add(relation.required_state);
    }
  });

  const sortedEndpoints = [...endpointRecords.values()].sort(
    (left, right) =>
      left.order - right.order || left.endpointKey.localeCompare(right.endpointKey),
  );
  const nodeIdByEndpointKey = new Map<string, string>();
  const endpointKeyByNodeId = new Map<string, string>();
  const overlaysByNodeId = new Map<string, TraceRelationNodeOverlay>();
  const nodes: FactoryGraphNode[] = [];
  const reservedNodeIds = new Set<string>();

  for (const endpoint of sortedEndpoints) {
    const node = resolveRelationFactoryNode(
      endpoint,
      workItemsByWorkId,
      messages.unknownRelationSource,
      reservedNodeIds,
    );
    nodeIdByEndpointKey.set(endpoint.endpointKey, node.id);
    endpointKeyByNodeId.set(node.id, endpoint.endpointKey);
    overlaysByNodeId.set(node.id, {
      displayLabel: endpoint.displayLabel,
      endpointKey: endpoint.endpointKey,
      relationStates: [...endpoint.relationStates.values()].sort(),
      relationTypes: [...endpoint.relationTypes.values()].sort(),
      workID: endpoint.workID,
    });
    nodes.push(node);
  }

  const edgeOverlaysByEdgeId = new Map<string, TraceRelationEdgeOverlay>();
  const edges: FactoryGraphEdge[] = [];

  relations.forEach((relation, index) => {
    const source = relationEndpoint(relation, "source", index, locale);
    const target = relationEndpoint(relation, "target", index, locale);
    const sourceNodeId = nodeIdByEndpointKey.get(source.endpointKey);
    const targetNodeId = nodeIdByEndpointKey.get(target.endpointKey);
    if (!sourceNodeId || !targetNodeId) {
      return;
    }

    const sourceNode = nodes.find((node) => node.id === sourceNodeId);
    const targetNode = nodes.find((node) => node.id === targetNodeId);
    if (!sourceNode || !targetNode) {
      return;
    }

    const edge = {
      ...buildEdge("work-type-state", sourceNode.key, targetNode.key),
      id: relationEdgeId(relation, index),
    };
    const sourceOverlay = overlaysByNodeId.get(sourceNodeId);
    const targetOverlay = overlaysByNodeId.get(targetNodeId);

    edgeOverlaysByEdgeId.set(edge.id, {
      ariaLabel: messages.relationEdgeLabel({
        relationState: relation.required_state,
        relationType: relation.type,
        sourceLabel: sourceOverlay?.displayLabel ?? source.displayLabel,
        targetLabel: targetOverlay?.displayLabel ?? target.displayLabel,
      }),
      relationType: relation.type,
      requiredState: relation.required_state,
      requestId: relation.request_id,
    });
    edges.push(edge);
  });

  return {
    edgeOverlaysByEdgeId,
    endpointKeyByNodeId,
    nodeIdByEndpointKey,
    overlaysByNodeId,
    topology: {
      edges: edges.sort((left, right) => left.id.localeCompare(right.id)),
      nodes,
    },
  };
}

function relationEdgeId(
  relation: DashboardWorkRelation,
  index: number,
): string {
  return [
    "work-type-state",
    relation.type,
    relation.source_work_id ?? `source-${index}`,
    relation.target_work_id,
    relation.required_state ?? "",
    relation.request_id ?? "",
  ].join("|");
}

function upsertEndpointRecord(
  endpointRecords: Map<string, RelationEndpointRecord>,
  endpoint: {
    displayLabel: string;
    endpointKey: string;
    workID?: string;
    workName?: string;
  },
  order: number,
) {
  if (!endpointRecords.has(endpoint.endpointKey)) {
    endpointRecords.set(endpoint.endpointKey, {
      displayLabel: endpoint.displayLabel,
      endpointKey: endpoint.endpointKey,
      order,
      relationStates: new Set<string>(),
      relationTypes: new Set<string>(),
      workID: endpoint.workID,
      workName: endpoint.workName,
    });
  }
}

function resolveRelationFactoryNode(
  endpoint: RelationEndpointRecord,
  workItemsByWorkId: ReadonlyMap<string, DashboardWorkItemRef> | undefined,
  unknownSourceLabel: string,
  reservedNodeIds: Set<string>,
): FactoryGraphNode {
  const workTypeName = resolveWorkTypeName(endpoint, workItemsByWorkId);
  const primaryState = [...endpoint.relationStates.values()].sort()[0];

  if (primaryState && workTypeName) {
    const workStateKey = {
      kind: "work-state" as const,
      stateName: normalizeStateName(primaryState),
      workTypeName,
    };
    const canonicalNodeId = nodeKeyId(workStateKey);
    if (!reservedNodeIds.has(canonicalNodeId)) {
      reservedNodeIds.add(canonicalNodeId);
      return buildNode(workStateKey);
    }

    const disambiguatedWorkTypeKey = {
      kind: "work-type" as const,
      name: disambiguatedWorkTypeName(workTypeName, endpoint.endpointKey),
    };
    reservedNodeIds.add(nodeKeyId(disambiguatedWorkTypeKey));
    return buildNode(disambiguatedWorkTypeKey);
  }

  const workTypeKey = {
    kind: "work-type" as const,
    name:
      workTypeName ??
      slugifyWorkTypeName(endpoint.displayLabel, unknownSourceLabel),
  };
  reservedNodeIds.add(nodeKeyId(workTypeKey));
  return buildNode(workTypeKey);
}

function disambiguatedWorkTypeName(
  workTypeName: string,
  endpointKey: string,
): string {
  return `${workTypeName}:${slugifyWorkTypeName(endpointKey)}`;
}

function resolveWorkTypeName(
  endpoint: RelationEndpointRecord,
  workItemsByWorkId?: ReadonlyMap<string, DashboardWorkItemRef>,
): string | undefined {
  const workItem = endpoint.workID
    ? workItemsByWorkId?.get(endpoint.workID)
    : undefined;
  const workTypeID =
    workItem?.work_type_id?.trim() || workItem?.workTypeId?.trim();
  if (workTypeID) {
    return workTypeID;
  }

  const workName = endpoint.workName?.trim() || endpoint.displayLabel.trim();
  if (workName) {
    return slugifyWorkTypeName(workName);
  }

  return undefined;
}

function normalizeStateName(state: string): string {
  return state.trim().toLowerCase().replace(/\s+/g, "-");
}

function slugifyWorkTypeName(
  value: string,
  fallback = "work",
): string {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || fallback;
}

function relationEndpoint(
  relation: DashboardWorkRelation,
  side: "source" | "target",
  index: number,
  locale?: string,
): {
  displayLabel: string;
  endpointKey: string;
  workID?: string;
  workName?: string;
} {
  if (side === "source") {
    const workID = relation.source_work_id?.trim();
    const displayLabel =
      relation.source_work_name?.trim() ||
      workID ||
      getTraceDrilldownMessages(locale).unknownRelationSource;
    return {
      displayLabel,
      endpointKey: workID || `relation-${index}-source`,
      workID: workID || undefined,
      workName: relation.source_work_name?.trim(),
    };
  }

  const workID = relation.target_work_id.trim();
  return {
    displayLabel: relation.target_work_name?.trim() || workID,
    endpointKey: workID,
    workID,
    workName: relation.target_work_name?.trim(),
  };
}
