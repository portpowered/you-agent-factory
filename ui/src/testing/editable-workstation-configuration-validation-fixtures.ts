import type { EditableWorkstationDraft } from "../features/current-factory-definition/lib/workstation-editable-values";

export const nameValidationContext = {
  originalWorkstationName: "Review",
  workstationNames: ["Plan", "Review"],
};

export const editableWorkstationValidationMessages = {
  editableConfigurationNameDuplicate: (workstationName: string) =>
    `A workstation named "${workstationName}" already exists in the running factory definition.`,
  editableConfigurationNameRequired:
    "Enter a workstation name before saving this workstation.",
  editableConfigurationBehaviorPollerWorkerUnsupported:
    "Poller workstations must use a script or hosted worker before saving this workstation.",
  editableConfigurationEmpty: "No editable workstation values are available.",
  editableConfigurationInputGuardMatchInputInvalid: (workType: string) =>
    `Peer input ${workType} is not available on this workstation.`,
  editableConfigurationInputGuardMatchInputRequired:
    "Select a peer input for this guard.",
  editableConfigurationInputGuardMatchInputSelfReference:
    "Peer input cannot reference the same input slot.",
  editableConfigurationInputGuardMultipleGuards:
    "Each input slot can have at most one guard.",
  editableConfigurationInputGuardParentInputInvalid: (workType: string) =>
    `Parent input ${workType} is not available on this workstation.`,
  editableConfigurationInputGuardParentInputRequired:
    "Select a parent input for this guard.",
  editableConfigurationInputGuardParentInputSelfReference:
    "Parent input cannot reference the same input slot.",
  editableConfigurationInputGuardSpawnedByInvalid: (workstation: string) =>
    `Spawned-by workstation ${workstation} is not available in this factory.`,
  editableConfigurationMatchesFieldsInputKeyRequired:
    "Enter a field selector for this guard.",
  editableConfigurationPromptFieldHint: "Resolve prompt diagnostics.",
  editableConfigurationPromptRequired:
    "Enter a prompt before saving this workstation.",
  editableConfigurationPromptValidationErrorPrefix:
    "Prompt validation unavailable.",
  editableConfigurationPromptValidationLoading:
    "Validating prompt variables for the current draft.",
  editableConfigurationVisitCountMaxVisitsInvalid:
    "Max visits must be a positive whole number.",
  editableConfigurationVisitCountWorkstationInvalid: (workstation: string) =>
    `Counted workstation ${workstation} is not available in this factory.`,
  editableConfigurationVisitCountWorkstationRequired:
    "Select the workstation whose visits are counted.",
  editableConfigurationWorkerMissing:
    "The selected worker is not available in the current factory definition.",
  editableConfigurationWorkerOptionsEmpty:
    "No workers are configured in this factory.",
  editableConfigurationWorkerRequired:
    "Select a worker before saving this workstation.",
  editableConfigurationWorkerUnavailable:
    "The selected worker is not available in the current factory definition.",
  editableConfigurationModelInvokeOperationInvalid:
    "Operation names must be uppercase letters, digits, or underscores.",
  editableConfigurationModelInvokeOperationMissing:
    "The selected operation is not declared on the chosen model worker.",
  editableConfigurationModelInvokeOperationRequired:
    "Select an operation before saving this workstation.",
  editableConfigurationModelInvokeBindingDuplicate: (slotName: string) =>
    `Operation binding for slot "${slotName}" is declared more than once.`,
  editableConfigurationModelInvokeBindingRequired: (slotName: string) =>
    `Required slot "${slotName}" needs a selector, config content, or default content.`,
  editableConfigurationModelInvokeBindingsSummary:
    "Resolve the highlighted operation binding fields before saving this workstation.",
  editableConfigurationModelInvokeWorkerOptionsEmpty:
    "No model workers with compatible operations are available in the current factory definition.",
} as const;

export const modelWorkstationValues = {
  behavior: "STANDARD" as const,
  behaviorOptions: ["STANDARD", "POLLER"] as const,
  cron: null,
  effectiveRunnerName: "antigravity",
  factoryRunnerName: "codex",
  guards: [],
  inputs: [],
  modelInvokeWorkerOptions: [],
  modelOperationsByWorkerName: {},
  operation: "",
  operationBindings: [],
  prompt: "Review prompt",
  resolvedRunnerSelection: {
    runnerId: "antigravity",
    source: "workstation" as const,
  },
  runnerName: "antigravity",
  runnerOptions: ["antigravity"],
  runnerSelectionSource: "workstation" as const,
  sharedWorkerWorkstationNames: [],
  sharedWorkerWorkstationNamesByWorkerName: {},
  workerModelProvider: null,
  workerName: "reviewer",
  workerOptions: ["reviewer", "planner"],
  workerTypeByName: {
    planner: "MODEL_WORKER" as const,
    reviewer: "MODEL_WORKER" as const,
  },
  workstationName: "Review",
  workstationOptions: ["Review"],
  workstationType: "MODEL_WORKSTATION" as const,
  workstationTypeOptions: ["MODEL_WORKSTATION", "MODEL_INVOKE"] as const,
};

export const baseEditableWorkstationDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  cron: null,
  guards: [],
  inputs: [],
  name: "Review",
  operation: "",
  operationBindings: [],
  prompt: "",
  runnerName: "antigravity",
  workerName: "",
  workstationType: "MODEL_WORKSTATION",
};
