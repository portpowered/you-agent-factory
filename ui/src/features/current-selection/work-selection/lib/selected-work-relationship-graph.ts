import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkRelation,
} from "../../../../api/dashboard/types";
import type { FactoryRelation } from "../../../../api/events/types";
import { findWorkItemReference } from "../../base/state/dashboardSelection";

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
      relations: [];
      relatedWork: [];
      selectedWork: SelectedWorkRelationshipNode;
      status: "empty";
    }
  | {
      edges: SelectedWorkRelationshipEdge[];
      relations: DashboardWorkRelation[];
      relatedWork: SelectedWorkRelationshipNode[];
      selectedWork: SelectedWorkRelationshipNode;
      status: "ready";
    };

interface RelationshipAwareSnapshot extends DashboardSnapshot {
  relationsByWorkID?: Record<string, FactoryRelation[]>;
}

type ConnectedDashboardWorkRelation = DashboardWorkRelation & {
  source_work_id: string;
  target_work_id: string;
  type: string;
};

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
        // hardcoded-ui-copy-exception: non-product-diagnostic
        "Work relationship data is unavailable for the selected timeline snapshot.",
      selectedWork,
      status: "error",
    };
  }

  const relations = connectedSupportedRelations(
    relationsByWorkID,
    selectedWork,
  );
  const relatedWorkByID = new Map<string, SelectedWorkRelationshipNode>();
  const edges = relations.map(edgeForRelation).sort(compareEdges);

  for (const relation of relations) {
    if (relation.source_work_id !== selectedWork.workID) {
      relatedWorkByID.set(
        relation.source_work_id,
        resolveRelatedNode(
          snapshot,
          relation.source_work_id,
          relation.source_work_name,
        ),
      );
    }
    if (relation.target_work_id !== selectedWork.workID) {
      relatedWorkByID.set(
        relation.target_work_id,
        resolveRelatedNode(
          snapshot,
          relation.target_work_id,
          relation.target_work_name,
        ),
      );
    }
  }

  if (edges.length === 0) {
    return {
      edges: [],
      relations: [],
      relatedWork: [],
      selectedWork,
      status: "empty",
    };
  }

  return {
    edges,
    relations,
    relatedWork: [...relatedWorkByID.values()].sort(compareNodes),
    selectedWork,
    status: "ready",
  };
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

function _dedupeEdges(
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

function connectedSupportedRelations(
  relationsByWorkID: Record<string, FactoryRelation[]>,
  selectedWork: SelectedWorkRelationshipNode,
): ConnectedDashboardWorkRelation[] {
  const relations = dedupeDashboardRelations(
    Object.values(relationsByWorkID)
      .flat()
      .map(normalizeSupportedRelation)
      .filter(
        (relation): relation is ConnectedDashboardWorkRelation =>
          relation !== null,
      ),
  );
  const incidentByWorkID = new Map<string, ConnectedDashboardWorkRelation[]>();

  for (const relation of relations) {
    const sourceRelations = incidentByWorkID.get(relation.source_work_id) ?? [];
    sourceRelations.push(relation);
    incidentByWorkID.set(relation.source_work_id, sourceRelations);

    const targetRelations = incidentByWorkID.get(relation.target_work_id) ?? [];
    targetRelations.push(relation);
    incidentByWorkID.set(relation.target_work_id, targetRelations);
  }

  const visited = new Set<string>([selectedWork.workID]);
  const queue = [selectedWork.workID];

  while (queue.length > 0) {
    const workID = queue.shift();
    if (!workID) {
      continue;
    }

    for (const relation of incidentByWorkID.get(workID) ?? []) {
      for (const neighborID of [
        relation.source_work_id,
        relation.target_work_id,
      ]) {
        if (!visited.has(neighborID)) {
          visited.add(neighborID);
          queue.push(neighborID);
        }
      }
    }
  }

  return relations
    .filter(
      (relation) =>
        visited.has(relation.source_work_id) &&
        visited.has(relation.target_work_id),
    )
    .sort(compareRelations);
}

function normalizeSupportedRelation(
  relation: FactoryRelation,
): ConnectedDashboardWorkRelation | null {
  const type = relation.type?.trim().toUpperCase();
  if (type !== "PARENT" && type !== "PARENT_CHILD" && type !== "DEPENDS_ON") {
    return null;
  }

  const sourceWorkID = trimString(
    "source_work_id" in relation ? relation.source_work_id : undefined,
  );
  const targetWorkID =
    trimString(
      "targetWorkId" in relation ? relation.targetWorkId : undefined,
    ) ||
    trimString(
      "target_work_id" in relation ? relation.target_work_id : undefined,
    );

  if (!sourceWorkID || !targetWorkID || sourceWorkID === targetWorkID) {
    return null;
  }

  return {
    required_state:
      trimString(
        "requiredState" in relation ? relation.requiredState : undefined,
      ) ||
      trimString(
        "required_state" in relation ? relation.required_state : undefined,
      ),
    source_work_id: sourceWorkID,
    source_work_name:
      trimString(
        "sourceWorkName" in relation ? relation.sourceWorkName : undefined,
      ) ||
      trimString(
        "source_work_name" in relation ? relation.source_work_name : undefined,
      ),
    target_work_id: targetWorkID,
    target_work_name:
      trimString(
        "targetWorkName" in relation ? relation.targetWorkName : undefined,
      ) ||
      trimString(
        "target_work_name" in relation ? relation.target_work_name : undefined,
      ),
    type: type === "PARENT" ? "PARENT_CHILD" : type,
  };
}

function edgeForRelation(
  relation: ConnectedDashboardWorkRelation,
): SelectedWorkRelationshipEdge {
  return {
    relationship: relation.type === "PARENT_CHILD" ? "PARENT" : "DEPENDS_ON",
    requiredState: relation.required_state,
    sourceWorkID: relation.source_work_id,
    targetWorkID: relation.target_work_id,
  };
}

function dedupeDashboardRelations(
  relations: ConnectedDashboardWorkRelation[],
): ConnectedDashboardWorkRelation[] {
  const deduped = new Map<string, ConnectedDashboardWorkRelation>();
  for (const relation of relations) {
    const key = [
      relation.type,
      relation.source_work_id,
      relation.target_work_id,
      relation.required_state ?? "",
    ].join("|");
    if (!deduped.has(key)) {
      deduped.set(key, relation);
    }
  }
  return [...deduped.values()];
}

function compareRelations(
  left: ConnectedDashboardWorkRelation,
  right: ConnectedDashboardWorkRelation,
): number {
  return (
    left.source_work_id.localeCompare(right.source_work_id) ||
    left.target_work_id.localeCompare(right.target_work_id) ||
    left.type.localeCompare(right.type) ||
    (left.required_state ?? "").localeCompare(right.required_state ?? "")
  );
}

function trimString(value: unknown): string | undefined {
  return typeof value === "string" ? value.trim() || undefined : undefined;
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
