import type {
  DashboardTrace,
  DashboardWorkRelation,
} from "../../api/dashboard/types";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../components/dashboard/typography";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import {
  formatList,
  formatWorkItemLabel,
} from "../../components/ui/formatters";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { WORK_SELECTION_BUTTON_CLASS } from "./detail-card-shared";
import type { WorkItemDetailCardProps } from "./detail-card-types";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

export function WorkItemDetailCard({
  activeTraceID,
  dispatchAttempts,
  executionDetails,
  onSelectTraceID,
  onSelectWorkID,
  selectedNode,
  selection,
  selectedTrace,
  workstationRequests,
  traceTargetId = "trace",
  widgetId = "current-selection",
}: WorkItemDetailCardProps) {
  const workRelationships = buildWorkRelationships(
    selectedTrace,
    selection.workItem.work_id,
  );

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>
        {formatWorkItemLabel(selection.workItem)}
      </p>
      <dl>
        <div>
          <dt>Work ID</dt>
          <dd>{selection.workItem.work_id}</dd>
        </div>
        <div>
          <dt>Work type</dt>
          <dd>{selection.workItem.work_type_id || "Unknown"}</dd>
        </div>
        <div>
          <dt>Workstation</dt>
          <dd>
            {selectedNode?.workstation_name ??
              executionDetails.workstationName ??
              "Unavailable"}
          </dd>
        </div>

        <div>
          <dt>Runtime labels</dt>
          <dd>
            {formatList(
              selection.execution?.work_type_ids ??
                [selection.workItem.work_type_id ?? ""].filter(Boolean),
            )}
          </dd>
        </div>
        <div>
          <dt>Workstation dispatches</dt>
          <dd>{dispatchAttempts.length}</dd>
        </div>
      </dl>
      <WorkRelationshipsSection
        onSelectWorkID={onSelectWorkID}
        relationships={workRelationships}
        selectedWorkLabel={formatWorkItemLabel(selection.workItem)}
      />
      <SelectedWorkDispatchHistorySection
        activeTraceID={activeTraceID}
        currentDispatchID={selection.dispatchId}
        fallbackProviderSessions={dispatchAttempts}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        requests={workstationRequests}
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
  label: string;
}

function WorkRelationshipsSection({
  onSelectWorkID,
  relationships,
  selectedWorkLabel,
}: {
  onSelectWorkID?: (workID: string) => void;
  relationships: RelatedWorkItem[];
  selectedWorkLabel: string;
}) {
  const relationshipGroups = buildRelationshipGroups(relationships);

  return (
    <section
      aria-label="Work relationships"
      className="mt-4 grid gap-[0.65rem] [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Work relationships</h4>
      {relationships.length > 0 ? (
        <div className="grid gap-3 rounded-xl border border-af-overlay/10 bg-af-overlay/3 p-3">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(14rem,16rem)_minmax(0,1fr)] md:grid-rows-[auto_auto_auto] md:items-start">
            <RelationshipLane
              className="md:col-start-2 md:row-start-1"
              items={findRelationshipItems(relationshipGroups, "parent")}
              label="Parent"
              onSelectWorkID={onSelectWorkID}
            />
            <RelationshipLane
              className="md:col-start-1 md:row-start-2"
              items={findRelationshipItems(relationshipGroups, "depends-on")}
              label="Depends on"
              onSelectWorkID={onSelectWorkID}
            />
            <div className="grid gap-2 rounded-xl border border-af-signal/20 bg-af-signal/8 p-3 md:col-start-2 md:row-start-2">
              <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                Selected work
              </span>
              <code className="text-sm text-af-ink">{selectedWorkLabel}</code>
            </div>
            <RelationshipLane
              className="md:col-start-3 md:row-start-2"
              items={findRelationshipItems(relationshipGroups, "required-by")}
              label="Required by"
              onSelectWorkID={onSelectWorkID}
            />
            <RelationshipLane
              className="md:col-start-2 md:row-start-3"
              items={findRelationshipItems(relationshipGroups, "child")}
              label="Child"
              onSelectWorkID={onSelectWorkID}
            />
          </div>
          <RelationshipLane
            items={findRelationshipItems(relationshipGroups, "related")}
            label="Related"
            onSelectWorkID={onSelectWorkID}
          />
        </div>
      ) : (
        <p className="m-0 text-sm text-af-ink/68">
          No parent, child, or dependency relationships are available for this
          work item.
        </p>
      )}
    </section>
  );
}

function buildWorkRelationships(
  trace: DashboardTrace | undefined,
  selectedWorkID: string,
): RelatedWorkItem[] {
  return (trace?.relations ?? [])
    .flatMap((relation) => buildRelationshipItems(relation, selectedWorkID))
    .sort(
      (left, right) =>
        left.description.localeCompare(right.description) ||
        left.workLabel.localeCompare(right.workLabel),
    );
}

function buildRelationshipItems(
  relation: DashboardWorkRelation,
  selectedWorkID: string,
): RelatedWorkItem[] {
  const items: RelatedWorkItem[] = [];
  const relationType = relation.type.trim().toUpperCase();
  const stateSuffix = relation.required_state
    ? ` (${relation.required_state})`
    : "";

  if (relation.source_work_id === selectedWorkID && relation.target_work_id) {
    items.push({
      description: `${forwardRelationshipLabel(relationType)}${stateSuffix}`,
      group: forwardRelationshipGroup(relationType),
      key: `${relation.type}:${selectedWorkID}:${relation.target_work_id}:forward`,
      workID: relation.target_work_id,
      workLabel: relation.target_work_name || relation.target_work_id,
    });
  }

  if (relation.target_work_id === selectedWorkID && relation.source_work_id) {
    items.push({
      description: `${reverseRelationshipLabel(relationType)}${stateSuffix}`,
      group: reverseRelationshipGroup(relationType),
      key: `${relation.type}:${relation.source_work_id}:${selectedWorkID}:reverse`,
      workID: relation.source_work_id,
      workLabel: relation.source_work_name || relation.source_work_id,
    });
  }

  return items;
}

function forwardRelationshipLabel(relationType: string): string {
  if (relationType.includes("PARENT")) {
    return "Child";
  }
  if (relationType.includes("DEPENDS")) {
    return "Depends on";
  }
  return relationType.toLowerCase().replace(/_/g, " ");
}

function reverseRelationshipLabel(relationType: string): string {
  if (relationType.includes("PARENT")) {
    return "Parent";
  }
  if (relationType.includes("DEPENDS")) {
    return "Required by";
  }
  return relationType.toLowerCase().replace(/_/g, " ");
}

function RelationshipLane({
  className,
  items,
  label,
  onSelectWorkID,
}: {
  className?: string;
  items: RelatedWorkItem[];
  label: string;
  onSelectWorkID?: (workID: string) => void;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <section
      aria-label={`${label} relationships`}
      className={`grid gap-2 rounded-xl border border-af-overlay/8 bg-af-overlay/4 p-3 ${className ?? ""}`.trim()}
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <ul className="m-0 grid list-none gap-2 p-0">
        {items.map((relationship) => (
          <li
            className="grid gap-[0.3rem] rounded-lg border border-af-overlay/8 bg-af-base/80 p-[0.75rem]"
            key={relationship.key}
          >
            <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
              {relationship.description}
            </span>
            {onSelectWorkID ? (
              <button
                aria-label={`Select related work item ${relationship.workLabel}`}
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
    .map((group) => ({
      ...group,
      items:
        grouped
          .get(group.key)
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

function forwardRelationshipGroup(relationType: string): RelationshipGroupKey {
  if (relationType.includes("PARENT")) {
    return "child";
  }
  if (relationType.includes("DEPENDS")) {
    return "depends-on";
  }
  return "related";
}

function reverseRelationshipGroup(relationType: string): RelationshipGroupKey {
  if (relationType.includes("PARENT")) {
    return "parent";
  }
  if (relationType.includes("DEPENDS")) {
    return "required-by";
  }
  return "related";
}

const relationshipGroupOrder: Array<{
  key: RelationshipGroupKey;
  label: string;
}> = [
  { key: "parent", label: "Parent" },
  { key: "depends-on", label: "Depends on" },
  { key: "required-by", label: "Required by" },
  { key: "child", label: "Child" },
  { key: "related", label: "Related" },
];
