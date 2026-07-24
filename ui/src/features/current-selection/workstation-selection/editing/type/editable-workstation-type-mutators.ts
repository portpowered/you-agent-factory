import { isModelInvokeWorkstationType } from "../../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type { EditableWorkstationType } from "../../../../current-factory-definition/lib/workstation/workstation-type";
import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../../current-factory-definition/lib/workstation-editable-values";
import { resolveModelInvokeDraftForWorkerChange } from "../model-invoke/editable-workstation-model-invoke-mutators";

export function resolveDraftForWorkstationTypeChange(
  draft: EditableWorkstationDraft,
  workstationType: EditableWorkstationType,
  selectedEditableValues: EditableWorkstationValues,
): EditableWorkstationDraft {
  if (workstationType === draft.workstationType) {
    return draft;
  }

  if (isModelInvokeWorkstationType(workstationType)) {
    const workerName = selectedEditableValues.modelInvokeWorkerOptions.includes(
      draft.workerName,
    )
      ? draft.workerName
      : (selectedEditableValues.modelInvokeWorkerOptions[0] ?? "");

    return {
      ...resolveModelInvokeDraftForWorkerChange(
        draft,
        workerName,
        selectedEditableValues,
      ),
      workstationType,
    };
  }

  return {
    ...draft,
    workstationType,
    operation: "",
    operationBindings: [],
    prompt: draft.prompt || selectedEditableValues.prompt || "",
    workerName: selectedEditableValues.workerOptions.includes(draft.workerName)
      ? draft.workerName
      : (selectedEditableValues.workerOptions[0] ?? draft.workerName),
  };
}
