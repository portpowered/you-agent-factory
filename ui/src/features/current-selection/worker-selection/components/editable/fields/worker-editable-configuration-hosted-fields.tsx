import { EDITABLE_HOSTED_PROVIDERS } from "../../../../../current-factory-definition/lib/worker-editable-values";
import {
  WorkerEditableConfigurationField,
  WorkerEditableConfigurationServerChangedHint,
  WorkerOptionalEnumSelect,
} from "./primitives/worker-editable-configuration-field-primitives";
import type {
  ReadyWorkerEditableConfigurationState,
  ReadyWorkerEditableValidationErrors,
  WorkerEditableConfigurationMessages,
} from "./primitives/worker-editable-configuration-field-types";
import { LinearHostedWorkerEditableFields } from "./worker-editable-configuration-linear-hosted-fields";

export function WorkerEditableConfigurationHostedFields({
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
      {state.draft.provider === "LINEAR" ? (
        <LinearHostedWorkerEditableFields
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
      ) : null}
    </>
  );
}
