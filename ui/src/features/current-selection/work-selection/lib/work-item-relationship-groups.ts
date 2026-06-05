import type { CurrentSelectionDispatchHistoryMessages } from "../../base/messages/current-selection-dispatch-history";
import type {
  SelectedWorkRelationshipEdge,
  SelectedWorkRelationshipGraph,
  SelectedWorkRelationshipNode,
} from "./selected-work-relationship-graph";

export interface RelatedWorkItem {
  description: string;
  edgeLabel: string;
  group: RelationshipGroupKey;
  key: string;
  node: SelectedWorkRelationshipNode;
  workID: string;
  workLabel: string;
}

export type RelationshipGroupKey =
  | "parent"
  | "depends-on"
  | "required-by"
  | "child"
  | "related";

export interface RelationshipGroup {
  key: RelationshipGroupKey;
  items: RelatedWorkItem[];
}

export function buildWorkRelationships(
  relationshipGraph: SelectedWorkRelationshipGraph | undefined,
  messages: CurrentSelectionDispatchHistoryMessages,
): RelatedWorkItem[] {
  if (!relationshipGraph || relationshipGraph.status !== "ready") {
    return [];
  }

  const nodesByID = new Map<string, SelectedWorkRelationshipNode>(
    relationshipGraph.relatedWork.map((node) => [node.workID, node]),
  );

  return relationshipGraph.edges
    .flatMap((edge) =>
      buildRelationshipItems(edge, nodesByID.get(edge.targetWorkID), messages),
    )
    .sort(
      (left, right) =>
        left.description.localeCompare(right.description) ||
        left.workLabel.localeCompare(right.workLabel),
    );
}

export function buildRelationshipGroups(
  relationships: RelatedWorkItem[],
): RelationshipGroup[] {
  const grouped = new Map<RelationshipGroupKey, RelatedWorkItem[]>();

  for (const relationship of relationships) {
    const items = grouped.get(relationship.group) ?? [];
    items.push(relationship);
    grouped.set(relationship.group, items);
  }

  return relationshipGroupOrder
    .map((groupKey) => ({
      key: groupKey,
      items:
        grouped
          .get(groupKey)
          ?.sort(
            (left, right) =>
              left.description.localeCompare(right.description) ||
              left.workLabel.localeCompare(right.workLabel),
          ) ?? [],
    }))
    .filter((group) => group.items.length > 0);
}

export function findRelationshipItems(
  groups: RelationshipGroup[],
  key: RelationshipGroupKey,
): RelatedWorkItem[] {
  return groups.find((group) => group.key === key)?.items ?? [];
}

export function relationshipLegendItems(
  messages: CurrentSelectionDispatchHistoryMessages,
) {
  return [
    {
      glyph: relationshipDirectionGlyph(
        messages.relationshipParentLabel,
        messages,
      ),
      label: messages.relationshipParentLegend,
      tone: "active" as const,
    },
    {
      glyph: relationshipDirectionGlyph(
        messages.relationshipDependsOnLabel,
        messages,
      ),
      label: messages.relationshipDependsOnLegend,
      tone: "warning" as const,
    },
    {
      glyph: relationshipDirectionGlyph(
        messages.relationshipRequiredByLabel,
        messages,
      ),
      label: messages.relationshipRequiredByLegend,
      tone: "neutral" as const,
    },
    {
      glyph: relationshipDirectionGlyph(
        messages.relationshipChildLabel,
        messages,
      ),
      label: messages.relationshipChildLegend,
      tone: "active" as const,
    },
  ];
}

export function relationshipDirectionGlyph(
  label: string,
  messages: CurrentSelectionDispatchHistoryMessages,
) {
  switch (label) {
    case messages.relationshipParentLabel:
      return "↑";
    case messages.relationshipDependsOnLabel:
      return "←";
    case messages.relationshipRequiredByLabel:
      return "→";
    case messages.relationshipChildLabel:
      return "↓";
    default:
      return "•";
  }
}

export function relationshipDirectionTone(
  label: string,
  messages: CurrentSelectionDispatchHistoryMessages,
) {
  switch (label) {
    case messages.relationshipParentLabel:
    case messages.relationshipChildLabel:
      return "active" as const;
    case messages.relationshipDependsOnLabel:
      return "warning" as const;
    default:
      return "neutral" as const;
  }
}

export function relationshipPillTone(group: RelationshipGroupKey) {
  switch (group) {
    case "parent":
    case "child":
      return "active" as const;
    case "depends-on":
      return "warning" as const;
    default:
      return "neutral" as const;
  }
}

function buildRelationshipItems(
  edge: SelectedWorkRelationshipEdge,
  relatedNode: SelectedWorkRelationshipNode | undefined,
  messages: CurrentSelectionDispatchHistoryMessages,
): RelatedWorkItem[] {
  if (!relatedNode) {
    return [];
  }

  const label = relationshipLabel(edge.relationship, messages);
  return [
    {
      description: edge.requiredState
        ? messages.relationshipStateLabel(label, edge.requiredState)
        : label,
      edgeLabel: label,
      group: relationshipGroup(edge.relationship),
      key: `${edge.relationship}:${edge.sourceWorkID}:${edge.targetWorkID}:${edge.requiredState ?? ""}`,
      node: relatedNode,
      workID: relatedNode.workID,
      workLabel: relatedNode.label,
    },
  ];
}

function relationshipLabel(
  relationship: SelectedWorkRelationshipEdge["relationship"],
  messages: CurrentSelectionDispatchHistoryMessages,
): string {
  switch (relationship) {
    case "PARENT":
      return messages.relationshipParentLabel;
    case "CHILD":
      return messages.relationshipChildLabel;
    case "DEPENDS_ON":
      return messages.relationshipDependsOnLabel;
    case "REQUIRED_BY":
      return messages.relationshipRequiredByLabel;
    default:
      return messages.relationshipRelatedLabel;
  }
}

function relationshipGroup(
  relationship: SelectedWorkRelationshipEdge["relationship"],
): RelationshipGroupKey {
  switch (relationship) {
    case "PARENT":
      return "parent";
    case "CHILD":
      return "child";
    case "DEPENDS_ON":
      return "depends-on";
    case "REQUIRED_BY":
      return "required-by";
    default:
      return "related";
  }
}

const relationshipGroupOrder: RelationshipGroupKey[] = [
  "parent",
  "depends-on",
  "required-by",
  "child",
  "related",
];
