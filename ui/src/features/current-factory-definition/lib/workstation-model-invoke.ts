import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { components } from "../../../api/generated/openapi";
import { WorkstationType } from "../../../api/generated/openapi";
import type { EditableWorkstationType } from "./workstation-type";

type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type ModelOperation = components["schemas"]["ModelOperation"];
type ModelOperationSlot = components["schemas"]["ModelOperationSlot"];
type WorkstationOperationBinding =
  components["schemas"]["WorkstationOperationBinding"];
type ModelOperationContentType =
  components["schemas"]["ModelOperationContentType"];

export interface EditableModelInvokeBindingSelectorDraft {
  label: string;
  role: string;
  slot: string;
  type: ModelOperationContentType | "";
}

export interface EditableModelInvokeBindingDraft {
  selector: EditableModelInvokeBindingSelectorDraft;
  slot: string;
}

export function isModelInvokeWorkstationType(
  workstationType: EditableWorkstationType,
): boolean {
  return workstationType === WorkstationType.WorkstationTypeModelInvoke;
}

export function workstationUsesPromptOrientedEditing(
  workstationType: EditableWorkstationType,
): boolean {
  return !isModelInvokeWorkstationType(workstationType);
}

export function modelOperationHasCompatibleSlots(
  operation: ModelOperation,
): boolean {
  const inputCount = operation.inputs?.length ?? 0;
  const outputCount = operation.outputs?.length ?? 0;
  return inputCount > 0 && outputCount > 0;
}

export function resolveCompatibleModelWorkerNames(
  factory: CanonicalFactoryDefinition,
): string[] {
  return (factory.workers ?? [])
    .filter(
      (worker) =>
        worker.type === "MODEL_WORKER" &&
        resolveModelWorkerOperations(factory, worker.name).length > 0,
    )
    .map((worker) => worker.name)
    .filter((name) => name.length > 0);
}

export function resolveModelWorkerOperations(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): ModelOperation[] {
  const worker = (factory.workers ?? []).find(
    (entry) => entry.name === workerName,
  );
  if (!worker || worker.type !== "MODEL_WORKER") {
    return [];
  }

  return (worker.operations ?? []).filter(modelOperationHasCompatibleSlots);
}

export function resolveModelOperationsByWorkerName(
  factory: CanonicalFactoryDefinition,
): Record<string, ModelOperation[]> {
  return Object.fromEntries(
    (factory.workers ?? [])
      .filter((worker) => worker.type === "MODEL_WORKER")
      .map((worker) => [
        worker.name,
        resolveModelWorkerOperations(factory, worker.name),
      ]),
  );
}

export function resolveModelOperationInputSlots(
  operation: ModelOperation | undefined,
): ModelOperationSlot[] {
  return operation?.inputs ?? [];
}

export function resolveEditableModelInvokeBindings(
  bindings: WorkstationOperationBinding[] | undefined,
): EditableModelInvokeBindingDraft[] {
  return (bindings ?? []).map((binding) => ({
    slot: binding.slot,
    selector: {
      label: binding.selector?.label ?? "",
      role: binding.selector?.role ?? "",
      slot: binding.selector?.slot ?? "",
      type: binding.selector?.type ?? "",
    },
  }));
}

export function buildCanonicalModelInvokeBindingsFromDraft(
  bindings: EditableModelInvokeBindingDraft[],
): WorkstationOperationBinding[] {
  return bindings
    .map((binding) => buildCanonicalModelInvokeBindingFromDraft(binding))
    .filter(
      (binding): binding is WorkstationOperationBinding => binding != null,
    );
}

function buildCanonicalModelInvokeBindingFromDraft(
  binding: EditableModelInvokeBindingDraft,
): WorkstationOperationBinding | null {
  const slot = binding.slot.trim();
  if (slot.length === 0) {
    return null;
  }

  const selector = buildCanonicalModelInvokeBindingSelector(binding.selector);
  if (!selector) {
    return { slot };
  }

  return {
    slot,
    selector,
  };
}

function buildCanonicalModelInvokeBindingSelector(
  selector: EditableModelInvokeBindingSelectorDraft,
): WorkstationOperationBinding["selector"] | undefined {
  const nextSelector: NonNullable<WorkstationOperationBinding["selector"]> = {};
  const label = selector.label.trim();
  const slot = selector.slot.trim();
  const role = selector.role.trim();
  const type = selector.type;

  if (label.length > 0) {
    nextSelector.label = label;
  }
  if (slot.length > 0) {
    nextSelector.slot = slot;
  }
  if (role.length > 0) {
    nextSelector.role = role;
  }
  if (type) {
    nextSelector.type = type;
  }

  return Object.keys(nextSelector).length > 0 ? nextSelector : undefined;
}

export function syncEditableModelInvokeBindingsForOperation(
  operation: ModelOperation | undefined,
  currentBindings: EditableModelInvokeBindingDraft[],
): EditableModelInvokeBindingDraft[] {
  const bindingsBySlot = new Map(
    currentBindings.map((binding) => [binding.slot, binding]),
  );

  return resolveModelOperationInputSlots(operation).map((inputSlot) => {
    const existing = bindingsBySlot.get(inputSlot.name);
    return (
      existing ?? {
        slot: inputSlot.name,
        selector: createEmptyEditableModelInvokeBindingSelectorDraft(),
      }
    );
  });
}

export function createEmptyEditableModelInvokeBindingSelectorDraft(): EditableModelInvokeBindingSelectorDraft {
  return {
    label: "",
    role: "",
    slot: "",
    type: "",
  };
}

export function editableModelInvokeBindingsEqual(
  left: EditableModelInvokeBindingDraft[],
  right: EditableModelInvokeBindingDraft[],
): boolean {
  if (left.length !== right.length) {
    return false;
  }

  return left.every((binding, index) => {
    const other = right[index];
    return (
      binding.slot === other.slot &&
      binding.selector.label === other.selector.label &&
      binding.selector.role === other.selector.role &&
      binding.selector.slot === other.selector.slot &&
      binding.selector.type === other.selector.type
    );
  });
}

export function resolveModelOperationByName(
  operations: ModelOperation[],
  operationName: string,
): ModelOperation | undefined {
  const trimmed = operationName.trim();
  if (trimmed.length === 0) {
    return undefined;
  }

  return operations.find((operation) => operation.name === trimmed);
}

export function workerDeclaresModelOperation(
  worker: CanonicalWorker | undefined,
  operationName: string,
): boolean {
  if (!worker || worker.type !== "MODEL_WORKER") {
    return false;
  }

  return (worker.operations ?? []).some(
    (operation) =>
      operation.name === operationName.trim() &&
      modelOperationHasCompatibleSlots(operation),
  );
}
