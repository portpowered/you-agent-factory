import type { DashboardWorkRelation } from "../../../../api/dashboard/types";

import type {
  SelectedWorkRelationshipEdge,
  SelectedWorkRelationshipGraph,
  SelectedWorkRelationshipNode,
} from "./selected-work-relationship-graph";

// hardcoded-ui-copy-exception: non-product-diagnostic
const RELATION_TYPE_PARENT_CHILD = "PARENT_CHILD";
// hardcoded-ui-copy-exception: non-product-diagnostic
const RELATION_TYPE_DEPENDS_ON = "DEPENDS_ON";

export function projectSelectedWorkRelationshipGraphToDashboardRelations(
  relationshipGraph: SelectedWorkRelationshipGraph | null | undefined,
): DashboardWorkRelation[] | undefined {
  if (!relationshipGraph || relationshipGraph.status !== "ready") {
    return undefined;
  }

  const workNodeEntries: Array<readonly [string, SelectedWorkRelationshipNode]> =
    [
      [relationshipGraph.selectedWork.workID, relationshipGraph.selectedWork],
      ...relationshipGraph.relatedWork.map(
        (node): readonly [string, SelectedWorkRelationshipNode] => [
          node.workID,
          node,
        ],
      ),
    ];
  const workNodesByID = new Map<string, SelectedWorkRelationshipNode>(
    workNodeEntries,
  );
  const relationsByKey = new Map<string, DashboardWorkRelation>();

  for (const edge of relationshipGraph.edges) {
    const relation = toDashboardRelation(edge, workNodesByID);
    const key = [
      relation.type,
      relation.source_work_id ?? "",
      relation.target_work_id,
      relation.required_state ?? "",
    ].join("|");
    if (!relationsByKey.has(key)) {
      relationsByKey.set(key, relation);
    }
  }

  return [...relationsByKey.values()];
}

function toDashboardRelation(
  edge: SelectedWorkRelationshipEdge,
  workNodesByID: ReadonlyMap<string, SelectedWorkRelationshipNode>,
): DashboardWorkRelation {
  const sourceWorkID = relationSourceWorkID(edge);
  const targetWorkID = relationTargetWorkID(edge);
  const sourceNode = workNodesByID.get(sourceWorkID);
  const targetNode = workNodesByID.get(targetWorkID);

  return {
    required_state: edge.requiredState,
    source_work_id: sourceWorkID,
    source_work_name: sourceNode?.label,
    target_work_id: targetWorkID,
    target_work_name: targetNode?.label,
    type: relationType(edge),
  };
}

function relationSourceWorkID(edge: SelectedWorkRelationshipEdge): string {
  switch (edge.relationship) {
    case "CHILD":
    case "REQUIRED_BY":
      return edge.targetWorkID;
    case "PARENT":
    case "DEPENDS_ON":
      return edge.sourceWorkID;
  }
}

function relationTargetWorkID(edge: SelectedWorkRelationshipEdge): string {
  switch (edge.relationship) {
    case "CHILD":
    case "REQUIRED_BY":
      return edge.sourceWorkID;
    case "PARENT":
    case "DEPENDS_ON":
      return edge.targetWorkID;
  }
}

function relationType(edge: SelectedWorkRelationshipEdge): string {
  switch (edge.relationship) {
    case "PARENT":
    case "CHILD":
      return RELATION_TYPE_PARENT_CHILD;
    case "DEPENDS_ON":
    case "REQUIRED_BY":
      return RELATION_TYPE_DEPENDS_ON;
  }
}
