import {
  resolveModelOperationByName,
  resolveModelOperationInputSlots,
} from "../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type {
  EditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import type { EditableWorkstationOperationOptionsState } from "./keys/detail-card-types";

export function resolveModelInvokeOperationOptionsState(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: {
    editableConfigurationModelInvokeOperationMissing: string;
    editableConfigurationModelInvokeOperationOptionsEmpty: string;
    editableConfigurationModelInvokeWorkerRequired: string;
  },
): EditableWorkstationOperationOptionsState {
  if (draft.workerName.trim().length === 0) {
    return {
      message: messages.editableConfigurationModelInvokeWorkerRequired,
      status: "empty",
    };
  }

  const operations =
    selectedEditableValues?.modelOperationsByWorkerName[draft.workerName] ?? [];
  if (operations.length === 0) {
    return {
      message: messages.editableConfigurationModelInvokeOperationOptionsEmpty,
      status: "empty",
    };
  }

  const operationNames = operations.map((operation) => operation.name);
  if (
    draft.operation.trim().length > 0 &&
    !operationNames.includes(draft.operation)
  ) {
    return {
      message: messages.editableConfigurationModelInvokeOperationMissing,
      status: "error",
    };
  }

  return {
    operations,
    options: operationNames,
    status: "ready",
  };
}

export function resolveSelectedModelInvokeOperation(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
) {
  const operations =
    selectedEditableValues?.modelOperationsByWorkerName[draft.workerName] ?? [];
  return resolveModelOperationByName(operations, draft.operation);
}

export function resolveModelInvokeBindingInputSlots(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
) {
  const operation = resolveSelectedModelInvokeOperation(
    draft,
    selectedEditableValues,
  );
  return resolveModelOperationInputSlots(operation);
}
