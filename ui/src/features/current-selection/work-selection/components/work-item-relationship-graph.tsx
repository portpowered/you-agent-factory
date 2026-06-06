import { AlertPanel, AlertPanelText } from "../../../../components/ui";
import { TraceRelationFlow } from "../../../trace-drilldown/public";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import {
  CurrentSelectionExpandableSection,
  CurrentSelectionSupportingText,
} from "../../base/public";
import { projectSelectedWorkRelationshipGraphToDashboardRelations } from "../lib/selected-work-relationship-relations";
import type { SelectedWorkRelationshipGraph } from "../lib/selected-work-relationship-graph";
import { FocusedRelationshipSummary } from "./work-item-relationship-summary";

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
