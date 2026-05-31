import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { useWorkerDetailState } from "../hooks/use-worker-detail-state";
import type { WorkerDetailCardProps } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";
import { WorkerTopologyDeleteSection } from "./worker-topology-delete-section";

export function WorkerDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  onSaveConfiguration,
  saveState,
  widgetId = "current-selection",
  workerName,
}: WorkerDetailCardProps) {
  const messages = getWorkerDetailMessages(locale);
  const detailState = useWorkerDetailState(workerName);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      {editableConfigurationState?.status !== "ready" ? (
        <p className={WIDGET_SUBTITLE_CLASS}>{workerName}</p>
      ) : null}
      {detailState.status === "loading" ? (
        <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.configurationLoading}
        </p>
      ) : null}
      {detailState.status === "error" ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.configurationErrorPrefix} {detailState.errorMessage}
        </p>
      ) : null}
      {detailState.status === "empty" ? (
        <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.configurationEmpty}
        </p>
      ) : null}
      {detailState.status === "ready" ? (
        <>
          <WorkerTopologyDeleteSection
            messages={messages}
            workerName={workerName}
          />
          <WorkerEditableConfigurationSection
            messages={messages}
            onSaveConfiguration={onSaveConfiguration}
            saveState={saveState}
            state={editableConfigurationState}
            workerName={workerName}
          />
        </>
      ) : null}
    </SelectionDetailLayout>
  );
}
