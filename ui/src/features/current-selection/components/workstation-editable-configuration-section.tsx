// biome-ignore lint/nursery/noExcessiveLinesPerFile: current-selection editable workstation fields stay colocated so save feedback, overwrite hints, and responsive form structure evolve together.
import { type ReactNode, useId, useState } from "react";

import {
  DashboardActionButton,
  DashboardActionRow,
  Select,
} from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { formatList } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  CurrentSelectionSectionHeader,
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_WARNING_PANEL_CLASS,
  HISTORY_TOGGLE_CLASS,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "./detail-card-shared";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
  EditableWorkstationValidationErrors,
  WorkstationDetailCardProps,
  WorkstationSummaryItemProps,
  WorkstationSummaryProps,
} from "./detail-card-types";
import { formatEditableOverwriteFieldLabels } from "../editing/editable-workstation-overwrite-fields";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";
import {
  EditableConfigurationRunnerField,
  resolveWorkstationSummaryRunnerValue,
} from "./workstation-runner-field";
import { EditableConfigurationPromptInput } from "./workstation-prompt-field";

export function EditableConfigurationSection({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  saveState?: EditableWorkstationSaveState;
  state?: WorkstationDetailCardProps["editableConfigurationState"];
}) {
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <section
      aria-labelledby={headingId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        action={
          <button
            aria-label={
              expanded
                ? messages.editableConfigurationCollapseActionLabel
                : messages.editableConfigurationExpandActionLabel
            }
            aria-controls={contentId}
            aria-expanded={expanded}
            className={HISTORY_TOGGLE_CLASS}
            onClick={() => setExpanded((current) => !current)}
            type="button"
          >
            {expanded ? messages.collapseAction : messages.expandAction}
          </button>
        }
        headingId={headingId}
        title={messages.editableConfigurationHeading}
      />
      {expanded ? (
        <div className="grid gap-2.5" id={contentId}>
          {state?.status === "loading" ? (
            <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
              {messages.editableConfigurationLoading}
            </p>
          ) : null}
          {state?.status === "error" ? (
            <p
              className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
              role="alert"
            >
              {messages.editableConfigurationErrorPrefix} {state.errorMessage}
            </p>
          ) : null}
          {state?.status === "empty" ? (
            <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
              {state.message || messages.editableConfigurationEmpty}
            </p>
          ) : null}
          {state?.status === "ready" ? (
            <EditableConfigurationReadyForm
              messages={messages}
              saveState={saveState}
              state={state}
            />
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function EditableConfigurationReadyForm({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  saveState?: EditableWorkstationSaveState;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const validationErrors = mergeEditableValidationErrors(
    state.validationErrors,
    saveState,
  );
  const renderState = {
    ...state,
    validationErrors,
  };

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <EditableConfigurationSaveFeedback
        messages={messages}
        saveState={saveState}
      />
      <EditableConfigurationOverwriteWarning
        messages={messages}
        overwriteFieldNames={state.overwriteFieldNames ?? []}
      />
      <EditableConfigurationDraftStatus messages={messages} state={state} />
      <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(13rem,1fr))]">
        <EditableConfigurationField
          fieldId="editable-workstation-worker"
          errorMessage={validationErrors.workerName}
          input={
            <EditableConfigurationWorkerInput
              messages={messages}
              state={renderState}
            />
          }
          label={messages.workerFieldLabel}
          supportingContent={
            <>
              <EditableConfigurationSharedWorkerHint
                messages={messages}
                state={state}
              />
              <EditableConfigurationServerChangedHint
                fieldName="worker"
                messages={messages}
                state={state}
              />
            </>
          }
        />
        <EditableConfigurationField
          fieldId="editable-workstation-kind"
          errorMessage={validationErrors.behavior}
          input={
            <EditableConfigurationBehaviorInput
              messages={messages}
              state={renderState}
            />
          }
          label={messages.kindLabel}
          supportingContent={
            <EditableConfigurationServerChangedHint
              fieldName="behavior"
              messages={messages}
              state={state}
            />
          }
        />
        <EditableConfigurationField
          fieldId="editable-workstation-runner"
          errorMessage={validationErrors.runnerName}
          input={
            <EditableConfigurationRunnerField
              messages={messages}
              state={renderState}
            />
          }
          label={messages.runnerFieldLabel}
          supportingContent={
            <EditableConfigurationServerChangedHint
              fieldName="runner"
              messages={messages}
              state={state}
            />
          }
        />
      </div>
      <EditableConfigurationField
        errorMessage={validationErrors.prompt}
        fieldId="editable-workstation-prompt"
        input={
          <EditableConfigurationPromptInput
            messages={messages}
            state={renderState}
          />
        }
        label={messages.promptFieldLabel}
        supportingContent={
          <EditableConfigurationServerChangedHint
            fieldName="prompt"
            messages={messages}
            state={state}
          />
        }
      />
      {state.isDirty ? (
        <DashboardActionRow
          actions={
            <DashboardActionButton
              disabled={saveState?.status === "submitting"}
              onClick={state.onResetToLatest}
              type="button"
            >
              {messages.editableConfigurationResetAction}
            </DashboardActionButton>
          }
        />
      ) : null}
    </form>
  );
}


function EditableConfigurationSaveFeedback({
  messages,
  saveState,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  saveState?: EditableWorkstationSaveState;
}) {
  if (saveState?.status === "success") {
    return (
      <p
        className={cn("m-0 text-af-success-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="status"
      >
        {messages.editableConfigurationSaveSuccess}
      </p>
    );
  }

  if (saveState?.status === "warning") {
    return (
      <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
        <p
          className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {saveState.message}
        </p>
        <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {messages.editableConfigurationSaveStaleVersionDetail}
        </p>
      </div>
    );
  }

  if (saveState?.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationSaveErrorPrefix} {saveState.errorMessage}
      </p>
    );
  }

  return null;
}

function mergeEditableValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
  saveState?: EditableWorkstationSaveState,
): EditableWorkstationValidationErrors {
  if (saveState?.status !== "error" || !saveState.fieldErrors) {
    return validationErrors;
  }

  return {
    ...validationErrors,
    ...saveState.fieldErrors,
  };
}

function EditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <p
        className={cn(
          "m-0",
          state.hasValidationErrors ? "text-af-danger-text" : "text-af-text-muted",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
        role={state.hasValidationErrors ? "alert" : "status"}
      >
        {state.hasValidationErrors
          ? messages.editableConfigurationValidationStatus
          : state.isDirty
            ? messages.editableConfigurationDirtyStatus
            : messages.editableConfigurationDraftNote}
      </p>
      <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationDraftNote}
      </p>
    </div>
  );
}

function EditableConfigurationOverwriteWarning({
  messages,
  overwriteFieldNames,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  overwriteFieldNames?: EditableWorkstationOverwriteField[];
}) {
  if (!overwriteFieldNames || overwriteFieldNames.length === 0) {
    return null;
  }

  const formattedFields = formatEditableOverwriteFieldLabels(
    overwriteFieldNames,
    messages,
  );

  return (
    <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
      <p
        className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </p>
      <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationOverwriteWarningDetail}
      </p>
    </div>
  );
}

function EditableConfigurationWorkerInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (state.workerOptionsState.status === "empty") {
    return (
      <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
        {state.workerOptionsState.message}
      </p>
    );
  }

  if (state.workerOptionsState.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationWorkerUnavailablePrefix}{" "}
        {state.workerOptionsState.message}
      </p>
    );
  }

  return (
    <Select
      aria-describedby={
        state.validationErrors.workerName
          ? "editable-workstation-worker-error"
          : undefined
      }
      aria-invalid={state.validationErrors.workerName ? "true" : undefined}
      className={DASHBOARD_BODY_TEXT_CLASS}
      id="editable-workstation-worker"
      onChange={(event) => state.onWorkerChange(event.target.value)}
      value={state.draft.workerName}
    >
      {state.workerOptionsState.options.map((workerName) => (
        <option key={workerName} value={workerName}>
          {valueOrFallback(workerName, messages.notConfiguredValue)}
        </option>
      ))}
    </Select>
  );
}

function EditableConfigurationBehaviorInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  return (
    <Select
      aria-describedby={
        state.validationErrors.behavior
          ? "editable-workstation-kind-error"
          : undefined
      }
      aria-invalid={state.validationErrors.behavior ? "true" : undefined}
      className={DASHBOARD_BODY_TEXT_CLASS}
      id="editable-workstation-kind"
      onChange={(event) =>
        state.onBehaviorChange(event.target.value as typeof state.draft.behavior)
      }
      value={state.draft.behavior}
    >
      {state.initialValues.behaviorOptions.map((behavior) => (
        <option key={behavior} value={behavior}>
          {messages.localizeWorkstationBehavior(behavior)}
        </option>
      ))}
    </Select>
  );
}

function EditableConfigurationSharedWorkerHint({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const sharedWorkstationNames =
    state.draft.workerName === state.initialValues.workerName
      ? state.initialValues.sharedWorkerWorkstationNames
      : [];
  if (sharedWorkstationNames.length === 0) {
    return null;
  }

  return (
    <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
      {messages.editableConfigurationSharedWorkerScopeHint(
        valueOrFallback(state.draft.workerName, messages.notConfiguredValue),
        formatList(sharedWorkstationNames),
      )}
    </p>
  );
}

function EditableConfigurationServerChangedHint({
  fieldName,
  messages,
  state,
}: {
  fieldName: EditableWorkstationOverwriteField;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (!state.overwriteFieldNames.includes(fieldName)) {
    return null;
  }

  return (
    <p
      className={cn(
        "m-0 text-af-warning-text",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
    >
      {messages.editableConfigurationServerFieldChangedHint}
    </p>
  );
}

export function WorkstationSummary({
  activeRunCount,
  editableConfigurationState,
  historyCount,
  historyLabel,
  messages,
  selectedNode,
}: WorkstationSummaryProps) {
  const sectionId = `workstation-summary-${selectedNode.node_id}`;

  return (
    <section aria-labelledby={sectionId} className="mt-4 grid gap-2.5 [&_h4]:m-0">
      <CurrentSelectionSectionHeader
        headingId={sectionId}
        title={messages.summaryHeading}
      />
      <ul className="m-0 grid list-none gap-2 p-0 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        <WorkstationSummaryItem
          label={messages.workerTypeLabel}
          value={selectedNode.worker_type || messages.unknownWorkerTypeValue}
        />
        <WorkstationSummaryItem
          label={messages.selectedRunnerLabel}
          value={resolveWorkstationSummaryRunnerValue(
            editableConfigurationState,
            messages,
          )}
        />
        <WorkstationSummaryItem
          label={messages.kindLabel}
          value={messages.localizeWorkstationKind(
            selectedNode.workstation_kind || messages.kindDefaultValue,
          )}
        />
        <WorkstationSummaryItem
          label={messages.inputWorkTypesLabel}
          value={formatList(selectedNode.input_work_type_ids)}
        />
        <WorkstationSummaryItem
          label={messages.outputWorkTypesLabel}
          value={formatList(selectedNode.output_work_type_ids)}
        />
        <WorkstationSummaryItem
          label={messages.activeRunsLabel}
          value={activeRunCount}
        />
        <WorkstationSummaryItem label={historyLabel} value={historyCount} />
      </ul>
    </section>
  );
}

function EditableConfigurationField({
  errorMessage,
  fieldId,
  input,
  label,
  supportingContent,
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
  supportingContent?: ReactNode;
}) {
  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <p
          className={cn(
            "m-0 text-af-danger-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
          id={`${fieldId}-error`}
        >
          {errorMessage}
        </p>
      ) : null}
    </div>
  );
}

function WorkstationSummaryItem({ label, value }: WorkstationSummaryItemProps) {
  return (
    <li className={WORKSTATION_SUMMARY_ITEM_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <strong className="min-w-0 text-sm text-af-text [overflow-wrap:anywhere]">
        {value}
      </strong>
    </li>
  );
}

function valueOrFallback(value: string | null, fallback: string) {
  return value && value.trim().length > 0 ? value : fallback;
}
