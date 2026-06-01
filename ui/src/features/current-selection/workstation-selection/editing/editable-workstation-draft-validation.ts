import { validateEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/editable-workstation-cron-validation";
import {
  workerSupportsPollerBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import type {
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
} from "../lib/detail-card-types";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages/workstation-detail";

export function validateEditableWorkstationDraft(
  draft: EditableWorkstationDraft,
  selectedEditableValues?: ReturnType<typeof resolveEditableWorkstationValues>,
  promptValidationState: EditableWorkstationPromptValidationState = {
    status: "idle",
  },
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationPromptRequired"
    | "editableConfigurationPromptValidationLoading"
    | "editableConfigurationPromptValidationErrorPrefix"
    | "editableConfigurationPromptFieldHint"
    | "editableConfigurationBehaviorPollerWorkerUnsupported"
    | "editableConfigurationCronExpiryWindowInvalid"
    | "editableConfigurationCronJitterInvalid"
    | "editableConfigurationCronScheduleInvalid"
    | "editableConfigurationCronScheduleRequired"
    | "editableConfigurationWorkerRequired"
    | "editableConfigurationWorkerUnavailable"
  > = getWorkstationDetailMessages(undefined),
): EditableWorkstationValidationErrors {
  const validationErrors: EditableWorkstationValidationErrors = {};
  const promptIsRequired = workstationBehaviorRequiresPrompt(draft.behavior);

  if (draft.workerName.trim().length === 0) {
    validationErrors.workerName = messages.editableConfigurationWorkerRequired;
  } else if (
    selectedEditableValues &&
    !selectedEditableValues.workerOptions.includes(draft.workerName)
  ) {
    validationErrors.workerName =
      messages.editableConfigurationWorkerUnavailable;
  }
  if (
    draft.behavior === "POLLER" &&
    selectedEditableValues &&
    !workerSupportsPollerBehavior(
      draft.workerName.trim().length === 0
        ? null
        : {
            type: selectedEditableValues.workerTypeByName[draft.workerName],
          },
    )
  ) {
    validationErrors.behavior =
      messages.editableConfigurationBehaviorPollerWorkerUnsupported;
  }

  if (promptIsRequired && draft.prompt.trim().length === 0) {
    validationErrors.prompt = messages.editableConfigurationPromptRequired;
  } else if (promptIsRequired && promptValidationState.status === "loading") {
    validationErrors.prompt =
      messages.editableConfigurationPromptValidationLoading;
  } else if (promptIsRequired && promptValidationState.status === "error") {
    validationErrors.prompt = `${messages.editableConfigurationPromptValidationErrorPrefix} ${promptValidationState.errorMessage}`;
  } else if (
    promptIsRequired &&
    promptValidationState.status === "ready" &&
    (!promptValidationState.result.valid ||
      promptValidationState.diagnostics.length > 0)
  ) {
    validationErrors.prompt = messages.editableConfigurationPromptFieldHint;
  }

  if (draft.behavior === "CRON") {
    Object.assign(
      validationErrors,
      validateEditableWorkstationCronDraft(draft.cron, {
        cronExpiryWindowInvalid:
          messages.editableConfigurationCronExpiryWindowInvalid,
        cronJitterInvalid: messages.editableConfigurationCronJitterInvalid,
        cronScheduleInvalid:
          messages.editableConfigurationCronScheduleInvalid,
        cronScheduleRequired:
          messages.editableConfigurationCronScheduleRequired,
      }),
    );
  }

  return validationErrors;
}

export function hasEditableWorkstationValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
): boolean {
  return Boolean(
    validationErrors.behavior ||
      validationErrors.cronExpiryWindow ||
      validationErrors.cronJitter ||
      validationErrors.cronSchedule ||
      validationErrors.cronTriggerAtStart ||
      validationErrors.prompt ||
      validationErrors.runnerName ||
      validationErrors.workerName,
  );
}
