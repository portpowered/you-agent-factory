import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  hasEditableWorkTypeValidationErrors,
  mergeEditableWorkTypeContractValidationErrors,
  validateEditableWorkTypeDraft,
} from "./work-type-editable-validation";
import type { EditableWorkTypeDraft } from "./work-type-editable-values";

const messages = {
  editableConfigurationContractInvalidPrefix: "Factory contract is invalid.",
  editableConfigurationNameDuplicate: (workTypeName: string) =>
    `Work type name "${workTypeName}" is already in use.`,
  editableConfigurationNameRequired: "Work type name is required.",
};

function buildDraft(
  overrides: Partial<EditableWorkTypeDraft> = {},
): EditableWorkTypeDraft {
  return {
    handlingBehavior: null,
    name: "story",
    ...overrides,
  };
}

const validationContext = {
  originalWorkTypeName: "story",
  workTypeNames: ["story", "bug"],
};

describe("validateEditableWorkTypeDraft", () => {
  it("requires a non-empty work type name", () => {
    expect(
      validateEditableWorkTypeDraft(
        buildDraft({ name: "   " }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameRequired,
    });
  });

  it("rejects duplicate work type names", () => {
    expect(
      validateEditableWorkTypeDraft(
        buildDraft({ name: "bug" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameDuplicate("bug"),
    });
  });

  it("allows keeping the current work type name", () => {
    expect(
      validateEditableWorkTypeDraft(buildDraft(), messages, validationContext),
    ).toEqual({});
  });
});

describe("mergeEditableWorkTypeContractValidationErrors", () => {
  it("surfaces normalizeFactoryDefinition failures on the pending factory", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        {},
        pendingFactoryDefinition,
        messages,
      ),
    ).toMatchObject({
      name: expect.stringContaining(
        messages.editableConfigurationContractInvalidPrefix,
      ),
    });
  });
});

describe("hasEditableWorkTypeValidationErrors", () => {
  it("returns true when any validation message is present", () => {
    expect(hasEditableWorkTypeValidationErrors({ name: "required" })).toBe(
      true,
    );
    expect(hasEditableWorkTypeValidationErrors({})).toBe(false);
  });
});
