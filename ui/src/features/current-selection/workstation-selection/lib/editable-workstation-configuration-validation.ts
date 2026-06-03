import { validateEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/editable-workstation-cron-validation";
import {
  workerSupportsPollerBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages/workstation-detail";
import type {
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
} from "./detail-card-types";
import {
  type ValidateEditableWorkstationNameDraftContext,
  validateEditableWorkstationGuardDraft,
  validateEditableWorkstationNameDraft,
} from "./workstation-editable-validation";

export function validateEditableWorkstationDraft(
  draft: EditableWorkstationDraft,
  selectedEditableValues?: ReturnType<typeof resolveEditableWorkstationValues>,
  promptValidationState: EditableWorkstationPromptValidationState = {
    status: "idle",
  },
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationNameDuplicate"
    | "editableConfigurationNameRequired"
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
    | "editableConfigurationCronExpiryWindowInvalid"
    | "editableConfigurationCronJitterInvalid"
    | "editableConfigurationCronScheduleInvalid"
    | "editableConfigurationCronScheduleRequired"
  > = getWorkstationDetailMessages(undefined),
  nameValidationContext?: ValidateEditableWorkstationNameDraftContext,
): EditableWorkstationValidationErrors {
  const validationErrors: EditableWorkstationValidationErrors = {
    ...validateEditableWorkstationNameDraft(
      draft,
      messages,
      nameValidationContext,
    ),
  };
  const requiresWorkerAssignment =
    selectedEditableValues == null ||
    workstationRequiresWorkerAssignment({
      type: selectedEditableValues.workstationType,
    });
  const promptIsRequired =
    requiresWorkerAssignment &&
    workstationBehaviorRequiresPrompt(draft.behavior);

  if (requiresWorkerAssignment) {
    if (draft.workerName.trim().length === 0) {
      validationErrors.workerName =
        messages.editableConfigurationWorkerRequired;
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
  }

  if (promptIsRequired && draft.prompt.trim().length === 0) {
    validationErrors.prompt = messages.editableConfigurationPromptRequired;
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

  const guardValidationErrors = validateEditableWorkstationGuardDraft(
    draft,
    {
      workstationOptions: selectedEditableValues?.workstationOptions ?? [],
    },
    messages,
  );

  if (draft.behavior === "CRON") {
    Object.assign(
      validationErrors,
      validateEditableWorkstationCronDraft(draft.cron, {
        cronExpiryWindowInvalid:
          messages.editableConfigurationCronExpiryWindowInvalid,
        cronJitterInvalid: messages.editableConfigurationCronJitterInvalid,
        cronScheduleInvalid: messages.editableConfigurationCronScheduleInvalid,
        cronScheduleRequired:
          messages.editableConfigurationCronScheduleRequired,
      }),
    );
  }

  return {
    ...validationErrors,
    ...guardValidationErrors,
  };
}

export function hasEditableWorkstationValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}

export function resolveWorkerOptionsState(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationEmpty"
    | "editableConfigurationWorkerMissing"
    | "editableConfigurationWorkerOptionsEmpty"
  >,
): EditableWorkstationWorkerOptionsState {
  if (!selectedEditableValues) {
    return {
      message: messages.editableConfigurationEmpty,
      status: "error",
    };
  }

  if (
    !workstationRequiresWorkerAssignment({
      type: selectedEditableValues.workstationType,
    })
  ) {
    return {
      options: selectedEditableValues.workerOptions,
      status: "ready",
    };
  }

  if (selectedEditableValues.workerOptions.length === 0) {
    return {
      message: messages.editableConfigurationWorkerOptionsEmpty,
      status: "empty",
    };
  }

  if (!selectedEditableValues.workerOptions.includes(draft.workerName)) {
    return {
      message: messages.editableConfigurationWorkerMissing,
      status: "error",
    };
  }

  return {
    options: selectedEditableValues.workerOptions,
    status: "ready",
  };
}
