import { useMemo } from "react";
import { AlertPanel, AlertPanelText } from "@you-agent-factory/components/feedback";
import { TraceRelationFlow } from "../../../../trace-drilldown/components/trace-relation-flow";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import type { SelectedWorkRelationshipGraph } from "../../lib/selected-work-relationship-graph";
import { projectSelectedWorkRelationshipGraphToDashboardRelations } from "../../lib/selected-work-relationship-relations";
import { FocusedRelationshipSummary } from "./work-item-relationship-summary";

function workItemsByWorkIdFromRelationshipGraph(
  relationshipGraph: SelectedWorkRelationshipGraph,
) {
  if (relationshipGraph.status !== "ready") {
    return undefined;
  }

  const workItems = [
    relationshipGraph.selectedWork,
    ...relationshipGraph.relatedWork,
  ].map((node) => ({
    display_name: node.label,
    state: node.state,
    trace_id: node.traceID,
    work_id: node.workID,
    work_type_id: node.workTypeID,
  }));

  return new Map(workItems.map((workItem) => [workItem.work_id, workItem]));
}

export function WorkRelationshipsSection({
  activeTraceID,
  locale,
  messages,
  onSelectTraceID,
  onSelectWorkID,
  relationshipGraph,
  widgetId = "current-selection",
}: {
  activeTraceID?: string | null;
  locale?: string;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  relationshipGraph?: SelectedWorkRelationshipGraph;
  widgetId?: string;
}) {
  const readyRelationshipGraph =
    relationshipGraph?.status === "ready" ? relationshipGraph : undefined;
  const relationships =
    projectSelectedWorkRelationshipGraphToDashboardRelations(
      readyRelationshipGraph,
    );
  const workItemsByWorkId = useMemo(
    () =>
      relationshipGraph
        ? workItemsByWorkIdFromRelationshipGraph(relationshipGraph)
        : undefined,
    [relationshipGraph],
  );
  const graphStatus = relationshipGraph?.status ?? "loading";

  return (
    <CurrentSelectionExpandableSection
      contentId={`${widgetId}-work-item-relationships-content`}
      defaultExpanded
      headingId={`${widgetId}-work-item-relationships-heading`}
      title={messages.workRelationshipsHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {graphStatus === "loading" ? (
        <CurrentSelectionSupportingText role="status">
          {messages.workRelationshipsLoading}
        </CurrentSelectionSupportingText>
      ) : relationshipGraph?.status === "error" ? (
        <AlertPanel role="alert" tone="danger">
          <AlertPanelText>{messages.workRelationshipsError}</AlertPanelText>
          <AlertPanelText variant="supporting">
            {relationshipGraph.message}
          </AlertPanelText>
        </AlertPanel>
      ) : readyRelationshipGraph &&
        relationships &&
        relationships.length > 0 ? (
        <div className="grid gap-3">
          <TraceRelationFlow
            locale={locale}
            onSelectWorkID={onSelectWorkID}
            relations={relationships}
            selectedWorkID={readyRelationshipGraph.selectedWork.workID}
            workItemsByWorkId={workItemsByWorkId}
          />
          <FocusedRelationshipSummary
            activeTraceID={activeTraceID}
            messages={messages}
            node={readyRelationshipGraph.selectedWork}
            onSelectTraceID={onSelectTraceID}
          />
        </div>
      ) : (
        <CurrentSelectionSupportingText>
          {messages.workRelationshipsEmpty}
        </CurrentSelectionSupportingText>
      )}
    </CurrentSelectionExpandableSection>
  );
}
