import { Input, Textarea } from "../../../../../../components/ui";
import {
  WorkerEditableConfigurationField,
  WorkerEditableConfigurationServerChangedHint,
} from "./primitives/worker-editable-configuration-field-primitives";
import type {
  ReadyWorkerEditableConfigurationState,
  ReadyWorkerEditableValidationErrors,
  WorkerEditableConfigurationMessages,
} from "./primitives/worker-editable-configuration-field-types";

export function WorkerEditableConfigurationScriptFields({
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
