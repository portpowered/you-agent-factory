import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { resolvePeerInputWorkTypes } from "../../../current-factory-definition/lib/workstation-guards";
import type { EditableWorkstationValidationErrors } from "./detail-card-types";

export interface ValidateEditableWorkstationNameDraftContext {
  originalWorkstationName: string;
  workstationNames: string[];
}

export interface WorkstationNameValidationMessages {
  editableConfigurationNameDuplicate: (workstationName: string) => string;
  editableConfigurationNameRequired: string;
}

export function validateEditableWorkstationNameDraft(
  draft: EditableWorkstationDraft,
  messages: WorkstationNameValidationMessages,
  context?: ValidateEditableWorkstationNameDraftContext,
): Pick<EditableWorkstationValidationErrors, "name"> {
  const validationErrors: Pick<EditableWorkstationValidationErrors, "name"> =
    {};
  const trimmedName = (draft.name ?? "").trim();

  if (trimmedName.length === 0) {
    validationErrors.name = messages.editableConfigurationNameRequired;
  } else if (
    context &&
    trimmedName !== context.originalWorkstationName.trim() &&
    context.workstationNames.some(
      (workstationName) => workstationName.trim() === trimmedName,
    )
  ) {
    validationErrors.name =
      messages.editableConfigurationNameDuplicate(trimmedName);
  }

  return validationErrors;
}

export interface ValidateEditableWorkstationGuardDraftContext {
  workstationOptions: string[];
}

export interface WorkstationGuardValidationMessages {
  editableConfigurationInputGuardMatchInputInvalid: (
    workType: string,
  ) => string;
  editableConfigurationInputGuardMatchInputRequired: string;
  editableConfigurationInputGuardMatchInputSelfReference: string;
  editableConfigurationInputGuardMultipleGuards: string;
  editableConfigurationInputGuardParentInputInvalid: (
    workType: string,
  ) => string;
  editableConfigurationInputGuardParentInputRequired: string;
  editableConfigurationInputGuardParentInputSelfReference: string;
  editableConfigurationInputGuardSpawnedByInvalid: (
    workstation: string,
  ) => string;
  editableConfigurationMatchesFieldsInputKeyRequired: string;
  editableConfigurationVisitCountMaxVisitsInvalid: string;
  editableConfigurationVisitCountWorkstationInvalid: (
    workstation: string,
  ) => string;
  editableConfigurationVisitCountWorkstationRequired: string;
}

export function validateEditableWorkstationGuardDraft(
  draft: EditableWorkstationDraft,
  context: ValidateEditableWorkstationGuardDraftContext,
  messages: WorkstationGuardValidationMessages,
): EditableWorkstationValidationErrors {
  const validationErrors: EditableWorkstationValidationErrors = {};

  draft.guards.forEach((guard, guardIndex) => {
    if (guard.type === "VISIT_COUNT") {
      const workstation = guard.workstation?.trim() ?? "";
      if (workstation.length === 0) {
        validationErrors[`guards[${guardIndex}].workstation`] =
          messages.editableConfigurationVisitCountWorkstationRequired;
      } else if (!context.workstationOptions.includes(workstation)) {
        validationErrors[`guards[${guardIndex}].workstation`] =
          messages.editableConfigurationVisitCountWorkstationInvalid(
            workstation,
          );
      }

      if (
        guard.maxVisits == null ||
        !Number.isInteger(guard.maxVisits) ||
        guard.maxVisits <= 0
      ) {
        validationErrors[`guards[${guardIndex}].maxVisits`] =
          messages.editableConfigurationVisitCountMaxVisitsInvalid;
      }
    }

    if (guard.type === "MATCHES_FIELDS") {
      const inputKey = guard.matchConfig?.inputKey?.trim() ?? "";
      if (inputKey.length === 0) {
        validationErrors[`guards[${guardIndex}].matchConfig.inputKey`] =
          messages.editableConfigurationMatchesFieldsInputKeyRequired;
      }
    }
  });

  draft.inputs.forEach((input, slotIndex) => {
    if (input.guards.length > 1) {
      validationErrors[`inputs[${slotIndex}].guard.type`] =
        messages.editableConfigurationInputGuardMultipleGuards;
    }

    const guard = input.guards[0];
    if (!guard) {
      return;
    }

    const peerWorkTypes = resolvePeerInputWorkTypes(draft.inputs, slotIndex);
    const peerWorkTypeSet = new Set(peerWorkTypes);
    const workType = input.workType.trim();

    if (guard.type === "SAME_NAME" || guard.type === "SAME_TRACE_ID") {
      const matchInput = guard.matchInput?.trim() ?? "";
      if (matchInput.length === 0) {
        validationErrors[`inputs[${slotIndex}].guard.matchInput`] =
          messages.editableConfigurationInputGuardMatchInputRequired;
      } else if (matchInput === workType) {
        validationErrors[`inputs[${slotIndex}].guard.matchInput`] =
          messages.editableConfigurationInputGuardMatchInputSelfReference;
      } else if (!peerWorkTypeSet.has(matchInput)) {
        validationErrors[`inputs[${slotIndex}].guard.matchInput`] =
          messages.editableConfigurationInputGuardMatchInputInvalid(matchInput);
      }
    }

    if (
      guard.type === "ALL_CHILDREN_COMPLETE" ||
      guard.type === "ANY_CHILD_FAILED"
    ) {
      const parentInput = guard.parentInput?.trim() ?? "";
      if (parentInput.length === 0) {
        validationErrors[`inputs[${slotIndex}].guard.parentInput`] =
          messages.editableConfigurationInputGuardParentInputRequired;
      } else if (parentInput === workType) {
        validationErrors[`inputs[${slotIndex}].guard.parentInput`] =
          messages.editableConfigurationInputGuardParentInputSelfReference;
      } else if (!peerWorkTypeSet.has(parentInput)) {
        validationErrors[`inputs[${slotIndex}].guard.parentInput`] =
          messages.editableConfigurationInputGuardParentInputInvalid(
            parentInput,
          );
      }

      const spawnedBy = guard.spawnedBy?.trim() ?? "";
      if (
        spawnedBy.length > 0 &&
        !context.workstationOptions.includes(spawnedBy)
      ) {
        validationErrors[`inputs[${slotIndex}].guard.spawnedBy`] =
          messages.editableConfigurationInputGuardSpawnedByInvalid(spawnedBy);
      }
    }
  });

  return validationErrors;
}
