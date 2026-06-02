import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../../../../api/factory-definition";
import {
  type EditableResourceDraft,
  parseResourceCapacityText,
} from "../../../current-factory-definition/lib/resource-editable-values";
import type { ResourceDetailMessages } from "../messages/resource-detail-types";

export type EditableResourceValidationField =
  | "backend"
  | "capacity"
  | "contract"
  | "loadPolicy"
  | "model"
  | "name"
  | "provider"
  | "type";

export type EditableResourceValidationErrors = Partial<
  Record<EditableResourceValidationField, string>
>;

export interface ValidateEditableResourceDraftContext {
  originalResourceName: string;
  resourceNames: string[];
}

export function validateEditableResourceDraft(
  draft: EditableResourceDraft,
  messages: Pick<
    ResourceDetailMessages,
    | "editableConfigurationCapacityInvalid"
    | "editableConfigurationNameDuplicate"
    | "editableConfigurationNameRequired"
  >,
  context?: ValidateEditableResourceDraftContext,
): EditableResourceValidationErrors {
  const validationErrors: EditableResourceValidationErrors = {};
  const trimmedName = draft.name.trim();

  if (trimmedName.length === 0) {
    validationErrors.name = messages.editableConfigurationNameRequired;
  } else if (
    context &&
    trimmedName !== context.originalResourceName &&
    context.resourceNames.includes(trimmedName)
  ) {
    validationErrors.name =
      messages.editableConfigurationNameDuplicate(trimmedName);
  }

  const capacity = parseResourceCapacityText(draft.capacityText);
  if (capacity == null || capacity < 1) {
    validationErrors.capacity = messages.editableConfigurationCapacityInvalid;
  }

  return validationErrors;
}

export function mergeEditableResourceContractValidationErrors(
  validationErrors: EditableResourceValidationErrors,
  pendingFactoryDefinition: CanonicalFactoryDefinition | null,
  resourceName: string,
  messages: Pick<
    ResourceDetailMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
  resourceIndex?: number,
): EditableResourceValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  const resource =
    resourceIndex != null && resourceIndex >= 0
      ? pendingFactoryDefinition.resources?.[resourceIndex]
      : pendingFactoryDefinition.resources?.find(
          (candidate) => candidate.name === resourceName,
        );
  if (!resource) {
    return validationErrors;
  }

  try {
    normalizeFactoryDefinition(
      buildResourceContractValidationFactory(resource),
    );
    return validationErrors;
  } catch (error) {
    const contractMessage = resolveEditableResourceContractErrorMessage(
      error,
      messages,
    );
    const contractField = resolveEditableResourceContractField(error);

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

export function hasEditableResourceValidationErrors(
  validationErrors: EditableResourceValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}

function resolveEditableResourceContractErrorMessage(
  error: unknown,
  messages: Pick<
    ResourceDetailMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): string {
  if (error instanceof FactoryDefinitionAPIError) {
    return `${messages.editableConfigurationContractInvalidPrefix} ${error.message}`;
  }

  return messages.editableConfigurationContractInvalidPrefix;
}

function buildResourceContractValidationFactory(
  resource: NonNullable<CanonicalFactoryDefinition["resources"]>[number],
): CanonicalFactoryDefinition {
  return {
    name: "__resource_validation__",
    resources: [{ capacity: resource.capacity, name: resource.name }],
    workTypes: [
      {
        name: "__validation__",
        states: [
          { name: "init", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
  };
}

function resolveEditableResourceContractField(
  error: unknown,
): EditableResourceValidationField | null {
  if (!(error instanceof FactoryDefinitionAPIError)) {
    return null;
  }

  const fieldMatch = error.message.match(
    /factory\.resources\[\d+\]\.([a-zA-Z]+)/,
  );
  if (!fieldMatch) {
    return null;
  }

  const fieldName = fieldMatch[1];
  switch (fieldName) {
    case "backend":
      return "backend";
    case "capacity":
      return "capacity";
    case "loadPolicy":
      return "loadPolicy";
    case "model":
      return "model";
    case "name":
      return "name";
    case "provider":
      return "provider";
    case "type":
      return "type";
    default:
      return null;
  }
}
