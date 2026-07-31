import type { EditableWorkStateDraft } from "../../../current-factory-definition/lib/work-state-editable-values";
import {
  hasEditableWorkStateValidationErrors,
  mergeEditableWorkStateContractValidationErrors,
  validateEditableWorkStateDraft,
} from "./work-state-editable-validation";

const messages = {
  editableConfigurationContractInvalidPrefix:
    "Factory configuration is invalid.",
  editableConfigurationNameDuplicate: (name: string) =>
    `A work state named "${name}" already exists for this work type.`,
  editableConfigurationNameRequired: "Work state name is required.",
};

function buildDraft(
  overrides: Partial<EditableWorkStateDraft> = {},
): EditableWorkStateDraft {
  return {
    name: "queued",
    type: "INITIAL",
    ...overrides,
  };
}

const validationContext = {
  originalStateName: "queued",
  stateNamesInWorkType: ["queued", "done"],
};

describe("validateEditableWorkStateDraft", () => {
  it("requires a non-empty trimmed work state name", () => {
    expect(
      validateEditableWorkStateDraft(
        buildDraft({ name: "   " }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameRequired,
    });
  });

  it("rejects duplicate work state names within the same work type", () => {
    expect(
      validateEditableWorkStateDraft(
        buildDraft({ name: "done" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameDuplicate("done"),
    });
  });

  it("allows keeping the current work state name", () => {
    expect(
      validateEditableWorkStateDraft(buildDraft(), messages, validationContext),
    ).toEqual({});
  });
});

describe("mergeEditableWorkStateContractValidationErrors", () => {
  it("surfaces normalizeFactoryDefinition failures on the name field when applicable", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          name: "story",
          states: [{ type: "INITIAL" }],
        },
      ],
      workers: [
        { modelProvider: "CODEX", name: "reviewer", type: "MODEL_WORKER" },
      ],
    };

    expect(
      mergeEditableWorkStateContractValidationErrors(
        {},
        pendingFactoryDefinition,
        messages,
      ).name,
    ).toContain(messages.editableConfigurationContractInvalidPrefix);
  });

  it("falls back to a scoped contract banner when the error is not field-specific", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          name: "story",
          states: [{ name: "queued", type: "INVALID" }],
        },
      ],
      workers: [
        { modelProvider: "CODEX", name: "reviewer", type: "MODEL_WORKER" },
      ],
      workstations: [
        {
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          worker: "reviewer",
        },
      ],
    };

    const errors = mergeEditableWorkStateContractValidationErrors(
      {},
      pendingFactoryDefinition,
      messages,
    );

    expect(errors.contract).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
    expect(hasEditableWorkStateValidationErrors(errors)).toBe(true);
  });
});

describe("hasEditableWorkStateValidationErrors", () => {
  it("returns false for an empty error map", () => {
    expect(hasEditableWorkStateValidationErrors({})).toBe(false);
  });

  it("returns true when any field error is present", () => {
    expect(
      hasEditableWorkStateValidationErrors({
        name: messages.editableConfigurationNameRequired,
      }),
    ).toBe(true);
  });
});
