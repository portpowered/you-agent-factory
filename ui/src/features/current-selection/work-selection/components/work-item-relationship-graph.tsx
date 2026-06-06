import { AlertPanel, AlertPanelText } from "../../../../components/ui";
import { TraceRelationFlow } from "../../../trace-drilldown/public";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import {
  CurrentSelectionContentSection,
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
}: {
  activeTraceID?: string | null;
  locale?: string;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  relationshipGraph?: SelectedWorkRelationshipGraph;
}) {
  const readyRelationshipGraph =
    relationshipGraph?.status === "ready" ? relationshipGraph : undefined;
  const relationships =
    projectSelectedWorkRelationshipGraphToDashboardRelations(
      readyRelationshipGraph,
    );
  const graphStatus = relationshipGraph?.status ?? "loading";

  return (
    <CurrentSelectionContentSection
      aria-label={messages.workRelationshipsHeading}
      title={messages.workRelationshipsHeading}
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
    </CurrentSelectionContentSection>
  );
}
