import { describe, expect, it } from "vitest";

import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  hasEditableWorkstationValidationErrors,
  resolveWorkerOptionsState,
  validateEditableWorkstationDraft,
} from "./editable-workstation-configuration-validation";

const messages = {
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
};

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  guards: [],
  inputs: [],
  prompt: "",
  runnerName: "gemini",
  workerName: "",
};

describe("validateEditableWorkstationDraft", () => {
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
    expect(
      hasEditableWorkstationValidationErrors(errors),
    ).toBe(true);
  });
});

describe("resolveWorkerOptionsState", () => {
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
