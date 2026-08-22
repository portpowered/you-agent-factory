import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import type { components } from "../../../../api/generated/openapi";
import {
  isInferenceRunWorkstationType,
  isModelProviderWorkerType,
  isScriptRunWorkstationType,
} from "../worker-workstation-taxonomy";
import type { EditableWorkstationType } from "./workstation-type";

type ModelOperation = components["schemas"]["ModelOperation"];
type ModelOperationSlot = components["schemas"]["ModelOperationSlot"];
type WorkstationOperationBinding =
  components["schemas"]["WorkstationOperationBinding"];
type ModelOperationContentType =
  components["schemas"]["ModelOperationContentType"];
type WorkContent = components["schemas"]["WorkContent"];

export interface EditableModelInvokeBindingSelectorDraft {
  label: string;
  role: string;
  slot: string;
  type: ModelOperationContentType | "";
}

export interface EditableModelInvokeBindingDraft {
  configText: string;
  defaultContentText: string;
  selector: EditableModelInvokeBindingSelectorDraft;
  slot: string;
}

export interface ModelInvokeBindingValidationMessages {
  bindingDuplicate: (slotName: string) => string;
  bindingRequired: (slotName: string) => string;
  bindingSummary: string;
}

export function isModelInvokeWorkstationType(
  workstationType: EditableWorkstationType,
): boolean {
  return isInferenceRunWorkstationType(workstationType);
}

export function workstationUsesPromptOrientedEditing(
  workstationType: EditableWorkstationType,
): boolean {
  return (
    !isModelInvokeWorkstationType(workstationType) &&
    !isScriptRunWorkstationType(workstationType)
  );
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
        isModelProviderWorkerType(worker.type) &&
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
  if (!worker || !isModelProviderWorkerType(worker.type)) {
    return [];
  }

  return (worker.operations ?? []).filter(modelOperationHasCompatibleSlots);
}

export function resolveModelOperationsByWorkerName(
  factory: CanonicalFactoryDefinition,
): Record<string, ModelOperation[]> {
  return Object.fromEntries(
    (factory.workers ?? [])
      .filter((worker) => isModelProviderWorkerType(worker.type))
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
    configText: resolveEditableModelInvokeBindingText(binding.config),
    defaultContentText: resolveEditableModelInvokeBindingText(
      binding.defaultContent,
    ),
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
  const config = buildCanonicalModelInvokeBindingContent(binding.configText);
  const defaultContent = buildCanonicalModelInvokeBindingContent(
    binding.defaultContentText,
  );

  if (!selector && !config && !defaultContent) {
    return null;
  }

  return {
    slot,
    ...(selector ? { selector } : {}),
    ...(config ? { config } : {}),
    ...(defaultContent ? { defaultContent } : {}),
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

function buildCanonicalModelInvokeBindingContent(
  text: string,
): WorkContent | undefined {
  const trimmed = text.trim();
  if (trimmed.length === 0) {
    return undefined;
  }

  return [{ text: trimmed, type: "text" }];
}

function resolveEditableModelInvokeBindingText(
  content: WorkContent | undefined,
): string {
  const firstPart = content?.[0];
  if (!firstPart || !("text" in firstPart)) {
    return "";
  }

  return firstPart.text ?? "";
}

export function modelInvokeBindingDraftHasContent(
  binding: EditableModelInvokeBindingDraft,
): boolean {
  return (
    modelInvokeBindingSelectorHasContent(binding.selector) ||
    binding.configText.trim().length > 0 ||
    binding.defaultContentText.trim().length > 0
  );
}

function modelInvokeBindingSelectorHasContent(
  selector: EditableModelInvokeBindingSelectorDraft,
): boolean {
  return (
    selector.label.trim().length > 0 ||
    selector.role.trim().length > 0 ||
    selector.slot.trim().length > 0 ||
    selector.type !== ""
  );
}

export function validateEditableModelInvokeBindings(
  bindings: EditableModelInvokeBindingDraft[],
  operation: ModelOperation | undefined,
  messages: ModelInvokeBindingValidationMessages,
): Record<string, string> {
  const validationErrors: Record<string, string> = {};
  const seenSlots = new Set<string>();

  for (const binding of bindings) {
    const slotName = binding.slot.trim();
    if (slotName.length === 0) {
      continue;
    }

    if (seenSlots.has(slotName)) {
      validationErrors[`operationBindings[${slotName}]`] =
        messages.bindingDuplicate(slotName);
      continue;
    }
    seenSlots.add(slotName);
  }

  for (const inputSlot of resolveModelOperationInputSlots(operation)) {
    const binding = bindings.find((entry) => entry.slot === inputSlot.name);
    if (!binding) {
      if (inputSlot.required) {
        validationErrors[`operationBindings[${inputSlot.name}]`] =
          messages.bindingRequired(inputSlot.name);
      }
      continue;
    }

    if (inputSlot.required && !modelInvokeBindingDraftHasContent(binding)) {
      validationErrors[`operationBindings[${inputSlot.name}]`] =
        messages.bindingRequired(inputSlot.name);
    }
  }

  if (Object.keys(validationErrors).length > 0) {
    validationErrors.operationBindings = messages.bindingSummary;
  }

  return validationErrors;
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
        configText: "",
        defaultContentText: "",
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
      binding.configText === other.configText &&
      binding.defaultContentText === other.defaultContentText &&
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
