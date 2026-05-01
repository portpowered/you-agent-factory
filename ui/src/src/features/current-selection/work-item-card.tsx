import { formatDurationFromISO, formatList, formatWorkItemLabel } from "../../components/dashboard/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS, DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../components/dashboard/typography";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import type { DashboardTrace, DashboardWorkRelation } from "../../../api/dashboard/types";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import type { WorkItemDetailCardProps } from "./detail-card-types";
import { WORK_SELECTION_BUTTON_CLASS } from "./detail-card-shared";
import { ExecutionDetailsSection } from "./execution-details";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

export function WorkItemDetailCard({
  activeTraceID,
  dispatchAttempts,
  executionDetails,
  failureMessage,
  failureReason,
  now,
  onSelectTraceID,
  onSelectWorkID,
  selectedNode,
  selection,
  selectedTrace,
  workstationRequests,
  traceTargetId = "trace",
  widgetId = "current-selection",
}: WorkItemDetailCardProps) {
  const workRelationships = buildWorkRelationships(selectedTrace, selection.workItem.work_id);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{formatWorkItemLabel(selection.workItem)}</p>
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
          <dd>{selectedNode?.workstation_name ?? executionDetails.workstationName ?? "Unavailable"}</dd>
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
      {failureReason || failureMessage ? (
        <section aria-label="Failure details" className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Failure details</h4>
          <dl>
            {failureReason ? (
              <div>
                <dt>Failure reason</dt>
                <dd>{failureReason}</dd>
              </div>
            ) : null}
            {failureMessage ? (
              <div>
                <dt>Failure message</dt>
                <dd>{failureMessage}</dd>
              </div>
            ) : null}
          </dl>
        </section>
      ) : null}
      <ExecutionDetailsSection
        activeTraceID={activeTraceID}
        details={executionDetails}
        now={now}
        onSelectTraceID={onSelectTraceID}
        traceTargetId={traceTargetId}
      />
      <WorkRelationshipsSection
        onSelectWorkID={onSelectWorkID}
        relationships={workRelationships}
      />
      <SelectedWorkDispatchHistorySection
        activeTraceID={activeTraceID}
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
  key: string;
  workID: string;
  workLabel: string;
}

function WorkRelationshipsSection({
  onSelectWorkID,
  relationships,
}: {
  onSelectWorkID?: (workID: string) => void;
  relationships: RelatedWorkItem[];
}) {
  return (
    <section aria-label="Work relationships" className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Work relationships</h4>
      {relationships.length > 0 ? (
        <ul className="m-0 grid list-none gap-[0.55rem] p-0">
          {relationships.map((relationship) => (
            <li
              className="grid gap-[0.3rem] rounded-lg border border-af-overlay/8 bg-af-overlay/4 p-[0.85rem]"
              key={relationship.key}
            >
              <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{relationship.description}</span>
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
      ) : (
        <p className="m-0 text-sm text-af-ink/68">
          No parent, child, or dependency relationships are available for this work item.
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
    .sort((left, right) => left.description.localeCompare(right.description) || left.workLabel.localeCompare(right.workLabel));
}

function buildRelationshipItems(
  relation: DashboardWorkRelation,
  selectedWorkID: string,
): RelatedWorkItem[] {
  const items: RelatedWorkItem[] = [];
  const relationType = relation.type.trim().toUpperCase();
  const stateSuffix = relation.required_state ? ` (${relation.required_state})` : "";

  if (relation.source_work_id === selectedWorkID && relation.target_work_id) {
    items.push({
      description: `${forwardRelationshipLabel(relationType)}${stateSuffix}`,
      key: `${relation.type}:${selectedWorkID}:${relation.target_work_id}:forward`,
      workID: relation.target_work_id,
      workLabel: relation.target_work_name || relation.target_work_id,
    });
  }

  if (relation.target_work_id === selectedWorkID && relation.source_work_id) {
    items.push({
      description: `${reverseRelationshipLabel(relationType)}${stateSuffix}`,
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
