import {
  type EditableModelInvokeBindingDraft,
  resolveModelOperationByName,
  syncEditableModelInvokeBindingsForOperation,
} from "../../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../../current-factory-definition/lib/workstation-editable-values";

function resolveOperationsForWorker(
  selectedEditableValues: EditableWorkstationValues,
  workerName: string,
) {
  return selectedEditableValues.modelOperationsByWorkerName[workerName] ?? [];
}

export function resolveModelInvokeDraftForWorkerChange(
  draft: EditableWorkstationDraft,
  workerName: string,
  selectedEditableValues: EditableWorkstationValues,
): EditableWorkstationDraft {
  const operations = resolveOperationsForWorker(
    selectedEditableValues,
    workerName,
  );
  const nextOperation = operations[0]?.name ?? "";

  return {
    ...draft,
    workerName,
    operation: nextOperation,
    operationBindings: syncEditableModelInvokeBindingsForOperation(
      operations[0],
      [],
    ),
  };
}

export function resolveModelInvokeDraftForOperationChange(
  draft: EditableWorkstationDraft,
  operationName: string,
  selectedEditableValues: EditableWorkstationValues,
): EditableWorkstationDraft {
  const operations = resolveOperationsForWorker(
    selectedEditableValues,
    draft.workerName,
  );
  const operation = resolveModelOperationByName(operations, operationName);

  return {
    ...draft,
    operation: operationName,
    operationBindings: syncEditableModelInvokeBindingsForOperation(
      operation,
      draft.operationBindings,
    ),
  };
}

export function updateEditableModelInvokeBindingDraft(
  bindings: EditableModelInvokeBindingDraft[],
  slot: string,
  updater: (
    binding: EditableModelInvokeBindingDraft,
  ) => EditableModelInvokeBindingDraft,
): EditableModelInvokeBindingDraft[] {
  return bindings.map((binding) =>
    binding.slot === slot ? updater(binding) : binding,
  );
}
