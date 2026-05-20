import { type ReactNode, useId, useState } from "react";

import { Select } from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { formatList } from "../../components/ui/formatters";
import { cn } from "../../lib/cn";
import {
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "./detail-card-shared";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
  WorkstationDetailCardProps,
  WorkstationSummaryItemProps,
  WorkstationSummaryProps,
} from "./detail-card-types";
import { formatEditableOverwriteFieldLabels } from "./editable-workstation-overwrite-fields";
import type { getWorkstationDetailMessages } from "./messages";
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
      <div className={HISTORY_HEADER_CLASS}>
        <div className="min-w-0">
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={headingId}>
            {messages.editableConfigurationHeading}
          </h4>
        </div>
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
      </div>
      {expanded ? (
        <div className="grid gap-2.5" id={contentId}>
          {state?.status === "loading" ? (
            <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
              {messages.editableConfigurationLoading}
            </p>
          ) : null}
          {state?.status === "error" ? (
            <p
              className={cn("m-0 text-af-danger", DASHBOARD_BODY_TEXT_CLASS)}
              role="alert"
            >
              {messages.editableConfigurationErrorPrefix} {state.errorMessage}
            </p>
          ) : null}
          {state?.status === "empty" ? (
            <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
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
          errorMessage={state.validationErrors.workerName}
          input={
            <EditableConfigurationWorkerInput
              messages={messages}
              state={state}
            />
          }
          label={messages.workerFieldLabel}
        />
      </div>
      <EditableConfigurationField
        errorMessage={state.validationErrors.prompt}
        fieldId="editable-workstation-prompt"
        input={
          <EditableConfigurationPromptInput
            messages={messages}
            state={state}
          />
        }
        label={messages.promptFieldLabel}
        supportingContent={
          <EditableConfigurationPromptHelp messages={messages} state={state} />
        }
      />
    </form>
  );
}

function EditableConfigurationPromptHelp({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const helpState = state.promptHelpState;

  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p
          className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}
        >
          {messages.editableConfigurationPromptHelpHeading}
        </p>
        <button
          aria-controls={contentId}
          aria-expanded={expanded}
          className="inline-flex min-h-9 items-center rounded-lg border border-af-accent/24 bg-af-accent/10 px-3 py-2 text-xs font-semibold text-af-accent outline-none transition-colors hover:bg-af-accent/16 hover:text-af-accent-glow focus-visible:ring-2 focus-visible:ring-af-accent/25"
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded
            ? messages.editableConfigurationPromptHelpCloseActionLabel
            : messages.editableConfigurationPromptHelpOpenActionLabel}
        </button>
      </div>
      {expanded ? (
        <div
          className="grid gap-3 rounded-2xl border border-af-overlay/10 bg-af-surface/72 p-3"
          id={contentId}
        >
          {helpState.status === "loading" ? (
            <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
              {messages.editableConfigurationPromptHelpLoading}
            </p>
          ) : null}
          {helpState.status === "error" ? (
            <p
              className={cn(
                "m-0 text-af-danger-ink",
                DASHBOARD_BODY_TEXT_CLASS,
              )}
              role="alert"
            >
              {messages.editableConfigurationPromptHelpErrorPrefix}{" "}
              {helpState.errorMessage}
            </p>
          ) : null}
          {helpState.status === "empty" ? (
            <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
              {helpState.message}
            </p>
          ) : null}
          {helpState.status === "ready" ? (
            <>
              <p
                className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}
              >
                {messages.editableConfigurationPromptHelpInputCountSummary(
                  helpState.contract.inputCount,
                )}
              </p>
              <PromptTemplateReferenceList
                heading={
                  messages.editableConfigurationPromptHelpAvailableHeading
                }
                items={helpState.contract.availableVariables.map(
                  (variable) => ({
                    body: variable.description,
                    detail: variable.example,
                    key: `${variable.path}:${variable.example}`,
                    label: variable.path,
                  }),
                )}
              />
              <PromptTemplateReferenceList
                heading={
                  messages.editableConfigurationPromptHelpUnavailableHeading
                }
                items={helpState.contract.unavailableAccessPatterns.map(
                  (pattern) => ({
                    body: pattern.reason,
                    detail: pattern.example,
                    key: `${pattern.path}:${pattern.example}`,
                    label: pattern.path,
                  }),
                )}
              />
            </>
          ) : null}
        </div>
      ) : null}
    </div>
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
        className={cn("m-0 text-af-success-ink", DASHBOARD_BODY_TEXT_CLASS)}
        role="status"
      >
        {messages.editableConfigurationSaveSuccess}
      </p>
    );
  }

  if (saveState?.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationSaveErrorPrefix} {saveState.errorMessage}
      </p>
    );
  }

  return null;
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
    <div className="grid gap-2 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3">
      <p
        className={cn(
          "m-0",
          state.hasValidationErrors ? "text-af-danger-ink" : "text-af-ink/72",
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
      <p className={cn("m-0 text-af-ink/58", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
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
    <div className="grid gap-2 rounded-2xl border border-af-warning/35 bg-af-warning/8 p-3">
      <p
        className={cn("m-0 text-af-warning-ink", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </p>
      <p className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
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
      <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
        {state.workerOptionsState.message}
      </p>
    );
  }

  if (state.workerOptionsState.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}
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

export function WorkstationSummary({
  activeRunCount,
  historyCount,
  historyLabel,
  messages,
  selectedNode,
}: WorkstationSummaryProps) {
  return (
    <section className="mt-4 grid gap-2.5 [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.summaryHeading}
      </h4>
      <ul className="m-0 grid list-none gap-2 p-0 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        <WorkstationSummaryItem
          label={messages.workerTypeLabel}
          value={selectedNode.worker_type || messages.unknownWorkerTypeValue}
        />
        <WorkstationSummaryItem
          label={messages.kindLabel}
          value={selectedNode.workstation_kind || messages.kindDefaultValue}
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
    <div className="grid gap-2 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3">
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <p
          className={cn(
            "m-0 text-af-danger-ink",
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

function PromptTemplateReferenceList({
  heading,
  items,
}: {
  heading: string;
  items: Array<{
    body: string;
    detail: string;
    key: string;
    label: string;
  }>;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <section className="grid gap-2">
      <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>{heading}</h5>
      <ul className="m-0 grid list-none gap-2 p-0">
        {items.map((item) => (
          <li
            className="grid gap-1 rounded-xl border border-af-overlay/8 bg-af-overlay/4 p-3"
            key={item.key}
          >
            <code className={DASHBOARD_BODY_TEXT_CLASS}>{item.label}</code>
            <p className={cn("m-0 text-af-ink/72", DASHBOARD_BODY_TEXT_CLASS)}>
              {item.body}
            </p>
            <pre className="m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 text-xs text-af-ink/78 [overflow-wrap:anywhere]">
              {item.detail}
            </pre>
          </li>
        ))}
      </ul>
    </section>
  );
}

function WorkstationSummaryItem({ label, value }: WorkstationSummaryItemProps) {
  return (
    <li className={WORKSTATION_SUMMARY_ITEM_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <strong className="min-w-0 text-sm text-af-ink [overflow-wrap:anywhere]">
        {value}
      </strong>
    </li>
  );
}

function valueOrFallback(value: string | null, fallback: string) {
  return value && value.trim().length > 0 ? value : fallback;
}
