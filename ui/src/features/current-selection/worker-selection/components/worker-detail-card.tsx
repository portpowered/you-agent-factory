import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import { CurrentSelectionDetailFeedback } from "../../base/components/detail/current-selection-detail-feedback";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionBodyLayout } from "../../base/components/layout/current-selection-body-layout";
import { useWorkerDetailState } from "../hooks/use-worker-detail-state";
import type { WorkerDetailCardProps } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./editable/worker-editable-configuration-section";

export function WorkerDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  saveState,
  widgetId = "current-selection",
  worker,
  workerName,
  workstationNames,
}: WorkerDetailCardProps) {
  const messages = getWorkerDetailMessages(locale);
  const detailState = useWorkerDetailState({ worker, workstationNames });

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={workerName}>
        {detailState.status === "ready" ? (
          <WorkerEditableConfigurationSection
            messages={messages}
            saveState={saveState}
            state={editableConfigurationState}
            workerName={workerName}
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
        )}
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
