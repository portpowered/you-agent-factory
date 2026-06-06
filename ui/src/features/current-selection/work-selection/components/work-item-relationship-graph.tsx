import {
  AlertPanel,
  AlertPanelText,
  SurfacePanel,
} from "../../../../components/ui";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import {
  CurrentSelectionExpandableSection,
  CurrentSelectionSupportingText,
} from "../../base/public";
import type { SelectedWorkRelationshipGraph } from "../lib/selected-work-relationship-graph";
import {
  buildRelationshipGroups,
  buildWorkRelationships,
  findRelationshipItems,
} from "../lib/work-item-relationship-groups";
import {
  RelationshipLane,
  RelationshipLegend,
  RelationshipNodeCard,
} from "./work-item-relationship-map";
import { FocusedRelationshipSummary } from "./work-item-relationship-summary";

export function WorkRelationshipsSection({
  activeTraceID,
  messages,
  onSelectTraceID,
  onSelectWorkID,
  relationshipGraph,
  selectedWorkLabel,
  widgetId = "current-selection",
}: {
  activeTraceID?: string | null;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  relationshipGraph?: SelectedWorkRelationshipGraph;
  selectedWorkLabel: string;
  widgetId?: string;
}) {
  const relationships = buildWorkRelationships(relationshipGraph, messages);
  const relationshipGroups = buildRelationshipGroups(relationships);
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
      ) : relationships.length > 0 ? (
        <SurfacePanel className="grid gap-3">
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
          {relationshipGraph?.status === "ready" ? (
            <FocusedRelationshipSummary
              activeTraceID={activeTraceID}
              messages={messages}
              node={relationshipGraph.selectedWork}
              onSelectTraceID={onSelectTraceID}
            />
          ) : null}
          <RelationshipLane
            items={findRelationshipItems(relationshipGroups, "related")}
            label={messages.relationshipRelatedLabel}
            messages={messages}
            onSelectWorkID={onSelectWorkID}
          />
        </SurfacePanel>
      ) : (
        <CurrentSelectionSupportingText>
          {messages.workRelationshipsEmpty}
        </CurrentSelectionSupportingText>
      )}
    </CurrentSelectionExpandableSection>
  );
}
