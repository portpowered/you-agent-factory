import type { ReactNode } from "react";

import { Input, Textarea } from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { formatList } from "../../components/ui/formatters";
import { cx } from "../../lib/cx";
import { WORKSTATION_SUMMARY_ITEM_CLASS } from "./detail-card-shared";
import type {
  WorkstationDetailCardProps,
  WorkstationSummaryItemProps,
  WorkstationSummaryProps,
} from "./detail-card-types";
import type { getWorkstationDetailMessages } from "./messages";

export function EditableConfigurationSection({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state?: WorkstationDetailCardProps["editableConfigurationState"];
}) {
  return (
    <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <div className="grid gap-[0.18rem]">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.editableConfigurationHeading}
        </h4>
        <p
          className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}
        >
          {messages.editableConfigurationSummary}
        </p>
      </div>
      {state?.status === "loading" ? (
        <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.editableConfigurationLoading}
        </p>
      ) : null}
      {state?.status === "error" ? (
        <p
          className={cx("m-0 text-af-danger", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.editableConfigurationErrorPrefix} {state.errorMessage}
        </p>
      ) : null}
      {state?.status === "empty" ? (
        <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
          {state.message || messages.editableConfigurationEmpty}
        </p>
      ) : null}
      {state?.status === "ready" ? (
        <form
          className="grid gap-3"
          onSubmit={(event) => event.preventDefault()}
        >
          <div className="grid gap-2 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3">
            <p
              className={cx(
                "m-0",
                state.hasValidationErrors
                  ? "text-af-danger-ink"
                  : "text-af-ink/72",
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
            <p
              className={cx(
                "m-0 text-af-ink/58",
                DASHBOARD_SUPPORTING_TEXT_CLASS,
              )}
            >
              {messages.editableConfigurationDraftNote}
            </p>
          </div>

          <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(13rem,1fr))]">
            <EditableConfigurationField
              errorMessage={state.validationErrors.model}
              fieldId="editable-workstation-model"
              input={
                <Input
                  aria-describedby={
                    state.validationErrors.model
                      ? "editable-workstation-model-error"
                      : undefined
                  }
                  aria-invalid={
                    state.validationErrors.model ? "true" : undefined
                  }
                  className={DASHBOARD_BODY_TEXT_CLASS}
                  id="editable-workstation-model"
                  onChange={(event) => state.onModelChange(event.target.value)}
                  value={state.draft.model}
                />
              }
              label={messages.modelFieldLabel}
            />
            <EditableConfigurationField
              errorMessage={state.validationErrors.promptFile}
              fieldId="editable-workstation-template"
              input={
                <Input
                  aria-describedby={
                    state.validationErrors.promptFile
                      ? "editable-workstation-template-error"
                      : undefined
                  }
                  aria-invalid={
                    state.validationErrors.promptFile ? "true" : undefined
                  }
                  className={DASHBOARD_BODY_TEXT_CLASS}
                  id="editable-workstation-template"
                  onChange={(event) =>
                    state.onPromptFileChange(event.target.value)
                  }
                  placeholder={messages.notConfiguredValue}
                  value={state.draft.promptFile}
                />
              }
              label={messages.templateFieldLabel}
            />
          </div>

          <dl className="m-0 grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(11rem,1fr))]">
            <EditableConfigurationItem
              label={messages.workerFieldLabel}
              value={valueOrFallback(
                state.initialValues.workerName,
                messages.notConfiguredValue,
              )}
            />
          </dl>

          <EditableConfigurationField
            errorMessage={state.validationErrors.prompt}
            fieldId="editable-workstation-prompt"
            input={
              <Textarea
                aria-describedby={
                  state.validationErrors.prompt
                    ? "editable-workstation-prompt-error"
                    : undefined
                }
                aria-invalid={
                  state.validationErrors.prompt ? "true" : undefined
                }
                className={DASHBOARD_BODY_TEXT_CLASS}
                id="editable-workstation-prompt"
                onChange={(event) => state.onPromptChange(event.target.value)}
                value={state.draft.prompt}
              />
            }
            label={messages.promptFieldLabel}
          />
        </form>
      ) : null}
    </section>
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
    <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
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
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
}) {
  return (
    <div className="grid gap-2 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3">
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {errorMessage ? (
        <p
          className={cx(
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

function EditableConfigurationItem({
  className,
  label,
  preserveWhitespace = false,
  value,
}: {
  className?: string;
  label: string;
  preserveWhitespace?: boolean;
  value: string;
}) {
  return (
    <div
      className={cx(
        "grid gap-[0.3rem] rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3",
        className,
      )}
    >
      <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</dt>
      <dd
        className={cx(
          "m-0 text-sm text-af-ink [overflow-wrap:anywhere]",
          preserveWhitespace && "whitespace-pre-wrap",
        )}
      >
        {value}
      </dd>
    </div>
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
