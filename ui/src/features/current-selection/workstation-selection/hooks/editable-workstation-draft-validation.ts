import {
  workerSupportsPollerBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { resolveEditableWorkstationValues } from "../../../current-factory-definition/lib/workstation-editable-values";
import type {
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
} from "../lib/detail-card-types";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages/workstation-detail";
import { validateEditableWorkstationGuardDraft } from "../lib/workstation-editable-validation";

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
    | "editableConfigurationWorkerRequired"
    | "editableConfigurationWorkerUnavailable"
    | "editableConfigurationVisitCountMaxVisitsInvalid"
    | "editableConfigurationVisitCountWorkstationInvalid"
    | "editableConfigurationVisitCountWorkstationRequired"
    | "editableConfigurationMatchesFieldsInputKeyRequired"
    | "editableConfigurationInputGuardMultipleGuards"
    | "editableConfigurationInputGuardMatchInputRequired"
    | "editableConfigurationInputGuardMatchInputInvalid"
    | "editableConfigurationInputGuardMatchInputSelfReference"
    | "editableConfigurationInputGuardParentInputRequired"
    | "editableConfigurationInputGuardParentInputInvalid"
    | "editableConfigurationInputGuardParentInputSelfReference"
    | "editableConfigurationInputGuardSpawnedByInvalid"
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

  return {
    ...validationErrors,
    ...validateEditableWorkstationGuardDraft(
      draft,
      {
        workstationOptions: selectedEditableValues?.workstationOptions ?? [],
      },
      messages,
    ),
  };
}

export function hasEditableWorkstationValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}
