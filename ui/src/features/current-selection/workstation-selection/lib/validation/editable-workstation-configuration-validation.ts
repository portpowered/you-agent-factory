import { validateEditableWorkstationCronDraft } from "../../../../current-factory-definition/lib/editable-workstation-cron-validation";
import {
  isModelInvokeWorkstationType,
  resolveModelOperationByName,
  validateEditableModelInvokeBindings,
} from "../../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import {
  workerSupportsPollerBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "../../../../current-factory-definition/lib/workstation-editable-values";
import { workstationRequiresWorkerAssignment } from "../../../../current-factory-definition/lib/workstation-worker-assignment";
import { isValidUppercaseModelOperationName } from "../../../../factory-graph-editor/lib/factory-graph-add-model-operation-draft";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../../messages/workstation-detail";
import type {
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
} from "../keys/detail-card-types";
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
    | "editableConfigurationModelInvokeBindingDuplicate"
    | "editableConfigurationModelInvokeBindingRequired"
    | "editableConfigurationModelInvokeBindingsSummary"
    | "editableConfigurationModelInvokeOperationMissing"
    | "editableConfigurationModelInvokeOperationRequired"
    | "editableConfigurationModelInvokeOperationInvalid"
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
      type: draft.workstationType,
    });
  const isModelInvoke = isModelInvokeWorkstationType(draft.workstationType);
  const promptIsRequired =
    requiresWorkerAssignment &&
    !isModelInvoke &&
    workstationBehaviorRequiresPrompt(draft.behavior);

  if (isModelInvoke && selectedEditableValues != null) {
    appendModelInvokeValidationErrors(
      validationErrors,
      draft,
      selectedEditableValues,
      messages,
    );
  }

  if (requiresWorkerAssignment) {
    if (draft.workerName.trim().length === 0) {
      validationErrors.workerName =
        messages.editableConfigurationWorkerRequired;
    } else if (selectedEditableValues) {
      const workerOptions = isModelInvoke
        ? selectedEditableValues.modelInvokeWorkerOptions
        : selectedEditableValues.workerOptions;
      if (!workerOptions.includes(draft.workerName)) {
        validationErrors.workerName =
          messages.editableConfigurationWorkerUnavailable;
      }
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

function appendModelInvokeValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationModelInvokeBindingDuplicate"
    | "editableConfigurationModelInvokeBindingRequired"
    | "editableConfigurationModelInvokeBindingsSummary"
    | "editableConfigurationModelInvokeOperationInvalid"
    | "editableConfigurationModelInvokeOperationMissing"
    | "editableConfigurationModelInvokeOperationRequired"
  >,
) {
  const trimmedOperation = draft.operation.trim();
  if (trimmedOperation.length === 0) {
    validationErrors.operation =
      messages.editableConfigurationModelInvokeOperationRequired;
    return;
  }

  if (!isValidUppercaseModelOperationName(trimmedOperation)) {
    validationErrors.operation =
      messages.editableConfigurationModelInvokeOperationInvalid;
    return;
  }

  const operations =
    selectedEditableValues?.modelOperationsByWorkerName[draft.workerName] ?? [];
  const operation = resolveModelOperationByName(operations, trimmedOperation);
  if (!operation) {
    validationErrors.operation =
      messages.editableConfigurationModelInvokeOperationMissing;
    return;
  }

  Object.assign(
    validationErrors,
    validateEditableModelInvokeBindings(draft.operationBindings, operation, {
      bindingDuplicate:
        messages.editableConfigurationModelInvokeBindingDuplicate,
      bindingRequired: messages.editableConfigurationModelInvokeBindingRequired,
      bindingSummary: messages.editableConfigurationModelInvokeBindingsSummary,
    }),
  );
}

export function resolveWorkerOptionsState(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationEmpty"
    | "editableConfigurationModelInvokeWorkerOptionsEmpty"
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
      type: draft.workstationType,
    })
  ) {
    return {
      options: selectedEditableValues.workerOptions,
      status: "ready",
    };
  }

  const workerOptions = isModelInvokeWorkstationType(draft.workstationType)
    ? selectedEditableValues.modelInvokeWorkerOptions
    : selectedEditableValues.workerOptions;
  const workerOptionsEmptyMessage = isModelInvokeWorkstationType(
    draft.workstationType,
  )
    ? messages.editableConfigurationModelInvokeWorkerOptionsEmpty
    : messages.editableConfigurationWorkerOptionsEmpty;

  if (workerOptions.length === 0) {
    return {
      message: workerOptionsEmptyMessage,
      status: "empty",
    };
  }

  if (!workerOptions.includes(draft.workerName)) {
    return {
      message: messages.editableConfigurationWorkerMissing,
      status: "error",
    };
  }

  return {
    options: workerOptions,
    status: "ready",
  };
}
