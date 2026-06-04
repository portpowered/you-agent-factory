import { describe, expect, it } from "vitest";

import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  validateEditableWorkstationGuardDraft,
  validateEditableWorkstationNameDraft,
} from "./workstation-editable-validation";

const nameMessages = {
  editableConfigurationNameDuplicate: (workstationName: string) =>
    `A workstation named "${workstationName}" already exists in the running factory definition.`,
  editableConfigurationNameRequired:
    "Enter a workstation name before saving this workstation.",
};

const messages = {
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
  editableConfigurationVisitCountMaxVisitsInvalid:
    "Max visits must be a positive whole number.",
  editableConfigurationVisitCountWorkstationInvalid: (workstation: string) =>
    `Counted workstation ${workstation} is not available in this factory.`,
  editableConfigurationVisitCountWorkstationRequired:
    "Select the workstation whose visits are counted.",
};

const context = {
  workstationOptions: ["Plan", "Review"],
};

function buildDraft(
  overrides?: Partial<EditableWorkstationDraft>,
): EditableWorkstationDraft {
  return {
    behavior: "STANDARD",
    cron: null,
    guards: [],
    inputs: [],
    name: "Review",
    prompt: "Review the story.",
    runnerName: null,
    workerName: "reviewer",
    ...overrides,
  };
}

describe("validateEditableWorkstationNameDraft", () => {
  const nameContext = {
    originalWorkstationName: "Review",
    workstationNames: ["Plan", "Review"],
  };

  it("requires a non-empty trimmed workstation name", () => {
    expect(
      validateEditableWorkstationNameDraft(
        buildDraft({ name: "   " }),
        nameMessages,
        nameContext,
      ),
    ).toEqual({
      name: nameMessages.editableConfigurationNameRequired,
    });
  });

  it("rejects duplicate workstation names", () => {
    expect(
      validateEditableWorkstationNameDraft(
        buildDraft({ name: "Plan" }),
        nameMessages,
        nameContext,
      ),
    ).toEqual({
      name: nameMessages.editableConfigurationNameDuplicate("Plan"),
    });
  });

  it("treats a trimmed unchanged name as valid", () => {
    expect(
      validateEditableWorkstationNameDraft(
        buildDraft({ name: "  Review  " }),
        nameMessages,
        nameContext,
      ),
    ).toEqual({});
  });
});

describe("validateEditableWorkstationGuardDraft workstation-level guards", () => {
  it("requires VISIT_COUNT workstation and positive maxVisits", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          guards: [{ maxVisits: 0, type: "VISIT_COUNT", workstation: "" }],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "guards[0].maxVisits": "Max visits must be a positive whole number.",
      "guards[0].workstation":
        "Select the workstation whose visits are counted.",
    });
  });

  it("rejects VISIT_COUNT workstation names outside the factory", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          guards: [
            { maxVisits: 2, type: "VISIT_COUNT", workstation: "Missing" },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "guards[0].workstation":
        "Counted workstation Missing is not available in this factory.",
    });
  });

  it("requires MATCHES_FIELDS inputKey", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          guards: [{ matchConfig: { inputKey: "  " }, type: "MATCHES_FIELDS" }],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "guards[0].matchConfig.inputKey":
        "Enter a field selector for this guard.",
    });
  });

  it("accepts non-curated MATCHES_FIELDS inputKey selectors", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          guards: [
            {
              matchConfig: { inputKey: '.Tags["_last_output"]' },
              type: "MATCHES_FIELDS",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({});
  });
});

describe("validateEditableWorkstationGuardDraft SAME_NAME guards", () => {
  it("validates matchInput requirements", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [{ matchInput: "", type: "SAME_NAME" }],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[1].guard.matchInput": "Select a peer input for this guard.",
    });

    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [{ matchInput: "missing", type: "SAME_NAME" }],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[1].guard.matchInput":
        "Peer input missing is not available on this workstation.",
    });

    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [{ matchInput: "task", type: "SAME_NAME" }],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[1].guard.matchInput":
        "Peer input cannot reference the same input slot.",
    });
  });
});

describe("validateEditableWorkstationGuardDraft SAME_TRACE_ID guards", () => {
  it("accepts valid matchInput values", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [{ matchInput: "plan", type: "SAME_TRACE_ID" }],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({});
  });
});

describe("validateEditableWorkstationGuardDraft parent-aware guards", () => {
  it("validates parentInput and spawnedBy", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [{ parentInput: "", type: "ALL_CHILDREN_COMPLETE" }],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[1].guard.parentInput": "Select a parent input for this guard.",
    });

    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            { guards: [], state: "ready", workType: "plan" },
            {
              guards: [
                {
                  parentInput: "task",
                  spawnedBy: "Missing",
                  type: "ANY_CHILD_FAILED",
                },
              ],
              state: "ready",
              workType: "task",
            },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[1].guard.parentInput":
        "Parent input cannot reference the same input slot.",
      "inputs[1].guard.spawnedBy":
        "Spawned-by workstation Missing is not available in this factory.",
    });
  });
});

describe("validateEditableWorkstationGuardDraft input guard count", () => {
  it("rejects more than one guard on an input slot", () => {
    expect(
      validateEditableWorkstationGuardDraft(
        buildDraft({
          inputs: [
            {
              guards: [
                { matchInput: "plan", type: "SAME_NAME" },
                { parentInput: "plan", type: "ALL_CHILDREN_COMPLETE" },
              ],
              state: "ready",
              workType: "task",
            },
            { guards: [], state: "ready", workType: "plan" },
          ],
        }),
        context,
        messages,
      ),
    ).toEqual({
      "inputs[0].guard.type": "Each input slot can have at most one guard.",
    });
  });
});
