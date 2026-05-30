import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import {
  CurrentSelectionSectionHeader,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "../../base/components/detail-card-shared";
import { useWorkerDetailState } from "../hooks/use-worker-detail-state";
import type { WorkerDetailCardProps } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";

export function WorkerDetailCard({
  editableConfigurationState,
  locale,
  onSaveWorker,
  saveState,
  widgetId = "current-selection",
  workerName,
}: WorkerDetailCardProps) {
  const messages = getWorkerDetailMessages(locale);
  const detailState = useWorkerDetailState(workerName);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{workerName}</p>
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
          <WorkerSummary
            messages={messages}
            worker={detailState.worker}
            workerName={workerName}
          />
          <WorkerReferencingWorkstations
            messages={messages}
            workstationNames={detailState.workstationNames}
          />
          <WorkerEditableConfigurationSection
            messages={messages}
            onSaveWorker={onSaveWorker}
            saveState={saveState}
            state={editableConfigurationState}
            workerName={workerName}
          />
        </>
      ) : null}
    </SelectionDetailLayout>
  );
}

function WorkerSummary({
  messages,
  worker,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  worker: Extract<
    ReturnType<typeof useWorkerDetailState>,
    { status: "ready" }
  >["worker"];
  workerName: string;
}) {
  const sectionId = `worker-summary-${workerName}`;

  return (
    <section
      aria-labelledby={sectionId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        headingId={sectionId}
        title={messages.summaryHeading}
      />
      <ul className="m-0 grid list-none gap-2 p-0 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        <WorkerSummaryItem
          label={messages.typeLabel}
          value={messages.localizeWorkerType(
            worker.type ?? messages.unknownTypeValue,
          )}
        />
        {worker.modelProvider ? (
          <WorkerSummaryItem
            label={messages.modelProviderLabel}
            value={messages.localizeModelProvider(worker.modelProvider)}
          />
        ) : null}
        {worker.model ? (
          <WorkerSummaryItem label={messages.modelLabel} value={worker.model} />
        ) : null}
        {worker.executorProvider ? (
          <WorkerSummaryItem
            label={messages.executorProviderLabel}
            value={messages.localizeExecutorProvider(worker.executorProvider)}
          />
        ) : null}
      </ul>
    </section>
  );
}

function WorkerReferencingWorkstations({
  messages,
  workstationNames,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  workstationNames: string[];
}) {
  const sectionId = "worker-referencing-workstations";

  return (
    <section
      aria-labelledby={sectionId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={sectionId}>
        {messages.referencingWorkstationsHeading}
      </h4>
      {workstationNames.length > 0 ? (
        <ul className="m-0 grid list-none gap-2 p-0">
          {workstationNames.map((workstationName) => (
            <li
              className="rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2"
              key={workstationName}
            >
              <span className={DASHBOARD_BODY_TEXT_CLASS}>
                {workstationName}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p
          className={cn(
            "m-0 text-af-text-muted",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.referencingWorkstationsEmpty}
        </p>
      )}
    </section>
  );
}

function WorkerSummaryItem({
  label,
  value,
}: {
  label: string;
  value: string | number;
}) {
  return (
    <li className={WORKSTATION_SUMMARY_ITEM_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <span className={DASHBOARD_BODY_TEXT_CLASS}>{value}</span>
    </li>
  );
}
