import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { cn } from "../../../lib/cn";
import type {
  SelectedWorkRelationshipEdge,
  SelectedWorkRelationshipGraph,
  SelectedWorkRelationshipNode,
} from "../lib/selected-work-relationship-graph";
import type { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import {
  CURRENT_SELECTION_ALERT_PANEL_CLASS,
  CURRENT_SELECTION_NOTICE_SUBTLE_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";

interface RelatedWorkItem {
  description: string;
  edgeLabel: string;
  group: RelationshipGroupKey;
  key: string;
  node: SelectedWorkRelationshipNode;
  workID: string;
  workLabel: string;
}

type RelationshipGroupKey =
  | "parent"
  | "depends-on"
  | "required-by"
  | "child"
  | "related";

interface RelationshipGroup {
  key: RelationshipGroupKey;
  items: RelatedWorkItem[];
}

export function WorkRelationshipsSection({
  messages,
  onSelectWorkID,
  relationshipGraph,
  selectedWorkLabel,
}: {
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  onSelectWorkID?: (workID: string) => void;
  relationshipGraph?: SelectedWorkRelationshipGraph;
  selectedWorkLabel: string;
}) {
  const relationships = buildWorkRelationships(relationshipGraph, messages);
  const relationshipGroups = buildRelationshipGroups(relationships);
  const graphStatus = relationshipGraph?.status ?? "loading";

  return (
    <section
      aria-label={messages.workRelationshipsHeading}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.workRelationshipsHeading}
      </h4>
      {graphStatus === "loading" ? (
        <p className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS} role="status">
          {messages.workRelationshipsLoading}
        </p>
      ) : relationshipGraph?.status === "error" ? (
        <div className={CURRENT_SELECTION_ALERT_PANEL_CLASS} role="alert">
          <p className="m-0">{messages.workRelationshipsError}</p>
          <p className="m-0 text-sm text-af-danger">
            {relationshipGraph.message}
          </p>
        </div>
      ) : relationships.length > 0 ? (
        <div className="grid gap-3 rounded-xl border border-af-border bg-af-surface-subtle p-3">
          <RelationshipLegend messages={messages} />
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(14rem,16rem)_minmax(0,1fr)] md:grid-rows-[auto_auto_auto] md:items-start">
            <RelationshipLane
              className="md:col-start-2 md:row-start-1"
              items={findRelationshipItems(relationshipGroups, "parent")}
              label={messages.relationshipParentLabel}
              messages={messages}
              onSelectWorkID={onSelectWorkID}
            />
            <RelationshipLane
              className="md:col-start-1 md:row-start-2"
              items={findRelationshipItems(relationshipGroups, "depends-on")}
              label={messages.relationshipDependsOnLabel}
              messages={messages}
              onSelectWorkID={onSelectWorkID}
            />
            <RelationshipNodeCard
              ariaCurrent="true"
              className="md:col-start-2 md:row-start-2"
              heading={messages.selectedWorkHeading}
              isSelected
              label={selectedWorkLabel}
              messages={messages}
              node={
                relationshipGraph?.status === "loading"
                  ? undefined
                  : relationshipGraph?.selectedWork
              }
            />
            <RelationshipLane
              className="md:col-start-3 md:row-start-2"
              items={findRelationshipItems(relationshipGroups, "required-by")}
              label={messages.relationshipRequiredByLabel}
              messages={messages}
              onSelectWorkID={onSelectWorkID}
            />
            <RelationshipLane
              className="md:col-start-2 md:row-start-3"
              items={findRelationshipItems(relationshipGroups, "child")}
              label={messages.relationshipChildLabel}
              messages={messages}
              onSelectWorkID={onSelectWorkID}
            />
          </div>
          <RelationshipLane
            items={findRelationshipItems(relationshipGroups, "related")}
            label={messages.relationshipRelatedLabel}
            messages={messages}
            onSelectWorkID={onSelectWorkID}
          />
        </div>
      ) : (
        <p className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS}>
          {messages.workRelationshipsEmpty}
        </p>
      )}
    </section>
  );
}

function buildWorkRelationships(
  relationshipGraph: SelectedWorkRelationshipGraph | undefined,
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
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

function buildRelationshipItems(
  edge: SelectedWorkRelationshipEdge,
  relatedNode: SelectedWorkRelationshipNode | undefined,
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
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
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
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

function RelationshipLane({
  className,
  items,
  label,
  messages,
  onSelectWorkID,
}: {
  className?: string;
  items: RelatedWorkItem[];
  label: string;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  onSelectWorkID?: (workID: string) => void;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <section
      aria-label={messages.relationshipLaneAriaLabel(label)}
      className={cn(
        "grid gap-2 rounded-xl border border-af-border bg-af-surface-raised p-3",
        className,
      )}
    >
      <div className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className={cn(
            "inline-flex h-8 w-8 items-center justify-center rounded-full border text-sm font-bold",
            relationshipDirectionBadgeClass(label, messages),
          )}
        >
          {relationshipDirectionGlyph(label, messages)}
        </span>
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      </div>
      <ul className="m-0 grid list-none gap-2 p-0">
        {items.map((relationship) => (
          <li
            className="grid gap-2 rounded-lg border border-af-border bg-af-surface-subtle p-3"
            key={relationship.key}
          >
            <div className="flex items-center gap-2">
              <DashboardStatusPill
                aria-hidden="true"
                className="min-h-6 px-2 py-0.5 text-[11px]"
                tone={relationshipPillTone(relationship.group)}
              >
                {relationshipDirectionGlyph(label, messages)}
              </DashboardStatusPill>
              <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                {relationship.description}
              </span>
            </div>
            <RelationshipNodeCard
              heading={relationship.edgeLabel}
              label={relationship.workLabel}
              messages={messages}
              node={relationship.node}
              onSelectWorkID={onSelectWorkID}
            />
          </li>
        ))}
      </ul>
    </section>
  );
}

function RelationshipLegend({
  messages,
}: {
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
}) {
  return (
    <section
      aria-label={messages.relationshipLegendHeading}
      className="grid gap-2 rounded-lg border border-af-border bg-af-surface-raised p-3"
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
        {messages.relationshipLegendHeading}
      </span>
      <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
        {relationshipLegendItems(messages).map((item) => (
          <li key={item.label}>
            <DashboardStatusPill tone={item.tone}>
              <span aria-hidden="true">{item.glyph}</span>
              <span>{item.label}</span>
            </DashboardStatusPill>
          </li>
        ))}
      </ul>
    </section>
  );
}

function RelationshipNodeCard({
  ariaCurrent,
  className,
  heading,
  isSelected = false,
  label,
  messages,
  node,
  onSelectWorkID,
}: {
  ariaCurrent?: "true";
  className?: string;
  heading: string;
  isSelected?: boolean;
  label: string;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  node?: SelectedWorkRelationshipNode;
  onSelectWorkID?: (workID: string) => void;
}) {
  const metadata = buildRelationshipMetadata(node, messages);

  return (
    <div
      aria-current={ariaCurrent}
      className={cn(
        "grid min-w-0 gap-2 rounded-xl border p-3",
        isSelected
          ? "border-af-accent-border bg-af-accent-surface"
          : "border-af-border bg-af-surface-raised",
        className,
      )}
      data-selected-work-relationship-node={isSelected ? "selected" : "related"}
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{heading}</span>
      {node && onSelectWorkID && !isSelected ? (
        <button
          aria-label={messages.relatedWorkSelectLabel(label)}
          className={cn(WORK_SELECTION_BUTTON_CLASS, "w-full justify-start")}
          onClick={() => onSelectWorkID(node.workID)}
          type="button"
        >
          <span className="min-w-0 break-words text-left leading-5">{label}</span>
        </button>
      ) : (
        <code className="min-w-0 break-words text-sm leading-5 text-af-text">
          {label}
        </code>
      )}
      <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
        {metadata.map((item) => (
          <li key={item.key}>
            <DashboardStatusPill
              className="min-h-6 max-w-full gap-1 px-2 py-0.5 text-[11px]"
              tone={item.tone}
            >
              <span>{item.label}</span>
              <code className="min-w-0 break-words text-[11px] font-semibold">
                {item.value}
              </code>
            </DashboardStatusPill>
          </li>
        ))}
      </ul>
    </div>
  );
}

function buildRelationshipMetadata(
  node: SelectedWorkRelationshipNode | undefined,
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
) {
  return [
    {
      key: "state",
      label: messages.stateLabel,
      tone: "active" as const,
      value: node?.state ?? messages.relationshipMetadataUnavailable,
    },
    {
      key: "work-type",
      label: messages.workTypeLabel,
      tone: "neutral" as const,
      value: node?.workTypeID ?? messages.relationshipMetadataUnavailable,
    },
    {
      key: "trace-id",
      label: messages.traceIdsLabel,
      tone: "warning" as const,
      value: node?.traceID ?? messages.relationshipMetadataUnavailable,
    },
  ];
}

function relationshipLegendItems(
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
) {
  return [
    {
      glyph: relationshipDirectionGlyph(messages.relationshipParentLabel, messages),
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
      glyph: relationshipDirectionGlyph(messages.relationshipChildLabel, messages),
      label: messages.relationshipChildLegend,
      tone: "active" as const,
    },
  ];
}

function relationshipDirectionGlyph(
  label: string,
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
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

function relationshipDirectionBadgeClass(
  label: string,
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>,
) {
  switch (label) {
    case messages.relationshipParentLabel:
    case messages.relationshipChildLabel:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "border-af-accent-border bg-af-accent-surface text-af-text";
    case messages.relationshipDependsOnLabel:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "border-af-warning-border bg-af-warning-surface text-af-warning-text";
    case messages.relationshipRequiredByLabel:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "border-af-border bg-af-overlay text-af-text";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "border-af-border bg-af-surface-subtle text-af-text-muted";
  }
}

function buildRelationshipGroups(
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

function findRelationshipItems(
  groups: RelationshipGroup[],
  key: RelationshipGroupKey,
): RelatedWorkItem[] {
  return groups.find((group) => group.key === key)?.items ?? [];
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

function relationshipPillTone(group: RelationshipGroupKey) {
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

const relationshipGroupOrder: RelationshipGroupKey[] = [
  "parent",
  "depends-on",
  "required-by",
  "child",
  "related",
];
