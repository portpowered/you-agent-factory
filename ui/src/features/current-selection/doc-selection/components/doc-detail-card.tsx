import { factoryBundledDocDisplayLabel } from "../../../workflow-activity/lib/factory-bundled-docs";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import {
  CurrentSelectionBodyLayout,
  CurrentSelectionDetailFeedback,
  CurrentSelectionExpandableSection,
} from "../../base/public";
import { useDocDetailState } from "../hooks/use-doc-detail-state";
import type { DocDetailCardProps } from "../lib/detail-card-types";
import { getDocDetailMessages } from "../messages/doc-detail";
import { DocEditableConfigurationSection } from "./doc-editable-configuration-section";

export function DocDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  saveState,
  targetPath,
  widgetId = "current-selection",
}: DocDetailCardProps) {
  const messages = getDocDetailMessages(locale);
  const detailState = useDocDetailState(targetPath, locale);
  const title =
    detailState.status === "ready"
      ? detailState.displayLabel
      : factoryBundledDocDisplayLabel(targetPath);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={title}>
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
        {detailState.status === "ready" &&
        editableConfigurationState?.status === "loading" ? (
          <CurrentSelectionDetailFeedback>
            {messages.editableConfigurationLoading}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "ready" &&
        editableConfigurationState?.status === "error" ? (
          <CurrentSelectionDetailFeedback role="alert" tone="danger">
            {messages.editableConfigurationErrorPrefix}{" "}
            {editableConfigurationState.errorMessage}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "ready" &&
        editableConfigurationState?.status === "empty" ? (
          <CurrentSelectionDetailFeedback>
            {editableConfigurationState.message || messages.configurationEmpty}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "ready" &&
        editableConfigurationState?.status === "ready" ? (
          <DocEditableConfigurationSection
            messages={messages}
            saveState={saveState}
            state={editableConfigurationState}
            targetPath={targetPath}
          />
        ) : null}
        {detailState.status === "ready" && !editableConfigurationState ? (
          <CurrentSelectionDetailFeedback>
            {messages.docKindLabel}: {detailState.targetPath}
          </CurrentSelectionDetailFeedback>
        ) : null}
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
