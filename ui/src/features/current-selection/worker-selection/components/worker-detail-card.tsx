import { WidgetSubtitle } from "../../../../components/ui/widget-frame";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { CurrentSelectionDetailFeedback } from "../../base/public";
import { useWorkerDetailState } from "../hooks/use-worker-detail-state";
import type { WorkerDetailCardProps } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";

export function WorkerDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  saveState,
  widgetId = "current-selection",
  workerName,
}: WorkerDetailCardProps) {
  const messages = getWorkerDetailMessages(locale);
  const detailState = useWorkerDetailState(workerName);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      {editableConfigurationState?.status !== "ready" ? (
        <WidgetSubtitle>{workerName}</WidgetSubtitle>
      ) : null}
      {detailState.status === "loading" ? (
        <CurrentSelectionDetailFeedback>
          {messages.configurationLoading}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {detailState.status === "error" ? (
        <CurrentSelectionDetailFeedback
          role="alert"
          tone="danger"
        >
          {messages.configurationErrorPrefix} {detailState.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {detailState.status === "empty" ? (
        <CurrentSelectionDetailFeedback>
          {messages.configurationEmpty}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {detailState.status === "ready" ? (
        <WorkerEditableConfigurationSection
          messages={messages}
          saveState={saveState}
          state={editableConfigurationState}
          workerName={workerName}
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
