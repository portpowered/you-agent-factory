import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../../../../api/factory-definition";
import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import type { WorkerDetailMessages } from "../messages/worker-detail-types";

export type EditableWorkerValidationField =
  | "args"
  | "body"
  | "command"
  | "contract"
  | "executorProvider"
  | "model"
  | "modelLocality"
  | "modelProvider"
  | "provider"
  | "type";

export type EditableWorkerValidationErrors = Partial<
  Record<EditableWorkerValidationField, string>
>;

export function validateEditableWorkerDraft(
  draft: EditableWorkerDraft,
  messages: Pick<
    WorkerDetailMessages,
    | "editableConfigurationArgsInvalid"
    | "editableConfigurationBodyRequired"
    | "editableConfigurationCommandRequired"
    | "editableConfigurationModelProviderRequired"
    | "editableConfigurationModelRequired"
    | "editableConfigurationProviderRequired"
    | "editableConfigurationScriptCommandOrBodyRequired"
  >,
): EditableWorkerValidationErrors {
  const validationErrors: EditableWorkerValidationErrors = {};

  if (draft.type === "MODEL_WORKER") {
    if (!draft.modelProvider) {
      validationErrors.modelProvider =
        messages.editableConfigurationModelProviderRequired;
    }
    if (draft.model.trim().length === 0) {
      validationErrors.model = messages.editableConfigurationModelRequired;
    }
  }

  if (draft.type === "SCRIPT_WORKER") {
    const hasCommand = draft.command.trim().length > 0;
    const hasBody = draft.body.trim().length > 0;
    if (!hasCommand && !hasBody) {
      validationErrors.command =
        messages.editableConfigurationScriptCommandOrBodyRequired;
      validationErrors.body =
        messages.editableConfigurationScriptCommandOrBodyRequired;
    }
  }

  if (draft.type === "HOSTED_WORKER" && !draft.provider) {
    validationErrors.provider = messages.editableConfigurationProviderRequired;
  }

  if (draft.type === "SCRIPT_WORKER" && draft.argsText.includes("\0")) {
    validationErrors.args = messages.editableConfigurationArgsInvalid;
  }

  return validationErrors;
}

export function mergeEditableWorkerContractValidationErrors(
  validationErrors: EditableWorkerValidationErrors,
  pendingFactoryDefinition: CanonicalFactoryDefinition | null,
  workerName: string,
  messages: Pick<
    WorkerDetailMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): EditableWorkerValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  const worker = pendingFactoryDefinition.workers?.find(
    (candidate) => candidate.name === workerName,
  );
  if (!worker) {
    return validationErrors;
  }

  try {
    normalizeFactoryDefinition(buildWorkerContractValidationFactory(worker));
    return validationErrors;
  } catch (error) {
    const contractMessage = resolveEditableWorkerContractErrorMessage(
      error,
      messages,
    );
    const contractField = resolveEditableWorkerContractField(error);

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

export function hasEditableWorkerValidationErrors(
  validationErrors: EditableWorkerValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => typeof message === "string" && message.length > 0,
  );
}

function resolveEditableWorkerContractErrorMessage(
  error: unknown,
  messages: Pick<
    WorkerDetailMessages,
    "editableConfigurationContractInvalidPrefix"
  >,
): string {
  if (error instanceof FactoryDefinitionAPIError) {
    return `${messages.editableConfigurationContractInvalidPrefix} ${error.message}`;
  }

  return messages.editableConfigurationContractInvalidPrefix;
}

function buildWorkerContractValidationFactory(
  worker: NonNullable<CanonicalFactoryDefinition["workers"]>[number],
): CanonicalFactoryDefinition {
  return {
    name: "__worker_validation__",
    workers: [worker],
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

function resolveEditableWorkerContractField(
  error: unknown,
): EditableWorkerValidationField | null {
  if (!(error instanceof FactoryDefinitionAPIError)) {
    return null;
  }

  const fieldMatch = error.message.match(
    /factory\.workers\[\d+\]\.([a-zA-Z]+)/,
  );
  if (!fieldMatch) {
    return null;
  }

  const fieldName = fieldMatch[1];
  switch (fieldName) {
    case "args":
      return "args";
    case "body":
      return "body";
    case "command":
      return "command";
    case "executorProvider":
      return "executorProvider";
    case "model":
      return "model";
    case "modelLocality":
      return "modelLocality";
    case "modelProvider":
      return "modelProvider";
    case "provider":
      return "provider";
    case "type":
      return "type";
    default:
      return null;
  }
}
