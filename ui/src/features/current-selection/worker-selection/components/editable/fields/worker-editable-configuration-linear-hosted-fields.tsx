import { Input, Textarea } from "../../../../../../components/ui";
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: hosted Linear poller fields stay grouped for parity with other worker sections.
export function LinearHostedWorkerEditableFields({
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
        errorMessage={validationErrors.authSecretRef}
        fieldId="editable-worker-auth-secret-ref"
        input={
          <Input
            aria-describedby={
              validationErrors.authSecretRef
                ? "editable-worker-auth-secret-ref-error"
                : "editable-worker-auth-secret-ref-hint"
            }
            aria-invalid={validationErrors.authSecretRef ? "true" : undefined}
            autoComplete="off"
            id="editable-worker-auth-secret-ref"
            onChange={(event) =>
              state.onAuthSecretRefChange(event.target.value)
            }
            type="text"
            value={state.draft.authSecretRef}
          />
        }
        label={messages.authSecretRefFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-auth-secret-ref-hint">
                {messages.authSecretRefFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="authSecretRef"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearPollInterval}
        fieldId="editable-worker-linear-poll-interval"
        input={
          <Input
            aria-describedby={
              validationErrors.linearPollInterval
                ? "editable-worker-linear-poll-interval-error"
                : "editable-worker-linear-poll-interval-hint"
            }
            aria-invalid={
              validationErrors.linearPollInterval ? "true" : undefined
            }
            id="editable-worker-linear-poll-interval"
            onChange={(event) =>
              state.onLinearPollIntervalChange(event.target.value)
            }
            type="text"
            value={state.draft.linearPollInterval}
          />
        }
        label={messages.linearPollIntervalFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-poll-interval-hint">
                {messages.linearPollIntervalFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearPollInterval"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearTeamIds}
        fieldId="editable-worker-linear-team-ids"
        input={
          <Textarea
            aria-describedby={
              validationErrors.linearTeamIds
                ? "editable-worker-linear-team-ids-error"
                : "editable-worker-linear-team-ids-hint"
            }
            aria-invalid={validationErrors.linearTeamIds ? "true" : undefined}
            className="min-h-24"
            id="editable-worker-linear-team-ids"
            onChange={(event) =>
              state.onLinearTeamIdsTextChange(event.target.value)
            }
            value={state.draft.linearTeamIdsText}
          />
        }
        label={messages.linearTeamIdsFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-team-ids-hint">
                {messages.linearTeamIdsFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearTeamIds"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearStateIds}
        fieldId="editable-worker-linear-state-ids"
        input={
          <Textarea
            aria-describedby={
              validationErrors.linearStateIds
                ? "editable-worker-linear-state-ids-error"
                : "editable-worker-linear-state-ids-hint"
            }
            aria-invalid={validationErrors.linearStateIds ? "true" : undefined}
            className="min-h-24"
            id="editable-worker-linear-state-ids"
            onChange={(event) =>
              state.onLinearStateIdsTextChange(event.target.value)
            }
            value={state.draft.linearStateIdsText}
          />
        }
        label={messages.linearStateIdsFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-state-ids-hint">
                {messages.linearStateIdsFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearStateIds"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearMappingWorkType}
        fieldId="editable-worker-linear-mapping-work-type"
        input={
          <Input
            aria-describedby={
              validationErrors.linearMappingWorkType
                ? "editable-worker-linear-mapping-work-type-error"
                : "editable-worker-linear-mapping-work-type-hint"
            }
            aria-invalid={
              validationErrors.linearMappingWorkType ? "true" : undefined
            }
            id="editable-worker-linear-mapping-work-type"
            onChange={(event) =>
              state.onLinearMappingWorkTypeChange(event.target.value)
            }
            type="text"
            value={state.draft.linearMappingWorkType}
          />
        }
        label={messages.linearMappingWorkTypeFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-mapping-work-type-hint">
                {messages.linearMappingWorkTypeFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearMappingWorkType"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearMappingState}
        fieldId="editable-worker-linear-mapping-state"
        input={
          <Input
            aria-describedby={
              validationErrors.linearMappingState
                ? "editable-worker-linear-mapping-state-error"
                : "editable-worker-linear-mapping-state-hint"
            }
            aria-invalid={
              validationErrors.linearMappingState ? "true" : undefined
            }
            id="editable-worker-linear-mapping-state"
            onChange={(event) =>
              state.onLinearMappingStateChange(event.target.value)
            }
            type="text"
            value={state.draft.linearMappingState}
          />
        }
        label={messages.linearMappingStateFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-mapping-state-hint">
                {messages.linearMappingStateFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearMappingState"
              messages={messages}
              state={state}
            />
          </>
        }
      />
      <WorkerEditableConfigurationField
        errorMessage={validationErrors.linearClaimAssigneeField}
        fieldId="editable-worker-linear-claim-assignee-field"
        input={
          <Input
            aria-describedby={
              validationErrors.linearClaimAssigneeField
                ? "editable-worker-linear-claim-assignee-field-error"
                : "editable-worker-linear-claim-assignee-field-hint"
            }
            aria-invalid={
              validationErrors.linearClaimAssigneeField ? "true" : undefined
            }
            id="editable-worker-linear-claim-assignee-field"
            onChange={(event) =>
              state.onLinearClaimAssigneeFieldChange(event.target.value)
            }
            type="text"
            value={state.draft.linearClaimAssigneeField}
          />
        }
        label={messages.linearClaimAssigneeFieldLabel}
        supportingContent={
          <>
            <WorkerEditableConfigurationFieldHelp>
              <span id="editable-worker-linear-claim-assignee-field-hint">
                {messages.linearClaimAssigneeFieldFieldHelp}
              </span>
            </WorkerEditableConfigurationFieldHelp>
            <WorkerEditableConfigurationServerChangedHint
              fieldName="linearClaimAssigneeField"
              messages={messages}
              state={state}
            />
          </>
        }
      />
    </>
  );
}
