import { WidgetSubtitle } from "../../../../components/ui/widget-frame";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { CurrentSelectionDetailFeedback } from "../../base/public";
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
      {editableConfigurationState?.status !== "ready" ? (
        <WidgetSubtitle>{workTypeName}</WidgetSubtitle>
      ) : null}
      {editableConfigurationState?.status === "loading" ? (
        <CurrentSelectionDetailFeedback>
          {messages.configurationLoading}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {editableConfigurationState?.status === "error" ? (
        <CurrentSelectionDetailFeedback
          role="alert"
          tone="danger"
        >
          {messages.configurationErrorPrefix}{" "}
          {editableConfigurationState.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {editableConfigurationState?.status === "empty" ? (
        <CurrentSelectionDetailFeedback>
          {editableConfigurationState.message ?? messages.configurationEmpty}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {editableConfigurationState?.status === "ready" ? (
        <WorkTypeEditableConfigurationSection
          messages={messages}
          onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
          saveState={saveState}
          state={editableConfigurationState}
          workTypeName={workTypeName}
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
