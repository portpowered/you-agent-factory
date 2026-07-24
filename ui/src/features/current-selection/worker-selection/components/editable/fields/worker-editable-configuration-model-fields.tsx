import { Checkbox, Input } from "../../../../../../components/ui";
import {
  EDITABLE_EXECUTOR_PROVIDERS,
  EDITABLE_MODEL_LOCALITIES,
  EDITABLE_MODEL_PROVIDERS,
} from "../../../../../current-factory-definition/lib/worker-editable-values";
import {
  WorkerEditableConfigurationField,
  WorkerEditableConfigurationFieldHelp,
  WorkerEditableConfigurationServerChangedHint,
  WorkerOptionalEnumSelect,
} from "./primitives/worker-editable-configuration-field-primitives";
import type {
  ReadyWorkerEditableConfigurationState,
  ReadyWorkerEditableValidationErrors,
  WorkerEditableConfigurationMessages,
} from "./primitives/worker-editable-configuration-field-types";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: model worker fields stay grouped for parity with script/hosted sections.
export function WorkerEditableConfigurationModelFields({
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
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  validationErrors: ReadyWorkerEditableValidationErrors;
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
