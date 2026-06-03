import { describe, expect, it } from "vitest";
import type {
  EditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  hasEditableWorkstationValidationErrors,
  resolveWorkerOptionsState,
  validateEditableWorkstationDraft,
} from "./editable-workstation-configuration-validation";

const nameValidationContext = {
  originalWorkstationName: "Review",
  workstationNames: ["Plan", "Review"],
};

const messages = {
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
} as const;

const modelWorkstationValues = {
  behavior: "STANDARD" as const,
  behaviorOptions: ["STANDARD", "POLLER"] as const,
  effectiveRunnerName: "gemini",
  factoryRunnerName: "codex",
  guards: [],
  inputs: [],
  prompt: "Review prompt",
  resolvedRunnerSelection: {
    runnerId: "gemini",
    source: "workstation" as const,
  },
  runnerName: "gemini",
  runnerOptions: ["gemini"],
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
};

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  guards: [],
  inputs: [],
  name: "Review",
  prompt: "",
  runnerName: "gemini",
  workerName: "",
};

describe("validateEditableWorkstationDraft workstation name", () => {
  it("blocks empty, duplicate, and unchanged-after-trim names", () => {
    expect(
      validateEditableWorkstationDraft(
        { ...baseDraft, name: "   " },
        modelWorkstationValues,
        { status: "idle" },
        messages,
        nameValidationContext,
      ).name,
    ).toBe(messages.editableConfigurationNameRequired);

    expect(
      validateEditableWorkstationDraft(
        { ...baseDraft, name: "Plan" },
        modelWorkstationValues,
        { status: "idle" },
        messages,
        nameValidationContext,
      ).name,
    ).toBe(messages.editableConfigurationNameDuplicate("Plan"));

    expect(
      validateEditableWorkstationDraft(
        { ...baseDraft, name: "  Review  " },
        modelWorkstationValues,
        { status: "idle" },
        messages,
        nameValidationContext,
      ).name,
    ).toBeUndefined();
  });
});

describe("validateEditableWorkstationDraft logical move", () => {
  it("skips worker and prompt requirements for LOGICAL_MOVE workstations", () => {
    const errors = validateEditableWorkstationDraft(
      baseDraft,
      {
        behavior: "STANDARD",
        behaviorOptions: ["STANDARD"],
        effectiveRunnerName: "gemini",
        factoryRunnerName: "codex",
        guards: [],
        inputs: [],
        prompt: "",
        resolvedRunnerSelection: {
          runnerId: "gemini",
          source: "workstation",
        },
        runnerName: "gemini",
        runnerOptions: ["gemini"],
        runnerSelectionSource: "workstation",
        sharedWorkerWorkstationNames: [],
        sharedWorkerWorkstationNamesByWorkerName: {},
        workerModelProvider: null,
        workerName: "reviewer",
        workerOptions: ["reviewer"],
        workerTypeByName: { reviewer: "MODEL_WORKER" },
        workstationName: "Move",
        workstationOptions: ["Move"],
        workstationType: "LOGICAL_MOVE",
      },
      { status: "idle" },
      messages,
    );

    expect(errors.workerName).toBeUndefined();
    expect(errors.prompt).toBeUndefined();
  });

  it("still validates guard drafts for LOGICAL_MOVE workstations", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        guards: [{ type: "VISIT_COUNT", workstation: "", maxVisits: 0 }],
      },
      {
        behavior: "STANDARD",
        behaviorOptions: ["STANDARD"],
        effectiveRunnerName: "gemini",
        factoryRunnerName: "codex",
        guards: [],
        inputs: [],
        prompt: "",
        resolvedRunnerSelection: {
          runnerId: "gemini",
          source: "workstation",
        },
        runnerName: "gemini",
        runnerOptions: ["gemini"],
        runnerSelectionSource: "workstation",
        sharedWorkerWorkstationNames: [],
        sharedWorkerWorkstationNamesByWorkerName: {},
        workerModelProvider: null,
        workerName: "reviewer",
        workerOptions: ["reviewer"],
        workerTypeByName: { reviewer: "MODEL_WORKER" },
        workstationName: "Move",
        workstationOptions: ["Move"],
        workstationType: "LOGICAL_MOVE",
      },
      { status: "idle" },
      messages,
    );

    expect(errors["guards[0].workstation"]).toBe(
      messages.editableConfigurationVisitCountWorkstationRequired,
    );
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(true);
  });
});

describe("validateEditableWorkstationDraft model workstation", () => {
  it("requires worker and prompt for MODEL_WORKSTATION drafts", () => {
    const errors = validateEditableWorkstationDraft(
      baseDraft,
      modelWorkstationValues,
      { status: "idle" },
      messages,
    );

    expect(errors.workerName).toBe(
      messages.editableConfigurationWorkerRequired,
    );
    expect(errors.prompt).toBe(messages.editableConfigurationPromptRequired);
  });

  it("reports unavailable workers and unsupported poller behavior", () => {
    const workerErrors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        workerName: "missing-worker",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "idle" },
      messages,
    );

    expect(workerErrors.workerName).toBe(
      messages.editableConfigurationWorkerUnavailable,
    );

    const pollerErrors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        behavior: "POLLER",
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "idle" },
      messages,
    );

    expect(pollerErrors.behavior).toBe(
      messages.editableConfigurationBehaviorPollerWorkerUnsupported,
    );
  });

  it("does not render loading prompt validation as a field error", () => {
    const loadingErrors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "loading" },
      messages,
    );

    expect(loadingErrors.prompt).toBeUndefined();

    const errorPromptErrors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      {
        errorMessage: "Prompt validation API unavailable.",
        status: "error",
      },
      messages,
    );
    expect(errorPromptErrors.prompt).toBe(
      "Prompt validation unavailable. Prompt validation API unavailable.",
    );
  });

  it("maps failed prompt validation states to field errors", () => {
    const invalidPromptErrors = validateEditableWorkstationDraft(
      {
        ...baseDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      {
        diagnostics: [{ message: "Unknown variable.", severity: "error" }],
        result: { diagnostics: [], valid: false },
        status: "ready",
      },
      messages,
    );

    expect(invalidPromptErrors.prompt).toBe(
      messages.editableConfigurationPromptFieldHint,
    );
  });
});

describe("resolveWorkerOptionsState logical move", () => {
  it("returns ready options without worker membership checks for LOGICAL_MOVE", () => {
    const state = resolveWorkerOptionsState(
      { ...baseDraft, workerName: "missing-worker" },
      {
        behavior: "STANDARD",
        behaviorOptions: ["STANDARD"],
        effectiveRunnerName: "gemini",
        factoryRunnerName: "codex",
        guards: [],
        inputs: [],
        prompt: "",
        resolvedRunnerSelection: {
          runnerId: "gemini",
          source: "workstation",
        },
        runnerName: "gemini",
        runnerOptions: ["gemini"],
        runnerSelectionSource: "workstation",
        sharedWorkerWorkstationNames: [],
        sharedWorkerWorkstationNamesByWorkerName: {},
        workerModelProvider: null,
        workerName: "reviewer",
        workerOptions: ["reviewer"],
        workerTypeByName: { reviewer: "MODEL_WORKER" },
        workstationName: "Move",
        workstationOptions: ["Move"],
        workstationType: "LOGICAL_MOVE",
      },
      messages,
    );

    expect(state).toEqual({
      options: ["reviewer"],
      status: "ready",
    });
  });
});

describe("resolveWorkerOptionsState model workstation", () => {
  it("returns error when editable values are unavailable", () => {
    const state = resolveWorkerOptionsState(
      baseDraft,
      undefined as unknown as ReturnType<
        typeof resolveEditableWorkstationValues
      >,
      messages,
    );

    expect(state).toEqual({
      message: messages.editableConfigurationEmpty,
      status: "error",
    });
  });

  it("returns empty and error states for MODEL_WORKSTATION worker selection", () => {
    const emptyState = resolveWorkerOptionsState(
      { ...baseDraft, workerName: "reviewer" },
      {
        ...modelWorkstationValues,
        workerOptions: [],
      },
      messages,
    );

    expect(emptyState).toEqual({
      message: messages.editableConfigurationWorkerOptionsEmpty,
      status: "empty",
    });

    const missingWorkerState = resolveWorkerOptionsState(
      { ...baseDraft, workerName: "missing-worker" },
      modelWorkstationValues,
      messages,
    );

    expect(missingWorkerState).toEqual({
      message: messages.editableConfigurationWorkerMissing,
      status: "error",
    });

    const readyState = resolveWorkerOptionsState(
      { ...baseDraft, workerName: "reviewer" },
      modelWorkstationValues,
      messages,
    );

    expect(readyState).toEqual({
      options: ["reviewer", "planner"],
      status: "ready",
    });
  });
});

describe("hasEditableWorkstationValidationErrors", () => {
  it("returns false when no validation messages are present", () => {
    expect(hasEditableWorkstationValidationErrors({})).toBe(false);
  });
});
