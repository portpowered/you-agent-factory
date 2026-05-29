// biome-ignore lint/nursery/noExcessiveLinesPerFile: worker editable fields, validation feedback, and shared-impact warnings stay colocated with save gating until story 006 adds save wiring.
import { type ReactNode, useId, useState } from "react";

import {
  DashboardActionButton,
  DashboardActionRow,
  DisclosureButton,
  Select,
} from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { formatList } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import {
  EDITABLE_EXECUTOR_PROVIDERS,
  EDITABLE_HOSTED_PROVIDERS,
  EDITABLE_MODEL_LOCALITIES,
  EDITABLE_MODEL_PROVIDERS,
  EDITABLE_WORKER_TYPES,
} from "../../../current-factory-definition/lib/worker-editable-values";
import {
  CurrentSelectionSectionHeader,
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_WARNING_PANEL_CLASS,
  HISTORY_TOGGLE_CLASS,
} from "../../base/components/detail-card-shared";
import type {
  EditableWorkerConfigurationState,
  WorkerDetailCardProps,
} from "../lib/detail-card-types";
import type { getWorkerDetailMessages } from "../messages/worker-detail";

export function WorkerEditableConfigurationSection({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state?: WorkerDetailCardProps["editableConfigurationState"];
}) {
  const [expanded, setExpanded] = useState(true);
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
          <DisclosureButton
            aria-label={
              expanded
                ? messages.editableConfigurationCollapseActionLabel
                : messages.editableConfigurationExpandActionLabel
            }
            className={HISTORY_TOGGLE_CLASS}
            controlsID={contentId}
            expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
            type="button"
          >
            {expanded ? messages.collapseAction : messages.expandAction}
          </DisclosureButton>
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
            <WorkerEditableConfigurationReadyForm
              messages={messages}
              state={state}
              workerName={state.initialValues.workerName}
            />
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function WorkerEditableConfigurationReadyForm({
  messages,
  state,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  workerName: string;
}) {
  const { validationErrors } = state;

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <WorkerEditableConfigurationSharedImpactWarning
        messages={messages}
        state={state}
        workerName={workerName}
      />
      {validationErrors.contract ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {validationErrors.contract}
        </p>
      ) : null}
      <WorkerEditableConfigurationDraftStatus messages={messages} state={state} />
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <WorkerEditableConfigurationField
          errorMessage={validationErrors.type}
          fieldId="editable-worker-type"
          input={
            <Select
              aria-describedby={
                validationErrors.type ? "editable-worker-type-error" : undefined
              }
              aria-invalid={validationErrors.type ? "true" : undefined}
              aria-label={messages.typeFieldLabel}
              id="editable-worker-type"
              onChange={(event) =>
                state.onTypeChange(
                  event.target.value as typeof state.draft.type,
                )
              }
              value={state.draft.type}
            >
              {EDITABLE_WORKER_TYPES.map((workerType) => (
                <option key={workerType} value={workerType}>
                  {messages.localizeWorkerType(workerType)}
                </option>
              ))}
            </Select>
          }
          label={messages.typeFieldLabel}
        />
        <WorkerTypeSpecificFields
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
      </div>
      <DashboardActionRow
        actions={
          <>
            <DashboardActionButton disabled={!state.canSave} type="button">
              {messages.editableConfigurationSaveAction}
            </DashboardActionButton>
            {state.isDirty ? (
              <DashboardActionButton onClick={state.onResetToLatest} type="button">
                {messages.discardDraftAction}
              </DashboardActionButton>
            ) : null}
          </>
        }
      />
    </form>
  );
}

function WorkerEditableConfigurationSharedImpactWarning({
  messages,
  state,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  workerName: string;
}) {
  const workstationNames = state.initialValues.workstationNames;
  if (workstationNames.length <= 1) {
    return null;
  }

  return (
    <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
      <p
        className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationSharedImpactWarning(
          workerName,
          formatList(workstationNames),
        )}
      </p>
      <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationSharedImpactWarningDetail}
      </p>
    </div>
  );
}

function WorkerEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
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
      {state.hasValidationErrors ? (
        <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {messages.editableConfigurationSaveDisabledValidationDetail}
        </p>
      ) : (
        <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {messages.editableConfigurationDraftNote}
        </p>
      )}
    </div>
  );
}

function WorkerTypeSpecificFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  validationErrors: Extract<
    EditableWorkerConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}) {
  if (state.draft.type === "MODEL_WORKER") {
    return (
      <ModelWorkerEditableFields
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    );
  }

  if (state.draft.type === "SCRIPT_WORKER") {
    return (
      <ScriptWorkerEditableFields
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    );
  }

  return (
    <HostedWorkerEditableFields
      messages={messages}
      state={state}
      validationErrors={validationErrors}
    />
  );
}

function ModelWorkerEditableFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  validationErrors: Extract<
    EditableWorkerConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}) {
  return (
    <>
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.modelProvider}
        fieldId="editable-worker-model-provider"
        input={
          <WorkerOptionalEnumSelect
            ariaDescribedBy={
              validationErrors.modelProvider
                ? "editable-worker-model-provider-error"
                : undefined
            }
            ariaInvalid={Boolean(validationErrors.modelProvider)}
            id="editable-worker-model-provider"
            label={messages.modelProviderLabel}
            notConfiguredLabel={messages.notConfiguredOptionLabel}
            onChange={state.onModelProviderChange}
            options={EDITABLE_MODEL_PROVIDERS}
            renderLabel={messages.localizeModelProvider}
            value={state.draft.modelProvider}
          />
        }
        label={messages.modelProviderLabel}
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.model}
        fieldId="editable-worker-model"
        input={
          <input
            aria-describedby={
              validationErrors.model ? "editable-worker-model-error" : undefined
            }
            aria-invalid={validationErrors.model ? "true" : undefined}
            className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
            id="editable-worker-model"
            onChange={(event) => state.onModelChange(event.target.value)}
            type="text"
            value={state.draft.model}
          />
        }
        label={messages.modelLabel}
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.modelLocality}
        fieldId="editable-worker-model-locality"
        input={
          <WorkerOptionalEnumSelect
            ariaDescribedBy={
              validationErrors.modelLocality
                ? "editable-worker-model-locality-error"
                : undefined
            }
            ariaInvalid={Boolean(validationErrors.modelLocality)}
            id="editable-worker-model-locality"
            label={messages.modelLocalityLabel}
            notConfiguredLabel={messages.notConfiguredOptionLabel}
            onChange={state.onModelLocalityChange}
            options={EDITABLE_MODEL_LOCALITIES}
            renderLabel={messages.localizeModelLocality}
            value={state.draft.modelLocality}
          />
        }
        label={messages.modelLocalityLabel}
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.executorProvider}
        fieldId="editable-worker-executor-provider"
        input={
          <WorkerOptionalEnumSelect
            ariaDescribedBy={
              validationErrors.executorProvider
                ? "editable-worker-executor-provider-error"
                : undefined
            }
            ariaInvalid={Boolean(validationErrors.executorProvider)}
            id="editable-worker-executor-provider"
            label={messages.executorProviderLabel}
            notConfiguredLabel={messages.notConfiguredOptionLabel}
            onChange={state.onExecutorProviderChange}
            options={EDITABLE_EXECUTOR_PROVIDERS}
            renderLabel={messages.localizeExecutorProvider}
            value={state.draft.executorProvider}
          />
        }
        label={messages.executorProviderLabel}
      />
    </>
  );
}

function ScriptWorkerEditableFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  validationErrors: Extract<
    EditableWorkerConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}) {
  return (
    <>
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.command}
        fieldId="editable-worker-command"
        input={
          <input
            aria-describedby={
              validationErrors.command ? "editable-worker-command-error" : undefined
            }
            aria-invalid={validationErrors.command ? "true" : undefined}
            className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
            id="editable-worker-command"
            onChange={(event) => state.onCommandChange(event.target.value)}
            type="text"
            value={state.draft.command}
          />
        }
        label={messages.commandFieldLabel}
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.args}
        fieldId="editable-worker-args"
        input={
          <textarea
            aria-describedby={
              validationErrors.args ? "editable-worker-args-error" : undefined
            }
            aria-invalid={validationErrors.args ? "true" : undefined}
            className="min-h-24 w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
            id="editable-worker-args"
            onChange={(event) => state.onArgsTextChange(event.target.value)}
            value={state.draft.argsText}
          />
        }
        label={messages.argsFieldLabel}
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.body}
        fieldId="editable-worker-body"
        input={
          <textarea
            aria-describedby={
              validationErrors.body ? "editable-worker-body-error" : undefined
            }
            aria-invalid={validationErrors.body ? "true" : undefined}
            className="min-h-32 w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
            id="editable-worker-body"
            onChange={(event) => state.onBodyChange(event.target.value)}
            value={state.draft.body}
          />
        }
        label={messages.bodyFieldLabel}
      />
    </>
  );
}

function HostedWorkerEditableFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  validationErrors: Extract<
    EditableWorkerConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}) {
  return (
    <WorkerEditableConfigurationField
      errorMessage={validationErrors.provider}
      fieldId="editable-worker-provider"
      input={
        <WorkerOptionalEnumSelect
          ariaDescribedBy={
            validationErrors.provider ? "editable-worker-provider-error" : undefined
          }
          ariaInvalid={Boolean(validationErrors.provider)}
          id="editable-worker-provider"
          label={messages.providerFieldLabel}
          notConfiguredLabel={messages.notConfiguredOptionLabel}
          onChange={state.onProviderChange}
          options={EDITABLE_HOSTED_PROVIDERS}
          renderLabel={(value) => value}
          value={state.draft.provider}
        />
      }
      label={messages.providerFieldLabel}
    />
  );
}

function WorkerOptionalEnumSelect<T extends string>({
  ariaDescribedBy,
  ariaInvalid,
  id,
  label,
  notConfiguredLabel,
  onChange,
  options,
  renderLabel,
  value,
}: {
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  id: string;
  label: string;
  notConfiguredLabel: string;
  onChange: (value: T | null) => void;
  options: readonly T[];
  renderLabel: (value: string) => string;
  value: T | null;
}) {
  return (
    <Select
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid ? "true" : undefined}
      aria-label={label}
      id={id}
      onChange={(event) => {
        const nextValue = event.target.value;
        onChange(nextValue.length > 0 ? (nextValue as T) : null);
      }}
      value={value ?? ""}
    >
      <option value="">{notConfiguredLabel}</option>
      {options.map((option) => (
        <option key={option} value={option}>
          {renderLabel(option)}
        </option>
      ))}
    </Select>
  );
}

function WorkerEditableConfigurationField({
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
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
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
