import { EnumSelect } from "@you-agent-factory/components/forms";
import { Input } from "../../../../../../components/ui/input";
import { WORKER_TIMEOUT_UNITS } from "../../../../../current-factory-definition/lib/worker-timeout-duration";
import { resolveEditableWorkerTypeOptions } from "../../../../../current-factory-definition/lib/worker-workstation-taxonomy";
import {
  WorkerEditableConfigurationField,
  WorkerEditableConfigurationFieldHelp,
  WorkerEditableConfigurationServerChangedHint,
} from "./primitives/worker-editable-configuration-field-primitives";
import type {
  ReadyWorkerEditableConfigurationState,
  ReadyWorkerEditableValidationErrors,
  WorkerEditableConfigurationMessages,
} from "./primitives/worker-editable-configuration-field-types";

export function WorkerEditableConfigurationSharedFields({
  messages,
  state,
  validationErrors,
}: {
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  validationErrors: ReadyWorkerEditableValidationErrors;
}) {
  return (
    <>
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
          <EnumSelect
            aria-describedby={
              validationErrors.type ? "editable-worker-type-error" : undefined
            }
            aria-invalid={validationErrors.type ? "true" : undefined}
            aria-label={messages.typeFieldLabel}
            id="editable-worker-type"
            onValueChange={(nextValue) =>
              state.onTypeChange(nextValue as typeof state.draft.type)
            }
            options={resolveEditableWorkerTypeOptions(state.draft.type).map(
              (workerType) => ({
                label: messages.localizeWorkerType(workerType),
                value: workerType,
              }),
            )}
            value={state.draft.type}
          />
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
    </>
  );
}

function WorkerEditableConfigurationTimeoutField({
  messages,
  state,
  validationErrors,
}: {
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  validationErrors: ReadyWorkerEditableValidationErrors;
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
          <EnumSelect
            aria-describedby={
              validationErrors.timeout
                ? `${amountFieldId}-error`
                : "editable-worker-timeout-hint"
            }
            aria-invalid={validationErrors.timeout ? "true" : undefined}
            aria-label={messages.timeoutFieldLabel}
            disabled={!isConfigured}
            id={unitFieldId}
            onValueChange={(nextValue) =>
              state.onTimeoutUnitChange(
                nextValue as typeof state.draft.timeoutUnit,
              )
            }
            options={WORKER_TIMEOUT_UNITS.map((unit) => ({
              label: messages.localizeTimeoutUnit(unit),
              value: unit,
            }))}
            value={state.draft.timeoutUnit}
          />
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
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  validationErrors: ReadyWorkerEditableValidationErrors;
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
