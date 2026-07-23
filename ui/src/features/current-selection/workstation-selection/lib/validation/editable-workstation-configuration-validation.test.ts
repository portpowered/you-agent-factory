import { describe, expect, it } from "vitest";
import {
  baseEditableWorkstationDraft,
  editableWorkstationValidationMessages,
  modelWorkstationValues,
  nameValidationContext,
} from "../../../../../testing/editable-workstation-configuration-validation-fixtures";
import {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "./editable-workstation-configuration-validation";

describe("validateEditableWorkstationDraft workstation name", () => {
  it("blocks empty, duplicate, and unchanged-after-trim names", () => {
    expect(
      validateEditableWorkstationDraft(
        { ...baseEditableWorkstationDraft, name: "   " },
        modelWorkstationValues,
        { status: "idle" },
        editableWorkstationValidationMessages,
        nameValidationContext,
      ).name,
    ).toBe(
      editableWorkstationValidationMessages.editableConfigurationNameRequired,
    );

    expect(
      validateEditableWorkstationDraft(
        { ...baseEditableWorkstationDraft, name: "Plan" },
        modelWorkstationValues,
        { status: "idle" },
        editableWorkstationValidationMessages,
        nameValidationContext,
      ).name,
    ).toBe(
      editableWorkstationValidationMessages.editableConfigurationNameDuplicate(
        "Plan",
      ),
    );

    expect(
      validateEditableWorkstationDraft(
        { ...baseEditableWorkstationDraft, name: "  Review  " },
        modelWorkstationValues,
        { status: "idle" },
        editableWorkstationValidationMessages,
        nameValidationContext,
      ).name,
    ).toBeUndefined();
  });
});

describe("validateEditableWorkstationDraft logical move", () => {
  it("skips worker and prompt requirements for LOGICAL_MOVE workstations", () => {
    const errors = validateEditableWorkstationDraft(
      { ...baseEditableWorkstationDraft, workstationType: "LOGICAL_MOVE" },
      {
        behavior: "STANDARD",
        behaviorOptions: ["STANDARD"],
        effectiveRunnerName: "gemini",
        factoryRunnerName: "codex",
        guards: [],
        inputs: [],
        modelInvokeWorkerOptions: [],
        modelOperationsByWorkerName: {},
        operation: "",
        operationBindings: [],
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
        workstationTypeOptions: ["LOGICAL_MOVE"],
      },
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors.workerName).toBeUndefined();
    expect(errors.prompt).toBeUndefined();
  });

  it("still validates guard drafts for LOGICAL_MOVE workstations", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        guards: [{ type: "VISIT_COUNT", workstation: "", maxVisits: 0 }],
        workstationType: "LOGICAL_MOVE",
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
      editableWorkstationValidationMessages,
    );

    expect(errors["guards[0].workstation"]).toBe(
      editableWorkstationValidationMessages.editableConfigurationVisitCountWorkstationRequired,
    );
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(true);
  });
});

describe("validateEditableWorkstationDraft model workstation", () => {
  it("requires worker and prompt for MODEL_WORKSTATION drafts", () => {
    const errors = validateEditableWorkstationDraft(
      baseEditableWorkstationDraft,
      modelWorkstationValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors.workerName).toBe(
      editableWorkstationValidationMessages.editableConfigurationWorkerRequired,
    );
    expect(errors.prompt).toBe(
      editableWorkstationValidationMessages.editableConfigurationPromptRequired,
    );
  });

  it("reports unavailable workers and unsupported poller behavior", () => {
    const workerErrors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "missing-worker",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(workerErrors.workerName).toBe(
      editableWorkstationValidationMessages.editableConfigurationWorkerUnavailable,
    );

    const pollerErrors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        behavior: "POLLER",
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(pollerErrors.behavior).toBe(
      editableWorkstationValidationMessages.editableConfigurationBehaviorPollerWorkerUnsupported,
    );
  });

  it("does not render loading prompt validation as a field error", () => {
    const loadingErrors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      { status: "loading" },
      editableWorkstationValidationMessages,
    );

    expect(loadingErrors.prompt).toBeUndefined();

    const errorPromptErrors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      {
        errorMessage: "Prompt validation API unavailable.",
        status: "error",
      },
      editableWorkstationValidationMessages,
    );
    expect(errorPromptErrors.prompt).toBe(
      "Prompt validation unavailable. Prompt validation API unavailable.",
    );
  });

  it("maps failed prompt validation states to field errors", () => {
    const invalidPromptErrors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
      },
      modelWorkstationValues,
      {
        diagnostics: [{ message: "Unknown variable.", severity: "error" }],
        result: { diagnostics: [], valid: false },
        status: "ready",
      },
      editableWorkstationValidationMessages,
    );

    expect(invalidPromptErrors.prompt).toBe(
      editableWorkstationValidationMessages.editableConfigurationPromptFieldHint,
    );
  });
});

describe("validateEditableWorkstationDraft MATCHES_FIELDS guards", () => {
  it("requires inputKey for empty or whitespace-only selectors", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
        guards: [{ matchConfig: { inputKey: "   " }, type: "MATCHES_FIELDS" }],
      },
      modelWorkstationValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors["guards[0].matchConfig.inputKey"]).toBe(
      editableWorkstationValidationMessages.editableConfigurationMatchesFieldsInputKeyRequired,
    );
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(true);
  });

  it("does not block save for non-curated selector values", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...baseEditableWorkstationDraft,
        workerName: "reviewer",
        prompt: "Configured prompt",
        guards: [
          {
            matchConfig: { inputKey: '.Tags["_last_output"]' },
            type: "MATCHES_FIELDS",
          },
        ],
      },
      modelWorkstationValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors["guards[0].matchConfig.inputKey"]).toBeUndefined();
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(false);
  });
});

describe("hasEditableWorkstationValidationErrors", () => {
  it("returns false when no validation messages are present", () => {
    expect(hasEditableWorkstationValidationErrors({})).toBe(false);
  });
});
