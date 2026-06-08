import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import {
  CurrentSelectionBodyLayout,
  CurrentSelectionDetailFeedback,
  CurrentSelectionExpandableSection,
} from "../../base/public";
import { useResourceDetailState } from "../hooks/use-resource-detail-state";
import type { ResourceDetailCardProps } from "../lib/detail-card-types";
import { getResourceDetailMessages } from "../messages/resource-detail";
import { ResourceDetailContextSection } from "./resource-detail-context-section";
import { ResourceEditableConfigurationSection } from "./resource-editable-configuration-section";

export function ResourceDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  resourceName,
  saveState,
  tokenCount = null,
  widgetId = "current-selection",
}: ResourceDetailCardProps) {
  const messages = getResourceDetailMessages(locale);
  const detailState = useResourceDetailState(resourceName);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={resourceName}>
        {detailState.status !== "ready" ? (
          <CurrentSelectionExpandableSection
            defaultExpanded
            title={messages.editableConfigurationHeading}
            toggleLabel={(expanded) =>
              expanded
                ? messages.editableConfigurationCollapseActionLabel
                : messages.editableConfigurationExpandActionLabel
            }
          >
            {detailState.status === "loading" ? (
              <CurrentSelectionDetailFeedback>
                {messages.configurationLoading}
              </CurrentSelectionDetailFeedback>
            ) : null}
            {detailState.status === "error" ? (
              <CurrentSelectionDetailFeedback role="alert" tone="danger">
                {messages.configurationErrorPrefix} {detailState.errorMessage}
              </CurrentSelectionDetailFeedback>
            ) : null}
            {detailState.status === "empty" ? (
              <CurrentSelectionDetailFeedback>
                {messages.configurationEmpty}
              </CurrentSelectionDetailFeedback>
            ) : null}
          </CurrentSelectionExpandableSection>
        ) : null}
        {detailState.status === "ready" && editableConfigurationState ? (
          <ResourceEditableConfigurationSection
            detailState={detailState}
            messages={messages}
            resourceName={resourceName}
            saveState={saveState}
            state={editableConfigurationState}
            tokenCount={tokenCount}
          />
        ) : null}
        {detailState.status === "ready" && !editableConfigurationState ? (
          <ResourceDetailContextSection
            detailState={detailState}
            messages={messages}
            tokenCount={tokenCount}
          />
        ) : null}
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
