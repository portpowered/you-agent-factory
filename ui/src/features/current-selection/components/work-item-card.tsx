import { WIDGET_SUBTITLE_CLASS } from "../../../components/dashboard/widget-board";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  formatList,
  formatWorkItemLabel,
} from "../../../components/ui/formatters";
import type {
  SelectedWorkRelationshipEdge,
  SelectedWorkRelationshipGraph,
  SelectedWorkRelationshipNode,
} from "../lib/selected-work-relationship-graph";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import { WORK_SELECTION_BUTTON_CLASS } from "./detail-card-shared";
import type { WorkItemDetailCardProps } from "./detail-card-types";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

export function WorkItemDetailCard({
  activeTraceID,
  dispatchAttempts,
  executionDetails,
  locale,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  relationshipGraph,
  selectedNode,
  selectedProviderSessionKey,
  selection,
  workstationRequests,
  traceTargetId = "trace",
  widgetId = "current-selection",
}: WorkItemDetailCardProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>
        {formatWorkItemLabel(selection.workItem)}
      </p>
      <dl>
        <div>
          <dt>{messages.workIdLabel}</dt>
          <dd>{selection.workItem.work_id}</dd>
        </div>
        <div>
          <dt>{messages.workTypeLabel}</dt>
          <dd>
            {selection.workItem.work_type_id ||
              messages.currentSelectionUnavailableValue}
          </dd>
        </div>
        <div>
          <dt>{messages.workstationLabel}</dt>
          <dd>
            {selectedNode?.workstation_name ??
              executionDetails.workstationName ??
              messages.workstationUnavailableValue}
          </dd>
        </div>

        <div>
          <dt>{messages.runtimeLabelsLabel}</dt>
          <dd>
            {formatList(
              selection.execution?.work_type_ids ??
                [selection.workItem.work_type_id ?? ""].filter(Boolean),
            )}
          </dd>
        </div>
        <div>
          <dt>{messages.workstationDispatchesLabel}</dt>
          <dd>{dispatchAttempts.length}</dd>
        </div>
      </dl>
      <WorkRelationshipsSection
        messages={messages}
        onSelectWorkID={onSelectWorkID}
        relationshipGraph={relationshipGraph}
        selectedWorkLabel={
          relationshipGraph?.status !== "loading" &&
          relationshipGraph?.selectedWork.label
            ? relationshipGraph.selectedWork.label
            : formatWorkItemLabel(selection.workItem)
        }
      />
      <SelectedWorkDispatchHistorySection
        activeTraceID={activeTraceID}
        currentDispatchID={selection.dispatchId}
        fallbackProviderSessions={dispatchAttempts}
        locale={locale}
        onSelectProviderSession={onSelectProviderSession}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        requests={workstationRequests}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedWorkID={selection.workItem.work_id}
        traceTargetId={traceTargetId}
        workstationKind={selectedNode?.workstation_kind}
      />
    </SelectionDetailLayout>
  );
}

interface RelatedWorkItem {
  description: string;
  group: RelationshipGroupKey;
  key: string;
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

function WorkRelationshipsSection({
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

  return (
    <section
      aria-label={messages.workRelationshipsHeading}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.workRelationshipsHeading}
      </h4>
      {relationships.length > 0 ? (
        <div className="grid gap-3 rounded-xl border border-af-border bg-af-surface-subtle p-3">
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
            <div className="grid gap-2 rounded-xl border border-af-accent-border bg-af-accent-surface p-3 md:col-start-2 md:row-start-2">
              <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                {messages.selectedWorkHeading}
              </span>
              <code className="text-sm text-af-text">{selectedWorkLabel}</code>
            </div>
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
        <p className="m-0 text-sm text-af-text-subtle">
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
      group: relationshipGroup(edge.relationship),
      key: `${edge.relationship}:${edge.sourceWorkID}:${edge.targetWorkID}:${edge.requiredState ?? ""}`,
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
      className={`grid gap-2 rounded-xl border border-af-border bg-af-surface-subtle p-3 ${className ?? ""}`.trim()}
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <ul className="m-0 grid list-none gap-2 p-0">
        {items.map((relationship) => (
          <li
            className="grid gap-1 rounded-lg border border-af-border bg-af-surface-raised p-3"
            key={relationship.key}
          >
            <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
              {relationship.description}
            </span>
            {onSelectWorkID ? (
              <button
                aria-label={messages.relatedWorkSelectLabel(
                  relationship.workLabel,
                )}
                className={WORK_SELECTION_BUTTON_CLASS}
                onClick={() => onSelectWorkID(relationship.workID)}
                type="button"
              >
                {relationship.workLabel}
              </button>
            ) : (
              <code>{relationship.workLabel}</code>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
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

const relationshipGroupOrder: RelationshipGroupKey[] = [
  "parent",
  "depends-on",
  "required-by",
  "child",
  "related",
];
