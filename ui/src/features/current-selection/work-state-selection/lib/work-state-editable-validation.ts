import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../../../../api/factory-definition";
import type { EditableWorkStateDraft } from "../../../current-factory-definition/lib/work-state-editable-values";

export type EditableWorkStateValidationField = "contract" | "name";

export type EditableWorkStateValidationErrors = Partial<
  Record<EditableWorkStateValidationField, string>
>;

export interface ValidateEditableWorkStateDraftContext {
  originalStateName: string;
  stateNamesInWorkType: string[];
}

export interface EditableWorkStateValidationMessages {
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationNameDuplicate: (name: string) => string;
  editableConfigurationNameRequired: string;
}

export function validateEditableWorkStateDraft(
  draft: EditableWorkStateDraft,
  messages: EditableWorkStateValidationMessages,
  context?: ValidateEditableWorkStateDraftContext,
): EditableWorkStateValidationErrors {
  const validationErrors: EditableWorkStateValidationErrors = {};
  const trimmedName = draft.name.trim();

  if (trimmedName.length === 0) {
    validationErrors.name = messages.editableConfigurationNameRequired;
  } else if (context) {
    const duplicateNames = context.stateNamesInWorkType.filter(
      (stateName) =>
        stateName !== context.originalStateName && stateName === trimmedName,
    );
    if (duplicateNames.length > 0) {
      validationErrors.name =
        messages.editableConfigurationNameDuplicate(trimmedName);
    }
  }

  return validationErrors;
}

export function mergeEditableWorkStateContractValidationErrors(
  validationErrors: EditableWorkStateValidationErrors,
  pendingFactoryDefinition: CanonicalFactoryDefinition | null,
  messages: Pick<
    EditableWorkStateValidationMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): EditableWorkStateValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  try {
    normalizeFactoryDefinition(pendingFactoryDefinition);
    return validationErrors;
  } catch (error) {
    const contractMessage = resolveEditableWorkStateContractErrorMessage(
      error,
      messages,
    );
    const contractField = resolveEditableWorkStateContractField(error);

    if (contractField && !validationErrors[contractField]) {
      return {
        ...validationErrors,
        [contractField]: contractMessage,
      };
    }

    if (validationErrors.contract) {
      return validationErrors;
    }

    return {
      ...validationErrors,
      contract: contractMessage,
    };
  }
}

export function hasEditableWorkStateValidationErrors(
  validationErrors: EditableWorkStateValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}

function resolveEditableWorkStateContractErrorMessage(
  error: unknown,
  messages: Pick<
    EditableWorkStateValidationMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): string {
  if (error instanceof FactoryDefinitionAPIError) {
    return `${messages.editableConfigurationContractInvalidPrefix} ${error.message}`;
  }

  return messages.editableConfigurationContractInvalidPrefix;
}

function resolveEditableWorkStateContractField(
  error: unknown,
): EditableWorkStateValidationField | null {
  if (!(error instanceof FactoryDefinitionAPIError)) {
    return null;
  }

  if (
    /factory\.workTypes\[\d+\]\.states\[\d+\]\.name/.test(error.message) ||
    /factory\.workTypes\[\d+\]\.name/.test(error.message)
  ) {
    return "name";
  }

  return null;
}
