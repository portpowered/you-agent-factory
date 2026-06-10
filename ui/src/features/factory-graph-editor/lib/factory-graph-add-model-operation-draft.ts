import type { components } from "../../../api/generated/openapi";
import { ModelOperationContentType } from "../../../api/generated/openapi";

type ModelOperation = components["schemas"]["ModelOperation"];
type ModelOperationContentTypeValue =
  components["schemas"]["ModelOperationContentType"];

export const FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES = [
  ModelOperationContentType.ModelOperationContentTypeText,
  ModelOperationContentType.ModelOperationContentTypeImage,
  ModelOperationContentType.ModelOperationContentTypeAudio,
  ModelOperationContentType.ModelOperationContentTypeJSON,
  ModelOperationContentType.ModelOperationContentTypeBinary,
] as const satisfies readonly ModelOperationContentTypeValue[];

export interface FactoryGraphAddModelOperationSlotDraft {
  contentTypes: ModelOperationContentTypeValue[];
  name: string;
  required: boolean;
}

export interface FactoryGraphAddModelOperationDraft {
  inputs: FactoryGraphAddModelOperationSlotDraft[];
  name: string;
  outputs: FactoryGraphAddModelOperationSlotDraft[];
}

export interface FactoryGraphAddModelOperationSlotFieldErrors {
  contentTypes?: string;
  name?: string;
}

export interface FactoryGraphAddModelOperationItemFieldErrors {
  inputSlots?: Record<number, FactoryGraphAddModelOperationSlotFieldErrors>;
  inputs?: string;
  name?: string;
  outputSlots?: Record<number, FactoryGraphAddModelOperationSlotFieldErrors>;
  outputs?: string;
}

export interface FactoryGraphAddModelOperationValidationErrors {
  byIndex?: Record<number, FactoryGraphAddModelOperationItemFieldErrors>;
  summary?: string;
}

const UPPERCASE_MODEL_OPERATION_NAME_PATTERN = /^[A-Z][A-Z0-9_]*$/;

export function createEmptyFactoryGraphAddModelOperationSlotDraft(
  direction: "input" | "output",
): FactoryGraphAddModelOperationSlotDraft {
  return {
    contentTypes: [],
    name: "",
    required: direction === "input",
  };
}

export function createEmptyFactoryGraphAddModelOperationDraft(): FactoryGraphAddModelOperationDraft {
  return {
    inputs: [createEmptyFactoryGraphAddModelOperationSlotDraft("input")],
    name: "",
    outputs: [createEmptyFactoryGraphAddModelOperationSlotDraft("output")],
  };
}

export function isValidUppercaseModelOperationName(value: string): boolean {
  const trimmed = value.trim();
  return (
    trimmed.length > 0 && UPPERCASE_MODEL_OPERATION_NAME_PATTERN.test(trimmed)
  );
}

export function validateFactoryGraphAddModelOperationsDraft(
  operations: FactoryGraphAddModelOperationDraft[] | undefined,
): FactoryGraphAddModelOperationValidationErrors {
  if (!operations || operations.length === 0) {
    return {};
  }

  const byIndex: Record<number, FactoryGraphAddModelOperationItemFieldErrors> =
    {};
  const seenOperationNames = new Set<string>();

  for (const [operationIndex, operation] of operations.entries()) {
    const itemErrors: FactoryGraphAddModelOperationItemFieldErrors = {};
    const operationName = operation.name.trim();

    if (operationName.length === 0) {
      itemErrors.name = "Enter an uppercase operation name.";
    } else if (!isValidUppercaseModelOperationName(operationName)) {
      itemErrors.name =
        "Operation names must be uppercase letters, digits, or underscores.";
    } else if (seenOperationNames.has(operationName)) {
      itemErrors.name = `Operation name "${operationName}" is already used on this worker.`;
    } else {
      seenOperationNames.add(operationName);
    }

    const inputSlotErrors = validateFactoryGraphAddModelOperationSlots(
      operation.inputs,
      "input",
    );
    if (inputSlotErrors.summary) {
      itemErrors.inputs = inputSlotErrors.summary;
    }
    if (inputSlotErrors.byIndex) {
      itemErrors.inputSlots = inputSlotErrors.byIndex;
    }

    const outputSlotErrors = validateFactoryGraphAddModelOperationSlots(
      operation.outputs,
      "output",
    );
    if (outputSlotErrors.summary) {
      itemErrors.outputs = outputSlotErrors.summary;
    }
    if (outputSlotErrors.byIndex) {
      itemErrors.outputSlots = outputSlotErrors.byIndex;
    }

    if (Object.keys(itemErrors).length > 0) {
      byIndex[operationIndex] = itemErrors;
    }
  }

  if (Object.keys(byIndex).length === 0) {
    return {};
  }

  return {
    byIndex,
    summary: "Fix model operation contract errors before adding this worker.",
  };
}

function validateFactoryGraphAddModelOperationSlots(
  slots: FactoryGraphAddModelOperationSlotDraft[],
  direction: "input" | "output",
): {
  byIndex?: Record<number, FactoryGraphAddModelOperationSlotFieldErrors>;
  summary?: string;
} {
  const byIndex: Record<number, FactoryGraphAddModelOperationSlotFieldErrors> =
    {};
  const seenSlotNames = new Set<string>();

  if (slots.length === 0) {
    return {
      summary: `Add at least one ${direction} slot for each operation.`,
    };
  }

  for (const [slotIndex, slot] of slots.entries()) {
    const slotErrors: FactoryGraphAddModelOperationSlotFieldErrors = {};
    const slotName = slot.name.trim();

    if (slotName.length === 0) {
      slotErrors.name = "Enter a slot name.";
    } else if (seenSlotNames.has(slotName)) {
      slotErrors.name = `Slot name "${slotName}" is already used in this ${direction} list.`;
    } else {
      seenSlotNames.add(slotName);
    }

    if (slot.contentTypes.length === 0) {
      slotErrors.contentTypes = "Select at least one content type.";
    }

    if (Object.keys(slotErrors).length > 0) {
      byIndex[slotIndex] = slotErrors;
    }
  }

  if (Object.keys(byIndex).length > 0) {
    return { byIndex };
  }

  return {};
}

export function buildCanonicalModelOperationsFromDraft(
  operations: FactoryGraphAddModelOperationDraft[] | undefined,
): ModelOperation[] | undefined {
  if (!operations || operations.length === 0) {
    return undefined;
  }

  return operations.map((operation) => ({
    inputs: operation.inputs.map((slot) => ({
      contentTypes: [...slot.contentTypes],
      name: slot.name.trim(),
      ...(slot.required ? { required: true } : {}),
    })),
    name: operation.name.trim(),
    outputs: operation.outputs.map((slot) => ({
      contentTypes: [...slot.contentTypes],
      name: slot.name.trim(),
    })),
  }));
}
