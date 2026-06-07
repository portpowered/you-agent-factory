// biome-ignore lint/nursery/noExcessiveLinesPerFile: worker editable fields, validation feedback, save wiring, and shared-impact warnings stay colocated in one section.
import { type ReactNode, useId } from "react";

import {
  AlertPanel,
  AlertPanelText,
  Checkbox,
  DashboardLabel,
  DashboardText,
  FormWarning,
  Input,
  NativeSelect,
  Textarea,
} from "../../../../components/ui";
import { formatList } from "../../../../components/ui/formatters";
import {
  EDITABLE_EXECUTOR_PROVIDERS,
  EDITABLE_HOSTED_PROVIDERS,
  EDITABLE_MODEL_LOCALITIES,
  EDITABLE_MODEL_PROVIDERS,
  EDITABLE_WORKER_TYPES,
} from "../../../current-factory-definition/lib/worker-editable-values";
import { WORKER_TIMEOUT_UNITS } from "../../../current-factory-definition/lib/worker-timeout-duration";
import { CurrentSelectionExpandableSection } from "../../base/components/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
} from "../../base/public";
import { formatEditableWorkerOverwriteFieldLabels } from "../editing/editable-worker-overwrite-fields";
import type {
  EditableWorkerConfigurationState,
  EditableWorkerOverwriteField,
  EditableWorkerSaveState,
  EditableWorkerSaveValidationErrors,
  EditableWorkerValidationErrors,
  WorkerDetailCardProps,
} from "../lib/detail-card-types";
import type { getWorkerDetailMessages } from "../messages/worker-detail";

export function WorkerEditableConfigurationSection({
  messages,
  saveState,
  state,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  saveState?: EditableWorkerSaveState;
  state?: WorkerDetailCardProps["editableConfigurationState"];
  workerName: string;
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.editableConfigurationHeading}
      toggleLabel={(expanded) =>
        expanded
          ? messages.editableConfigurationCollapseActionLabel
          : messages.editableConfigurationExpandActionLabel
      }
    >
      {state?.status === "loading" ? (
        <CurrentSelectionDetailFeedback>
          {messages.editableConfigurationLoading}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "error" ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {messages.editableConfigurationErrorPrefix} {state.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "empty" ? (
        <CurrentSelectionDetailFeedback>
          {state.message || messages.editableConfigurationEmpty}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "ready" ? (
        <WorkerEditableConfigurationReadyForm
          messages={messages}
          saveState={saveState}
          state={state}
          workerName={workerName}
        />
      ) : null}
    </CurrentSelectionExpandableSection>
  );
}

function WorkerEditableConfigurationReadyForm({
  messages,
  saveState,
  state,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  saveState?: EditableWorkerSaveState;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
  workerName: string;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableWorkerValidationErrors & Record<string, string | undefined>,
    EditableWorkerSaveValidationErrors
  >(state.validationErrors, saveState);

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <WorkerEditableConfigurationSharedImpactWarning
        messages={messages}
        state={state}
        workerName={workerName}
      />
      <WorkerEditableConfigurationOverwriteWarning
        messages={messages}
        overwriteFieldNames={state.overwriteFieldNames}
      />
      {validationErrors.contract ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {validationErrors.contract}
        </CurrentSelectionDetailFeedback>
      ) : null}
      <WorkerEditableConfigurationDraftStatus
        messages={messages}
        state={state}
      />
      <CurrentSelectionFormFields>
        <WorkerEditableConfigurationField
          errorMessage={validationErrors.name}
          fieldId="editable-worker-name"
          input={
            <Input
              aria-describedby={
                validationErrors.name ? "editable-worker-name-error" : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
              id="editable-worker-name"
              onChange={(event) => state.onNameChange(event.target.value)}
              type="text"
              value={state.draft.name}
            />
          }
          label={messages.nameFieldLabel}
          supportingContent={
            <WorkerEditableConfigurationServerChangedHint
              fieldName="name"
              messages={messages}
              state={state}
            />
          }
        />
        <WorkerEditableConfigurationField
          errorMessage={validationErrors.type}
          fieldId="editable-worker-type"
          input={
            <NativeSelect
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
            </NativeSelect>
          }
          label={messages.typeFieldLabel}
          supportingContent={
            <WorkerEditableConfigurationServerChangedHint
              fieldName="type"
              messages={messages}
              state={state}
            />
          }
        />
        <WorkerEditableConfigurationTimeoutField
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
        <WorkerEditableConfigurationStopTokenField
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
        <WorkerTypeSpecificFields
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
      </CurrentSelectionFormFields>
    </form>
  );
}

function WorkerEditableConfigurationTimeoutField({
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
  const amountFieldId = "editable-worker-timeout-amount";
  const unitFieldId = "editable-worker-timeout-unit";
  const isConfigured = (state.draft.timeoutAmount ?? "").trim().length > 0;

  return (
    <WorkerEditableConfigurationField
      errorMessage={validationErrors.timeout}
      fieldId={amountFieldId}
      input={
        <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_10rem]">
          <Input
            aria-describedby={
              validationErrors.timeout
                ? `${amountFieldId}-error`
                : "editable-worker-timeout-hint"
            }
            aria-invalid={validationErrors.timeout ? "true" : undefined}
            id={amountFieldId}
            inputMode="decimal"
            min={0}
            onChange={(event) =>
              state.onTimeoutAmountChange(event.target.value)
            }
            placeholder={messages.notConfiguredOptionLabel}
            type="number"
            value={isConfigured ? (state.draft.timeoutAmount ?? "") : ""}
          />
          <NativeSelect
            aria-describedby={
              validationErrors.timeout
                ? `${amountFieldId}-error`
                : "editable-worker-timeout-hint"
            }
            aria-invalid={validationErrors.timeout ? "true" : undefined}
            aria-label={messages.timeoutFieldLabel}
            disabled={!isConfigured}
            id={unitFieldId}
            onChange={(event) =>
              state.onTimeoutUnitChange(
                event.target.value as typeof state.draft.timeoutUnit,
              )
            }
            value={state.draft.timeoutUnit}
          >
            {WORKER_TIMEOUT_UNITS.map((unit) => (
              <option key={unit} value={unit}>
                {messages.localizeTimeoutUnit(unit)}
              </option>
            ))}
          </NativeSelect>
        </div>
      }
      label={messages.timeoutFieldLabel}
      supportingContent={
        <>
          <WorkerEditableConfigurationFieldHelp>
            <span id="editable-worker-timeout-hint">
              {messages.timeoutFieldHelp}
            </span>
          </WorkerEditableConfigurationFieldHelp>
          <WorkerEditableConfigurationServerChangedHint
            fieldName="timeout"
            messages={messages}
            state={state}
          />
        </>
      }
    />
  );
}

function WorkerEditableConfigurationStopTokenField({
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
  const fieldId = "editable-worker-stop-token";

  return (
    <WorkerEditableConfigurationField
      errorMessage={validationErrors.stopToken}
      fieldId={fieldId}
      input={
        <Input
          aria-describedby={
            validationErrors.stopToken
              ? `${fieldId}-error`
              : "editable-worker-stop-token-hint"
          }
          aria-invalid={validationErrors.stopToken ? "true" : undefined}
          id={fieldId}
          onChange={(event) => state.onStopTokenChange(event.target.value)}
          type="text"
          value={state.draft.stopToken}
        />
      }
      label={messages.stopTokenFieldLabel}
      supportingContent={
        <>
          <WorkerEditableConfigurationFieldHelp>
            <span id="editable-worker-stop-token-hint">
              {messages.stopTokenFieldHelp}
            </span>
          </WorkerEditableConfigurationFieldHelp>
          <WorkerEditableConfigurationServerChangedHint
            fieldName="stopToken"
            messages={messages}
            state={state}
          />
        </>
      }
    />
  );
}

function WorkerEditableConfigurationOverwriteWarning({
  messages,
  overwriteFieldNames,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  overwriteFieldNames: EditableWorkerOverwriteField[];
}) {
  if (overwriteFieldNames.length === 0) {
    return null;
  }

  const formattedFields = formatEditableWorkerOverwriteFieldLabels(
    overwriteFieldNames,
    messages,
  );

  return (
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </AlertPanelText>
    </AlertPanel>
  );
}

function WorkerEditableConfigurationServerChangedHint({
  fieldName,
  messages,
  state,
}: {
  fieldName: EditableWorkerOverwriteField;
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  if (!state.overwriteFieldNames.includes(fieldName)) {
    return null;
  }

  return (
    <FormWarning>
      {messages.editableConfigurationServerFieldChangedHint}
    </FormWarning>
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
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationSharedImpactWarning(
          state.draft.name.trim() || workerName,
          formatList(workstationNames),
        )}
      </AlertPanelText>
    </AlertPanel>
  );
}

function WorkerEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  state: Extract<EditableWorkerConfigurationState, { status: "ready" }>;
}) {
  if (!state.hasValidationErrors) {
    return null;
  }

  return (
    <CurrentSelectionFormField>
      <CurrentSelectionDetailFeedback role="alert" tone="danger">
        {messages.editableConfigurationValidationStatus}
      </CurrentSelectionDetailFeedback>
      <DashboardText
        className="m-0 text-on-surface-subtle"
        variant="supporting"
      >
        {messages.editableConfigurationSaveDisabledValidationDetail}
      </DashboardText>
    </CurrentSelectionFormField>
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: model worker fields stay grouped for parity with script/hosted sections.
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
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              {messages.modelProviderFieldHelp}
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="modelProvider"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.model}
        fieldId="editable-worker-model"
        input={
          <Input
            aria-describedby={
              validationErrors.model ? "editable-worker-model-error" : undefined
            }
            aria-invalid={validationErrors.model ? "true" : undefined}
            id="editable-worker-model"
            onChange={(event) => state.onModelChange(event.target.value)}
            type="text"
            value={state.draft.model}
          />
        }
        label={messages.modelLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              {messages.modelFieldHelp}
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="model"
              messages={messages}
              state={state}
            />
          </>
        }
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
        supportingContent={
          <WorkerEditableConfigurationServerChangedHint
            fieldName="modelLocality"
            messages={messages}
            state={state}
          />
        }
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
        supportingContent={
          <WorkerEditableConfigurationServerChangedHint
            fieldName="executorProvider"
            messages={messages}
            state={state}
          />
        }
      />
      <ModelWorkerSkipPermissionsField
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    </>
  );
}

function ModelWorkerSkipPermissionsField({
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
      errorMessage={validationErrors.skipPermissions}
      fieldId="editable-worker-skip-permissions"
      input={
        <Checkbox
          aria-describedby={
            validationErrors.skipPermissions
              ? "editable-worker-skip-permissions-error"
              : undefined
          }
          aria-invalid={validationErrors.skipPermissions ? "true" : undefined}
          checked={state.draft.skipPermissions}
          id="editable-worker-skip-permissions"
          onChange={(event) =>
            state.onSkipPermissionsChange(event.target.checked)
          }
        />
      }
      label={messages.skipPermissionsFieldLabel}
      supportingContent={
        <>
          <WorkerEditableConfigurationFieldHelp>
            {messages.skipPermissionsFieldHelp}
          </WorkerEditableConfigurationFieldHelp>
          <WorkerEditableConfigurationServerChangedHint
            fieldName="skipPermissions"
            messages={messages}
            state={state}
          />
        </>
      }
    />
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
          <Input
            aria-describedby={
              validationErrors.command
                ? "editable-worker-command-error"
                : undefined
            }
            aria-invalid={validationErrors.command ? "true" : undefined}
            id="editable-worker-command"
            onChange={(event) => state.onCommandChange(event.target.value)}
            type="text"
            value={state.draft.command}
          />
        }
        label={messages.commandFieldLabel}
        supportingContent={
          <WorkerEditableConfigurationServerChangedHint
            fieldName="command"
            messages={messages}
            state={state}
          />
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.args}
        fieldId="editable-worker-args"
        input={
          <Textarea
            aria-describedby={
              validationErrors.args ? "editable-worker-args-error" : undefined
            }
            aria-invalid={validationErrors.args ? "true" : undefined}
            className="min-h-24"
            id="editable-worker-args"
            onChange={(event) => state.onArgsTextChange(event.target.value)}
            value={state.draft.argsText}
          />
        }
        label={messages.argsFieldLabel}
        supportingContent={
          <WorkerEditableConfigurationServerChangedHint
            fieldName="args"
            messages={messages}
            state={state}
          />
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.body}
        fieldId="editable-worker-body"
        input={
          <Textarea
            aria-describedby={
              validationErrors.body ? "editable-worker-body-error" : undefined
            }
            aria-invalid={validationErrors.body ? "true" : undefined}
            className="min-h-32"
            id="editable-worker-body"
            onChange={(event) => state.onBodyChange(event.target.value)}
            value={state.draft.body}
          />
        }
        label={messages.bodyFieldLabel}
        supportingContent={
          <WorkerEditableConfigurationServerChangedHint
            fieldName="body"
            messages={messages}
            state={state}
          />
        }
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
            validationErrors.provider
              ? "editable-worker-provider-error"
              : undefined
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
      supportingContent={
        <WorkerEditableConfigurationServerChangedHint
          fieldName="provider"
          messages={messages}
          state={state}
        />
      }
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
    <NativeSelect
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
    </NativeSelect>
  );
}

function WorkerEditableConfigurationFieldHelp({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <DashboardText className="m-0 text-on-surface-subtle" variant="supporting">
      {children}
    </DashboardText>
  );
}

function WorkerEditableConfigurationField({
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
    <CurrentSelectionFormField>
      <DashboardLabel as="label" htmlFor={fieldId}>
        {label}
      </DashboardLabel>
      {input}
      {supportingContent}
      {errorMessage ? (
        <CurrentSelectionDetailFeedback id={`${fieldId}-error`} tone="danger">
          {errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
    </CurrentSelectionFormField>
  );
}
