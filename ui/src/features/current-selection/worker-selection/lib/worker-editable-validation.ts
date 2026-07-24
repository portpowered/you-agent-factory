import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../../../../api/factory-definition";
import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import { goDurationFromWorkerTimeoutPicker } from "../../../current-factory-definition/lib/worker-timeout-duration";
import {
  isModelProviderWorkerType,
  isPollerWorkerType,
  isScriptWorkerType,
} from "../../../current-factory-definition/public";
import type { WorkerDetailMessages } from "../messages/worker-detail-types";

export type EditableWorkerValidationField =
  | "args"
  | "authSecretRef"
  | "body"
  | "command"
  | "contract"
  | "executorProvider"
  | "linearClaimAssigneeField"
  | "linearMappingState"
  | "linearMappingWorkType"
  | "linearPollInterval"
  | "linearStateIds"
  | "linearTeamIds"
  | "model"
  | "modelLocality"
  | "modelProvider"
  | "name"
  | "provider"
  | "skipPermissions"
  | "stopToken"
  | "timeout"
  | "type";

export type EditableWorkerValidationErrors = Partial<
  Record<EditableWorkerValidationField, string>
>;

export interface ValidateEditableWorkerDraftContext {
  originalWorkerName: string;
  workerNames: string[];
}

export function validateEditableWorkerDraft(
  draft: EditableWorkerDraft,
  messages: Pick<
    WorkerDetailMessages,
    | "editableConfigurationArgsInvalid"
    | "editableConfigurationAuthSecretRefRequired"
    | "editableConfigurationBodyRequired"
    | "editableConfigurationCommandRequired"
    | "editableConfigurationLinearMappingStateRequired"
    | "editableConfigurationLinearMappingWorkTypeRequired"
    | "editableConfigurationModelProviderRequired"
    | "editableConfigurationNameDuplicate"
    | "editableConfigurationNameRequired"
    | "editableConfigurationProviderRequired"
    | "editableConfigurationScriptCommandOrBodyRequired"
    | "editableConfigurationTimeoutInvalid"
  >,
  context?: ValidateEditableWorkerDraftContext,
): EditableWorkerValidationErrors {
  const validationErrors: EditableWorkerValidationErrors = {};
  const trimmedName = draft.name.trim();

  if (trimmedName.length === 0) {
    validationErrors.name = messages.editableConfigurationNameRequired;
  } else if (
    context &&
    trimmedName !== context.originalWorkerName &&
    context.workerNames.includes(trimmedName)
  ) {
    validationErrors.name =
      messages.editableConfigurationNameDuplicate(trimmedName);
  }

  if (isModelProviderWorkerType(draft.type) && !draft.modelProvider) {
    validationErrors.modelProvider =
      messages.editableConfigurationModelProviderRequired;
  }

  if (isScriptWorkerType(draft.type)) {
    const hasCommand = draft.command.trim().length > 0;
    const hasBody = draft.body.trim().length > 0;
    if (!hasCommand && !hasBody) {
      validationErrors.command =
        messages.editableConfigurationScriptCommandOrBodyRequired;
      validationErrors.body =
        messages.editableConfigurationScriptCommandOrBodyRequired;
    }
  }

  if (isPollerWorkerType(draft.type) && !draft.provider) {
    validationErrors.provider = messages.editableConfigurationProviderRequired;
  }

  if (isPollerWorkerType(draft.type) && draft.provider === "LINEAR") {
    if (draft.authSecretRef.trim().length === 0) {
      validationErrors.authSecretRef =
        messages.editableConfigurationAuthSecretRefRequired;
    }
    if (draft.linearMappingWorkType.trim().length === 0) {
      validationErrors.linearMappingWorkType =
        messages.editableConfigurationLinearMappingWorkTypeRequired;
    }
    if (draft.linearMappingState.trim().length === 0) {
      validationErrors.linearMappingState =
        messages.editableConfigurationLinearMappingStateRequired;
    }
  }

  if (isScriptWorkerType(draft.type) && draft.argsText.includes("\0")) {
    validationErrors.args = messages.editableConfigurationArgsInvalid;
  }

  const trimmedTimeoutAmount = (draft.timeoutAmount ?? "").trim();
  if (trimmedTimeoutAmount.length > 0) {
    const timeout = goDurationFromWorkerTimeoutPicker({
      amount: draft.timeoutAmount,
      unit: draft.timeoutUnit,
    });
    if (timeout === null) {
      validationErrors.timeout =
        messages.editableConfigurationTimeoutInvalid(trimmedTimeoutAmount);
    }
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
  workerIndex?: number,
): EditableWorkerValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  const worker =
    workerIndex != null && workerIndex >= 0
      ? pendingFactoryDefinition.workers?.[workerIndex]
      : pendingFactoryDefinition.workers?.find(
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

  const fieldPathMatch = error.message.match(
    /factory\.workers\[\d+\]\.([^\s]+)/,
  );
  if (fieldPathMatch) {
    const fieldFromPath = resolveEditableWorkerValidationFieldFromPath(
      fieldPathMatch[1],
    );
    if (fieldFromPath) {
      return fieldFromPath;
    }
  }

  const fieldMatch = error.message.match(
    /factory\.workers\[\d+\]\.([a-zA-Z]+)/,
  );
  if (!fieldMatch) {
    return null;
  }

  return resolveEditableWorkerValidationFieldFromPath(fieldMatch[1]);
}

function resolveEditableWorkerValidationFieldFromPath(
  fieldPath: string,
): EditableWorkerValidationField | null {
  switch (fieldPath) {
    case "args":
      return "args";
    case "auth.secretRef":
      return "authSecretRef";
    case "auth.clientId":
    case "auth.clientSecret":
      return "authSecretRef";
    case "body":
      return "body";
    case "command":
      return "command";
    case "executorProvider":
      return "executorProvider";
    case "linear.claim.assigneeField":
      return "linearClaimAssigneeField";
    case "linear.mapping.state":
      return "linearMappingState";
    case "linear.mapping.workType":
      return "linearMappingWorkType";
    case "linear.pollInterval":
      return "linearPollInterval";
    case "linear.stateIds":
      return "linearStateIds";
    case "linear.teamIds":
      return "linearTeamIds";
    case "model":
      return "model";
    case "modelLocality":
      return "modelLocality";
    case "modelProvider":
      return "modelProvider";
    case "name":
      return "name";
    case "provider":
      return "provider";
    case "skipPermissions":
      return "skipPermissions";
    case "stopToken":
      return "stopToken";
    case "timeout":
      return "timeout";
    case "type":
      return "type";
    default:
      return null;
  }
}
