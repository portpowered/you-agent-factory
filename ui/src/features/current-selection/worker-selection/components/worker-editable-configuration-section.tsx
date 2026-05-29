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
} from "../../../../components/ui/dashboard-typography";
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
            <WorkerEditableConfigurationReadyForm messages={messages} state={state} />
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function WorkerEditableConfigurationReadyForm({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <WorkerEditableConfigurationField
          fieldId="editable-worker-type"
          input={
            <Select
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
        <WorkerTypeSpecificFields messages={messages} state={state} />
      </div>
      {state.isDirty ? (
        <DashboardActionRow
          actions={
            <DashboardActionButton onClick={state.onResetToLatest} type="button">
              {messages.discardDraftAction}
            </DashboardActionButton>
          }
        />
      ) : null}
    </form>
  );
}

function WorkerTypeSpecificFields({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  if (state.draft.type === "MODEL_WORKER") {
    return <ModelWorkerEditableFields messages={messages} state={state} />;
  }

  if (state.draft.type === "SCRIPT_WORKER") {
    return <ScriptWorkerEditableFields messages={messages} state={state} />;
  }

  return <HostedWorkerEditableFields messages={messages} state={state} />;
}

function ModelWorkerEditableFields({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  return (
    <>
      <WorkerEditableConfigurationField
        fieldId="editable-worker-model-provider"
        input={
          <WorkerOptionalEnumSelect
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
        fieldId="editable-worker-model"
        input={
          <input
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
        fieldId="editable-worker-model-locality"
        input={
          <WorkerOptionalEnumSelect
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
        fieldId="editable-worker-executor-provider"
        input={
          <WorkerOptionalEnumSelect
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
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  return (
    <>
      <WorkerEditableConfigurationField
        fieldId="editable-worker-command"
        input={
          <input
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
        fieldId="editable-worker-args"
        input={
          <textarea
            className="min-h-24 w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
            id="editable-worker-args"
            onChange={(event) => state.onArgsTextChange(event.target.value)}
            value={state.draft.argsText}
          />
        }
        label={messages.argsFieldLabel}
      />
      <WorkerEditableConfigurationField
        fieldId="editable-worker-body"
        input={
          <textarea
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
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  return (
    <WorkerEditableConfigurationField
      fieldId="editable-worker-provider"
      input={
        <WorkerOptionalEnumSelect
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
  id,
  label,
  notConfiguredLabel,
  onChange,
  options,
  renderLabel,
  value,
}: {
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
  fieldId,
  input,
  label,
}: {
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
    </div>
  );
}
