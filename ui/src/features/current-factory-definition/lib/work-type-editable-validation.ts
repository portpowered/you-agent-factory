import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../../../api/factory-definition";
import type { EditableWorkTypeDraft } from "./work-type-editable-values";

export type EditableWorkTypeValidationField =
  | "contract"
  | "handlingBehavior"
  | "name";

export type EditableWorkTypeValidationErrors = Partial<
  Record<EditableWorkTypeValidationField, string>
>;

export interface ValidateEditableWorkTypeDraftContext {
  originalWorkTypeName: string;
  workTypeNames: string[];
}

export interface EditableWorkTypeValidationMessages {
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationNameDuplicate: (workTypeName: string) => string;
  editableConfigurationNameRequired: string;
}

export function validateEditableWorkTypeDraft(
  draft: EditableWorkTypeDraft,
  messages: EditableWorkTypeValidationMessages,
  context?: ValidateEditableWorkTypeDraftContext,
): EditableWorkTypeValidationErrors {
  const validationErrors: EditableWorkTypeValidationErrors = {};
  const trimmedName = draft.name.trim();

  if (trimmedName.length === 0) {
    validationErrors.name = messages.editableConfigurationNameRequired;
  } else if (
    context &&
    trimmedName !== context.originalWorkTypeName &&
    context.workTypeNames.includes(trimmedName)
  ) {
    validationErrors.name =
      messages.editableConfigurationNameDuplicate(trimmedName);
  }

  return validationErrors;
}

export function mergeEditableWorkTypeContractValidationErrors(
  validationErrors: EditableWorkTypeValidationErrors,
  pendingFactoryDefinition: CanonicalFactoryDefinition | null,
  messages: Pick<
    EditableWorkTypeValidationMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): EditableWorkTypeValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  try {
    normalizeFactoryDefinition(pendingFactoryDefinition);
    return validationErrors;
  } catch (error) {
    const contractMessage = resolveEditableWorkTypeContractErrorMessage(
      error,
      messages,
    );
    const contractField = resolveEditableWorkTypeContractField(error);

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

export function hasEditableWorkTypeValidationErrors(
  validationErrors: EditableWorkTypeValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}

function resolveEditableWorkTypeContractErrorMessage(
  error: unknown,
  messages: Pick<
    EditableWorkTypeValidationMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): string {
  if (error instanceof FactoryDefinitionAPIError) {
    return `${messages.editableConfigurationContractInvalidPrefix} ${error.message}`;
  }

  return messages.editableConfigurationContractInvalidPrefix;
}

function resolveEditableWorkTypeContractField(
  error: unknown,
): EditableWorkTypeValidationField | null {
  if (!(error instanceof FactoryDefinitionAPIError)) {
    return null;
  }

  const fieldMatch = error.message.match(
    /factory\.workTypes\[\d+\]\.([a-zA-Z]+)/,
  );
  if (!fieldMatch) {
    return null;
  }

  const fieldName = fieldMatch[1];
  switch (fieldName) {
    case "handlingBehavior":
      return "handlingBehavior";
    case "name":
      return "name";
    default:
      return null;
  }
}
