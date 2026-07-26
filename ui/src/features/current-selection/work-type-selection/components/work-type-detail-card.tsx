import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import { CurrentSelectionDetailFeedback } from "../../base/components/detail/current-selection-detail-feedback";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionBodyLayout } from "../../base/components/layout/current-selection-body-layout";
import type { WorkTypeDetailCardProps } from "../lib/detail-card-types";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeEditableConfigurationSection } from "./work-type-ready-section";

export function WorkTypeDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  onSelectWorkStateGraphNode,
  saveState,
  widgetId = "current-selection",
  workTypeName,
}: WorkTypeDetailCardProps) {
  const messages = getWorkTypeDetailMessages(locale);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={workTypeName}>
        {editableConfigurationState?.status === "ready" ? (
          <WorkTypeEditableConfigurationSection
            messages={messages}
            onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
            saveState={saveState}
            state={editableConfigurationState}
            workTypeName={workTypeName}
          />
        ) : (
          <CurrentSelectionExpandableSection
            defaultExpanded
            title={messages.editableConfigurationHeading}
            toggleLabel={(expanded) =>
              expanded
                ? messages.editableConfigurationCollapseActionLabel
                : messages.editableConfigurationExpandActionLabel
            }
          >
            {editableConfigurationState?.status === "loading" ? (
              <CurrentSelectionDetailFeedback>
                {messages.configurationLoading}
              </CurrentSelectionDetailFeedback>
            ) : null}
            {editableConfigurationState?.status === "error" ? (
              <CurrentSelectionDetailFeedback role="alert" tone="danger">
                {messages.configurationErrorPrefix}{" "}
                {editableConfigurationState.errorMessage}
              </CurrentSelectionDetailFeedback>
            ) : null}
            {editableConfigurationState?.status === "empty" ? (
              <CurrentSelectionDetailFeedback>
                {editableConfigurationState.message ??
                  messages.configurationEmpty}
              </CurrentSelectionDetailFeedback>
            ) : null}
          </CurrentSelectionExpandableSection>
        )}
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
