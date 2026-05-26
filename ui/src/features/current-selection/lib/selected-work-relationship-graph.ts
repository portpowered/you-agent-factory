import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import type { FactoryRelation } from "../../../api/events/types";
import { findWorkItemReference } from "../state/dashboardSelection";

export type SelectedWorkRelationshipRole =
  | "PARENT"
  | "CHILD"
  | "DEPENDS_ON"
  | "REQUIRED_BY";

export interface SelectedWorkRelationshipNode {
  label: string;
  state?: string;
  traceID?: string;
  workID: string;
  workTypeID?: string;
}

export interface SelectedWorkRelationshipEdge {
  relationship: SelectedWorkRelationshipRole;
  requiredState?: string;
  sourceWorkID: string;
  targetWorkID: string;
}

export type SelectedWorkRelationshipGraph =
  | { status: "loading" }
  | {
      message: string;
      selectedWork: SelectedWorkRelationshipNode;
      status: "error";
    }
  | {
      edges: [];
      relatedWork: [];
      selectedWork: SelectedWorkRelationshipNode;
      status: "empty";
    }
  | {
      edges: SelectedWorkRelationshipEdge[];
      relatedWork: SelectedWorkRelationshipNode[];
      selectedWork: SelectedWorkRelationshipNode;
      status: "ready";
    };

interface RelationshipAwareSnapshot extends DashboardSnapshot {
  relationsByWorkID?: Record<string, FactoryRelation[]>;
}

export function buildSelectedWorkRelationshipGraph({
  selectedWorkItem,
  snapshot,
}: {
  selectedWorkItem: DashboardWorkItemRef | null | undefined;
  snapshot: DashboardSnapshot | null | undefined;
}): SelectedWorkRelationshipGraph {
  if (!selectedWorkItem) {
    return { status: "loading" };
  }

  const selectedWork = nodeForWorkItem(selectedWorkItem);
  if (!snapshot) {
    return { status: "loading" };
  }

  const relationsByWorkID = (snapshot as RelationshipAwareSnapshot)
    .relationsByWorkID;
  if (!relationsByWorkID) {
    return {
      message:
        "Work relationship data is unavailable for the selected timeline snapshot.",
      selectedWork,
      status: "error",
    };
  }

  const relatedWorkByID = new Map<string, SelectedWorkRelationshipNode>();
  const edges = [
    ...outboundEdges(
      snapshot,
      relationsByWorkID,
      selectedWorkItem,
      relatedWorkByID,
    ),
    ...inboundEdges(
      snapshot,
      relationsByWorkID,
      selectedWorkItem,
      relatedWorkByID,
    ),
  ].sort(compareEdges);

  if (edges.length === 0) {
    return {
      edges: [],
      relatedWork: [],
      selectedWork,
      status: "empty",
    };
  }

  return {
    edges,
    relatedWork: [...relatedWorkByID.values()].sort(compareNodes),
    selectedWork,
    status: "ready",
  };
}

function outboundEdges(
  snapshot: DashboardSnapshot,
  relationsByWorkID: Record<string, FactoryRelation[]>,
  selectedWorkItem: DashboardWorkItemRef,
  relatedWorkByID: Map<string, SelectedWorkRelationshipNode>,
): SelectedWorkRelationshipEdge[] {
  const outbound = relationsByWorkID[selectedWorkItem.work_id] ?? [];
  const edges: SelectedWorkRelationshipEdge[] = [];

  for (const relation of outbound) {
    const relationship = outboundRelationshipRole(relation.type);
    const targetWorkID = relation.targetWorkId;
    if (
      !relationship ||
      !targetWorkID ||
      targetWorkID === selectedWorkItem.work_id
    ) {
      continue;
    }

    relatedWorkByID.set(
      targetWorkID,
      resolveRelatedNode(snapshot, targetWorkID, relation.targetWorkName),
    );
    edges.push({
      relationship,
      requiredState: relation.requiredState,
      sourceWorkID: selectedWorkItem.work_id,
      targetWorkID,
    });
  }

  return dedupeEdges(edges);
}

function inboundEdges(
  snapshot: DashboardSnapshot,
  relationsByWorkID: Record<string, FactoryRelation[]>,
  selectedWorkItem: DashboardWorkItemRef,
  relatedWorkByID: Map<string, SelectedWorkRelationshipNode>,
): SelectedWorkRelationshipEdge[] {
  const edges: SelectedWorkRelationshipEdge[] = [];

  for (const relations of Object.values(relationsByWorkID)) {
    for (const relation of relations) {
      if (relation.targetWorkId !== selectedWorkItem.work_id) {
        continue;
      }

      const relationship = inboundRelationshipRole(relation.type);
      const sourceWorkID = relationSourceWorkID(relation);
      if (
        !relationship ||
        !sourceWorkID ||
        sourceWorkID === selectedWorkItem.work_id
      ) {
        continue;
      }

      relatedWorkByID.set(
        sourceWorkID,
        resolveRelatedNode(snapshot, sourceWorkID, relation.sourceWorkName),
      );
      edges.push({
        relationship,
        requiredState: relation.requiredState,
        sourceWorkID: selectedWorkItem.work_id,
        targetWorkID: sourceWorkID,
      });
    }
  }

  return dedupeEdges(edges);
}

function relationSourceWorkID(relation: FactoryRelation): string | undefined {
  return "source_work_id" in relation ? relation.source_work_id : undefined;
}

function outboundRelationshipRole(
  relationType: string,
): SelectedWorkRelationshipRole | null {
  switch (relationType.trim().toUpperCase()) {
    case "PARENT":
    case "PARENT_CHILD":
      return "PARENT";
    case "DEPENDS_ON":
      return "DEPENDS_ON";
    default:
      return null;
  }
}

function inboundRelationshipRole(
  relationType: string,
): SelectedWorkRelationshipRole | null {
  switch (relationType.trim().toUpperCase()) {
    case "PARENT":
    case "PARENT_CHILD":
      return "CHILD";
    case "DEPENDS_ON":
      return "REQUIRED_BY";
    default:
      return null;
  }
}

function resolveRelatedNode(
  snapshot: DashboardSnapshot,
  workID: string,
  fallbackLabel?: string,
): SelectedWorkRelationshipNode {
  return nodeForWorkItem(
    findWorkItemReference(snapshot, workID) ?? { work_id: workID },
    fallbackLabel,
  );
}

function nodeForWorkItem(
  workItem: DashboardWorkItemRef,
  fallbackLabel?: string,
): SelectedWorkRelationshipNode {
  return {
    label:
      workItem.display_name ||
      workItem.displayName ||
      fallbackLabel ||
      workItem.work_id,
    state: workItem.state,
    traceID: workItem.trace_id || workItem.traceId,
    workID: workItem.work_id,
    workTypeID: workItem.work_type_id || workItem.workTypeId,
  };
}

function dedupeEdges(
  edges: SelectedWorkRelationshipEdge[],
): SelectedWorkRelationshipEdge[] {
  const deduped = new Map<string, SelectedWorkRelationshipEdge>();
  for (const edge of edges) {
    const key = [
      edge.relationship,
      edge.sourceWorkID,
      edge.targetWorkID,
      edge.requiredState ?? "",
    ].join("|");
    deduped.set(key, edge);
  }
  return [...deduped.values()];
}

function compareEdges(
  left: SelectedWorkRelationshipEdge,
  right: SelectedWorkRelationshipEdge,
): number {
  return (
    left.relationship.localeCompare(right.relationship) ||
    left.targetWorkID.localeCompare(right.targetWorkID) ||
    (left.requiredState ?? "").localeCompare(right.requiredState ?? "")
  );
}

function compareNodes(
  left: SelectedWorkRelationshipNode,
  right: SelectedWorkRelationshipNode,
): number {
  return (
    left.label.localeCompare(right.label) ||
    left.workID.localeCompare(right.workID)
  );
}
